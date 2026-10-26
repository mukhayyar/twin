import AppKit
import QuickLookThumbnailing
import CoreImage

/// How a twin's icon is painted. Mirrors the CLI's `--color auto|none|<name>|#hex` and `--no-badge`.
struct IconOptions: Hashable {
    enum ColorChoice: Hashable {
        case auto
        case rgb(UInt32)
    }

    var color: ColorChoice = .auto
    var badge = true
    var tint = true

    var paintsAnything: Bool { badge || tint }
}

/// Recolors an app icon toward the label's color and stamps a label badge on it.
/// The spec matches the CLI's embedded JXA script so both produce the same icons.
enum IconPainter {
    static let labelColors: [String: UInt32] = [
        "work": 0x0A84FF, "personal": 0x30D158, "hobby": 0xFF9F0A, "school": 0xBF5AF2,
    ]
    static let presetSwatches: [UInt32] = [0x0A84FF, 0x30D158, 0xFF9F0A, 0xBF5AF2]
    static let palette: [UInt32] = [0xFF375F, 0x40C8E0, 0x5E5CE6, 0xFFD60A, 0xFF453A, 0x63E6BE, 0xAC8E68, 0x64D2FF]

    private static let context = CIContext()
    private static let sRGB = CGColorSpace(name: CGColorSpace.sRGB)!

    // MARK: Color

    static func autoColor(for label: String) -> UInt32 {
        let key = label.lowercased()
        if let preset = labelColors[key] { return preset }
        return palette[Int(fnv1a(key) % UInt32(palette.count))]
    }

    static func color(for label: String, options: IconOptions) -> UInt32 {
        switch options.color {
        case .auto: return autoColor(for: label)
        case .rgb(let hex): return hex
        }
    }

    static func fnv1a(_ string: String) -> UInt32 {
        var hash: UInt32 = 0x811C_9DC5
        for byte in string.utf8 {
            hash ^= UInt32(byte)
            hash = hash &* 0x0100_0193
        }
        return hash
    }

    static func nsColor(_ hex: UInt32) -> NSColor {
        NSColor(srgbRed: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255,
                blue: CGFloat(hex & 0xFF) / 255, alpha: 1)
    }

    static func hsb(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat) -> (hue: CGFloat, saturation: CGFloat, brightness: CGFloat) {
        let high = max(r, g, b), low = min(r, g, b), delta = high - low
        var hue: CGFloat = 0
        if delta > 0 {
            if high == r { hue = (g - b) / delta + (g < b ? 6 : 0) }
            else if high == g { hue = (b - r) / delta + 2 }
            else { hue = (r - g) / delta + 4 }
            hue /= 6
        }
        return (hue, high == 0 ? 0 : delta / high, high)
    }

    // MARK: Rendering

    /// The twin's icon at `size` px: the source icon, tinted and badged per `options`.
    static func render(source: URL, label: String, options: IconOptions, size: Int = 1024) -> CGImage? {
        guard var image = appIcon(source, size: size) else { return nil }

        let color = self.color(for: label, options: options)
        if options.tint, let tinted = tint(image, toward: color) { image = tinted }
        if options.badge, let badged = badge(image, label: label, color: color) { image = badged }
        return image
    }

