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
