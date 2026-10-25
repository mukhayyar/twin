import AppKit
import AudioToolbox
import SwiftUI
import UniformTypeIdentifiers

@MainActor
final class AppStore: ObservableObject {
    static let shared = AppStore()

    enum Selection: Hashable {
        case app(String)
        case clone(String)
    }

    enum DropVerdict: Equatable {
        case accept(AppInfo)
        case reject(String)
    }

    struct TrashRecord: Equatable {
        struct Item: Equatable {
            let original: URL
            let trashed: URL
        }
        let name: String
        let items: [Item]
    }

    struct Toast: Identifiable, Equatable {
        enum Kind: Equatable {
            case rejection
            case trashed(TrashRecord)
        }
        let id = UUID()
        let kind: Kind
        let message: String
    }

    @Published private(set) var apps: [AppInfo] = []
    @Published private(set) var clones: [AppInfo] = []
    @Published private(set) var hasLoaded = false
    @Published var selection: Selection? {
        didSet { selectionChanged(from: oldValue) }
    }
    @Published var search = ""
    /// Pre-filled with the most common label (Tesler's Law: the app absorbs the decision).
    @Published var label = CloneLabel.presets[0].name {
        didSet { if label != oldValue { schedulePreview() } }
    }
    @Published var iconOptions = IconOptions() {
        didSet { if iconOptions != oldValue { schedulePreview() } }
    }
    /// Live preview of the twin's icon, rendered by the engine's own IconPainter.
    /// Give new twins their own hardware IDs (off: apps see this Mac's).
    @Published var ownHardware = true
    @Published private(set) var iconPreview: NSImage?
    @Published private(set) var iconPreviewSourceID: String?
    @Published var job: CloneJob?
    @Published var confirmReplace = false

    @Published var isDropTargeted = false
    @Published var dropVerdict: DropVerdict?
    @Published private(set) var toast: Toast?
    @Published private(set) var shakeTrigger = 0

    /// Bumped to move keyboard focus into the label field (after a drop or Choose App…).
    @Published private(set) var labelFocusRequest = 0
    /// Driven by the scripted demo so the real UI shows a button press.
    @Published var pressingCreate = false
    /// The scripted demo turns this off so the sidebar holds still.
    var scrollsSidebarToSelection = true

    weak var undoManager: UndoManager?
    private var droppedApps: [AppInfo] = []
    private var toastTask: Task<Void, Never>?
    private var iconPreviewPlan: ClonePlan?
    private var isRenderingPreview = false
    private var previewIsStale = false

    // MARK: Lookups

    var filteredApps: [AppInfo] { filter(apps) }
    var filteredClones: [AppInfo] { filter(clones) }

    func app(withID id: String) -> AppInfo? { apps.first { $0.id == id } }
    func clone(withID id: String) -> AppInfo? { clones.first { $0.id == id } }

    func app(named name: String) -> AppInfo? {
        apps.first { $0.name.localizedCaseInsensitiveCompare(name) == .orderedSame }
    }

    var selectedApp: AppInfo? {
        if case .app(let id) = selection { return app(withID: id) }
        return nil
    }

    var selectedClone: AppInfo? {
        if case .clone(let id) = selection { return clone(withID: id) }
        return nil
    }

    var currentPlan: ClonePlan? {
        selectedApp.map { ClonePlan(source: $0, rawLabel: label, icon: iconOptions, ownHardware: ownHardware) }
    }
    var isCloning: Bool { job?.phase == .running }

    /// The job shown in the detail pane for the selected app, if any.
    var visibleJob: CloneJob? {
        guard let job, let app = selectedApp, job.plan.source.id == app.id else { return nil }
        return job
    }

    var canCreate: Bool {
        selectedApp != nil && visibleJob == nil && !isCloning && CloneLabel.isValid(label)
    }

    /// Existing twins of an app, so users don't create duplicates by accident.
    func twins(of app: AppInfo) -> [AppInfo] {
        clones.filter { $0.twinOf == app.name && (app.bundleID.isEmpty || $0.bundleID.hasPrefix(app.bundleID + ".")) }
    }

