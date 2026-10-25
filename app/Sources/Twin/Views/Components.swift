import AppKit
import SwiftUI

enum Theme {
    static let violet = Color(red: 125 / 255, green: 86 / 255, blue: 244 / 255)
    static let pink = Color(red: 255 / 255, green: 95 / 255, blue: 175 / 255)
    static let gradient = LinearGradient(colors: [violet, pink], startPoint: .topLeading, endPoint: .bottomTrailing)
}

extension Color {
    init(hex: UInt32) { self.init(nsColor: IconPainter.nsColor(hex)) }

    var hexValue: UInt32 {
        guard let c = NSColor(self).usingColorSpace(.sRGB) else { return 0 }
        func byte(_ v: CGFloat) -> UInt32 { UInt32((min(1, max(0, v)) * 255).rounded()) }
        return byte(c.redComponent) << 16 | byte(c.greenComponent) << 8 | byte(c.blueComponent)
    }
}

// MARK: Icons and badges

struct AppIcon: View {
    let url: URL
    var size: CGFloat

    var body: some View {
        Image(nsImage: IconCache.icon(for: url))
            .resizable()
            .interpolation(.high)
            .frame(width: size, height: size)
            .accessibilityHidden(true)
    }
}

struct IconImage: View {
    let image: NSImage
    var size: CGFloat

    var body: some View {
        Image(nsImage: image)
            .resizable()
            .interpolation(.high)
            .aspectRatio(contentMode: .fit)
            .frame(width: size, height: size)
            .accessibilityHidden(true)
    }
}

/// The label pill. Same proportions as the badge painted on twin icons (see IconPainter):
/// height 1.6× font, 0.44× font side padding, 0.144× font white stroke, soft drop shadow.
struct ProfileBadge: View {
    let label: String
    let color: Color
    var fontSize: CGFloat = 10

    init(label: String, color: Color, fontSize: CGFloat = 10) {
        self.label = label
        self.color = color
        self.fontSize = fontSize
    }

    /// Uses the label's automatic color.
    init(label: String, fontSize: CGFloat = 10) {
        self.init(label: label, color: Color(hex: IconPainter.autoColor(for: label)), fontSize: fontSize)
    }

    var body: some View {
        Text(label.replacingOccurrences(of: "-", with: " "))
            .font(.system(size: fontSize, weight: .heavy, design: .rounded))
            .foregroundStyle(.white)
            .lineLimit(1)
            .padding(.horizontal, fontSize * 0.44)
            .frame(height: fontSize * 1.6)
            .background(
                Capsule().fill(color)
                    .shadow(color: .black.opacity(0.35), radius: fontSize * 0.08, y: fontSize * 0.064)
            )
            .overlay(Capsule().strokeBorder(.white, lineWidth: max(1, fontSize * 0.144)))
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("\(label) profile")
    }
}

// MARK: Containers

struct Card<Content: View>: View {
    @Environment(\.colorSchemeContrast) private var contrast
    @ViewBuilder var content: Content

    var body: some View {
        content
            .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(Color(nsColor: .controlBackgroundColor)))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(Color(nsColor: .separatorColor).opacity(contrast == .increased ? 1 : 0.6),
                                  lineWidth: contrast == .increased ? 1.5 : 1)
            )
            .shadow(color: .black.opacity(0.04), radius: 6, y: 2)
    }
}

/// A titled group (Common Region): a small caption above a card.
struct FormCard<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title.uppercased())
                .font(.system(size: 11, weight: .semibold))
                .tracking(0.6)
                .foregroundStyle(.secondary)
                .padding(.leading, 4)
                .accessibilityAddTraits(.isHeader)
            Card {
                VStack(alignment: .leading, spacing: 12) { content }
                    .padding(16)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }
}

struct DetailRow: View {
    let symbol: String
    let title: String
    let value: String

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: symbol)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Theme.violet)
                .frame(width: 20)
                .accessibilityHidden(true)
            Text(title)
                .foregroundStyle(.secondary)
                .frame(width: 72, alignment: .leading)
            Text(value)
                .font(.system(.callout, design: .monospaced))
                .textSelection(.enabled)
                .lineLimit(1)
                .truncationMode(.middle)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
        .accessibilityElement(children: .combine)
    }
}

