import Foundation

/// An application bundle found on disk.
struct AppInfo: Identifiable, Hashable {
    let url: URL
    /// The .app file name without extension. Clone names are built from this.
    let name: String
    let bundleID: String
    let version: String?
    let executable: String
    /// True when `Contents/MacOS/<executable>.real` exists, i.e. this app is already a clone.
    let isClone: Bool

    var id: String { url.path }

    /// Clones carry their label as the last bundle ID component (`<origID>.<Label>`).
    var cloneLabel: String? {
        isClone ? bundleID.split(separator: ".").last.map(String.init) : nil
    }

    /// For clones: the original app's name (`<Name>-<Label>` minus the label).
    var twinOf: String? {
        guard let label = cloneLabel, name.hasSuffix("-\(label)") else { return nil }
        return String(name.dropLast(label.count + 1))
    }

    /// Profile folder of a clone: `~/.app_profiles/<clone name>`.
    var profileURL: URL { AppPaths.profilesRoot.appendingPathComponent(name, isDirectory: true) }
}

enum AppPaths {
    static var applications: URL {
        TwinEnv.destinationDir ?? URL(fileURLWithPath: "/Applications", isDirectory: true)
    }
    static var home: URL { FileManager.default.homeDirectoryForCurrentUser }
    static var profilesRoot: URL { home.appendingPathComponent(".app_profiles", isDirectory: true) }

    static func display(_ url: URL) -> String {
        (url.path as NSString).abbreviatingWithTildeInPath
    }
}

enum CloneLabel {
    static let fallback = "Clone"

    struct Preset: Identifiable {
        let name: String
        let symbol: String
        var id: String { name }
    }

    static let presets = [
        Preset(name: "Work", symbol: "briefcase.fill"),
        Preset(name: "Personal", symbol: "person.fill"),
        Preset(name: "Hobby", symbol: "paintpalette.fill"),
        Preset(name: "School", symbol: "graduationcap.fill"),
    ]

    /// Trim, collapse whitespace runs to "-", keep only [A-Za-z0-9-]; empty → "Clone".
    static func sanitize(_ raw: String) -> String {
        let cleaned = filtered(raw)
        return cleaned.isEmpty ? fallback : cleaned
    }

    /// A label is invalid only when the user typed something that sanitizes to nothing.
    static func isValid(_ raw: String) -> Bool {
        raw.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !filtered(raw).isEmpty
    }

    private static func filtered(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        let dashed = trimmed.replacingOccurrences(of: "\\s+", with: "-", options: .regularExpression)
        return String(dashed.filter { ($0.isASCII && ($0.isLetter || $0.isNumber)) || $0 == "-" })
    }
}

/// Everything derived from a source app and a label.
struct ClonePlan: Hashable {
    let source: AppInfo
    let label: String
    let icon: IconOptions

    init(source: AppInfo, rawLabel: String, icon: IconOptions = IconOptions()) {
        self.source = source
        self.label = CloneLabel.sanitize(rawLabel)
        self.icon = icon
    }

    var color: UInt32 { IconPainter.color(for: label, options: icon) }

    var cloneName: String { "\(source.name)-\(label)" }
    var destination: URL { AppPaths.applications.appendingPathComponent("\(cloneName).app", isDirectory: true) }
    var profileURL: URL { AppPaths.profilesRoot.appendingPathComponent(cloneName, isDirectory: true) }
    var bundleID: String { "\(source.bundleID).\(label)" }

    var destinationExists: Bool {
        !TwinEnv.isDemo && FileManager.default.fileExists(atPath: destination.path)
    }
}