    func isDuplicate(_ plan: ClonePlan) -> Bool {
        plan.destinationExists || clones.contains { $0.name == plan.cloneName }
    }

    /// What ⌘L opens: the selected clone or the one just created.
    var currentCloneURL: URL? {
        if let clone = selectedClone { return clone.url }
        if let job = visibleJob, job.phase == .succeeded { return job.plan.destination }
        return nil
    }

    /// What ⌘R reveals.
    var currentRevealURL: URL? { currentCloneURL ?? selectedApp?.url }

    // MARK: Loading

    func reload() async {
        let all = await Task.detached(priority: .userInitiated) { AppScanner.scan() }.value
        let scanned = Set(all.map(\.id))
        droppedApps = droppedApps.filter { !scanned.contains($0.id) && FileManager.default.fileExists(atPath: $0.url.path) }
        apps = (all.filter { !$0.isClone } + droppedApps)
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        clones = all.filter(\.isClone)
        hasLoaded = true

        switch selection {
        case .app(let id) where app(withID: id) == nil: selection = nil
        case .clone(let id) where clone(withID: id) == nil: selection = nil
        default: break
        }
    }

    func waitUntilLoaded() async {
        while !hasLoaded { try? await Task.sleep(for: .milliseconds(50)) }
    }

    private func selectionChanged(from old: Selection?) {
        guard old != selection else { return }
        confirmReplace = false
        schedulePreview()
        // A finished job belongs to the screen it was started on.
        if let job, job.phase != .running, job.plan.source.id != selectedApp?.id {
            self.job = nil
        }
    }

    // MARK: Cloning

    func requestCreate() {
        guard canCreate, let plan = currentPlan else { return }
        if isDuplicate(plan) {
            confirmReplace = true
        } else {
            start(plan, replacing: false)
        }
    }

    func confirmReplaceAndCreate() {
        guard let plan = currentPlan else { return }
        start(plan, replacing: true)
    }

    func retry() {
        guard let job else { return }
        start(job.plan, replacing: job.plan.destinationExists)
    }

    func dismissJob() {
        withAnimation(.snappy) { job = nil }
    }

    /// Screenshot helper: shows the success screen without cloning anything.
    func showPreviewSuccess() {
        guard let plan = currentPlan else { return }
        job = CloneJob(previewOf: plan, icon: previewIcon(for: plan))
    }

    private func previewIcon(for plan: ClonePlan) -> NSImage? {
        iconPreviewPlan == plan ? iconPreview : nil
    }

    /// Re-renders the preview with the engine's own painter. Renders are coalesced, never cancelled:
    /// while one is in flight, changes just mark the preview stale, and the latest values render next.
    /// So fast typing still updates the preview every render (~1 per 100–200 ms).
    private func schedulePreview() {
        guard let plan = currentPlan else {
            iconPreview = nil
            iconPreviewSourceID = nil
            iconPreviewPlan = nil
            return
        }
        guard plan.icon.paintsAnything else {
            iconPreview = IconCache.icon(for: plan.source.url)
            iconPreviewSourceID = plan.source.id
            iconPreviewPlan = plan
            return
        }
        guard !isRenderingPreview else {
            previewIsStale = true
            return
        }
        isRenderingPreview = true
        Task { [weak self] in
            let image = await Task.detached(priority: .userInitiated) {
                IconPainter.render(source: plan.source.url, label: plan.label, options: plan.icon)
            }.value
            guard let self else { return }
            if let image, self.currentPlan?.source.id == plan.source.id {
                self.iconPreview = NSImage(cgImage: image, size: NSSize(width: 1024, height: 1024))
                self.iconPreviewSourceID = plan.source.id
                self.iconPreviewPlan = plan
            }
            self.isRenderingPreview = false
            if self.previewIsStale || self.currentPlan != plan {
                self.previewIsStale = false
                self.schedulePreview()
            }
        }
    }

    private func start(_ plan: ClonePlan, replacing: Bool) {
        let job = CloneJob(plan: plan, icon: previewIcon(for: plan))
        job.onFinish = { [weak self, weak job] in
            guard let self, let job else { return }
            IconCache.invalidate(job.plan.destination)
            if job.phase == .succeeded { Feedback.success() }
            Task { await self.reload() }
        }
        withAnimation(.snappy) { self.job = job }
        job.start(replacing: replacing)
    }