/// Wraps children onto new lines when they don't fit.
struct FlowLayout: Layout {
    var spacing: CGFloat = 8
    var lineSpacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxWidth = proposal.width ?? .infinity
        var x: CGFloat = 0, y: CGFloat = 0, rowHeight: CGFloat = 0, widest: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > 0, x + size.width > maxWidth {
                y += rowHeight + lineSpacing
                x = 0
                rowHeight = 0
            }
            x += size.width + spacing
            rowHeight = max(rowHeight, size.height)
            widest = max(widest, x - spacing)
        }
        return CGSize(width: min(widest, maxWidth), height: y + rowHeight)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, rowHeight: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > bounds.minX, x + size.width > bounds.maxX {
                y += rowHeight + lineSpacing
                x = bounds.minX
                rowHeight = 0
            }
            view.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
            x += size.width + spacing
            rowHeight = max(rowHeight, size.height)
        }
    }
}

// MARK: Controls

struct Chip: View {
    @Environment(\.colorSchemeContrast) private var contrast
    let title: String
    let symbol: String
    let color: Color
    let isSelected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Label(title, systemImage: symbol)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(isSelected ? color : .primary)
                .padding(.horizontal, 12)
                .frame(height: 30)
                .background(Capsule().fill(isSelected ? color.opacity(0.14) : Color.primary.opacity(0.06)))
                .overlay(Capsule().strokeBorder(isSelected ? color : Color.primary.opacity(contrast == .increased ? 0.4 : 0.1),
                                                lineWidth: isSelected ? 1.5 : 1))
                .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .animation(.snappy(duration: 0.2), value: isSelected)
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }
}

struct Swatch: View {
    let color: Color
    let name: String
    let isSelected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Circle()
                .fill(color)
                .overlay(Circle().strokeBorder(.black.opacity(0.12)))
                .frame(width: 20, height: 20)
                .padding(3)
                .overlay(Circle().strokeBorder(isSelected ? Color.primary.opacity(0.8) : .clear, lineWidth: 2))
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .help(name)
        .accessibilityLabel(name)
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }
}

/// Routes the shared NSColorPanel's continuous changes to a closure.
final class ColorPanelBridge: NSObject {
    static let shared = ColorPanelBridge()
    private var onChange: ((NSColor) -> Void)?

    func show(initial: NSColor, onChange: @escaping (NSColor) -> Void) {
        self.onChange = onChange
        let panel = NSColorPanel.shared
        panel.showsAlpha = false
        panel.isContinuous = true
        panel.color = initial
        panel.setTarget(self)
        panel.setAction(#selector(colorChanged(_:)))
        panel.orderFront(nil)
    }

    @objc private func colorChanged(_ sender: NSColorPanel) {
        onChange?(sender.color)
    }
}

/// The one accent-filled control per screen (Von Restorff).
struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled
    var isPressed = false

    func makeBody(configuration: Configuration) -> some View {
        let pressed = configuration.isPressed || isPressed
        configuration.label
            .font(.system(size: 14, weight: .semibold))
            .foregroundStyle(.white)
            .frame(maxWidth: .infinity, minHeight: 44)
            .background(RoundedRectangle(cornerRadius: 11, style: .continuous).fill(Theme.gradient))
            .overlay(RoundedRectangle(cornerRadius: 11, style: .continuous).strokeBorder(.white.opacity(0.18)))
            .shadow(color: Theme.violet.opacity(isEnabled ? 0.32 : 0), radius: pressed ? 3 : 9, y: pressed ? 1 : 4)
            .scaleEffect(pressed ? 0.975 : 1)
            .brightness(pressed ? -0.06 : 0)
            .opacity(isEnabled ? 1 : 0.45)
            .contentShape(Rectangle())
            .animation(.spring(duration: 0.2), value: pressed)
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    var compact = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: compact ? 12 : 13, weight: .medium))
            .padding(.horizontal, compact ? 10 : 14)
            .frame(minHeight: compact ? 26 : 34)
            .background(RoundedRectangle(cornerRadius: 9, style: .continuous).fill(Color.primary.opacity(configuration.isPressed ? 0.14 : 0.07)))
            .contentShape(Rectangle())
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
    }
}

// MARK: Progress and feedback

/// Choose → Name → Clone (Goal-Gradient). `current` 3 means everything is done.
struct StepIndicator: View {
    let current: Int
    private let titles = ["Choose", "Name", "Clone"]

