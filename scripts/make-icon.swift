// Draws the Twin app icon and packs it into app/Resources/AppIcon.icns (+ assets/icon.png).
// Usage, from the repo root: swift scripts/make-icon.swift
import AppKit
import ImageIO
import UniformTypeIdentifiers

let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let resources = root.appendingPathComponent("app/Resources", isDirectory: true)
let assets = root.appendingPathComponent("assets", isDirectory: true)

func rgb(_ hex: UInt32, _ alpha: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255, alpha: alpha)
}

func gradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    CGGradient(colorsSpace: CGColorSpace(name: CGColorSpace.sRGB), colors: colors as CFArray, locations: locations)!
}

/// Superellipse (n = 5): close to Apple's continuous-corner icon shape.
func squircle(_ rect: CGRect, n: CGFloat = 5) -> CGPath {
    let path = CGMutablePath()
    let steps = 720
    for i in 0..<steps {
        let t = CGFloat(i) / CGFloat(steps) * 2 * .pi
        let c = cos(t), s = sin(t)
        let point = CGPoint(x: rect.midX + rect.width / 2 * copysign(pow(abs(c), 2 / n), c),
                            y: rect.midY + rect.height / 2 * copysign(pow(abs(s), 2 / n), s))
        if i == 0 { path.move(to: point) } else { path.addLine(to: point) }
    }
    path.closeSubpath()
    return path
}

func roundedRect(_ rect: CGRect, _ radius: CGFloat) -> CGPath {
    CGPath(roundedRect: rect, cornerWidth: radius, cornerHeight: radius, transform: nil)
}

/// One "app window" card: title-bar dots, a profile avatar and two text lines.
func drawCard(_ ctx: CGContext, _ rect: CGRect, ghost: Bool, detailed: Bool) {
    let shape = roundedRect(rect, 66)

    ctx.saveGState()
    if ghost {
        ctx.addPath(shape)
        ctx.setFillColor(rgb(0xFFFFFF, 0.26))
        ctx.fillPath()
        ctx.addPath(shape)
        ctx.setStrokeColor(rgb(0xFFFFFF, 0.55))
        ctx.setLineWidth(7)
        ctx.strokePath()
    } else {
        ctx.setShadow(offset: CGSize(width: 0, height: -22), blur: 44, color: rgb(0x2A0F6B, 0.45))
        ctx.addPath(shape)
        ctx.setFillColor(rgb(0xFFFFFF))
        ctx.fillPath()
        ctx.restoreGState()
        ctx.saveGState()
        ctx.addPath(shape)
        ctx.clip()
        ctx.drawLinearGradient(gradient([rgb(0xFFFFFF), rgb(0xF1EBFF)], [0, 1]),
                               start: CGPoint(x: rect.midX, y: rect.maxY), end: CGPoint(x: rect.midX, y: rect.minY), options: [])
    }
    ctx.restoreGState()

    let ink: (UInt32, CGFloat) -> CGColor = { hex, alpha in ghost ? rgb(0xFFFFFF, alpha * 0.8) : rgb(hex, alpha) }

    if detailed {
        let dots: [UInt32] = [0xFF5FAF, 0xB48CFF, 0x7D56F4]
        for (i, hex) in dots.enumerated() {
            let center = CGPoint(x: rect.minX + 58 + CGFloat(i) * 42, y: rect.maxY - 52)
            ctx.setFillColor(ink(hex, ghost ? 0.7 : 1))
            ctx.fillEllipse(in: CGRect(x: center.x - 14, y: center.y - 14, width: 28, height: 28))
        }
    }

    // Avatar.
    let avatarCenter = CGPoint(x: rect.minX + 132, y: rect.minY + (rect.height - 90) / 2)
    let radius: CGFloat = detailed ? 66 : 80
    let avatar = CGRect(x: avatarCenter.x - radius, y: avatarCenter.y - radius, width: radius * 2, height: radius * 2)
    ctx.saveGState()
    ctx.addEllipse(in: avatar)
    ctx.clip()
    if ghost {
        ctx.setFillColor(rgb(0xFFFFFF, 0.6))
        ctx.fill(avatar)
    } else {
        ctx.drawLinearGradient(gradient([rgb(0x7D56F4), rgb(0xFF5FAF)], [0, 1]),
                               start: CGPoint(x: avatar.minX, y: avatar.maxY), end: CGPoint(x: avatar.maxX, y: avatar.minY), options: [])
    }
    if detailed {
        let person = ghost ? rgb(0xFFFFFF, 0.0) : rgb(0xFFFFFF, 0.95)
        ctx.setFillColor(person)
        ctx.fillEllipse(in: CGRect(x: avatarCenter.x - 21, y: avatarCenter.y - 2, width: 42, height: 42))
        ctx.fillEllipse(in: CGRect(x: avatarCenter.x - 42, y: avatarCenter.y - radius - 18, width: 84, height: 74))
    }
    ctx.restoreGState()

    // Text lines.
    let lineX = avatar.maxX + 34
    let widths: [CGFloat] = detailed ? [rect.maxX - lineX - 48, (rect.maxX - lineX - 48) * 0.62] : [rect.maxX - lineX - 40]
    for (i, width) in widths.enumerated() {
        let height: CGFloat = detailed ? 28 : 40
        let y = avatarCenter.y + (detailed ? 14 - CGFloat(i) * 56 : -height / 2)
        ctx.addPath(roundedRect(CGRect(x: lineX, y: y, width: width, height: height), height / 2))
        ctx.setFillColor(ink(0x7D56F4, i == 0 ? 0.28 : 0.16))
        ctx.fillPath()
    }
}

