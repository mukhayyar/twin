import AppKit
import CryptoKit
import SwiftUI

/// Checks GitHub for a newer Twin, downloads it, verifies it, and swaps it in.
@MainActor
final class Updater: ObservableObject {
    static let shared = Updater()
    static let repo = "mukhayyar/twin"

    struct Release: Equatable {
        let version: String
        /// The "What's new" part of the release notes.
        let changes: [String]
        let page: URL
        let zip: URL
        let checksums: URL?
    }

    enum State: Equatable {
        case idle, checking, upToDate
        case available(Release)
        case downloading(Double)
        case installing
        case failed(String)
    }

    @Published private(set) var state: State = .idle
    @Published var automaticChecks: Bool {
        didSet { defaults.set(automaticChecks, forKey: Keys.automatic) }
    }

    private enum Keys {
        static let automatic = "TwinAutomaticUpdateChecks"
        static let lastCheck = "TwinLastUpdateCheck"
        static let skipped = "TwinSkippedVersion"
    }
    private let defaults = UserDefaults.standard

    private init() {
        automaticChecks = defaults.object(forKey: Keys.automatic) as? Bool ?? true
    }

    var currentVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.0.0"
    }

    /// Homebrew keeps its own record of the cask; it's worth mentioning, not blocking.
    var installedWithHomebrew: Bool {
        ["/opt/homebrew/Caskroom/twin", "/usr/local/Caskroom/twin"].contains { FileManager.default.fileExists(atPath: $0) }
    }

    // MARK: Checking

    /// At launch: at most once a day, quietly, and only says something when there's an update.
    func checkInBackground() {
        guard automaticChecks, !TwinEnv.isAutomated else { return }
        let last = defaults.object(forKey: Keys.lastCheck) as? Date ?? .distantPast
        guard Date().timeIntervalSince(last) > 24 * 60 * 60 else { return }
        Task { await check(userInitiated: false) }
    }

    func check(userInitiated: Bool) async {
        if case .downloading = state { return }
        if userInitiated { UpdateWindow.show() }
        state = .checking
        do {
            let release = try await fetchLatest()
            defaults.set(Date(), forKey: Keys.lastCheck)
            guard Self.isNewer(release.version, than: currentVersion) else {
                state = userInitiated ? .upToDate : .idle
                return
            }
            state = .available(release)
            if TwinEnv.autoInstallUpdate { install(release) }
            if userInitiated || defaults.string(forKey: Keys.skipped) != release.version {
                UpdateWindow.show()
            }
        } catch {
            state = userInitiated ? .failed(error.localizedDescription) : .idle
        }
    }

    func skip(_ release: Release) {
        defaults.set(release.version, forKey: Keys.skipped)
        state = .idle
        UpdateWindow.close()
    }

    func later() {
        if case .available = state {} else { state = .idle }
        UpdateWindow.close()
    }

    private func fetchLatest() async throws -> Release {
        var request = URLRequest(url: URL(string: "https://api.github.com/repos/\(Self.repo)/releases/latest")!)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("Twin/\(currentVersion)", forHTTPHeaderField: "User-Agent")
        let (data, response) = try await URLSession.shared.data(for: request)
        guard (response as? HTTPURLResponse)?.statusCode == 200 else {
            throw UpdateError("GitHub didn't answer (HTTP \((response as? HTTPURLResponse)?.statusCode ?? 0)). Try again later.")
        }
        struct GitHubRelease: Decodable {
            struct Asset: Decodable {
                let name: String
                let browser_download_url: URL
            }
            let tag_name: String
            let body: String?
            let html_url: URL
            let assets: [Asset]
        }
        let latest = try JSONDecoder().decode(GitHubRelease.self, from: data)
        let version = latest.tag_name.hasPrefix("v") ? String(latest.tag_name.dropFirst()) : latest.tag_name
        guard let zip = latest.assets.first(where: { $0.name == "Twin-\(version).zip" }) else {
            throw UpdateError("Twin \(version) has no app download yet. Try again in a few minutes.")
        }
        return Release(version: version, changes: Self.changes(in: latest.body ?? ""), page: latest.html_url,
                       zip: zip.browser_download_url,
                       checksums: latest.assets.first { $0.name == "checksums.txt" }?.browser_download_url)
    }

    /// Compares dotted versions numerically; a suffix like "-dev" counts as older.
    nonisolated static func isNewer(_ candidate: String, than current: String) -> Bool {
        func parts(_ v: String) -> [Int] {
            let core = v.split(separator: "-", maxSplits: 1).first.map(String.init) ?? v
            return core.split(separator: ".").map { Int($0) ?? 0 }
        }
        let a = parts(candidate), b = parts(current)
        for i in 0..<max(a.count, b.count) {
            let x = i < a.count ? a[i] : 0, y = i < b.count ? b[i] : 0
            if x != y { return x > y }
        }
        return current.contains("-") && !candidate.contains("-")
    }

    /// The bullet lines under "What's new" in the release notes.
    nonisolated static func changes(in notes: String) -> [String] {
        var out: [String] = [], inside = false
        for line in notes.components(separatedBy: .newlines) {
            if line.hasPrefix("#") {
                inside = line.lowercased().contains("what's new") || line.lowercased().contains("what’s new")
                continue
            }
            if inside, line.hasPrefix("- ") || line.hasPrefix("* ") { out.append(String(line.dropFirst(2))) }
        }
        return out
    }

    // MARK: Installing

    func install(_ release: Release) {
        Task {
            do {
                state = .downloading(0)
                let zip = try await Downloader.fetch(release.zip) { fraction in
                    Task { @MainActor in
                        if case .downloading = Updater.shared.state { Updater.shared.state = .downloading(fraction) }
                    }
                }
                if let checksums = release.checksums {
                    try await Self.verify(zip, named: release.zip.lastPathComponent, against: checksums)
                }
                state = .installing
                let app = try await Task.detached { try Self.unpack(zip, version: release.version) }.value
                try relaunch(into: app)
            } catch {
                state = .failed(error.localizedDescription)
            }
        }
    }

    private static func verify(_ file: URL, named name: String, against checksums: URL) async throws {
        let (data, _) = try await URLSession.shared.data(from: checksums)
        let expected = String(decoding: data, as: UTF8.self).components(separatedBy: .newlines)
            .map { $0.split(separator: " ", omittingEmptySubsequences: true) }
            .first { $0.count == 2 && $0[1] == name }?[0]
        let actual = SHA256.hash(data: try Data(contentsOf: file)).map { String(format: "%02x", $0) }.joined()
        guard let expected, expected.lowercased() == actual else {
            throw UpdateError("The download didn't match its published checksum, so it wasn't installed.")
        }
    }

    nonisolated private static func unpack(_ zip: URL, version: String) throws -> URL {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("TwinUpdate-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        try CloneEngine.run("/usr/bin/ditto", ["-x", "-k", zip.path, dir.path])
        try? FileManager.default.removeItem(at: zip)
        let app = dir.appendingPathComponent("Twin.app")
        let bundle = Bundle(url: app)
        guard bundle?.bundleIdentifier == "com.mukhayyar.twin",
              bundle?.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String == version else {
            throw UpdateError("The download isn't Twin \(version), so it wasn't installed.")
        }
        try CloneEngine.run("/usr/bin/codesign", ["--verify", "--deep", app.path])
        return app
    }

    /// Hands over to a small script that waits for Twin to quit, swaps the new version in
    /// (putting the old one back if anything fails), and opens it.
    private func relaunch(into newApp: URL) throws {
        let current = Bundle.main.bundleURL
        let folder = current.deletingLastPathComponent()
        guard current.pathExtension == "app", FileManager.default.isWritableFile(atPath: folder.path) else {
            NSWorkspace.shared.activateFileViewerSelecting([newApp])
            throw UpdateError("Twin can't replace itself in \(AppPaths.display(folder)). The new version is selected in Finder: drag it to Applications.")
        }
        let script = """
        while kill -0 "$1" 2>/dev/null; do sleep 0.2; done
        rm -rf "$2.old"
        if mv "$2" "$2.old" && ditto "$3" "$2"; then rm -rf "$2.old"; else rm -rf "$2"; mv "$2.old" "$2"; fi
        xattr -dr com.apple.quarantine "$2" 2>/dev/null
        rm -rf "$(dirname "$3")"
        open "$2"
        """
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/sh")
        process.arguments = ["-c", script, "twin-update", String(getpid()), current.path, newApp.path]
        try process.run()
        NSApp.terminate(nil)
    }
}

