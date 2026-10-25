import SwiftUI

struct CloneJobView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @ObservedObject var job: CloneJob

    private var plan: ClonePlan { job.plan }
    private var succeeded: Bool { job.phase == .succeeded }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                StepIndicator(current: succeeded ? 3 : 2)
                header
                phaseContent
                    .id(job.phase)
                    .transition(.asymmetric(
                        insertion: .opacity.animation(.easeOut(duration: 0.25).delay(0.14)),
                        removal: .opacity.animation(.easeIn(duration: 0.12))
                    ))
            }
            .padding(.horizontal, 28)
            .padding(.top, 14)
            .padding(.bottom, 28)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .animation(.snappy(duration: 0.35), value: job.phase)
    }

    /// Swapped as a whole so the old content fades out before the new fades in.
    @ViewBuilder
    private var phaseContent: some View {
        VStack(spacing: 20) {
                switch job.phase {
                case .running:
                    ProgressView(value: job.progress)
                        .progressViewStyle(.linear)
                        .tint(Theme.violet)
                        .frame(maxWidth: 440)
                        .animation(.snappy, value: job.progress)
                        .accessibilityLabel("Cloning progress")
                    StepsCard(steps: job.steps).frame(maxWidth: 520)
                case .succeeded:
                    successActions.frame(maxWidth: 440)
                case .failed:
                    StepsCard(steps: job.steps).frame(maxWidth: 520)
                    failureActions.frame(maxWidth: 440)
                }
        }
    }

    // MARK: Header

    private var iconURL: URL {
        FileManager.default.fileExists(atPath: plan.destination.path) ? plan.destination : plan.source.url
    }

    private var header: some View {
        VStack(spacing: 12) {
            ZStack {
                Circle()
                    .fill(RadialGradient(colors: [glow.opacity(0.32), .clear], center: .center, startRadius: 8, endRadius: 110))
                    .frame(width: 220, height: 220)
                    .scaleEffect(succeeded ? 1.1 : 0.8)
                Group {
                    if let icon = job.icon { IconImage(image: icon, size: 112) } else { AppIcon(url: iconURL, size: 112) }
                }
                .shadow(color: .black.opacity(0.18), radius: 12, y: 6)
                .scaleEffect(succeeded ? 1 : 0.9)
                .animation(reduceMotion ? nil : .spring(response: 0.45, dampingFraction: 0.55), value: succeeded)
                .overlay(alignment: .topTrailing) { badge.offset(x: 10, y: -6) }
                .onDrag { NSItemProvider(object: plan.destination as NSURL) }
                .help(succeeded ? "Drag to your Dock or a Finder window" : "")
            }
            .frame(height: 150)

            VStack(spacing: 12) {
                Text(title)
                    .font(.system(size: 26, weight: .bold, design: .rounded))
                    .multilineTextAlignment(.center)
                Text(subtitle)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 440)
            }
            .id(job.phase)
            .transition(.asymmetric(
                insertion: .opacity.animation(.easeOut(duration: 0.25).delay(0.14)),
                removal: .opacity.animation(.easeIn(duration: 0.12))
            ))
        }
    }

    @ViewBuilder
    private var badge: some View {
        switch job.phase {
        case .running:
            EmptyView()
        case .succeeded:
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 32, weight: .semibold))
                .symbolRenderingMode(.palette)
                .foregroundStyle(.white, .green)
                .background(Circle().fill(.white).padding(4))
                .symbolEffect(.bounce, options: .nonRepeating, value: reduceMotion ? false : succeeded)
                .transition(.scale(scale: 0.3).combined(with: .opacity))
                .accessibilityHidden(true)
        case .failed:
            Image(systemName: "xmark.circle.fill")
                .font(.system(size: 32, weight: .semibold))
                .symbolRenderingMode(.palette)
                .foregroundStyle(.white, .red)
                .background(Circle().fill(.white).padding(4))
                .transition(.scale(scale: 0.3).combined(with: .opacity))
                .accessibilityHidden(true)
        }
    }

    private var glow: Color {
        switch job.phase {
        case .running: return Theme.violet
        case .succeeded: return Color(hex: plan.color)
        case .failed: return .red
        }
    }

    private var title: String {
        switch job.phase {
        case .running: return "Creating \(plan.cloneName)…"
        case .succeeded: return "\(plan.cloneName) is ready"
        case .failed: return "Couldn’t create \(plan.cloneName)"
        }
    }

    private var subtitle: String {
        switch job.phase {
        case .running:
            return "Copying \(plan.source.name) and giving it a profile of its own."
        case .succeeded:
            return job.warnings.isEmpty
                ? "Sign in with a different account. Its logins, settings and data stay in its own profile."
                : "Created with warnings: \(job.warnings.joined(separator: " "))"
        case .failed:
            return "Nothing was left behind in \(AppPaths.display(AppPaths.applications))."
        }
    }

    // MARK: Actions

    private var successActions: some View {
        VStack(spacing: 12) {
            HStack(spacing: 6) {
                ProfileBadge(label: plan.label, color: Color(hex: plan.color), fontSize: 11)
                Text("Profile in \(AppPaths.display(plan.profileURL))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Button { store.open(plan.destination) } label: {
                Label("Open \(plan.cloneName)", systemImage: "arrow.up.forward.app.fill")
            }
            .buttonStyle(PrimaryButtonStyle())
            .keyboardShortcut(.defaultAction)
            HStack(spacing: 8) {
                Button { store.reveal(plan.destination) } label: {
                    Label("Show in Finder", systemImage: "folder").frame(maxWidth: .infinity)
                }
                Button { store.dismissJob() } label: {
                    Label("Clone Another", systemImage: "plus").frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(SecondaryButtonStyle())
            Label("Tip: drag the icon to your Dock.", systemImage: "hand.draw")
                .font(.caption)
                .foregroundStyle(.tertiary)
            StepsSummary(steps: job.steps)
        }
    }

    private var failureActions: some View {
        VStack(spacing: 10) {
            Button { store.retry() } label: {
                Label("Try Again", systemImage: "arrow.clockwise")
            }
            .buttonStyle(PrimaryButtonStyle())
            Button { store.dismissJob() } label: {
                Label("Back", systemImage: "chevron.left").frame(maxWidth: .infinity)
            }
            .buttonStyle(SecondaryButtonStyle())
        }
    }
}

/// "All 6 steps done in 3.2 s", expandable to the checklist.
struct StepsSummary: View {
    let steps: [CloneStep]

    var body: some View {
        DisclosureGroup {
            StepsCard(steps: steps).padding(.top, 6)
        } label: {
            Text(String(format: "All %d steps done in %.1f s", steps.count, steps.compactMap(\.duration).reduce(0, +)))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

struct StepsCard: View {
    let steps: [CloneStep]

    var body: some View {
        Card {
            VStack(spacing: 0) {
                ForEach(steps) { step in
                    StepRow(step: step)
                    if step.id != steps.last?.id {
                        Divider().padding(.leading, 46)
                    }
                }
            }
        }
    }
}

struct StepRow: View {
    let step: CloneStep

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            indicator
                .frame(width: 20, height: 20)
                .alignmentGuide(.firstTextBaseline) { $0[VerticalAlignment.center] + 4 }
            VStack(alignment: .leading, spacing: 2) {
                Text(step.title)
                    .fontWeight(step.status == .running ? .semibold : .regular)
                    .foregroundStyle(step.status == .pending ? .secondary : .primary)
                if let message {
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(messageColor)
                        .textSelection(.enabled)
                        .lineLimit(3)
                }
            }
            Spacer(minLength: 0)
            if let duration = step.duration {
                Text(String(format: "%.1fs", duration))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
        .background(step.status == .running ? Theme.violet.opacity(0.07) : .clear)
        .animation(.snappy(duration: 0.2), value: step.status)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var indicator: some View {
        switch step.status {
        case .pending:
            Text("\(step.id + 1)")
                .font(.system(size: 10, weight: .bold, design: .rounded))
                .foregroundStyle(.secondary)
                .frame(width: 18, height: 18)
                .overlay(Circle().strokeBorder(.tertiary, lineWidth: 1.5))
        case .running:
            ProgressView().controlSize(.small)
        case .done:
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 17))
                .symbolRenderingMode(.palette)
                .foregroundStyle(.white, .green)
                .transition(.scale.combined(with: .opacity))
        case .warning:
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.system(size: 15))
                .foregroundStyle(.yellow)
        case .failed:
            Image(systemName: "xmark.circle.fill")
                .font(.system(size: 17))
                .symbolRenderingMode(.palette)
                .foregroundStyle(.white, .red)
        }
    }

    private var message: String? {
        switch step.status {
        case .warning(let text), .failed(let text): return text
        default: return nil
        }
    }

    private var messageColor: Color {
        if case .failed = step.status { return .red }
        return .orange
    }
}
