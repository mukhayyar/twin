import AppKit
import Combine
import SwiftUI

/// A Dynamic Island-style pill that drops down from the top of the screen (from the notch, on
/// Macs that have one). It turns up while you drag an app anywhere, as a place to drop it and
/// make a twin, and while a clone runs, with its progress. Click it to open the finished twin.
@MainActor
final class TwinIsland: ObservableObject {
    static let shared = TwinIsland()

    enum Mode: Equatable { case hidden, drop, job }

    @Published private(set) var mode: Mode = .hidden
    @Published private(set) var expanded = false
    @Published private(set) var job: CloneJob?
    @Published var dropTargeted = false
    /// Height of the menu bar or notch the island hangs below.
    @Published private(set) var topInset: CGFloat = 32

    private var panel: NSPanel?
    private var subscriptions: Set<AnyCancellable> = []
    private var phaseSubscription: AnyCancellable?
    private var monitor: Any?
    private var seenDrag = NSPasteboard(name: .drag).changeCount
    private var hideTask: Task<Void, Never>?

    var panelView: NSView? { panel?.contentView }

    func start() {
        guard monitor == nil else { return }
        AppStore.shared.$job
            .sink { [weak self] job in self?.follow(job) }
            .store(in: &subscriptions)
        // Mouse drags in other apps reach a global monitor without any extra permission;
        // the drag pasteboard says whether an app is being dragged.
        monitor = NSEvent.addGlobalMonitorForEvents(matching: .leftMouseDragged) { [weak self] _ in
            Task { @MainActor in self?.mouseDragged() }
        }
    }

    // MARK: Dragging an app

    private func mouseDragged() {
        let pasteboard = NSPasteboard(name: .drag)
        guard pasteboard.changeCount != seenDrag else { return }
        seenDrag = pasteboard.changeCount
        let urls = pasteboard.readObjects(forClasses: [NSURL.self], options: [.urlReadingFileURLsOnly: true]) as? [URL] ?? []
        guard mode != .job, urls.contains(where: { $0.pathExtension.lowercased() == "app" }) else { return }
        show(.drop)
        // Leave once the drag ends, wherever it ends.
        hideTask?.cancel()
        hideTask = Task { [weak self] in
            while !Task.isCancelled, NSEvent.pressedMouseButtons & 1 != 0 {
                try? await Task.sleep(for: .milliseconds(200))
            }
            try? await Task.sleep(for: .milliseconds(400)) // a drop on the island lands first
            if !Task.isCancelled, self?.mode == .drop { self?.hide() }
        }
    }

    func drop(_ providers: [NSItemProvider]) -> Bool {
        guard let provider = providers.first else { return false }
        _ = provider.loadObject(ofClass: URL.self) { url, _ in
            Task { @MainActor in
                TwinIsland.shared.hide()
                guard let url, case .accept(let app) = AppStore.shared.evaluate(url) else { return }
                MenuBarController.shared.show(selecting: app)
            }
        }
        return true
    }

    // MARK: Clone progress

    private func follow(_ job: CloneJob?) {
        guard let job, job.phase == .running else { return }
        self.job = job
        show(.job)
        phaseSubscription = job.$phase.sink { [weak self] phase in
            guard phase != .running else { return }
            self?.hideTask?.cancel()
            self?.hideTask = Task { [weak self] in
                try? await Task.sleep(for: .seconds(phase == .succeeded ? 5 : 8))
                if !Task.isCancelled, self?.mode == .job { self?.hide() }
            }
        }
    }

    func tap() {
        guard mode == .job, let job else { return }
        switch job.phase {
        case .succeeded: AppStore.shared.open(job.plan.destination)
        case .failed: AppStore.shared.showMainWindow()
        case .running: MenuBarController.shared.show()
        }
        hide()
    }

    // MARK: The panel

    private func show(_ mode: Mode) {
        hideTask?.cancel()
        self.mode = mode
        let panel = self.panel ?? makePanel()
        guard let screen = NSScreen.main ?? NSScreen.screens.first else { return }
        topInset = screen.safeAreaInsets.top > 0 ? screen.safeAreaInsets.top : screen.frame.maxY - screen.visibleFrame.maxY
        let size = NSSize(width: 380, height: topInset + 58)
        panel.setFrame(NSRect(x: screen.frame.midX - size.width / 2, y: screen.frame.maxY - size.height,
                              width: size.width, height: size.height), display: true)
        panel.orderFrontRegardless()
        withAnimation(.spring(response: 0.38, dampingFraction: 0.78)) { expanded = true }
    }

