import Foundation

/// Finds .app bundles the same way the CLI does: depth 2 under each search root.
enum AppScanner {
    static var roots: [URL] {
        TwinEnv.searchDirs ?? [
            URL(fileURLWithPath: "/Applications", isDirectory: true),
            AppPaths.home.appendingPathComponent("Applications", isDirectory: true),
        ]
    }

    static func scan() -> [AppInfo] {
        let fm = FileManager.default
        var found: [AppInfo] = []

        for root in roots {
            guard let entries = try? fm.contentsOfDirectory(at: root, includingPropertiesForKeys: [.isDirectoryKey]) else { continue }
            for entry in entries {
                if entry.pathExtension == "app" {
                    if let app = load(entry) { found.append(app) }
                } else if isDirectory(entry),
                          let inner = try? fm.contentsOfDirectory(at: entry, includingPropertiesForKeys: nil) {
                    found += inner.filter { $0.pathExtension == "app" }.compactMap(load)
                }
            }
        }

        return found.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    static func load(_ url: URL) -> AppInfo? {
        let contents = url.appendingPathComponent("Contents", isDirectory: true)
        guard let info = NSDictionary(contentsOf: contents.appendingPathComponent("Info.plist")) as? [String: Any],
              let executable = info["CFBundleExecutable"] as? String, !executable.isEmpty
        else { return nil }

        let real = contents.appendingPathComponent("MacOS/\(executable).real")
        return AppInfo(
            url: url,
            name: url.deletingPathExtension().lastPathComponent,
            bundleID: info["CFBundleIdentifier"] as? String ?? "",
            version: info["CFBundleShortVersionString"] as? String,
            executable: executable,
            isClone: FileManager.default.fileExists(atPath: real.path)
        )
    }

    private static func isDirectory(_ url: URL) -> Bool {
        (try? url.resourceValues(forKeys: [.isDirectoryKey]))?.isDirectory == true
    }
}
