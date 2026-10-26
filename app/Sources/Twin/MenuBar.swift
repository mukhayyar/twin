import AppKit
import SwiftUI

/// Twin in the menu bar: click the icon for a quick-clone popover, or drop an app on it.
@MainActor
final class MenuBarController: NSObject, NSWindowDelegate, NSDraggingDestination {
    static let shared = MenuBarController()

    private var item: NSStatusItem?
    private let popover = NSPopover()

    private override init() {
        super.init()
        popover.behavior = .transient
        popover.animates = true
        popover.contentViewController = NSHostingController(
            rootView: QuickCloneView().environmentObject(AppStore.shared).environmentObject(QuickClone.shared))
    }

    var isVisible: Bool { item != nil }

    var popoverView: NSView? { popover.contentViewController?.view }

    func setVisible(_ visible: Bool) {
        if visible, item == nil {
            let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
            let image = NSImage(systemSymbolName: "square.on.square", accessibilityDescription: "Twin")
            image?.isTemplate = true
            item.button?.image = image
            item.button?.toolTip = "Twin: click to clone an app, or drop one here"
            item.button?.target = self
            item.button?.action = #selector(toggle)
            // The button's window passes drags to its delegate.
            item.button?.window?.registerForDraggedTypes([.fileURL])
            item.button?.window?.delegate = self
            self.item = item
        } else if !visible, let item {
            popover.performClose(nil)
            NSStatusBar.system.removeStatusItem(item)
            self.item = nil
        }
    }

    @objc private func toggle() {
        popover.isShown ? popover.performClose(nil) : show()
    }

    /// Opens the popover, optionally with an app already picked. Without a menu bar icon,
    /// the main window takes over.
    func show(selecting app: AppInfo? = nil) {
        if let app { QuickClone.shared.choose(app) }
        guard let button = item?.button else {
            if let app { AppStore.shared.adopt(app) }
            AppStore.shared.showMainWindow()
            return
        }
        NSApp.activate(ignoringOtherApps: true)
        guard !popover.isShown else { return }
        Task {
            // Right after launch the menu bar may not have placed the icon yet.
            for _ in 0..<20 where (button.window?.frame.height ?? 0) == 0 {
                try? await Task.sleep(for: .milliseconds(100))
            }
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
            popover.contentViewController?.view.window?.makeKey()
        }
    }

    // MARK: Dropping an app on the icon

    func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
        Self.droppedApp(sender) == nil ? [] : .copy
    }

    func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
        guard let app = Self.droppedApp(sender) else { return false }
        show(selecting: app)
        return true
    }

    static func droppedApp(_ info: NSDraggingInfo) -> AppInfo? {
        let urls = info.draggingPasteboard.readObjects(forClasses: [NSURL.self]) as? [URL] ?? []
        guard let url = urls.first, case .accept(let app) = AppStore.shared.evaluate(url) else { return nil }
        return app
    }
}

/// What the quick-clone popover is showing.
@MainActor
final class QuickClone: ObservableObject {
    static let shared = QuickClone()

    @Published var search = ""
    @Published var app: AppInfo?
    @Published var label = CloneLabel.presets[0].name
    /// The popover shows the clone it started until the user dismisses it.
    @Published var followsJob = false

    func choose(_ app: AppInfo) {
        self.app = app
        followsJob = false
    }

    func create(_ plan: ClonePlan, replacing: Bool) {
        followsJob = true
        AppStore.shared.quickCreate(plan, replacing: replacing)
    }

    func reset() {
        app = nil
        search = ""
        followsJob = false
    }
}