func renderIcon(pixels: Int) -> CGImage {
    let ctx = CGContext(data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
                        space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    let scale = CGFloat(pixels) / 1024
    ctx.scaleBy(x: scale, y: scale)
    ctx.interpolationQuality = .high
    let detailed = pixels >= 64

    // Body with the macOS icon-grid drop shadow.
    let body = CGRect(x: 100, y: 100, width: 824, height: 824)
    let shape = squircle(body)
    ctx.saveGState()
    ctx.setShadow(offset: CGSize(width: 0, height: -10), blur: 24, color: rgb(0x000000, 0.28))
    ctx.addPath(shape)
    ctx.setFillColor(rgb(0x7D56F4))
    ctx.fillPath()
    ctx.restoreGState()

    ctx.saveGState()
    ctx.addPath(shape)
    ctx.clip()
    ctx.drawLinearGradient(gradient([rgb(0x6A47F0), rgb(0x9B4FEA), rgb(0xFF5FAF)], [0, 0.55, 1]),
                           start: CGPoint(x: body.minX, y: body.maxY), end: CGPoint(x: body.maxX, y: body.minY), options: [])
    ctx.drawRadialGradient(gradient([rgb(0xFFFFFF, 0.28), rgb(0xFFFFFF, 0)], [0, 1]),
                           startCenter: CGPoint(x: 290, y: 860), startRadius: 0,
                           endCenter: CGPoint(x: 290, y: 860), endRadius: 620, options: [])
    ctx.drawLinearGradient(gradient([rgb(0x1B0A4A, 0.22), rgb(0x1B0A4A, 0)], [0, 1]),
                           start: CGPoint(x: 512, y: body.minY), end: CGPoint(x: 512, y: body.minY + 320), options: [])
    ctx.restoreGState()

    // Hairline inner edge for definition on light backgrounds.
    ctx.addPath(shape)
    ctx.setStrokeColor(rgb(0xFFFFFF, 0.18))
    ctx.setLineWidth(3)
    ctx.strokePath()

    // Two cards: the echo (up-left) and the twin in front (down-right).
    let size = CGSize(width: 500, height: 390)
    drawCard(ctx, CGRect(origin: CGPoint(x: 207, y: 372), size: size), ghost: true, detailed: detailed)
    drawCard(ctx, CGRect(origin: CGPoint(x: 317, y: 262), size: size), ghost: false, detailed: detailed)

    return ctx.makeImage()!
}

func writePNG(_ image: CGImage, to url: URL) {
    let destination = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(destination, image, nil)
    precondition(CGImageDestinationFinalize(destination), "could not write \(url.path)")
}

let fm = FileManager.default
let iconset = fm.temporaryDirectory.appendingPathComponent("Twin-\(UUID().uuidString).iconset", isDirectory: true)
try fm.createDirectory(at: iconset, withIntermediateDirectories: true)
try fm.createDirectory(at: resources, withIntermediateDirectories: true)
try fm.createDirectory(at: assets, withIntermediateDirectories: true)

for base in [16, 32, 128, 256, 512] {
    for factor in [1, 2] {
        let name = factor == 1 ? "icon_\(base)x\(base).png" : "icon_\(base)x\(base)@2x.png"
        writePNG(renderIcon(pixels: base * factor), to: iconset.appendingPathComponent(name))
    }
}
writePNG(renderIcon(pixels: 1024), to: assets.appendingPathComponent("icon.png"))

let iconutil = Process()
iconutil.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
iconutil.arguments = ["-c", "icns", iconset.path, "-o", resources.appendingPathComponent("AppIcon.icns").path]
try iconutil.run()
iconutil.waitUntilExit()
try? fm.removeItem(at: iconset)
guard iconutil.terminationStatus == 0 else { fatalError("iconutil failed") }
print("wrote app/Resources/AppIcon.icns and assets/icon.png")
