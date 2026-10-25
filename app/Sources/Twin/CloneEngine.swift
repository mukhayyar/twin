import AppKit

enum StepStatus: Equatable {
    case pending, running, done
    case warning(String)
    case failed(String)

    var isFinished: Bool {
        switch self {
        case .done, .warning: return true
        default: return false
        }
    }
}

struct CloneStep: Identifiable, Equatable {
    let id: Int
    let title: String
    var status: StepStatus = .pending
    var duration: TimeInterval?
}

/// The six clone steps. Titles and behavior mirror the CLI; steps 3, 5 and 6 are best effort.
enum CloneEngine {
    static let stepTitles = [
        "Copy app bundle",
        "Update bundle identifier",
        "Paint icon & badge",
        "Inject isolated profile launcher",
        "Clear quarantine & re-sign",
        "Register with Launch Services",
    ]

    /// Failures in these steps abort the clone and remove the partial copy.
    static let fatalSteps: Set<Int> = [0, 1, 3]

    enum Outcome {
        case ok
        case warning(String)
        case failure(String)
    }

    static let lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

    static func perform(step: Int, plan: ClonePlan, replacing: Bool) -> Outcome {
        if TwinEnv.isDemo {
            Thread.sleep(forTimeInterval: .random(in: 0.6...0.9))
            return .ok
        }

        let dest = plan.destination.path
        do {
            switch step {
            case 0: try copyBundle(plan, replacing: replacing)
            case 1: try updateIdentity(plan)
            case 2: return paintIcon(plan)
            case 3: try injectLauncher(plan)
            case 4: return bestEffort([
                    ["/usr/bin/xattr", "-cr", dest],
                    ["/usr/bin/codesign", "--force", "--deep", "--sign", "-", dest],
                ])
            case 5: return bestEffort([[lsregister, "-f", dest]])
            default: break
            }
            return .ok
        } catch {
            return .failure(error.localizedDescription)
        }
    }

    static func rollback(_ plan: ClonePlan) {
        guard !TwinEnv.isDemo else { return }
        try? FileManager.default.removeItem(at: plan.destination)
    }

    /// The script that replaces a twin's main executable. Every twin gets the same
    /// isolation; no app is special-cased. Byte-identical to the CLI's `launcher`.
    static func wrapperScript(plan: ClonePlan) -> String {
        let name = bashEscape(plan.cloneName)
        return """
        #!/usr/bin/env bash
        PROFILE_DIR="$HOME/.app_profiles/\(name)"
        mkdir -p "$PROFILE_DIR/Library"

        # macOS finds the login keychain under $HOME, so keep it reachable before HOME
        # moves. Keychain items stay separate: libtwinisolate gives them twin names.
        [ -e "$PROFILE_DIR/Library/Keychains" ] || ln -s "$HOME/Library/Keychains" "$PROFILE_DIR/Library/Keychains"

        # Isolate everything else. HOME and CFFIXED_USER_HOME move files and ~/Library;
        # libtwinisolate covers /tmp, temp and cache folders, preferences and keychain items.
        export TWIN_NAME="\(name)" TWIN_LABEL="\(plan.label)" TWIN_ID="\(bashEscape(plan.bundleID))" TWIN_ORIG_ID="\(bashEscape(plan.source.bundleID))"
        export TWIN_ROOT="/private/tmp/twin.$UID.\(String(format: "%08x", IconPainter.fnv1a(plan.cloneName)))" TWIN_CACHE="$PROFILE_DIR/Library/Caches/TwinDarwin"
        export TWIN_HARDWARE="\(plan.ownHardware ? "separate" : "shared")" # "shared" lets apps see this Mac's real hardware IDs
        export TMPDIR="$TWIN_ROOT/T/"
        export HOME="$PROFILE_DIR"
        export CFFIXED_USER_HOME="$PROFILE_DIR"
        export DYLD_INSERT_LIBRARIES="$(dirname "$0")/libtwinisolate.dylib"
        exec "$(dirname "$0")/\(bashEscape(plan.source.executable)).real" "$@"

        """
    }

    /// Makes a value safe inside a double-quoted bash string.
    static func bashEscape(_ s: String) -> String {
        s.replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
            .replacingOccurrences(of: "$", with: "\\$")
            .replacingOccurrences(of: "`", with: "\\`")
    }

    // MARK: Steps

    private static func infoPlist(_ plan: ClonePlan) -> String {
        plan.destination.appendingPathComponent("Contents/Info.plist").path
    }

    private static func copyBundle(_ plan: ClonePlan, replacing: Bool) throws {
        let fm = FileManager.default
        if fm.fileExists(atPath: plan.destination.path) {
            guard replacing else { throw CloneError("\(AppPaths.display(plan.destination)) already exists.") }
            try fm.removeItem(at: plan.destination)
        }
        try run("/usr/bin/ditto", [plan.source.url.path, plan.destination.path])
    }

    /// Only the bundle ID changes. CFBundleName must stay the original: Electron apps locate
    /// "<CFBundleName> Helper.app" and crash at launch if it's renamed. Finder and the Dock
    /// show the bundle's file name ("<Name>-<Label>") anyway.
    private static func updateIdentity(_ plan: ClonePlan) throws {
        try run("/usr/bin/plutil", ["-replace", "CFBundleIdentifier", "-string", plan.bundleID, infoPlist(plan)])
    }