struct QuickCloneView: View {
    @EnvironmentObject private var store: AppStore
    @EnvironmentObject private var model: QuickClone

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            Divider()
            Group {
                if let job = store.job, model.followsJob || job.phase == .running {
                    QuickJobView(job: job)
                } else if let app = model.app {
                    form(for: app)
                } else {
                    picker
                }
            }
            .padding(14)
        }
        .frame(width: 340)
    }

    private var header: some View {
        HStack(spacing: 8) {
            Image(systemName: "square.on.square.fill").foregroundStyle(Theme.violet)
            Text("Twin").font(.headline)
            Spacer()
            Button { store.showMainWindow() } label: { Image(systemName: "macwindow") }
                .buttonStyle(.borderless)
                .help("Open the Twin window")
            Menu {
                Button("Check for Updates…") { Task { await Updater.shared.check(userInitiated: true) } }
                Button("Hide from Menu Bar") { store.showInMenuBar = false }
                Divider()
                Button("Quit Twin") { NSApp.terminate(nil) }
            } label: {
                Image(systemName: "ellipsis.circle")
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    private var filteredApps: [AppInfo] {
        let query = model.search.trimmingCharacters(in: .whitespaces)
        return query.isEmpty ? store.apps : store.apps.filter { $0.name.localizedCaseInsensitiveContains(query) }
    }

    private var picker: some View {
        VStack(alignment: .leading, spacing: 10) {
            TextField("Search apps", text: $model.search)
                .textFieldStyle(.roundedBorder)
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(filteredApps) { app in
                        Button { model.choose(app) } label: {
                            HStack(spacing: 10) {
                                Image(nsImage: IconCache.icon(for: app.url)).resizable().frame(width: 24, height: 24)
                                Text(app.name).lineLimit(1)
                                Spacer()
                                let count = store.twins(of: app).count
                                if count > 0 {
                                    Text("\(count) twin\(count == 1 ? "" : "s")").font(.caption).foregroundStyle(.secondary)
                                }
                            }
                            .padding(.vertical, 5)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .frame(height: 260)
            Label("Or drop an app on the menu bar icon", systemImage: "arrow.down.app")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private func form(for app: AppInfo) -> some View {
        let plan = ClonePlan(source: app, rawLabel: model.label, icon: store.iconOptions,
                             ownHardware: store.ownHardware, bringData: store.bringData)
        let duplicate = store.isDuplicate(plan)
        let valid = CloneLabel.isValid(model.label)
        return VStack(alignment: .leading, spacing: 12) {
            Button { model.app = nil } label: { Label("All apps", systemImage: "chevron.left") }
                .buttonStyle(.link)
            HStack(spacing: 10) {
                Image(nsImage: IconCache.icon(for: app.url)).resizable().frame(width: 40, height: 40)
                VStack(alignment: .leading, spacing: 2) {
                    Text(app.name).font(.headline)
                    Text("Will be named \(plan.cloneName)").font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            TextField("Profile name", text: $model.label)
                .textFieldStyle(.roundedBorder)
                .onSubmit { if valid && !store.isCloning { model.create(plan, replacing: duplicate) } }
            HStack(spacing: 6) {
                ForEach(CloneLabel.presets) { preset in
                    if plan.label.caseInsensitiveCompare(preset.name) == .orderedSame {
                        Button(preset.name) {}
                            .buttonStyle(.borderedProminent)
                            .tint(Color(hex: IconPainter.autoColor(for: preset.name)))
                            .controlSize(.small)
                    } else {
                        Button(preset.name) { model.label = preset.name }
                            .buttonStyle(.bordered)
                            .controlSize(.small)
                    }
                }
            }
            VStack(alignment: .leading, spacing: 4) {
                Toggle("Own hardware ID", isOn: $store.ownHardware)
                Toggle("Copy data from \(app.name)", isOn: $store.bringData)
            }
            .toggleStyle(.checkbox)
            .font(.callout)
            if !valid {
                Text("Use letters, numbers or dashes.").font(.caption).foregroundStyle(.red)
            } else if duplicate {
                Text("\(plan.cloneName) already exists and will be replaced. Its data is kept.")
                    .font(.caption).foregroundStyle(.orange)
            }
            Button { model.create(plan, replacing: duplicate) } label: {
                Text(duplicate ? "Replace Twin" : "Create Twin").frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .disabled(!valid || store.isCloning)
        }
    }
}

/// A clone's progress and result, inside the popover.
struct QuickJobView: View {
    @EnvironmentObject private var store: AppStore
    @EnvironmentObject private var model: QuickClone
    @ObservedObject var job: CloneJob

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                Image(nsImage: job.icon ?? IconCache.icon(for: job.plan.source.url))
                    .resizable().frame(width: 40, height: 40)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.headline)
                    Text(subtitle).font(.caption).foregroundStyle(.secondary).lineLimit(3)
                }
            }
            switch job.phase {
            case .running:
                ProgressView(value: job.progress)
            case .succeeded:
                HStack {
                    Button("Done") { model.reset() }
                    Spacer()
                    Button("Open \(job.plan.cloneName)") {
                        store.open(job.plan.destination)
                        model.reset()
                    }
                    .buttonStyle(.borderedProminent)
                }
            case .failed:
                HStack {
                    Button("Done") { model.reset() }
                    Spacer()
                    Button("Show Details") { store.showMainWindow() }
                }
            }
        }
    }

    private var title: String {
        switch job.phase {
        case .running: return "Cloning \(job.plan.source.name)…"
        case .succeeded: return "\(job.plan.cloneName) is ready"
        case .failed: return "Couldn't create \(job.plan.cloneName)"
        }
    }

    private var subtitle: String {
        switch job.phase {
        case .running:
            return job.steps.first { $0.status == .running }?.title ?? ""
        case .succeeded:
            return job.warnings.first ?? "Its own profile, icon and badge"
        case .failed:
            for step in job.steps { if case .failed(let message) = step.status { return message } }
            return ""
        }
    }
}
