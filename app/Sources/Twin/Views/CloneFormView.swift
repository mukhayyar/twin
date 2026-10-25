import SwiftUI

/// Source → Name → Icon, chunked into three cards, with one primary action.
struct CloneFormView: View {
    @EnvironmentObject private var store: AppStore
    let app: AppInfo

    @FocusState private var labelFocused: Bool

    private var plan: ClonePlan {
        ClonePlan(source: app, rawLabel: store.label, icon: store.iconOptions, ownHardware: store.ownHardware)
    }
    private var labelIsValid: Bool { CloneLabel.isValid(store.label) }
    private var duplicate: Bool { store.isDuplicate(plan) }
    private var twins: [AppInfo] { store.twins(of: app) }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                StepIndicator(current: 1)
                HStack(alignment: .top, spacing: 16) {
                    VStack(spacing: 16) {
                        sourceCard
                        nameCard
                        details
                    }
                    .frame(minWidth: 280)
                    iconCard
                        .frame(width: 290)
                }
                primaryButton
            }
            .padding(.horizontal, 28)
            .padding(.top, 14)
            .padding(.bottom, 28)
            .frame(maxWidth: 880)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .onChange(of: store.labelFocusRequest) { _, _ in labelFocused = true }
        .onAppear {
            if store.labelFocusRequest > 0 { DispatchQueue.main.async { labelFocused = true } }
        }
        .confirmationDialog("Replace \(plan.cloneName)?", isPresented: $store.confirmReplace, titleVisibility: .visible) {
            Button("Replace", role: .destructive) { store.confirmReplaceAndCreate() }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The \(plan.cloneName) app is replaced with a fresh copy of \(app.name). Its profile data in \(AppPaths.display(plan.profileURL)) is kept, so you stay signed in.")
        }
    }

    // MARK: Source

    private var sourceCard: some View {
        FormCard(title: "Source") {
            HStack(spacing: 12) {
                AppIcon(url: app.url, size: 46)
                VStack(alignment: .leading, spacing: 2) {
                    Text(app.name)
                        .font(.system(size: 16, weight: .semibold))
                        .lineLimit(1)
                    Text([app.version.map { "Version \($0)" }, app.url.deletingLastPathComponent().lastPathComponent]
                        .compactMap { $0 }.joined(separator: " · "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .help(AppPaths.display(app.url))
                }
                Spacer(minLength: 8)
                Button("Change…") { store.chooseApp() }
                    .buttonStyle(SecondaryButtonStyle(compact: true))
                    .help("Choose a different app (⌘O)")
            }
            if !twins.isEmpty {
                Divider()
                existingTwins
            }
        }
    }

    /// Existing twins of the selected app, as a list: one row per twin, so long names
    /// and many twins never overflow the card. Past four rows it scrolls.
    private var existingTwins: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(twins.count == 1 ? "1 twin already" : "\(twins.count) twins already")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            ScrollView {
                VStack(spacing: 2) {
                    ForEach(twins) { twin in
                        TwinRow(twin: twin) { store.selection = .clone(twin.id) } open: { store.open(twin.url) }
                    }
                }
            }
            .frame(maxHeight: twins.count > 4 ? 4 * 40 : nil)
            .fixedSize(horizontal: false, vertical: twins.count <= 4)
        }
    }

    // MARK: Name

    private var nameCard: some View {
        FormCard(title: "Profile name") {
            TextField("Work", text: $store.label)
                .textFieldStyle(.plain)
                .font(.system(size: 19, weight: .medium, design: .rounded))
                .focused($labelFocused)
                .onSubmit { store.requestCreate() }
                .padding(.horizontal, 12)
                .frame(height: 40)
                .background(RoundedRectangle(cornerRadius: 9, style: .continuous).fill(Color(nsColor: .textBackgroundColor)))
                .overlay(
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .strokeBorder(labelFocused ? Theme.violet : Color(nsColor: .separatorColor), lineWidth: labelFocused ? 2 : 1)
                )
                .animation(.snappy(duration: 0.15), value: labelFocused)
                .accessibilityLabel("Profile name")

            FlowLayout(spacing: 8, lineSpacing: 8) {
                ForEach(CloneLabel.presets) { preset in
                    Chip(title: preset.name, symbol: preset.symbol,
                         color: Color(hex: IconPainter.autoColor(for: preset.name)),
                         isSelected: plan.label.caseInsensitiveCompare(preset.name) == .orderedSame) {
                        store.label = preset.name
                    }
                }
            }

            feedback
                .font(.caption)
                .animation(.snappy(duration: 0.2), value: duplicate)

            Divider()

            VStack(alignment: .leading, spacing: 3) {
                Toggle("Own hardware ID", isOn: $store.ownHardware)
                    .toggleStyle(.checkbox)
                    .font(.callout)
                Text(store.ownHardware
                     ? "Apps see this twin as a different Mac. Turn off for apps licensed per device."
                     : "Apps see this Mac's real hardware ID, so a device-bound license keeps working.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.leading, 20)
            }
            .help("Gives the twin its own MAC addresses, hardware UUID and serial number")
        }
    }

    @ViewBuilder
    private var feedback: some View {
        if !labelIsValid {
            Label("Use letters, numbers or dashes.", systemImage: "exclamationmark.circle.fill")
                .foregroundStyle(.red)
        } else if duplicate {
            HStack(spacing: 8) {
                Label("\(plan.cloneName) already exists.", systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                Spacer(minLength: 4)
                Button("Replace…") { store.requestCreate() }
                    .buttonStyle(SecondaryButtonStyle(compact: true))
            }
        } else {
            HStack(spacing: 4) {
                Image(systemName: "arrow.turn.down.right").foregroundStyle(.tertiary)
                Text("Will be named \(Text(plan.cloneName).fontWeight(.semibold).foregroundColor(.primary))")
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
        }
    }

    private var details: some View {
        DisclosureGroup("Details") {
            Card {
                VStack(spacing: 0) {
                    DetailRow(symbol: "app.badge.fill", title: "App", value: AppPaths.display(plan.destination))
                    Divider().padding(.leading, 46)
                    DetailRow(symbol: "number", title: "Bundle ID", value: plan.bundleID)
                    Divider().padding(.leading, 46)
                    DetailRow(symbol: "folder.fill", title: "Profile", value: AppPaths.display(plan.profileURL))
                }
            }
            .padding(.top, 6)
        }
        .font(.callout)
        .foregroundStyle(.secondary)
        .padding(.horizontal, 4)
    }

    // MARK: Icon

    private var preview: NSImage {
        if let image = store.iconPreview, store.iconPreviewSourceID == app.id { return image }
        return IconCache.icon(for: app.url)
    }

    private var iconCard: some View {
        FormCard(title: "Icon") {
            HStack(spacing: 10) {
                VStack(spacing: 6) {
                    AppIcon(url: app.url, size: 60)
                    Text("Original").font(.caption).foregroundStyle(.secondary)
                }
                Image(systemName: "arrow.right")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(.tertiary)
                    .accessibilityHidden(true)
                VStack(spacing: 6) {
                    IconImage(image: preview, size: 84)
                        .shadow(color: .black.opacity(0.12), radius: 6, y: 3)
                    Text(plan.cloneName)
                        .font(.caption.weight(.semibold))
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                .frame(maxWidth: 140)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Preview of the \(plan.cloneName) icon")
            }
            .frame(maxWidth: .infinity)

            Divider()

            VStack(alignment: .leading, spacing: 8) {
                Text("Color").font(.caption).foregroundStyle(.secondary)
                FlowLayout(spacing: 4, lineSpacing: 4) {
                    autoSwatch
                    ForEach(IconPainter.presetSwatches + IconPainter.palette, id: \.self) { hex in
                        Swatch(color: Color(hex: hex), name: String(format: "#%06X", hex),
                               isSelected: store.iconOptions.color == .rgb(hex)) {
                            store.iconOptions.color = .rgb(hex)
                        }
                    }
                    customSwatch
                }
            }

            Divider()

            HStack(spacing: 18) {
                Toggle("Badge", isOn: $store.iconOptions.badge)
                Toggle("Tint", isOn: $store.iconOptions.tint)
                Spacer(minLength: 0)
            }
            .toggleStyle(.checkbox)
            .font(.callout)
        }
    }

    private var autoSwatch: some View {
        let selected = store.iconOptions.color == .auto
        return Button { store.iconOptions.color = .auto } label: {
            HStack(spacing: 5) {
                Circle()
                    .fill(Color(hex: IconPainter.autoColor(for: plan.label)))
                    .frame(width: 14, height: 14)
                Text("Auto").font(.system(size: 11, weight: .semibold))
            }
            .padding(.horizontal, 8)
            .frame(height: 26)
            .background(Capsule().fill(Color.primary.opacity(0.06)))
            .overlay(Capsule().strokeBorder(selected ? Color.primary.opacity(0.8) : .clear, lineWidth: 2))
            .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .help("Pick a color from the label automatically")
        .accessibilityLabel("Automatic color")
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    /// A rainbow swatch that opens the system color panel, like Apple's own pickers.
    private var customSwatch: some View {
        let swatches = IconPainter.presetSwatches + IconPainter.palette
        let isCustom: Bool = {
            if case .rgb(let hex) = store.iconOptions.color { return !swatches.contains(hex) }
            return false
        }()
        return Button {
            ColorPanelBridge.shared.show(initial: IconPainter.nsColor(plan.color)) { color in
                store.iconOptions.color = .rgb(Color(nsColor: color).hexValue)
            }
        } label: {
            Circle()
                .fill(AngularGradient(colors: [.red, .yellow, .green, .cyan, .blue, .purple, .red], center: .center))
                .overlay(Circle().fill(isCustom ? Color(hex: plan.color) : .clear).padding(5))
                .frame(width: 20, height: 20)
                .padding(3)
                .overlay(Circle().strokeBorder(isCustom ? Color.primary.opacity(0.8) : .clear, lineWidth: 2))
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .help("Custom color…")
        .accessibilityLabel("Custom color")
        .accessibilityAddTraits(isCustom ? .isSelected : [])
    }

    // MARK: Action

    private var primaryButton: some View {
        Button { store.requestCreate() } label: {
            ZStack {
                Label(duplicate ? "Replace \(plan.cloneName)…" : "Create \(plan.cloneName)",
                      systemImage: duplicate ? "arrow.triangle.2.circlepath" : "plus.square.on.square")
                    .lineLimit(1)
                HStack {
                    Spacer()
                    Text("⌘↩").font(.system(size: 12, weight: .medium, design: .rounded)).opacity(0.75)
                }
                .padding(.trailing, 16)
            }
        }
        .buttonStyle(PrimaryButtonStyle(isPressed: store.pressingCreate))
        .disabled(!store.canCreate)
        .accessibilityHint("Creates a copy of \(app.name) with its own profile")
    }
}

/// One existing twin: icon, name and label (click to show it) and an Open button.
/// Stateless on purpose: `@State` needs a macro plugin the Command Line Tools lack.
private struct TwinRow: View {
    let twin: AppInfo
    let select: () -> Void
    let open: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            HStack(spacing: 10) {
                AppIcon(url: twin.url, size: 28)
                Text(twin.name)
                    .font(.system(size: 13, weight: .medium))
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .layoutPriority(1)
                if let label = twin.cloneLabel {
                    // The name already carries the label; the badge is a color cue, so keep it short.
                    ProfileBadge(label: label.count > 14 ? label.prefix(13) + "…" : label, fontSize: 10)
                        .fixedSize()
                }
                Spacer(minLength: 8)
            }
            .contentShape(Rectangle())
            .onTapGesture(perform: select)
            .help("Show \(twin.name)")
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isButton)
            .accessibilityAction { select() }

            Button("Open", action: open)
                .buttonStyle(SecondaryButtonStyle(compact: true))
                .help("Open \(twin.name)")
        }
        .padding(.horizontal, 8)
        .frame(height: 38)
        .background(RoundedRectangle(cornerRadius: 8, style: .continuous).fill(Color.primary.opacity(0.035)))
    }
}
