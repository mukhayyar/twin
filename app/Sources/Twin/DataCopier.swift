import AppKit

/// Starts a twin with a copy of the original's data, when asked. Mirrors the CLI's data.go and
/// data_darwin.go: the same names match, the same things are skipped, and nothing the twin
/// already has is overwritten.
enum DataCopier {
    struct Item: Equatable {
        let from: URL
        let to: URL
    }

    /// Copies the original's data into the twin's profile and returns a warning, or "" when all
    /// went well. Problems never fail the clone: the twin works either way.
    static func bringData(_ plan: ClonePlan) -> String {
        var notes: [String] = []
        if originalRunning(plan) {
            notes.append("\(plan.source.name) is open, so files it was writing may be incomplete; quit it for a clean copy")
        }
        var found = false, skipped = 0
        for item in items(for: plan) {
            found = true
            if exists(item.to.path) { continue }
            do { skipped += try copyTree(item.from, to: item.to) } catch {
                notes.append("couldn't copy \(AppPaths.display(item.from)): \(error.localizedDescription)")
            }
        }
        var prefs = false, prefsFailed = false
        do { prefs = try copyPrefs(plan) } catch {
            prefsFailed = true
            notes.append("preferences: \(error.localizedDescription)")
        }
        if !found && !prefs && !prefsFailed { notes.append("found no data of \(plan.source.name) to copy") }
        if skipped > 0 { notes.append("skipped \(skipped) files it couldn't read") }
        return notes.joined(separator: "; ")
    }

    // MARK: Finding the data

    /// The original's data and where the twin finds each copy. The twin's HOME is its profile,
    /// so items keep their place under it. Preference plists go through `copyPrefs` instead.
    static func items(for plan: ClonePlan, home: URL = AppPaths.home) -> [Item] {
        let lib = home.appendingPathComponent("Library")
        let twinLib = plan.profileURL.appendingPathComponent("Library")
        let id = plan.source.bundleID
        let names = Names([plan.source.name, id, bundleName(plan.source.url)])

        var items = names.match(lib.appendingPathComponent("Application Support"),
                                twinLib.appendingPathComponent("Application Support"))
        items += names.match(lib.appendingPathComponent("Preferences"), twinLib.appendingPathComponent("Preferences"))
            .filter { !$0.from.path.hasSuffix(".plist") } // folders, like RustDesk's config
        items += names.match(home.appendingPathComponent(".config"), plan.profileURL.appendingPathComponent(".config"))
        items += names.match(home, plan.profileURL, dotOnly: true)
        items += byBundleID(lib, twinLib, id: id, twinID: plan.bundleID)

        // A sandboxed app keeps everything in its container. Re-signing drops the twin's
        // sandbox, so the twin looks in its profile instead.
        let container = lib.appendingPathComponent("Containers/\(id)/Data")
        if !id.isEmpty, isDirectory(container.path) {
            let cLib = container.appendingPathComponent("Library")
            items += everything(cLib.appendingPathComponent("Application Support"),
                                twinLib.appendingPathComponent("Application Support"))
            items += everything(cLib.appendingPathComponent("Preferences"), twinLib.appendingPathComponent("Preferences"))
                .filter { !$0.from.path.hasSuffix(".plist") }
            items += everything(container.appendingPathComponent("Documents"),
                                plan.profileURL.appendingPathComponent("Documents"))
            items += byBundleID(cLib, twinLib, id: id, twinID: plan.bundleID)
        }
        return withoutApp(items, app: plan.source.url.path)
    }

    /// The names an app's data folders go by, compared ignoring case and punctuation: its
    /// name, bundle ID and that ID's last part, its CFBundleName. No app is listed by name.
    struct Names {
        private(set) var keys: Set<String> = []

        init(_ names: [String]) {
            for name in names {
                add(name)
                if let dot = name.lastIndex(of: "."), dot != name.startIndex {
                    add(String(name[name.index(after: dot)...]))
                }
            }
        }

        private mutating func add(_ name: String) {
            let k = Names.key(name)
            if k.count >= 3 { keys.insert(k) }
        }

        static func key(_ s: String) -> String {
            String(String.UnicodeScalarView(s.lowercased().unicodeScalars.filter { CharacterSet.alphanumerics.contains($0) }))
        }

        /// Whether a file or folder name is one of the app's names; leading dots are ignored.
        func has(_ name: String) -> Bool {
            keys.contains(Names.key(String(name.drop { $0 == "." })))
        }

        /// Whether a folder could be the publisher's folder the app keeps its data in, like
        /// "Google" in Google/Chrome: part of one of the app's names, but not all of it.
        func publisher(_ name: String) -> Bool {
            let k = Names.key(name)
            return k.count >= 3 && keys.contains { $0 != k && $0.contains(k) }
        }

