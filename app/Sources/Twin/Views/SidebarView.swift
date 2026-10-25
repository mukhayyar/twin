import SwiftUI

struct SidebarView: View {
    @EnvironmentObject private var store: AppStore

    var body: some View {
        ScrollViewReader { proxy in
            List(selection: $store.selection) {
                if !store.hasLoaded {
                    Section("Applications") {
                        ForEach(0..<10, id: \.self) { SkeletonRow(index: $0) }
                    }
                } else {
                    if !store.filteredClones.isEmpty {
                        Section("Clones") {
                            ForEach(store.filteredClones) { clone in
                                CloneRow(clone: clone)
                                    .tag(AppStore.Selection.clone(clone.id))
                                    .id(AppStore.Selection.clone(clone.id))
                                    .onDrag { NSItemProvider(object: clone.url as NSURL) }
                                    .contextMenu { cloneMenu(clone) }
                            }
                        }
                    }
                    Section("Applications") {
                        ForEach(store.filteredApps) { app in
                            AppRow(app: app)
                                .tag(AppStore.Selection.app(app.id))
                                .id(AppStore.Selection.app(app.id))
                                .contextMenu { appMenu(app) }
                        }
                    }
                }
            }
            .listStyle(.sidebar)
            .onChange(of: store.selection) { _, selection in
                guard let selection, store.scrollsSidebarToSelection else { return }
                withAnimation(.snappy) { proxy.scrollTo(selection) }
            }
        }
        .overlay {
            if store.hasLoaded && store.filteredApps.isEmpty && store.filteredClones.isEmpty {
                ContentUnavailableView.search(text: store.search)
            }
        }
        .safeAreaInset(edge: .bottom) {
            HStack {
                Button { store.chooseApp() } label: {
                    Label("Choose App…", systemImage: "plus")
                }
                .buttonStyle(.borderless)
                .help("Choose an app from any folder (⌘O)")
                Spacer()
                if store.hasLoaded {
                    Text("\(store.apps.count) apps")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
        }
        .toolbar {
            ToolbarItem {
                Button { Task { await store.reload() } } label: {
                    Label("Reload Apps", systemImage: "arrow.clockwise")
                }
                .help("Reload apps (⇧⌘R)")
            }
        }
    }

    @ViewBuilder
    private func cloneMenu(_ clone: AppInfo) -> some View {
        Button("Open") { store.open(clone.url) }
        Button("Show in Finder") { store.reveal(clone.url) }
        Button("Copy Path") { store.copyPath(clone.url) }
        Divider()
        Button("Move to Trash") { store.moveToTrash(clone) }
        if FileManager.default.fileExists(atPath: clone.profileURL.path) {
            Button("Move to Trash with Profile Data") { store.moveToTrash(clone, includingProfile: true) }
        }
    }

    @ViewBuilder
    private func appMenu(_ app: AppInfo) -> some View {
        Button("Clone…") { store.adopt(app) }
        Button("Show in Finder") { store.reveal(app.url) }
        Button("Copy Path") { store.copyPath(app.url) }
    }
}

struct AppRow: View {
    let app: AppInfo

    var body: some View {
        HStack(spacing: 10) {
            AppIcon(url: app.url, size: 28)
            VStack(alignment: .leading, spacing: 1) {
                Text(app.name).lineLimit(1)
                Text(app.bundleID)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, 2)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

struct CloneRow: View {
    let clone: AppInfo

    var body: some View {
        HStack(spacing: 10) {
            AppIcon(url: clone.url, size: 28)
            VStack(alignment: .leading, spacing: 1) {
                Text(clone.twinOf ?? clone.name).lineLimit(1)
                Text("\(clone.name).app")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Spacer(minLength: 4)
            if let label = clone.cloneLabel {
                ProfileBadge(label: label)
            }
        }
        .padding(.vertical, 2)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityHint("Twin. Drag to the Dock or Finder.")
    }
}

/// Placeholder while the background scan runs (Doherty Threshold: instant feedback).
struct SkeletonRow: View {
    let index: Int

    var body: some View {
        HStack(spacing: 10) {
            RoundedRectangle(cornerRadius: 7, style: .continuous)
                .fill(Color.primary.opacity(0.08))
                .frame(width: 28, height: 28)
            VStack(alignment: .leading, spacing: 5) {
                Capsule().fill(Color.primary.opacity(0.1)).frame(width: CGFloat(70 + (index * 37) % 60), height: 9)
                Capsule().fill(Color.primary.opacity(0.06)).frame(width: CGFloat(100 + (index * 53) % 50), height: 7)
            }
        }
        .padding(.vertical, 3)
        .accessibilityHidden(true)
    }
}
