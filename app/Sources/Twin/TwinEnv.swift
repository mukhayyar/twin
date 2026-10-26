import Foundation

/// Environment switches for screenshots, the promo video and safe testing.
enum TwinEnv {
    private static let env = ProcessInfo.processInfo.environment

    /// Colon-separated folders to scan instead of /Applications and ~/Applications.
    static var searchDirs: [URL]? {
        guard let raw = value("TWIN_DIRS") else { return nil }
        return raw.split(separator: ":").map { URL(fileURLWithPath: String($0), isDirectory: true) }
    }

    /// Where clones are written. Hidden testing hook; defaults to /Applications.
    static var destinationDir: URL? {
        value("TWIN_DEST_DIR").map { URL(fileURLWithPath: $0, isDirectory: true) }
    }

    static var snapshotPath: String? { value("TWIN_SNAPSHOT") }
    static var select: String? { value("TWIN_SELECT") }
    static var label: String? { value("TWIN_LABEL") }
    static var appearance: String? { value("TWIN_APPEARANCE")?.lowercased() }
    /// Installs an update as soon as a check finds one. Hidden testing hook.
    static var autoInstallUpdate: Bool { env["TWIN_UPDATE_AUTOINSTALL"] == "1" }
    /// Forces a UI state for README snapshots: `drop`, `success`, `empty`, `menubar`, `island`
    /// (needs TWIN_DEMO) or `update`.
    static var snapshotState: String? { value("TWIN_SNAPSHOT_STATE")?.lowercased() }

    /// Simulates every clone step without touching the disk.
    static var isDemo: Bool { env["TWIN_DEMO"] == "1" }
    /// Auto-plays the scripted promo sequence. Requires TWIN_DEMO, or TWIN_DEST_DIR to run the
    /// real engine into a scratch folder.
    static var isScripted: Bool { env["TWIN_SCRIPT"] == "1" && (isDemo || destinationDir != nil) }
    static var framesDir: String? { value("TWIN_FRAMES_DIR") }
    /// With TWIN_CAPTURE_MODE=content, frames render the content view only unless this is 1.
    static var framesIncludeChrome: Bool { env["TWIN_FRAMES_CHROME"] == "1" }
    /// Frames default to the window server's composited image (real chrome, ~1 ms per frame).
    /// `content` renders the content view with cacheDisplay instead (no chrome, ~1.4 s per frame);
    /// `custom` additionally forces the explicit 2x bitmap path.
    static var captureMode: String? { value("TWIN_CAPTURE_MODE")?.lowercased() }
    /// `drop` starts the scripted run on the drop overlay instead of the empty state.
    static var scriptStart: String? { value("TWIN_SCRIPT_START")?.lowercased() }
    static var framesPerSecond: Double { value("TWIN_FPS").flatMap(Double.init) ?? 30 }

    /// Snapshot and frame capture use a fixed window size so output is consistent.
    static var wantsFixedWindow: Bool { snapshotPath != nil || framesDir != nil }
    /// Snapshots, demos and recordings stay silent.
    static var isAutomated: Bool { isDemo || wantsFixedWindow }

    private static func value(_ key: String) -> String? {
        guard let raw = env[key]?.trimmingCharacters(in: .whitespaces), !raw.isEmpty else { return nil }
        return raw
    }
}
