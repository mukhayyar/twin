import AppKit
import ImageIO
import UniformTypeIdentifiers

/// Renders the window without screen-recording permission.
enum WindowCapture {
    private typealias WindowListCreateImage = @convention(c) (CGRect, UInt32, UInt32, UInt32) -> Unmanaged<CGImage>?

    /// CGWindowListCreateImage is hidden from newer SDKs but still exported; an app may capture its own
    /// windows with it without screen-recording permission. GPU-composited, so ~100x faster than cacheDisplay.
    private static let windowListCreateImage: WindowListCreateImage? = {
        guard let symbol = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "CGWindowListCreateImage") else { return nil }
        return unsafeBitCast(symbol, to: WindowListCreateImage.self)
    }()

    /// The composited window (chrome included, no shadow), or nil if the window server refuses.
    static func windowServerImage(of window: NSWindow) -> CGImage? {
        let includingWindow: UInt32 = 1 << 3
        let boundsIgnoreFraming: UInt32 = 1 << 0, bestResolution: UInt32 = 1 << 3
        return windowListCreateImage?(.null, includingWindow, CGWindowID(window.windowNumber),
                                      boundsIgnoreFraming | bestResolution)?.takeRetainedValue()
    }

    /// Renders `view` at a fixed 2x scale, independent of the display's backing scale.
    /// On a 2x display the view's own backing-store format is used, which is much faster.
    static func image(of view: NSView, scale: CGFloat = 2) -> CGImage? {
        let bounds = view.bounds
        if TwinEnv.captureMode != "custom", (view.window?.backingScaleFactor ?? 0) == scale,
           let rep = view.bitmapImageRepForCachingDisplay(in: bounds) {
            view.cacheDisplay(in: bounds, to: rep)
            return rep.cgImage
        }
        guard let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: Int(bounds.width * scale), pixelsHigh: Int(bounds.height * scale),
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
        ) else { return nil }
        rep.size = bounds.size
        view.cacheDisplay(in: bounds, to: rep)
        return rep.cgImage
    }

    /// The whole window including title bar, traffic lights and toolbar (the theme frame),
    /// falling back to the content view.
    static func frameView(of window: NSWindow) -> NSView? {
        window.contentView?.superview ?? window.contentView
    }

    /// README screenshot: the window with rounded corners, hairline border and a soft shadow.
    static func snapshot(_ window: NSWindow, to url: URL) throws {
        guard let view = frameView(of: window), let content = image(of: view) else { throw CloneError("Could not render the window.") }

        let scale: CGFloat = 2
        let padding: CGFloat = 56 * scale
        let radius: CGFloat = 14 * scale
        let width = CGFloat(content.width) + padding * 2
        let height = CGFloat(content.height) + padding * 2

        guard let ctx = CGContext(
            data: nil, width: Int(width), height: Int(height), bitsPerComponent: 8, bytesPerRow: 0,
            space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ) else { throw CloneError("Could not create a drawing context.") }

        let frame = CGRect(x: padding, y: padding, width: CGFloat(content.width), height: CGFloat(content.height))
        let shape = CGPath(roundedRect: frame, cornerWidth: radius, cornerHeight: radius, transform: nil)

        var background = NSColor.windowBackgroundColor.cgColor
        window.effectiveAppearance.performAsCurrentDrawingAppearance {
            background = NSColor.windowBackgroundColor.cgColor
        }

        ctx.saveGState()
        ctx.setShadow(offset: CGSize(width: 0, height: -14 * scale), blur: 40 * scale, color: NSColor.black.withAlphaComponent(0.32).cgColor)
        ctx.addPath(shape)
        ctx.setFillColor(background)
        ctx.fillPath()
        ctx.restoreGState()

        ctx.saveGState()
        ctx.addPath(shape)
        ctx.clip()
        ctx.draw(content, in: frame)
        ctx.restoreGState()

        ctx.addPath(shape)
        ctx.setStrokeColor(NSColor(white: 0.5, alpha: 0.35).cgColor)
        ctx.setLineWidth(1 * scale)
        ctx.strokePath()

        guard let output = ctx.makeImage() else { throw CloneError("Could not finish the snapshot.") }
        try writePNG(output, to: url)
    }

    static func writePNG(_ image: CGImage, to url: URL) throws {
        guard let destination = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil) else {
            throw CloneError("Could not write \(url.path).")
        }
        CGImageDestinationAddImage(destination, image, nil)
        guard CGImageDestinationFinalize(destination) else { throw CloneError("Could not write \(url.path).") }
    }
}

