import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.undoManager) private var undoManager

    var body: some View {
        NavigationSplitView {
            SidebarView()
                .navigationSplitViewColumnWidth(min: 240, ideal: 270, max: 360)
        } detail: {
            DetailView()
        }
        .searchable(text: $store.search, placement: .sidebar, prompt: "Search apps")
        .frame(minWidth: 900, minHeight: 600)
        .tint(Theme.violet)
        .onDrop(of: [.fileURL], delegate: AppDropDelegate(store: store))
        .overlay {
            if store.isDropTargeted {
                DropOverlay(verdict: store.dropVerdict).transition(.opacity)
            }
        }
        .task { await store.reload() }
        .onAppear { store.undoManager = undoManager }
        .onChange(of: undoManager) { _, manager in store.undoManager = manager }
    }
}

struct AppDropDelegate: DropDelegate {
    let store: AppStore

    func validateDrop(info: DropInfo) -> Bool {
        info.hasItemsConforming(to: [.fileURL])
    }

    func dropEntered(info: DropInfo) {
        let providers = info.itemProviders(for: [.fileURL])
        MainActor.assumeIsolated { store.dropEntered(providers) }
    }

    func dropUpdated(info: DropInfo) -> DropProposal? {
        let rejected = MainActor.assumeIsolated {
            if case .reject = store.dropVerdict { return true }
            return false
        }
        return DropProposal(operation: rejected ? .forbidden : .copy)
    }

    func dropExited(info: DropInfo) {
        MainActor.assumeIsolated { store.dropExited() }
    }

    func performDrop(info: DropInfo) -> Bool {
        let providers = info.itemProviders(for: [.fileURL])
        MainActor.assumeIsolated { store.performDrop(providers) }
        return true
    }
}

struct DetailView: View {
    @EnvironmentObject private var store: AppStore

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background { HeaderGlow() }
            .overlay(alignment: .bottom) {
                if let toast = store.toast {
                    ToastView(toast: toast)
                        .padding(.bottom, 20)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .animation(.snappy(duration: 0.3), value: routeKey)
            .toolbarBackground(.hidden, for: .windowToolbar)
            .navigationTitle("")
    }

    @ViewBuilder
    private var content: some View {
        switch store.selection {
        case .app(let id):
            if let app = store.app(withID: id) {
                if let job = store.visibleJob {
                    CloneJobView(job: job).transition(.opacity)
                } else {
                    CloneFormView(app: app).id(app.id).transition(.opacity)
                }
            } else {
                EmptyStateView()
            }
        case .clone(let id):
            if let clone = store.clone(withID: id) {
                CloneDetailView(clone: clone).id(clone.id).transition(.opacity)
            } else {
                EmptyStateView()
            }
        case nil:
            EmptyStateView().transition(.opacity)
        }
    }

    private var routeKey: String {
        switch store.selection {
        case .app(let id): return "app:\(id):\(store.visibleJob != nil)"
        case .clone(let id): return "clone:\(id)"
        case nil: return "empty"
        }
    }
}

/// The empty state is one big drop zone (Fitts's Law), with Choose App… as the keyboard path.
struct EmptyStateView: View {
    @EnvironmentObject private var store: AppStore

    var body: some View {
        VStack(spacing: 0) {
            StepIndicator(current: 0)
                .padding(.top, 16)
            VStack(spacing: 16) {
                AppStackGlyph()
                Text("Drop an app here")
                    .font(.system(size: 28, weight: .bold, design: .rounded))
                Text("Twin gives it a copy with its own logins, settings and data.\nAny app, from any folder. Or pick one from the sidebar.")
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                Button { store.chooseApp() } label: {
                    Label("Choose App…", systemImage: "folder")
                }
                .buttonStyle(SecondaryButtonStyle())
                .help("Choose an app to clone (⌘O)")
                .padding(.top, 4)
            }
            .padding(32)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(
                RoundedRectangle(cornerRadius: 22, style: .continuous)
                    .strokeBorder(Color.secondary.opacity(0.35), style: StrokeStyle(lineWidth: 2, dash: [10, 8]))
            )
            .contentShape(Rectangle())
            .padding(28)
            .accessibilityElement(children: .contain)
            .accessibilityLabel("Drop zone. Drop an app here, or use Choose App.")
        }
    }
}

/// Two stacked app tiles with a drop arrow.
struct AppStackGlyph: View {
    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 22, style: .continuous)
                .fill(Theme.violet.opacity(0.16))
                .frame(width: 84, height: 84)
                .offset(x: -14, y: -12)
            RoundedRectangle(cornerRadius: 22, style: .continuous)
                .fill(Theme.gradient)
                .frame(width: 84, height: 84)
                .shadow(color: Theme.violet.opacity(0.35), radius: 14, y: 8)
                .offset(x: 10, y: 10)
            Image(systemName: "arrow.down")
                .font(.system(size: 30, weight: .bold))
                .foregroundStyle(.white)
                .offset(x: 10, y: 10)
        }
        .frame(width: 120, height: 120)
        .accessibilityHidden(true)
    }
}