    /// Non-fatal: on any failure the clone keeps the original icon.
    private static func paintIcon(_ plan: ClonePlan) -> Outcome {
        guard plan.icon.paintsAnything else { return .ok }
        do {
            guard let image = IconPainter.render(source: plan.source.url, label: plan.label, options: plan.icon) else {
                throw CloneError("Could not render the icon.")
            }
            let resources = plan.destination.appendingPathComponent("Contents/Resources", isDirectory: true)
            try FileManager.default.createDirectory(at: resources, withIntermediateDirectories: true)
            try IconPainter.writeIcns(image, to: resources.appendingPathComponent("TwinIcon.icns"))
            try run("/usr/bin/plutil", ["-replace", "CFBundleIconFile", "-string", "TwinIcon", infoPlist(plan)])
            _ = try? run("/usr/bin/plutil", ["-remove", "CFBundleIconName", infoPlist(plan)])
            _ = try? run("/usr/bin/touch", [plan.destination.path])
            return .ok
        } catch {
            return .warning("Kept the original icon. \(error.localizedDescription)")
        }
    }

    private static func injectLauncher(_ plan: ClonePlan) throws {
        let fm = FileManager.default
        try fm.createDirectory(at: plan.profileURL, withIntermediateDirectories: true)

        let macOS = plan.destination.appendingPathComponent("Contents/MacOS", isDirectory: true)
        let launcher = macOS.appendingPathComponent(plan.source.executable)
        try fm.moveItem(at: launcher, to: macOS.appendingPathComponent("\(plan.source.executable).real"))

        guard let lib = Bundle.main.url(forResource: "libtwinisolate", withExtension: "dylib") else {
            throw CloneError("libtwinisolate.dylib is missing from Twin.app; rebuild it with scripts/build-app.sh.")
        }
        try fm.copyItem(at: lib, to: macOS.appendingPathComponent("libtwinisolate.dylib"))
        let script = wrapperScript(plan: plan)
        try script.write(to: launcher, atomically: true, encoding: .utf8)
        try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: launcher.path)
    }

    private static func bestEffort(_ commands: [[String]]) -> Outcome {
        var problems: [String] = []
        for command in commands {
            do { try run(command[0], Array(command.dropFirst())) }
            catch { problems.append(error.localizedDescription) }
        }
        return problems.isEmpty ? .ok : .warning(problems.joined(separator: "\n"))
    }

    @discardableResult
    static func run(_ tool: String, _ arguments: [String]) throws -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: tool)
        process.arguments = arguments
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe

        try process.run()
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()

        let output = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        guard process.terminationStatus == 0 else {
            let name = (tool as NSString).lastPathComponent
            throw CloneError(output.isEmpty ? "\(name) exited with status \(process.terminationStatus)." : "\(name): \(output)")
        }
        return output
    }
}

struct CloneError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

/// One clone run, published to the UI step by step.
@MainActor
final class CloneJob: ObservableObject {
    enum Phase { case running, succeeded, failed }

    let plan: ClonePlan
    @Published private(set) var steps: [CloneStep]
    @Published private(set) var phase: Phase = .running
    /// The painted icon, rendered with the same code path as the engine.
    @Published private(set) var icon: NSImage?
    var onFinish: (() -> Void)?

    init(plan: ClonePlan, icon: NSImage? = nil) {
        self.plan = plan
        self.icon = icon
        self.steps = CloneEngine.stepTitles.enumerated().map { CloneStep(id: $0.offset, title: $0.element) }
    }

    /// A finished job for screenshots; nothing runs.
    init(previewOf plan: ClonePlan, icon: NSImage?) {
        self.plan = plan
        self.icon = icon
        let durations = [0.9, 0.1, 0.4, 0.1, 1.4, 0.3]
        self.steps = CloneEngine.stepTitles.enumerated().map {
            CloneStep(id: $0.offset, title: $0.element, status: .done, duration: durations[$0.offset])
        }
        self.phase = .succeeded
    }

    var progress: Double {
        Double(steps.filter { $0.status.isFinished }.count) / Double(steps.count)
    }

    var warnings: [String] {
        steps.compactMap { if case .warning(let message) = $0.status { return message } else { return nil } }
    }

    func start(replacing: Bool) {
        let plan = self.plan
        if icon == nil, plan.icon.paintsAnything {
            Task {
                let image = await Task.detached(priority: .userInitiated) {
                    IconPainter.render(source: plan.source.url, label: plan.label, options: plan.icon, size: 512)
                }.value
                if let image { icon = NSImage(cgImage: image, size: NSSize(width: 512, height: 512)) }
            }
        }

        Task {
            for index in steps.indices {
                steps[index].status = .running
                let started = Date()
                let outcome = await Task.detached(priority: .userInitiated) {
                    CloneEngine.perform(step: index, plan: plan, replacing: replacing)
                }.value
                steps[index].duration = Date().timeIntervalSince(started)

                switch outcome {
                case .ok:
                    steps[index].status = .done
                case .warning(let message):
                    steps[index].status = .warning(message)
                case .failure(let message):
                    steps[index].status = .failed(message)
                    if CloneEngine.fatalSteps.contains(index) {
                        await Task.detached { CloneEngine.rollback(plan) }.value
                    }
                    phase = .failed
                    onFinish?()
                    return
                }
            }
            phase = .succeeded
            onFinish?()
        }
    }
}