struct UpdateError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

/// A download with progress, for the update archive.
private final class Downloader: NSObject, URLSessionDownloadDelegate {
    private let progress: (Double) -> Void
    private var continuation: CheckedContinuation<URL, Error>?

    private init(progress: @escaping (Double) -> Void) { self.progress = progress }

    static func fetch(_ url: URL, progress: @escaping (Double) -> Void) async throws -> URL {
        let delegate = Downloader(progress: progress)
        let session = URLSession(configuration: .ephemeral, delegate: delegate, delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        return try await withCheckedThrowingContinuation { continuation in
            delegate.continuation = continuation
            session.downloadTask(with: url).resume()
        }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData _: Int64,
                    totalBytesWritten written: Int64, totalBytesExpectedToWrite total: Int64) {
        if total > 0 { progress(Double(written) / Double(total)) }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard (downloadTask.response as? HTTPURLResponse)?.statusCode == 200 else {
            return finish(.failure(UpdateError("The download failed (HTTP \((downloadTask.response as? HTTPURLResponse)?.statusCode ?? 0)).")))
        }
        // The file is deleted when this returns, so move it first.
        let kept = FileManager.default.temporaryDirectory.appendingPathComponent("Twin-\(UUID().uuidString).zip")
        do {
            try FileManager.default.moveItem(at: location, to: kept)
            finish(.success(kept))
        } catch {
            finish(.failure(error))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error { finish(.failure(error)) }
    }

    private func finish(_ result: Result<URL, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }
}

/// The Software Update window, shown for manual checks and when an update turns up.
@MainActor
enum UpdateWindow {
    private static var window: NSWindow?

    static func show() {
        if window == nil {
            let host = NSHostingController(rootView: UpdateView().environmentObject(Updater.shared))
            let w = NSWindow(contentViewController: host)
            w.title = "Software Update"
            w.styleMask = [.titled, .closable, .fullSizeContentView]
            w.titlebarAppearsTransparent = true
            w.isReleasedWhenClosed = false
            w.center()
            window = w
        }
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }

    static func close() { window?.close() }

    static var contentView: NSView? { window?.contentView }
}

struct UpdateView: View {
    @EnvironmentObject private var updater: Updater

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .top, spacing: 14) {
                Image(nsImage: NSApp.applicationIconImage)
                    .resizable()
                    .frame(width: 56, height: 56)
                VStack(alignment: .leading, spacing: 4) {
                    Text(title).font(.headline)
                    Text(subtitle)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    if case .available(let release) = updater.state {
                        Link("Release notes", destination: release.page).font(.callout)
                    }
                }
            }
            content
            Divider()
            Toggle("Check for updates automatically", isOn: $updater.automaticChecks)
                .toggleStyle(.checkbox)
                .font(.callout)
        }
        .padding(20)
        .padding(.top, 8)
        .frame(width: 440)
    }

    private var title: String {
        switch updater.state {
        case .idle, .checking: return "Checking for updates…"
        case .upToDate: return "Twin is up to date"
        case .available(let release): return "Twin \(release.version) is available"
        case .downloading: return "Downloading the update…"
        case .installing: return "Installing…"
        case .failed: return "Couldn't update Twin"
        }
    }

    private var subtitle: String {
        switch updater.state {
        case .upToDate: return "Version \(updater.currentVersion) is the newest."
        case .available: return "You have \(updater.currentVersion). Twin relaunches after installing; your twins and their data aren't touched."
        case .installing: return "Twin will relaunch in a moment."
        case .failed(let message): return message
        default: return "Version \(updater.currentVersion)"
        }
    }

    @ViewBuilder
    private var content: some View {
        switch updater.state {
        case .idle, .checking, .installing:
            ProgressView().controlSize(.small)
        case .upToDate:
            buttons { Button("OK") { UpdateWindow.close() }.keyboardShortcut(.defaultAction) }
        case .available(let release):
            if !release.changes.isEmpty {
                ScrollView {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("What's new").font(.subheadline.weight(.semibold))
                        ForEach(release.changes, id: \.self) { change in
                            HStack(alignment: .firstTextBaseline, spacing: 6) {
                                Text("•").foregroundStyle(.secondary)
                                Text((try? AttributedString(markdown: change)) ?? AttributedString(change))
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            .font(.callout)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                }
                .frame(maxHeight: 180)
                .background(RoundedRectangle(cornerRadius: 8).fill(Color(nsColor: .textBackgroundColor)))
            }
            if updater.installedWithHomebrew {
                Text("Installed with Homebrew? `brew upgrade --cask twin` works too.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            buttons {
                Button("Skip This Version") { updater.skip(release) }
                Spacer()
                Button("Later") { updater.later() }
                Button("Install and Relaunch") { updater.install(release) }.keyboardShortcut(.defaultAction)
            }
        case .downloading(let fraction):
            ProgressView(value: fraction) {
                Text("\(Int(fraction * 100))%").font(.caption).foregroundStyle(.secondary)
            }
        case .failed:
            buttons {
                Spacer()
                Button("Close") { UpdateWindow.close() }
                Button("Try Again") { Task { await updater.check(userInitiated: true) } }.keyboardShortcut(.defaultAction)
            }
        }
    }

    private func buttons<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        HStack { content() }.frame(maxWidth: .infinity, alignment: .trailing)
    }
}
