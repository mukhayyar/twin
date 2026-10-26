import SwiftUI

struct CloneDetailView: View {
    @EnvironmentObject private var store: AppStore
    @StateObject private var insights = CloneInsights()
    let clone: AppInfo

    private var profileExists: Bool { FileManager.default.fileExists(atPath: clone.profileURL.path) }
    private var original: AppInfo? { store.original(of: clone) }

    /// Why this twin should be re-created, if it should.
    private var staleReasons: [String] {
        var reasons: [String] = []
        if let original, let theirs = original.version, let ours = clone.version,
           Updater.isNewer(theirs, than: ours) {
            reasons.append("\(original.name) was updated to \(theirs); this twin is still \(ours).")
        }
        switch insights.isolation {
        case .current: break
        case .older: reasons.append("It was made by an older Twin, so its isolation isn't the latest.")
        case .legacy: reasons.append("It was made before Twin 0.3, so it isn't fully isolated.")
        }
        return reasons
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                header
                if !staleReasons.isEmpty { updateBanner }
                stats
                if profileExists { storage }
                isolation
                details
                actions
            }
            .frame(maxWidth: 560)
            .padding(.horizontal, 28)
            .padding(.bottom, 28)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .task(id: clone.id) {
            await insights.load(clone)
            while !Task.isCancelled {
                await insights.refreshActivity(clone)
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }

    // MARK: Sections

    private var header: some View {
        VStack(spacing: 12) {
            AppIcon(url: clone.url, size: 112)
                .shadow(color: .black.opacity(0.18), radius: 12, y: 6)
                .onDrag { NSItemProvider(object: clone.url as NSURL) }
                .help("Drag to your Dock or a Finder window")
            HStack(spacing: 8) {
                Text(clone.twinOf ?? clone.name)
                    .font(.system(size: 26, weight: .bold, design: .rounded))
                    .lineLimit(1)
                if let label = clone.cloneLabel {
                    ProfileBadge(label: label, fontSize: 13)
                }
            }
            status
        }
        .padding(.top, 24)
    }

    private var status: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(insights.isRunning ? Color.green : Color.secondary.opacity(0.4))
                .frame(width: 8, height: 8)
            if insights.isRunning {
                Text("Running · \(insights.processes) process\(insights.processes == 1 ? "" : "es") · \(Self.bytes(insights.memoryBytes)) memory")
            } else if let last = insights.lastOpened {
                Text("Not running · last opened \(last.formatted(.relative(presentation: .named)))")
            } else {
                Text(profileExists ? "Not running" : "Never opened")
            }
        }
        .font(.callout)
        .foregroundStyle(.secondary)
        .accessibilityElement(children: .combine)
    }

    private var updateBanner: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "arrow.triangle.2.circlepath.circle.fill")
                .font(.title2)
                .foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 3) {
                Text("This twin can be updated").font(.headline)
                ForEach(staleReasons, id: \.self) { Text($0) }
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text("Updating re-creates the app and keeps its logins and data. macOS may ask you to allow its permissions (like Screen Recording) again.")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
            Spacer(minLength: 8)
            Button("Update Twin") { store.updateTwin(clone) }
                .buttonStyle(.borderedProminent)
                .tint(.orange)
                .disabled(original == nil || store.isCloning || insights.isRunning)
                .help(insights.isRunning ? "Quit \(clone.name) first" : "Re-create it from the current \(original?.name ?? "app")")
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(Color.orange.opacity(0.1)))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.orange.opacity(0.35)))
    }

    private var stats: some View {
        HStack(spacing: 12) {
            StatTile(symbol: "person.crop.square.fill", title: "Profile",
                     value: profileExists ? insights.profileBytes.map(Self.bytes) : "Empty",
                     caption: profileExists ? "logins, settings, data" : "made on first launch")
            StatTile(symbol: "app.fill", title: "App copy",
                     value: insights.appBytes.map(Self.bytes), caption: clone.version.map { "version \($0)" } ?? "")
            StatTile(symbol: "calendar", title: "Created",
                     value: insights.created.map { $0.formatted(.relative(presentation: .named)) },
                     caption: insights.created.map { $0.formatted(date: .abbreviated, time: .omitted) } ?? "")
        }
    }

    private var storage: some View {
        FormCard(title: "Storage") {
            if insights.profileBytes == nil {
                ProgressView().controlSize(.small).frame(maxWidth: .infinity)
            } else if insights.usage.isEmpty {
                Text("Nothing saved yet.").foregroundStyle(.secondary)
            } else {
                UsageBar(usage: insights.usage)
                VStack(spacing: 6) {
                    ForEach(Array(insights.usage.prefix(6).enumerated()), id: \.element.id) { index, item in
                        HStack(spacing: 8) {
                            Circle().fill(UsageBar.color(index)).frame(width: 8, height: 8)
                            Text(item.name)
                            Spacer()
                            Text(Self.bytes(item.bytes)).monospacedDigit().foregroundStyle(.secondary)
                        }
                        .font(.callout)
                    }
                }
                if insights.copiedData == true {
                    Text("Data copied from the original shares disk space with it until either side changes.")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            }
        }
    }

    private var isolation: some View {
        FormCard(title: "Kept separate") {
            IsolationRow(symbol: "folder.fill", title: "Files and settings", value: "Its own profile folder")
            IsolationRow(symbol: "slider.horizontal.3", title: "Preferences",
                         value: insights.preferenceCount.flatMap { $0 > 0 ? "\($0) saved, under its own bundle ID" : nil }
                            ?? "Its own bundle ID")
            IsolationRow(symbol: "key.fill", title: "Keychain", value: "Its own passwords and tokens")
            IsolationRow(symbol: "tray.full.fill", title: "Temp files and sockets", value: "Its own temporary folder")
            IsolationRow(symbol: "cpu", title: "Hardware ID",
                         value: insights.ownHardware ? "Its own, so apps see a different Mac" : "Shares this Mac's, by choice",
                         ok: insights.ownHardware ? true : nil)
            IsolationRow(symbol: "shield.lefthalf.filled", title: "Isolation",
                         value: insights.isolation == .current ? "Up to date" : "Needs an update",
                         ok: insights.isolation == .current)
        }
    }

    private var details: some View {
        Card {
            VStack(spacing: 0) {
                DetailRow(symbol: "app.badge.fill", title: "App", value: AppPaths.display(clone.url))
                Divider().padding(.leading, 46)
                DetailRow(symbol: "number", title: "Bundle ID", value: clone.bundleID)
                Divider().padding(.leading, 46)
                DetailRow(symbol: "folder.fill", title: "Profile",
                          value: profileExists ? AppPaths.display(clone.profileURL) : "Created on first launch")
                if let original {
                    Divider().padding(.leading, 46)
                    DetailRow(symbol: "square.on.square", title: "Twin of",
                              value: "\(original.name)\(original.version.map { " \($0)" } ?? "")")
                }
                if let copied = insights.copiedData {
                    Divider().padding(.leading, 46)
                    DetailRow(symbol: "tray.and.arrow.down.fill", title: "Started",
                              value: copied ? "With a copy of \(original?.name ?? "the original")'s data" : "With a fresh profile")
                }
                if let madeWith = insights.madeWith {
                    Divider().padding(.leading, 46)
                    DetailRow(symbol: "hammer.fill", title: "Made with", value: "Twin \(madeWith)")
                }
            }
        }
    }

    private var actions: some View {
        VStack(spacing: 10) {
            Button { store.open(clone.url) } label: {
                Label("Open \(clone.name)", systemImage: "arrow.up.forward.app.fill")
            }
            .buttonStyle(PrimaryButtonStyle())
            .keyboardShortcut(.defaultAction)
            HStack(spacing: 8) {
                Button { store.reveal(clone.url) } label: {
                    Label("Show in Finder", systemImage: "folder").frame(maxWidth: .infinity)
                }
                Button { NSWorkspace.shared.open(clone.profileURL) } label: {
                    Label("Profile Folder", systemImage: "person.crop.square").frame(maxWidth: .infinity)
                }
                .disabled(!profileExists)
                Button { store.moveToTrash(clone) } label: {
                    Label("Move to Trash", systemImage: "trash").frame(maxWidth: .infinity)
                }
                .help("Undo with ⌘Z")
            }
            .buttonStyle(SecondaryButtonStyle())
            Label("Tip: drag the icon to your Dock.", systemImage: "hand.draw")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
    }

    static func bytes(_ count: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: count, countStyle: .file)
    }
}