    var body: some View {
        HStack(spacing: 8) {
            ForEach(titles.indices, id: \.self) { index in
                if index > 0 {
                    ZStack(alignment: .leading) {
                        Capsule().fill(Color.primary.opacity(0.1))
                        Capsule().fill(Theme.gradient).frame(width: index <= current ? 36 : 0)
                    }
                    .frame(width: 36, height: 3)
                }
                HStack(spacing: 6) {
                    ZStack {
                        Circle().fill(index < current ? AnyShapeStyle(Theme.gradient) : AnyShapeStyle(Color.primary.opacity(0.08)))
                        if index < current {
                            Image(systemName: "checkmark").font(.system(size: 9, weight: .heavy)).foregroundStyle(.white)
                        } else {
                            Text("\(index + 1)")
                                .font(.system(size: 10, weight: .bold, design: .rounded))
                                .foregroundStyle(index == current ? Theme.violet : .secondary)
                        }
                    }
                    .frame(width: 20, height: 20)
                    .overlay(Circle().strokeBorder(index == current ? Theme.violet : .clear, lineWidth: 1.5))
                    Text(titles[index])
                        .font(.system(size: 12, weight: index == current ? .semibold : .medium))
                        .foregroundStyle(index <= current ? .primary : .secondary)
                }
            }
        }
        .animation(.snappy, value: current)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(current >= titles.count ? "All steps complete" : "Step \(current + 1) of 3, \(titles[current])")
    }
}

struct ShakeEffect: GeometryEffect {
    var animatableData: CGFloat

    func effectValue(size: CGSize) -> ProjectionTransform {
        ProjectionTransform(CGAffineTransform(translationX: 7 * sin(animatableData * .pi * 6), y: 0))
    }
}

struct ToastView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let toast: AppStore.Toast

    private var isRejection: Bool { toast.kind == .rejection }

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: isRejection ? "exclamationmark.circle.fill" : "trash.fill")
                .foregroundStyle(isRejection ? Color.orange : Color.secondary)
                .accessibilityHidden(true)
            Text(toast.message).lineLimit(2)
            if case .trashed(let record) = toast.kind {
                Button("Undo") { store.undoTrash(record) }
                    .buttonStyle(.plain)
                    .fontWeight(.semibold)
                    .foregroundStyle(Theme.violet)
            }
            Button { store.dismissToast() } label: {
                Image(systemName: "xmark").font(.system(size: 10, weight: .bold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .accessibilityLabel("Dismiss")
        }
        .font(.callout)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Capsule().fill(.regularMaterial).shadow(color: .black.opacity(0.18), radius: 14, y: 6))
        .overlay(Capsule().strokeBorder(Color(nsColor: .separatorColor)))
        .modifier(ShakeEffect(animatableData: isRejection && !reduceMotion ? CGFloat(store.shakeTrigger) : 0))
        .animation(.linear(duration: 0.45), value: store.shakeTrigger)
    }
}

/// Full-window drop target feedback.
struct DropOverlay: View {
    let verdict: AppStore.DropVerdict?

    private var tint: Color {
        if case .reject = verdict { return .red }
        return Theme.violet
    }

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .fill(tint.opacity(0.07))
                .overlay(RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .strokeBorder(tint, style: StrokeStyle(lineWidth: 3, dash: [12, 9])))
                .padding(20)
            VStack(spacing: 14) {
                icon
                Text(title)
                    .font(.system(size: 26, weight: .bold, design: .rounded))
                    .multilineTextAlignment(.center)
                Text(subtitle)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 420)
            }
            .padding(40)
        }
        .background(Color(nsColor: .windowBackgroundColor).ignoresSafeArea())
        .allowsHitTesting(false)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var icon: some View {
        switch verdict {
        case .accept(let app):
            AppIcon(url: app.url, size: 104)
                .shadow(color: Theme.violet.opacity(0.35), radius: 18, y: 8)
        case .reject:
            Image(systemName: "nosign")
                .font(.system(size: 56, weight: .semibold))
                .foregroundStyle(.red)
                .frame(height: 104)
        case nil:
            Image(systemName: "arrow.down.app.fill")
                .font(.system(size: 64))
                .foregroundStyle(Theme.gradient)
                .frame(height: 104)
        }
    }

    private var title: String {
        switch verdict {
        case .accept(let app): return "Drop to clone \(app.name)"
        case .reject: return "Twin can’t clone this"
        case nil: return "Drop to clone this app"
        }
    }

    private var subtitle: String {
        switch verdict {
        case .accept: return "Twin makes a copy with a profile of its own. The original stays untouched."
        case .reject(let reason): return reason
        case nil: return "Any .app works, from any folder."
        }
    }
}

/// Soft brand-colored glow at the top of the detail pane.
struct HeaderGlow: View {
    var body: some View {
        ZStack(alignment: .top) {
            Color(nsColor: .windowBackgroundColor)
            RadialGradient(colors: [Theme.violet.opacity(0.13), .clear], center: .init(x: 0.15, y: 0), startRadius: 0, endRadius: 520)
            RadialGradient(colors: [Theme.pink.opacity(0.09), .clear], center: .init(x: 0.95, y: 0.02), startRadius: 0, endRadius: 420)
        }
        .ignoresSafeArea()
    }
}