    /// The app's icon as Finder shows it. NSWorkspace may hand back its "still loading"
    /// placeholder, which a single draw bakes into the preview and the twin's icon; Quick Look
    /// answers only once the real icon is ready. NSWorkspace stays as the fallback, and on the
    /// main thread, where waiting for Quick Look would block.
    static func appIcon(_ source: URL, size: Int) -> CGImage? {
        // Resolve symlinks first, or the alias arrow gets baked into the icon (e.g. /Applications/Safari.app).
        let url = source.resolvingSymlinksInPath()
        if !Thread.isMainThread, let icon = quickLookIcon(url, size: size) {
            return draw(size: size) { ctx, rect in ctx.draw(icon, in: rect) }
        }
        let icon = NSWorkspace.shared.icon(forFile: url.path)
        return draw(size: size) { _, rect in icon.draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1) }
    }

    private static func quickLookIcon(_ url: URL, size: Int) -> CGImage? {
        let request = QLThumbnailGenerator.Request(fileAt: url, size: CGSize(width: size, height: size), scale: 1,
                                                   representationTypes: .icon)
        let done = DispatchSemaphore(value: 0)
        var icon: CGImage?
        QLThumbnailGenerator.shared.generateBestRepresentation(for: request) { representation, _ in
            icon = representation?.cgImage
            done.signal()
        }
        return done.wait(timeout: .now() + 5) == .success ? icon : nil
    }

    /// Rotates the icon's dominant hue onto the target color with an HSV color cube (hue shift plus a
    /// clamped saturation gain, value kept). Grayscale-ish icons are left alone.
    static func tint(_ image: CGImage, toward hex: UInt32) -> CGImage? {
        let input = CIImage(cgImage: image)
        guard let average = CIFilter(name: "CIAreaAverage", parameters: [
            kCIInputImageKey: input, kCIInputExtentKey: CIVector(cgRect: input.extent),
        ])?.outputImage else { return nil }

        var pixel = [Float](repeating: 0, count: 4)
        context.render(average, toBitmap: &pixel, rowBytes: 16, bounds: CGRect(x: 0, y: 0, width: 1, height: 1),
                       format: .RGBAf, colorSpace: sRGB)
        // Hue and saturation are unaffected by premultiplied alpha (uniform RGB scale).
        let mean = hsb(CGFloat(pixel[0]), CGFloat(pixel[1]), CGFloat(pixel[2]))
        guard mean.saturation >= 0.15 else { return image }

        let target = nsColor(hex)
        let goal = hsb(target.redComponent, target.greenComponent, target.blueComponent)
        let gain = min(1.8, max(0.6, goal.saturation / mean.saturation))
        guard let output = CIFilter(name: "CIColorCubeWithColorSpace", parameters: [
            kCIInputImageKey: input,
            "inputCubeDimension": cubeDimension,
            "inputCubeData": hsvCube(hueShift: goal.hue - mean.hue, saturationGain: gain),
            "inputColorSpace": sRGB,
        ])?.outputImage else { return nil }
        return context.createCGImage(output, from: input.extent, format: .RGBA8, colorSpace: sRGB)
    }

    static let cubeDimension = 32

    static func hsvCube(hueShift: CGFloat, saturationGain: CGFloat) -> Data {
        let n = cubeDimension
        var cube = [Float]()
        cube.reserveCapacity(n * n * n * 4)
        for b in 0..<n {
            for g in 0..<n {
                for r in 0..<n {
                    let source = hsb(CGFloat(r) / CGFloat(n - 1), CGFloat(g) / CGFloat(n - 1), CGFloat(b) / CGFloat(n - 1))
                    var hue = (source.hue + hueShift).truncatingRemainder(dividingBy: 1)
                    if hue < 0 { hue += 1 }
                    let rgb = rgb(hue, min(1, source.saturation * saturationGain), source.brightness)
                    cube += [Float(rgb.r), Float(rgb.g), Float(rgb.b), 1]
                }
            }
        }
        return cube.withUnsafeBufferPointer { Data(buffer: $0) }
    }

    static func rgb(_ h: CGFloat, _ s: CGFloat, _ v: CGFloat) -> (r: CGFloat, g: CGFloat, b: CGFloat) {
        let sector = h * 6
        let f = sector - floor(sector)
        let p = v * (1 - s), q = v * (1 - f * s), t = v * (1 - (1 - f) * s)
        switch Int(floor(sector)) % 6 {
        case 0: return (v, t, p)
        case 1: return (q, v, p)
        case 2: return (p, v, t)
        case 3: return (p, q, v)
        case 4: return (t, p, v)
        default: return (v, p, q)
        }
    }

    struct BadgeLayout {
        let rect: CGRect
        let text: String
        let font: NSFont
    }

    static func roundedHeavyFont(_ size: CGFloat) -> NSFont {
        let base = NSFont.systemFont(ofSize: size, weight: .heavy)
        guard let rounded = base.fontDescriptor.withDesign(.rounded) else { return base }
        return NSFont(descriptor: rounded, size: size) ?? base
    }

    /// Badge geometry on an S×S canvas (bottom-left origin): pill 0.20·S tall whose bottom edge
    /// sits 0.955·S from the top; width = text + 0.11·S, capped at 0.94·S.
    static func badgeLayout(label: String, canvas s: CGFloat) -> BadgeLayout {
        var text = label.replacingOccurrences(of: "-", with: " ")
        let padding = 0.11 * s
        let maxWidth = 0.94 * s
        func width(_ string: String, _ size: CGFloat) -> CGFloat {
            (string as NSString).size(withAttributes: [.font: roundedHeavyFont(size)]).width
        }

        var fontSize = 0.125 * s
        let natural = width(text, fontSize)
        if natural + padding > maxWidth {
            fontSize = max(0.075 * s, fontSize * (maxWidth - padding) / natural)
            if width(text, fontSize) + padding > maxWidth {
                while !text.isEmpty, width(text + "…", fontSize) + padding > maxWidth { text.removeLast() }
                text = text.trimmingCharacters(in: .whitespaces) + "…"
            }
        }

        let pillWidth = min(maxWidth, width(text, fontSize) + padding)
        let height = 0.20 * s
        let rect = CGRect(x: (s - pillWidth) / 2, y: s - 0.955 * s, width: pillWidth, height: height)
        return BadgeLayout(rect: rect, text: text, font: roundedHeavyFont(fontSize))
    }

    static func badge(_ image: CGImage, label: String, color hex: UInt32) -> CGImage? {
        let s = CGFloat(image.width)
        let layout = badgeLayout(label: label, canvas: s)
        return draw(size: image.width) { ctx, rect in
            ctx.draw(image, in: rect)

            let radius = layout.rect.height / 2
            let pill = NSBezierPath(roundedRect: layout.rect, xRadius: radius, yRadius: radius)
            NSGraphicsContext.saveGraphicsState()
            let shadow = NSShadow()
            shadow.shadowColor = NSColor.black.withAlphaComponent(0.35)
            shadow.shadowBlurRadius = 0.02 * s
            shadow.shadowOffset = NSSize(width: 0, height: -0.008 * s)
            shadow.set()
            nsColor(hex).setFill()
            pill.fill()
            NSGraphicsContext.restoreGraphicsState()

            pill.lineWidth = 0.018 * s
            NSColor.white.setStroke()
            pill.stroke()

            // Optical centering on cap height: baseline = pillY + h/2 − capHeight/2.
            let attributes: [NSAttributedString.Key: Any] = [.font: layout.font, .foregroundColor: NSColor.white]
            let width = (layout.text as NSString).size(withAttributes: attributes).width
            let baseline = layout.rect.midY - layout.font.capHeight / 2
            (layout.text as NSString).draw(at: CGPoint(x: layout.rect.midX - width / 2, y: baseline + layout.font.descender),
                                           withAttributes: attributes)
        }
    }

    // MARK: Output

    /// Downscales to every iconset size and packs `<url>` with iconutil.
    static func writeIcns(_ image: CGImage, to url: URL) throws {
        let fm = FileManager.default
        let iconset = fm.temporaryDirectory.appendingPathComponent("TwinIcon-\(UUID().uuidString).iconset", isDirectory: true)
        try fm.createDirectory(at: iconset, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: iconset) }

        for base in [16, 32, 128, 256, 512] {
            for factor in [1, 2] {
                let name = factor == 1 ? "icon_\(base)x\(base).png" : "icon_\(base)x\(base)@2x.png"
                guard let scaled = draw(size: base * factor, { ctx, rect in ctx.draw(image, in: rect) }) else {
                    throw CloneError("Could not scale the icon.")
                }
                try WindowCapture.writePNG(scaled, to: iconset.appendingPathComponent(name))
            }
        }
        try CloneEngine.run("/usr/bin/iconutil", ["-c", "icns", iconset.path, "-o", url.path])
    }

    /// Draws into a fresh sRGB bitmap with AppKit available as the current context.
    static func draw(size: Int, _ body: (CGContext, CGRect) -> Void) -> CGImage? {
        guard let ctx = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: 0,
                                  space: sRGB, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { return nil }
        ctx.interpolationQuality = .high
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(cgContext: ctx, flipped: false)
        body(ctx, CGRect(x: 0, y: 0, width: size, height: size))
        NSGraphicsContext.restoreGraphicsState()
        return ctx.makeImage()
    }
}