/// One number with a caption, in a row of three.
struct StatTile: View {
    let symbol: String
    let title: String
    let value: String?
    let caption: String

    var body: some View {
        Card {
            VStack(alignment: .leading, spacing: 4) {
                Label(title, systemImage: symbol)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.violet)
                if let value {
                    Text(value)
                        .font(.system(size: 20, weight: .semibold, design: .rounded))
                        .lineLimit(1)
                        .minimumScaleFactor(0.7)
                } else {
                    ProgressView().controlSize(.small).frame(height: 24)
                }
                Text(caption)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
    }
}

/// A stacked bar of where a profile's disk space goes.
struct UsageBar: View {
    let usage: [CloneInsights.Usage]

    static func color(_ index: Int) -> Color {
        [Theme.violet, .pink, .orange, .teal, .blue, .green, .gray][min(index, 6)]
    }

    var body: some View {
        let total = max(usage.reduce(0) { $0 + $1.bytes }, 1)
        GeometryReader { geo in
            HStack(spacing: 2) {
                ForEach(Array(usage.enumerated()), id: \.element.id) { index, item in
                    Rectangle()
                        .fill(Self.color(index))
                        .frame(width: max(2, geo.size.width * CGFloat(item.bytes) / CGFloat(total)))
                }
            }
        }
        .frame(height: 10)
        .clipShape(Capsule())
        .accessibilityHidden(true)
    }
}

struct IsolationRow: View {
    let symbol: String
    let title: String
    let value: String
    /// nil: a deliberate choice rather than a problem.
    var ok: Bool? = true

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: symbol)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Theme.violet)
                .frame(width: 20)
            Text(title)
            Spacer(minLength: 8)
            Text(value)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.trailing)
            switch ok {
            case true?: Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
            case false?: Image(systemName: "exclamationmark.circle.fill").foregroundStyle(.orange)
            case nil: Image(systemName: "minus.circle.fill").foregroundStyle(.secondary)
            }
        }
        .font(.callout)
        .accessibilityElement(children: .combine)
    }
}
