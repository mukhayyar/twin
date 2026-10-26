import AppKit

/// What the twin detail screen shows about a twin: whether it's running and how heavy it is,
/// where its disk space goes, how it's isolated, and whether it has fallen behind.
@MainActor
final class CloneInsights: ObservableObject {
    struct Usage: Identifiable, Equatable {
        let name: String
        let bytes: Int64
        var id: String { name }
    }

    enum Isolation: Equatable {
        case current
        /// Made by an older Twin: its isolation library isn't the one this Twin ships.
        case older
        /// Made before Twin 0.3, with a launcher script instead of the isolation library.
        case legacy
    }

    @Published private(set) var appBytes: Int64?
    @Published private(set) var profileBytes: Int64?
    @Published private(set) var usage: [Usage] = []
    @Published private(set) var processes = 0
    @Published private(set) var memoryBytes: Int64 = 0
    @Published private(set) var lastOpened: Date?
    @Published private(set) var created: Date?
    @Published private(set) var preferenceCount: Int?
    @Published private(set) var ownHardware = true
    @Published private(set) var madeWith: String?
    /// Whether the twin started with a copy of the original's data (twins made by 0.5.1 on).
    @Published private(set) var copiedData: Bool?
    @Published private(set) var isolation: Isolation = .current

    private var loadedID: String?

    var isRunning: Bool { processes > 0 }

    func load(_ clone: AppInfo) async {
        if loadedID != clone.id {
            loadedID = clone.id
            appBytes = nil
            profileBytes = nil
            usage = []
            preferenceCount = nil
            processes = 0
            memoryBytes = 0
        }
        let info = NSDictionary(contentsOf: clone.url.appendingPathComponent("Contents/Info.plist"))
        let environment = info?["LSEnvironment"] as? [String: String] ?? [:]
        ownHardware = environment["TWIN_HARDWARE"] != "shared"
        madeWith = info?["TwinVersion"] as? String
        copiedData = (info?["TwinCopiedData"] as? String).map { $0 == "true" }
        isolation = Self.isolation(of: clone)
        // Copying keeps the original's creation date; Twin writes Info.plist when it makes the twin.
        created = try? clone.url.appendingPathComponent("Contents/Info.plist")
            .resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate
        lastOpened = NSMetadataItem(url: clone.url)?.value(forAttribute: "kMDItemLastUsedDate") as? Date

        let id = clone.bundleID, app = clone.url, profile = clone.profileURL
        async let prefs = Task.detached(priority: .utility) { Self.preferenceCount(id) }.value
        async let appSize = Task.detached(priority: .utility) { Self.allocatedSize(app).total }.value
        async let profileSize = Task.detached(priority: .utility) { Self.allocatedSize(profile, categorize: true) }.value
        let (p, a, s) = await (prefs, appSize, profileSize)
        guard loadedID == clone.id else { return }
        preferenceCount = p
        appBytes = a
        profileBytes = FileManager.default.fileExists(atPath: profile.path) ? s.total : nil
        usage = s.usage
    }

    /// Processes running from inside the twin (Electron apps run several) and their memory.
    func refreshActivity(_ clone: AppInfo) async {
        let prefix = clone.url.path + "/"
        let (count, kilobytes) = await Task.detached(priority: .utility) { () -> (Int, Int64) in
            guard let out = try? CloneEngine.run("/bin/ps", ["-axo", "rss=,command="]) else { return (0, 0) }
            var count = 0, total: Int64 = 0
            for line in out.split(separator: "\n") {
                let trimmed = line.drop { $0 == " " }
                guard let space = trimmed.firstIndex(of: " ") else { continue }
                if trimmed[trimmed.index(after: space)...].hasPrefix(prefix) {
                    count += 1
                    total += Int64(trimmed[..<space]) ?? 0
                }
            }
            return (count, total)
        }.value
        guard loadedID == clone.id else { return }
        processes = count
        memoryBytes = kilobytes * 1024
    }

    // MARK: Measuring

    nonisolated private static func isolation(of clone: AppInfo) -> Isolation {
        let macOS = clone.url.appendingPathComponent("Contents/MacOS")
        guard let twinLib = try? Data(contentsOf: macOS.appendingPathComponent("libtwinisolate.dylib")) else {
            return .legacy
        }
        guard let ours = Bundle.main.url(forResource: "libtwinisolate", withExtension: "dylib"),
              let current = try? Data(contentsOf: ours) else { return .current } // a dev build can't tell
        return isolateVersion(twinLib) < isolateVersion(current) ? .older : .current
    }

    /// The TwinIsolateVersion compiled into libtwinisolate (see isolate.m). Comparing bytes
    /// doesn't work: making a twin signs its copy again, and every build of the same source
    /// differs. Libraries from before the marker are version 1.
    nonisolated static func isolateVersion(_ library: Data) -> Int {
        let marker = Data("TwinIsolateVersion=".utf8)
        guard let range = library.range(of: marker) else { return 1 }
        let digits = library[range.upperBound...].prefix { (48...57).contains($0) }
        return Int(String(decoding: digits, as: UTF8.self)) ?? 1
    }

    nonisolated private static func preferenceCount(_ domain: String) -> Int? {
        guard !domain.isEmpty, let out = try? CloneEngine.run("/usr/bin/defaults", ["export", domain, "-"]),
              let plist = try? PropertyListSerialization.propertyList(from: Data(out.utf8), format: nil) as? [String: Any]
        else { return nil }
        return plist.count
    }

    /// Disk space a folder takes, and for a profile, where it goes. Symlinks (the keychain
    /// link) aren't followed.
    nonisolated private static func allocatedSize(_ root: URL, categorize: Bool = false) -> (total: Int64, usage: [Usage]) {
        let keys: [URLResourceKey] = [.totalFileAllocatedSizeKey, .isRegularFileKey]
        guard let walker = FileManager.default.enumerator(at: root, includingPropertiesForKeys: keys) else { return (0, []) }
        var total: Int64 = 0
        var byCategory: [String: Int64] = [:]
        let base = root.standardizedFileURL.pathComponents.count
        for case let url as URL in walker {
            guard let values = try? url.resourceValues(forKeys: Set(keys)), values.isRegularFile == true else { continue }
            let size = Int64(values.totalFileAllocatedSize ?? 0)
            total += size
            if categorize {
                byCategory[category(Array(url.standardizedFileURL.pathComponents.dropFirst(base))), default: 0] += size
            }
        }
        let usage = byCategory.map { Usage(name: $0.key, bytes: $0.value) }.sorted { $0.bytes > $1.bytes }
        return (total, usage)
    }

    nonisolated private static func category(_ parts: [String]) -> String {
        guard let first = parts.first else { return "Other" }
        if first == "Library", parts.count > 1 {
            switch parts[1] {
            case "Application Support", "Containers", "Group Containers": return "App data"
            case "Caches": return "Caches"
            case "HTTPStorages", "WebKit", "Cookies": return "Web data"
            case "Preferences": return "Settings"
            case "Logs": return "Logs"
            case "Saved Application State": return "Window state"
            default: return "Other"
            }
        }
        switch first {
        case ".config": return "App data"
        case "Documents": return "Documents"
        default: return "Other"
        }
    }
}