        /// Entries of `from` that belong to the app, directly or one publisher folder down.
        func match(_ from: URL, _ to: URL, dotOnly: Bool = false) -> [Item] {
            var out: [Item] = []
            for name in DataCopier.entries(from.path) {
                if dotOnly && !name.hasPrefix(".") { continue }
                let path = from.appendingPathComponent(name)
                if has(name) {
                    out.append(Item(from: path, to: to.appendingPathComponent(name)))
                } else if !dotOnly, publisher(name), DataCopier.isRealDirectory(path.path) {
                    for sub in DataCopier.entries(path.path) where has(sub) {
                        out.append(Item(from: path.appendingPathComponent(sub),
                                        to: to.appendingPathComponent(name).appendingPathComponent(sub)))
                    }
                }
            }
            return out
        }
    }

    /// Every entry of a folder that belongs to the app as a whole. Symlinks are left out: in a
    /// container they point back at shared folders.
    static func everything(_ from: URL, _ to: URL) -> [Item] {
        entries(from.path).compactMap { name in
            let path = from.appendingPathComponent(name)
            return isSymlink(path.path) ? nil : Item(from: path, to: to.appendingPathComponent(name))
        }
    }

    /// Web data named after the bundle ID (cookies, website storage), renamed after the twin's ID.
    static func byBundleID(_ lib: URL, _ twinLib: URL, id: String, twinID: String) -> [Item] {
        guard !id.isEmpty else { return [] }
        var out: [Item] = []
        for dir in ["HTTPStorages", "WebKit", "Cookies"] {
            for suffix in ["", ".binarycookies"] {
                let from = lib.appendingPathComponent("\(dir)/\(id)\(suffix)")
                if exists(from.path) {
                    out.append(Item(from: from, to: twinLib.appendingPathComponent("\(dir)/\(twinID)\(suffix)")))
                }
            }
        }
        return out
    }

    /// Drops items that contain the app itself, or whose copy would land inside the source.
    static func withoutApp(_ items: [Item], app: String) -> [Item] {
        items.filter { item in
            let from = item.from.path
            return !(from == app || within(app, from) || within(item.to.path, from))
        }
    }

    // MARK: Copying

    /// Caches the twin rebuilds, crash-report state, and Chromium's Singleton* files (copied,
    /// those would send the twin's windows to the original) stay behind.
    static let cacheFolders: Set<String> = [
        "Cache", "Code Cache", "GPUCache", "ShaderCache", "GrShaderCache", "GraphiteDawnCache", "DawnCache",
        "DawnGraphiteCache", "DawnWebGPUCache", "CachedData", "Crashpad",
    ]

    static func skips(_ name: String, isDirectory: Bool) -> Bool {
        name.hasPrefix("Singleton") || (isDirectory && cacheFolders.contains(name))
    }

    /// Copies a file or folder into a temporary sibling, renamed at the end so an interrupted
    /// copy never looks finished. Files are APFS clones where possible (instant, no extra
    /// space). Symlinks into the source are re-pointed into the copy; sockets and other
    /// special files are left out. Returns how many files couldn't be read.
    static func copyTree(_ src: URL, to dst: URL) throws -> Int {
        let fm = FileManager.default
        let given = src.path
        guard let root = realPath(given) else { throw CloneError("\(AppPaths.display(src)) can't be read") }
        let tmp = dst.path + ".twin-partial"
        try? fm.removeItem(atPath: tmp)
        try fm.createDirectory(at: dst.deletingLastPathComponent(), withIntermediateDirectories: true)

        var skipped = 0
        func visit(_ path: String, _ target: String, isRoot: Bool) throws {
            var info = stat()
            guard lstat(path, &info) == 0 else {
                if isRoot { throw CloneError("\(path) can't be read") }
                skipped += 1
                return
            }
            let type = info.st_mode & S_IFMT
            if !isRoot && skips((path as NSString).lastPathComponent, isDirectory: type == S_IFDIR) { return }
            switch type {
            case S_IFDIR:
                try fm.createDirectory(atPath: target, withIntermediateDirectories: true,
                                       attributes: [.posixPermissions: Int(info.st_mode & 0o777) | 0o700])
                guard let children = try? fm.contentsOfDirectory(atPath: path) else {
                    skipped += 1
                    return
                }
                for child in children {
                    try visit(path + "/" + child, target + "/" + child, isRoot: false)
                }
            case S_IFLNK:
                guard var link = try? fm.destinationOfSymbolicLink(atPath: path) else {
                    skipped += 1
                    return
                }
                for base in [root, given] where link.hasPrefix("/") && within(link, base) { // either spelling
                    link = dst.path + link.dropFirst(base.count)
                    break
                }
                if (try? fm.createSymbolicLink(atPath: target, withDestinationPath: link)) == nil { skipped += 1 }
            case S_IFREG:
                guard access(path, R_OK) == 0 else {
                    skipped += 1
                    return
                }
                if copyfile(path, target, nil, copyfile_flags_t(COPYFILE_CLONE | COPYFILE_ALL)) != 0 {
                    throw CloneError("\(target): \(String(cString: strerror(errno)))")
                }
            default:
                break
            }
        }

        do {
            try visit(root, tmp, isRoot: true)
            try fm.moveItem(atPath: tmp, toPath: dst.path)
        } catch {
            try? fm.removeItem(atPath: tmp)
            throw error
        }
        return skipped
    }