    // MARK: Drag and drop, Open With, Choose App…

    func evaluate(_ url: URL) -> DropVerdict {
        var resolved = url
        if let target = try? URL(resolvingAliasFileAt: url) { resolved = target }
        resolved = resolved.resolvingSymlinksInPath()
        let name = resolved.deletingPathExtension().lastPathComponent

        guard resolved.pathExtension.lowercased() == "app", let app = AppScanner.load(resolved) else {
            return .reject("“\(name)” isn’t an app. Drop a .app from Finder.")
        }
        guard !app.isClone else {
            return .reject("“\(app.name)” is already a twin. Drop the original app instead.")
        }
        return .accept(app)
    }

    func dropEntered(_ providers: [NSItemProvider]) {
        withAnimation(.snappy(duration: 0.2)) { isDropTargeted = true }
        dropVerdict = nil
        Self.loadURL(from: providers) { [weak self] url in
            guard let self, self.isDropTargeted, let url else { return }
            withAnimation(.snappy(duration: 0.2)) { self.dropVerdict = self.evaluate(url) }
        }
    }

    func dropExited() {
        withAnimation(.snappy(duration: 0.2)) { isDropTargeted = false }
        dropVerdict = nil
    }

    func performDrop(_ providers: [NSItemProvider]) {
        dropExited()
        Self.loadURL(from: providers) { [weak self] url in
            guard let url else { return }
            self?.openApp(at: url)
        }
    }

    /// Entry point for drops, the Dock icon, Open With, and Choose App….
    func openApp(at url: URL) {
        switch evaluate(url) {
        case .accept(let app): adopt(app)
        case .reject(let message): reject(message)
        }
    }

    func adopt(_ app: AppInfo) {
        if !apps.contains(where: { $0.id == app.id }) {
            droppedApps.append(app)
            apps = (apps + [app]).sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        }
        search = ""
        withAnimation(.snappy) { selection = .app(app.id) }
        labelFocusRequest += 1
    }

