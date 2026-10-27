import AppKit
import SwiftUI

/// The About window: what Twin is, which version this is, and where to go next.
@MainActor
enum AboutWindow {
    private static var window: NSWindow?

    static func show() {
        if window == nil {
            let host = NSHostingController(rootView: AboutView()
                .environmentObject(AppStore.shared)
                .environmentObject(Updater.shared))
            let w = NSWindow(contentViewController: host)
            w.title = "About Twin"
            w.titleVisibility = .hidden
            w.styleMask = [.titled, .closable, .fullSizeContentView]
            w.titlebarAppearsTransparent = true
            w.isMovableByWindowBackground = true
            w.isReleasedWhenClosed = false
            w.center()
            window = w
        }
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }

    static var contentView: NSView? { window?.contentView }
}

struct AboutView: View {
    @EnvironmentObject private var store: AppStore
    @EnvironmentObject private var updater: Updater

    private static let repo = URL(string: "https://github.com/\(Updater.repo)")!

    /// Dev builds have no release page of their own.
    private var releaseNotes: URL {
        let version = updater.currentVersion
        return version.contains("-") ? Self.repo.appendingPathComponent("releases")
            : Self.repo.appendingPathComponent("releases/tag/v\(version)")
    }

    var body: some View {
        VStack(spacing: 20) {
            header
            if store.hasLoaded { counts }
            actions
            Text("© 2026 Tsaqif Mukhayyar · [MIT License](https://github.com/mukhayyar/twin/blob/main/LICENSE)")
                .font(.caption)
                .foregroundStyle(.tertiary)
                .tint(.secondary)
        }
        .padding(.horizontal, 32)
        .padding(.top, 40)
        .padding(.bottom, 22)
        .frame(width: 380)
        .background(alignment: .top) {
            RadialGradient(colors: [Theme.violet.opacity(0.16), .clear], center: .top, startRadius: 0, endRadius: 260)
                .frame(height: 260)
                .ignoresSafeArea()
                .accessibilityHidden(true)
        }
    }

    private var header: some View {
        VStack(spacing: 8) {
            Image(nsImage: NSApp.applicationIconImage)
                .resizable()
                .frame(width: 96, height: 96)
                .shadow(color: Theme.violet.opacity(0.3), radius: 14, y: 6)
                .accessibilityHidden(true)
            Text("Twin")
                .font(.system(size: 28, weight: .bold, design: .rounded))
            Text("Version \(updater.currentVersion)")
                .font(.callout)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
            Text("One app. Many lives.")
                .font(.system(size: 15, weight: .semibold, design: .rounded))
                .foregroundStyle(Theme.gradient)
                .padding(.top, 6)
            Text("Clone any Mac app with a profile of its own: its own logins, settings, keychain and data, side by side with the original.")
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private var counts: some View {
        HStack(spacing: 0) {
            count(store.clones.count, store.clones.count == 1 ? "twin on this Mac" : "twins on this Mac")
            Divider().frame(height: 30)
            count(store.apps.count, "apps to clone")
        }
        .padding(.vertical, 10)
        .background(RoundedRectangle(cornerRadius: 10, style: .continuous).fill(Color.primary.opacity(0.05)))
    }

    private func count(_ value: Int, _ caption: String) -> some View {
        VStack(spacing: 1) {
            Text("\(value)")
                .font(.system(size: 20, weight: .semibold, design: .rounded))
                .monospacedDigit()
            Text(caption)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
    }

    private var actions: some View {
        VStack(spacing: 10) {
            if case .available(let release) = updater.state {
                Button { UpdateWindow.show() } label: {
                    Label("Twin \(release.version) is available", systemImage: "arrow.down.circle.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
            } else {
                Button { Task { await updater.check(userInitiated: true) } } label: {
                    Text("Check for Updates…").frame(maxWidth: .infinity)
                }
                .controlSize(.large)
            }
            HStack(spacing: 6) {
                Link("GitHub", destination: Self.repo)
                Text("·").foregroundStyle(.tertiary)
                Link("Release Notes", destination: releaseNotes)
                Text("·").foregroundStyle(.tertiary)
                Link("Report an Issue", destination: Self.repo.appendingPathComponent("issues/new"))
            }
            .font(.callout)
        }
    }
}