    func hide() {
        hideTask?.cancel()
        withAnimation(.easeIn(duration: 0.2)) { expanded = false }
        Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(250))
            guard let self, !self.expanded else { return }
            self.panel?.orderOut(nil)
            self.mode = .hidden
            self.dropTargeted = false
        }
    }

    private func makePanel() -> NSPanel {
        let panel = NSPanel(contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel],
                            backing: .buffered, defer: false)
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = false
        panel.level = .statusBar
        panel.collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary, .ignoresCycle]
        panel.hidesOnDeactivate = false
        panel.isMovable = false
        panel.contentView = NSHostingView(rootView: IslandView().environmentObject(self))
        self.panel = panel
        return panel
    }
}

struct IslandView: View {
    @EnvironmentObject private var island: TwinIsland

    private var shape: UnevenRoundedRectangle {
        UnevenRoundedRectangle(topLeadingRadius: 0, bottomLeadingRadius: 24, bottomTrailingRadius: 24, topTrailingRadius: 0)
    }

    var body: some View {
        VStack(spacing: 0) {
            Color.clear.frame(height: island.topInset)
            content
                .frame(height: 58)
                .padding(.horizontal, 20)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(shape.fill(.black))
        .overlay(shape.strokeBorder(Theme.violet, lineWidth: island.dropTargeted ? 2 : 0))
        .contentShape(shape)
        .scaleEffect(x: island.expanded ? 1 : 0.5, y: island.expanded ? 1 : 0.3, anchor: .top)
        .opacity(island.expanded ? 1 : 0)
        .onDrop(of: [.fileURL], isTargeted: $island.dropTargeted) { island.drop($0) }
        .onTapGesture { island.tap() }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var content: some View {
        switch island.mode {
        case .drop:
            HStack(spacing: 12) {
                Image(systemName: island.dropTargeted ? "square.on.square.fill" : "square.on.square")
                    .font(.system(size: 22, weight: .semibold))
                    .foregroundStyle(Theme.violet)
                VStack(alignment: .leading, spacing: 1) {
                    Text("Drop to make a twin").font(.system(size: 13, weight: .semibold)).foregroundStyle(.white)
                    Text("Pick a profile name, then clone").font(.system(size: 11)).foregroundStyle(.white.opacity(0.6))
                }
                Spacer()
            }
        case .job:
            if let job = island.job { IslandJobView(job: job) }
        case .hidden:
            EmptyView()
        }
    }
}

struct IslandJobView: View {
    @ObservedObject var job: CloneJob

    var body: some View {
        HStack(spacing: 12) {
            Image(nsImage: job.icon ?? IconCache.icon(for: job.plan.source.url))
                .resizable()
                .frame(width: 30, height: 30)
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.system(size: 13, weight: .semibold)).foregroundStyle(.white).lineLimit(1)
                Text(subtitle).font(.system(size: 11)).foregroundStyle(.white.opacity(0.6)).lineLimit(1)
            }
            Spacer(minLength: 8)
            trailing
        }
    }

    private var title: String {
        switch job.phase {
        case .running: return "Cloning \(job.plan.source.name)"
        case .succeeded: return "\(job.plan.cloneName) is ready"
        case .failed: return "Couldn't create \(job.plan.cloneName)"
        }
    }

    private var subtitle: String {
        switch job.phase {
        case .running: return job.steps.first { $0.status == .running }?.title ?? "Starting…"
        case .succeeded: return "Click to open it"
        case .failed: return "Click for details"
        }
    }

    @ViewBuilder
    private var trailing: some View {
        switch job.phase {
        case .running:
            ZStack {
                Circle().stroke(.white.opacity(0.2), lineWidth: 3)
                Circle()
                    .trim(from: 0, to: job.progress)
                    .stroke(Theme.violet, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                    .rotationEffect(.degrees(-90))
                    .animation(.easeOut(duration: 0.3), value: job.progress)
            }
            .frame(width: 22, height: 22)
        case .succeeded:
            Text("Open")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(.white)
                .padding(.horizontal, 12)
                .padding(.vertical, 5)
                .background(Capsule().fill(Theme.violet))
        case .failed:
            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
        }
    }
}