    // MARK: Preferences

    /// Gives the twin's preferences domains a copy of the original's. cfprefsd keeps them all
    /// in ~/Library/Preferences, so this goes through `defaults`, and nothing cfprefsd caches
    /// goes stale. Domains the twin already has are left alone.
    static func copyPrefs(_ plan: ClonePlan) throws -> Bool {
        let id = plan.source.bundleID
        guard !id.isEmpty else { return false }
        let container = AppPaths.home.appendingPathComponent("Library/Containers/\(id)/Data/Library/Preferences")
        let main = container.appendingPathComponent("\(id).plist")
        var copied = try importDomain(id, file: exists(main.path) ? main.path : nil, into: plan.bundleID)
        // A container holds only the app's own domains; libtwinisolate gives the twin
        // "<domain>.twin-<label>" for each one besides the main domain.
        for name in entries(container.path) where name.hasSuffix(".plist") {
            let domain = String(name.dropLast(".plist".count))
            let path = container.appendingPathComponent(name).path
            if domain.caseInsensitiveCompare(id) == .orderedSame || domain.hasPrefix("com.apple.") || isSymlink(path) {
                continue
            }
            if try importDomain(domain, file: path, into: "\(domain).twin-\(plan.label)") { copied = true }
        }
        return copied
    }

    /// Copies a domain (or a plist file, when given) into `target`, unless `target` exists.
    private static func importDomain(_ domain: String, file: String?, into target: String) throws -> Bool {
        let defaults = "/usr/bin/defaults"
        if (try? CloneEngine.run(defaults, ["read", target])) != nil { return false }
        var source = file
        var temporary: URL?
        if source == nil {
            guard (try? CloneEngine.run(defaults, ["read", domain])) != nil else { return false } // nothing to copy
            let url = FileManager.default.temporaryDirectory.appendingPathComponent("twin-prefs-\(UUID().uuidString).plist")
            try CloneEngine.run(defaults, ["export", domain, url.path])
            temporary = url
            source = url.path
        }
        defer { if let temporary { try? FileManager.default.removeItem(at: temporary) } }
        try CloneEngine.run(defaults, ["import", target, source!])
        return true
    }

    // MARK: Helpers

    static func originalRunning(_ plan: ClonePlan) -> Bool {
        !plan.source.bundleID.isEmpty && !NSRunningApplication.runningApplications(withBundleIdentifier: plan.source.bundleID).isEmpty
    }

    /// CFBundleName, which Electron apps and others name their data folder after.
    static func bundleName(_ app: URL) -> String {
        let info = NSDictionary(contentsOf: app.appendingPathComponent("Contents/Info.plist"))
        return info?["CFBundleName"] as? String ?? ""
    }

    static func entries(_ path: String) -> [String] {
        ((try? FileManager.default.contentsOfDirectory(atPath: path)) ?? []).sorted()
    }

    static func within(_ path: String, _ dir: String) -> Bool { path.hasPrefix(dir + "/") }

    static func exists(_ path: String) -> Bool {
        var info = stat()
        return lstat(path, &info) == 0
    }

    static func isSymlink(_ path: String) -> Bool {
        var info = stat()
        return lstat(path, &info) == 0 && info.st_mode & S_IFMT == S_IFLNK
    }

    /// A directory, not a symlink to one (Go's DirEntry.IsDir).
    static func isRealDirectory(_ path: String) -> Bool {
        var info = stat()
        return lstat(path, &info) == 0 && info.st_mode & S_IFMT == S_IFDIR
    }

    static func isDirectory(_ path: String) -> Bool {
        var isDir: ObjCBool = false
        return FileManager.default.fileExists(atPath: path, isDirectory: &isDir) && isDir.boolValue
    }

    static func realPath(_ path: String) -> String? {
        guard let resolved = realpath(path, nil) else { return nil }
        defer { free(resolved) }
        return String(cString: resolved)
    }
}