    func chooseApp() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.applicationBundle]
        panel.directoryURL = URL(fileURLWithPath: "/Applications", isDirectory: true)
        panel.allowsMultipleSelection = false
        panel.canChooseDirectories = false
        panel.prompt = "Choose"
        panel.message = "Choose an app to clone."

        let handle: (NSApplication.ModalResponse) -> Void = { [weak self] response in
            guard response == .OK, let url = panel.url else { return }
            Task { @MainActor in self?.openApp(at: url) }
        }
        if let window = NSApp.keyWindow ?? NSApp.mainWindow {
            panel.beginSheetModal(for: window, completionHandler: handle)
        } else {
            handle(panel.runModal())
        }
    }

    private static func loadURL(from providers: [NSItemProvider], completion: @escaping @MainActor (URL?) -> Void) {
        guard let provider = providers.first(where: { $0.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) }) else {
            completion(nil)
            return
        }
        _ = provider.loadObject(ofClass: URL.self) { url, _ in
            Task { @MainActor in completion(url) }
        }
    }

    // MARK: Clone actions

    func open(_ url: URL) {
        NSWorkspace.shared.openApplication(at: url, configuration: NSWorkspace.OpenConfiguration())
    }

    func reveal(_ url: URL) {
        NSWorkspace.shared.activateFileViewerSelecting([url])
    }

    func copyPath(_ url: URL) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(url.path, forType: .string)
    }

    /// Moves a clone (and optionally its profile) to the Trash. Undoable with ⌘Z or the banner.
    func moveToTrash(_ clone: AppInfo, includingProfile: Bool = false) {
        let fm = FileManager.default
        var targets = [clone.url]
        if includingProfile, fm.fileExists(atPath: clone.profileURL.path) { targets.append(clone.profileURL) }

        var items: [TrashRecord.Item] = []
        for url in targets {
            var resulting: NSURL?
            do {
                try fm.trashItem(at: url, resultingItemURL: &resulting)
                if let trashed = resulting as URL? { items.append(.init(original: url, trashed: trashed)) }
            } catch {
                reject("Couldn’t move “\(clone.name)” to the Trash. \(error.localizedDescription)")
                return
            }
        }

        let record = TrashRecord(name: clone.name, items: items)
        undoManager?.registerUndo(withTarget: self) { store in
            MainActor.assumeIsolated { store.restore(record) }
        }
        undoManager?.setActionName("Move to Trash")
        if selection == .clone(clone.id) { selection = nil }
        show(Toast(kind: .trashed(record), message: "Moved “\(clone.name)” to the Trash"))
        Task { await reload() }
    }

    func trashSelectedClone() {
        if let clone = selectedClone { moveToTrash(clone) }
    }

    /// The banner's Undo button. Drops the registered undo so ⌘Z can't restore twice.
    func undoTrash(_ record: TrashRecord) {
        undoManager?.removeAllActions(withTarget: self)
        restore(record)
    }

    private func restore(_ record: TrashRecord) {
        for item in record.items {
            try? FileManager.default.moveItem(at: item.trashed, to: item.original)
        }
        dismissToast()
        Task {
            await reload()
            if let restored = clones.first(where: { $0.name == record.name }) {
                selection = .clone(restored.id)
            }
        }
    }

    // MARK: Feedback

    func reject(_ message: String) {
        show(Toast(kind: .rejection, message: message))
        shakeTrigger += 1
        NSSound.beep()
    }

    func dismissToast() {
        toastTask?.cancel()
        withAnimation(.snappy) { toast = nil }
    }

    private func show(_ toast: Toast) {
        toastTask?.cancel()
        withAnimation(.snappy) { self.toast = toast }
        let seconds: Double = toast.kind == .rejection ? 4 : 7
        toastTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(seconds))
            guard !Task.isCancelled else { return }
            self?.dismissToast()
        }
    }

    // MARK: Window helpers

    func focusSearch() {
        guard let window = NSApp.keyWindow ?? NSApp.mainWindow,
              let field = Self.searchField(in: window.contentView?.superview) else { return }
        window.makeFirstResponder(field)
    }

    func showAbout() {
        let paragraph = NSMutableParagraphStyle()
        paragraph.alignment = .center
        let credits = NSAttributedString(
            string: "One app. Many lives.\nClone any Mac app with a profile of its own.",
            attributes: [.font: NSFont.systemFont(ofSize: 11), .foregroundColor: NSColor.secondaryLabelColor, .paragraphStyle: paragraph]
        )
        NSApp.orderFrontStandardAboutPanel(options: [.applicationName: "Twin", .credits: credits])
        NSApp.activate(ignoringOtherApps: true)
    }

    private static func searchField(in view: NSView?) -> NSSearchField? {
        guard let view else { return nil }
        if let field = view as? NSSearchField { return field }
        for subview in view.subviews {
            if let field = searchField(in: subview) { return field }
        }
        return nil
    }

    private func filter(_ list: [AppInfo]) -> [AppInfo] {
        let query = search.trimmingCharacters(in: .whitespaces)
        guard !query.isEmpty else { return list }
        return list.filter { $0.name.localizedCaseInsensitiveContains(query) || $0.bundleID.localizedCaseInsensitiveContains(query) }
    }
}

/// App icons are slow-ish to fetch; cache them per path.
enum IconCache {
    private static var images: [String: NSImage] = [:]

    static func icon(for url: URL) -> NSImage {
        if let cached = images[url.path] { return cached }
        let image = NSWorkspace.shared.icon(forFile: url.path)
        images[url.path] = image
        return image
    }

    static func invalidate(_ url: URL) {
        images[url.path] = nil
    }
}

enum Feedback {
    /// The finishing touch (Peak-End Rule): the system "Glass" sound at the user's alert volume.
    static func success() {
        guard !TwinEnv.isAutomated else { return }
        var sound: SystemSoundID = 0
        let url = URL(fileURLWithPath: "/System/Library/Sounds/Glass.aiff")
        guard AudioServicesCreateSystemSoundID(url as CFURL, &sound) == noErr else { return }
        AudioServicesPlaySystemSoundWithCompletion(sound) { AudioServicesDisposeSystemSoundID(sound) }
    }
}