/// Captures the window into numbered PNGs at a fixed frame rate. When a capture runs late,
/// the frame is repeated for the missed slots so the sequence stays in sync with wall-clock time.
@MainActor
final class FrameRecorder {
    private let window: NSWindow
    private let directory: URL
    private let fps: Double
    private let queue = DispatchQueue(label: "twin.frames", qos: .userInitiated, attributes: .concurrent)
    private let group = DispatchGroup()
    private var timer: Timer?
    private var startTime: CFTimeInterval = 0
    private var nextIndex = 0
    private var inFlight = 0
    private var captures = 0
    private var captureTime: CFTimeInterval = 0
    private var pixelSize = CGSize.zero

    init(window: NSWindow, directory: URL, fps: Double) {
        self.window = window
        self.directory = directory
        self.fps = fps
    }

    func start() {
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        startTime = CACurrentMediaTime()
        let timer = Timer(timeInterval: 1 / fps, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.tick() }
        }
        RunLoop.main.add(timer, forMode: .common)
        self.timer = timer
        tick()
    }

    func finish() async {
        tick()
        timer?.invalidate()
        timer = nil
        await withCheckedContinuation { continuation in
            group.notify(queue: .main) { continuation.resume() }
        }
        let summary = String(format: "{\"frames\": %d, \"fps\": %.0f, \"seconds\": %.1f, \"captures\": %d, \"avgCaptureMs\": %.1f, \"width\": %d, \"height\": %d}\n",
                             nextIndex, fps, Double(nextIndex) / fps, captures, captures > 0 ? captureTime / Double(captures) * 1000 : 0,
                             Int(pixelSize.width), Int(pixelSize.height))
        print(summary, terminator: "")
        try? summary.write(to: directory.appendingPathComponent("frames.json"), atomically: true, encoding: .utf8)
    }

    private func tick() {
        let target = Int((CACurrentMediaTime() - startTime) * fps)
        // Skip while the encoder is backed up; the next capture fills the gap.
        guard target >= nextIndex, inFlight < 12,
              let view = TwinEnv.framesIncludeChrome ? WindowCapture.frameView(of: window) : window.contentView else { return }
        let began = CACurrentMediaTime()
        let fast = TwinEnv.captureMode == "content" ? nil : WindowCapture.windowServerImage(of: window)
        guard let image = fast ?? WindowCapture.image(of: view) else { return }
        captureTime += CACurrentMediaTime() - began

        let indices = Array(nextIndex...target)
        nextIndex = target + 1
        captures += 1
        inFlight += 1
        pixelSize = CGSize(width: image.width, height: image.height)

        let directory = self.directory
        let group = self.group
        group.enter()
        queue.async { [weak self] in
            let first = directory.appendingPathComponent(String(format: "frame_%05d.png", indices[0] + 1))
            try? WindowCapture.writePNG(image, to: first)
            for index in indices.dropFirst() {
                let copy = directory.appendingPathComponent(String(format: "frame_%05d.png", index + 1))
                try? FileManager.default.removeItem(at: copy)
                try? FileManager.default.copyItem(at: first, to: copy)
            }
            DispatchQueue.main.async { self?.inFlight -= 1 }
            group.leave()
        }
    }
}
