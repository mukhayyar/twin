import AppKit
import SwiftUI

@main
struct TwinApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var store = AppStore.shared

    var body: some Scene {
        Window("Twin", id: "main") {
            ContentView()
                .environmentObject(store)
        }
        .defaultSize(width: 1100, height: 700)
        .windowResizability(.contentMinSize)
        .windowToolbarStyle(.unified(showsTitle: false))
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("About Twin") { store.showAbout() }
            }
            CommandGroup(replacing: .newItem) {
                Button("Choose App…") { store.chooseApp() }
                    .keyboardShortcut("o")
            }
            CommandGroup(replacing: .textEditing) {
                Button("Find App…") { store.focusSearch() }
                    .keyboardShortcut("f")
            }
            CommandMenu("Clone") {
                Button("Create Clone") { store.requestCreate() }
                    .keyboardShortcut(.return)
                    .disabled(!store.canCreate)
                Divider()
                Button("Open Clone") { if let url = store.currentCloneURL { store.open(url) } }
                    .keyboardShortcut("l")
                    .disabled(store.currentCloneURL == nil)
                Button("Reveal in Finder") { if let url = store.currentRevealURL { store.reveal(url) } }
                    .keyboardShortcut("r")
                    .disabled(store.currentRevealURL == nil)
                Divider()
                Button("Move Clone to Trash") { store.trashSelectedClone() }
                    .keyboardShortcut(.delete)
                    .disabled(store.selectedClone == nil)
            }
            CommandGroup(after: .sidebar) {
                Button("Reload Apps") { Task { await store.reload() } }
                    .keyboardShortcut("r", modifiers: [.command, .shift])
            }
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationWillFinishLaunching(_ notification: Notification) {
        // Lets `swift run` (no bundle) show a proper Dock icon and menu bar.
        NSApp.setActivationPolicy(.regular)
        switch TwinEnv.appearance {
        case "dark": NSApp.appearance = NSAppearance(named: .darkAqua)
        case "light": NSApp.appearance = NSAppearance(named: .aqua)
        default: break
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.activate(ignoringOtherApps: true)
        Task { @MainActor in await Automation.run() }
    }

    /// Apps dropped on the Dock icon or opened with "Open With → Twin".
    func application(_ application: NSApplication, open urls: [URL]) {
        guard let url = urls.first else { return }
        Task { @MainActor in AppStore.shared.openApp(at: url) }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}
