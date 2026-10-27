import AppKit
import SwiftUI

/// Env-driven automation: README snapshots and the scripted promo-video run.
@MainActor
enum Automation {
    static let windowSize = NSSize(width: 1100, height: 700)

    static func run() async {
        let store = AppStore.shared
        let window = await mainWindow()

        if TwinEnv.wantsFixedWindow {
            window.setContentSize(windowSize)
            window.center()
            // Recordings should show the active window (colored traffic lights and selection).
            NSApp.activate(ignoringOtherApps: true)
            window.makeKeyAndOrderFront(nil)
        } else {
            window.setFrameAutosaveName("TwinMainWindow")
        }

        guard TwinEnv.wantsFixedWindow || TwinEnv.isScripted || TwinEnv.select != nil || TwinEnv.label != nil else { return }
        await store.waitUntilLoaded()

        if TwinEnv.isScripted {
            let recorder = TwinEnv.framesDir.map {
                FrameRecorder(window: window, directory: URL(fileURLWithPath: $0, isDirectory: true), fps: TwinEnv.framesPerSecond)
            }
            recorder?.start()
            await playScript(store: store)
            await recorder?.finish()
            NSApp.terminate(nil)
            return
        }

        let target = TwinEnv.select.flatMap(store.app(named:))
        if let target { store.selection = .app(target.id) }
        if target == nil, let twin = store.clones.first(where: { $0.name == TwinEnv.select }) { store.selection = .clone(twin.id) }
        if let label = TwinEnv.label { store.label = label }

        switch TwinEnv.snapshotState {
        case "drop":
            store.selection = nil
            store.isDropTargeted = true
            store.dropVerdict = target.map { .accept($0) }
        case "success":
            await pause(0.6) // let the icon preview render
            store.showPreviewSuccess()
        case "empty":
            store.selection = nil
        case "menubar":
            if let target { MenuBarController.shared.show(selecting: target) }
            if let label = TwinEnv.label { QuickClone.shared.label = label }
            await pause(1.2)
            saveImage(of: MenuBarController.shared.popoverView)
            NSApp.terminate(nil)
            return
        case "island":
            guard TwinEnv.isDemo, let target else { break }
            store.quickCreate(ClonePlan(source: target, rawLabel: TwinEnv.label ?? "Work", icon: store.iconOptions),
                              replacing: false)
            await pause(2.4)
            saveImage(of: TwinIsland.shared.panelView, suffix: "-running")
            while store.isCloning { await pause(0.05) }
            await pause(1.0)
            saveImage(of: TwinIsland.shared.panelView)
            NSApp.terminate(nil)
            return
        case "update":
            await Updater.shared.check(userInitiated: true)
            await pause(1.0)
            saveImage(of: UpdateWindow.contentView)
            NSApp.terminate(nil)
            return
        case "about":
            AboutWindow.show()
            await pause(2.0) // past the window's opening animation
            saveImage(of: AboutWindow.contentView)
            NSApp.terminate(nil)
            return
        default:
            break
        }

        if let path = TwinEnv.snapshotPath {
            await pause(1.5)
            do {
                try WindowCapture.snapshot(window, to: URL(fileURLWithPath: path))
                print("snapshot: \(path)")
            } catch {
                FileHandle.standardError.write(Data("snapshot failed: \(error)\n".utf8))
            }
            NSApp.terminate(nil)
        }
    }

    /// [Empty state →] app dragged over the window → drop → type label (icon preview updates live)
    /// → press Create → six steps → success. TWIN_SCRIPT_START=drop opens on the drop overlay.
    private static func playScript(store: AppStore) async {
        store.scrollsSidebarToSelection = false
        store.selection = nil
        guard let target = TwinEnv.select.flatMap(store.app(named:)) ?? store.apps.first else { return }

        if TwinEnv.scriptStart == "drop" {
            store.isDropTargeted = true
            store.dropVerdict = .accept(target)
            await pause(1.2)
        } else {
            await pause(1.2)
            withAnimation(.snappy(duration: 0.2)) {
                store.isDropTargeted = true
                store.dropVerdict = .accept(target)
            }
            await pause(1.4)
        }
        store.dropExited()
        store.adopt(target)
        await pause(1.1)

        // The pre-filled label is selected, so the first keystroke replaces it.
        for (index, character) in (TwinEnv.label ?? "Work").enumerated() {
            if index == 0 { store.label = String(character) } else { store.label.append(character) }
            await pause(0.12)
        }
        await pause(0.9)

        store.pressingCreate = true
        await pause(0.18)
        store.pressingCreate = false
        await pause(0.1)
        store.requestCreate()

        while store.isCloning { await pause(0.05) }
        await pause(2.4)
    }

    /// Saves a window as the window server composites it (the popover, island, update
    /// and About windows), falling back to drawing the view.
    private static func saveImage(of view: NSView?, suffix: String = "") {
        guard let path = TwinEnv.snapshotPath else { return }
        let out = URL(fileURLWithPath: (path as NSString).deletingPathExtension + suffix + ".png")
        guard let view, let image = view.window.flatMap(WindowCapture.windowServerImage(of:)) ?? WindowCapture.image(of: view) else {
            FileHandle.standardError.write(Data("snapshot failed: nothing drawn\n".utf8))
            return
        }
        try? WindowCapture.writePNG(image, to: out)
        print("snapshot: \(out.path)")
    }

    private static func pause(_ seconds: Double) async {
        try? await Task.sleep(for: .milliseconds(Int(seconds * 1000)))
    }

    private static func mainWindow() async -> NSWindow {
        while true {
            if let window = NSApp.windows.first(where: { $0.isVisible && $0.canBecomeMain && $0.contentView != nil }) {
                return window
            }
            try? await Task.sleep(for: .milliseconds(50))
        }
    }
}
