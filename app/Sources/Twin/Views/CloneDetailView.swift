import SwiftUI

struct CloneDetailView: View {
    @EnvironmentObject private var store: AppStore
    let clone: AppInfo

    private var profileExists: Bool { FileManager.default.fileExists(atPath: clone.profileURL.path) }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                VStack(spacing: 12) {
                    AppIcon(url: clone.url, size: 112)
                        .shadow(color: .black.opacity(0.18), radius: 12, y: 6)
                        .onDrag { NSItemProvider(object: clone.url as NSURL) }
                        .help("Drag to your Dock or a Finder window")
                    HStack(spacing: 8) {
                        Text(clone.twinOf ?? clone.name)
                            .font(.system(size: 26, weight: .bold, design: .rounded))
                            .lineLimit(1)
                        if let label = clone.cloneLabel {
                            ProfileBadge(label: label, fontSize: 13)
                        }
                    }
                    Text("A twin with its own logins, settings and data.")
                        .foregroundStyle(.secondary)
                }
                .padding(.top, 24)

                Card {
                    VStack(spacing: 0) {
                        DetailRow(symbol: "app.badge.fill", title: "App", value: AppPaths.display(clone.url))
                        Divider().padding(.leading, 46)
                        DetailRow(symbol: "number", title: "Bundle ID", value: clone.bundleID)
                        Divider().padding(.leading, 46)
                        DetailRow(symbol: "folder.fill", title: "Profile",
                                  value: profileExists ? AppPaths.display(clone.profileURL) : "Created on first launch")
                    }
                }
                .frame(maxWidth: 520)

                VStack(spacing: 10) {
                    Button { store.open(clone.url) } label: {
                        Label("Open \(clone.name)", systemImage: "arrow.up.forward.app.fill")
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .keyboardShortcut(.defaultAction)
                    HStack(spacing: 8) {
                        Button { store.reveal(clone.url) } label: {
                            Label("Show in Finder", systemImage: "folder").frame(maxWidth: .infinity)
                        }
                        Button { NSWorkspace.shared.open(clone.profileURL) } label: {
                            Label("Profile Folder", systemImage: "person.crop.square").frame(maxWidth: .infinity)
                        }
                        .disabled(!profileExists)
                        Button { store.moveToTrash(clone) } label: {
                            Label("Move to Trash", systemImage: "trash").frame(maxWidth: .infinity)
                        }
                        .help("Undo with ⌘Z")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    Label("Tip: drag the icon to your Dock.", systemImage: "hand.draw")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
                .frame(maxWidth: 520)
            }
            .padding(.horizontal, 28)
            .padding(.bottom, 28)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
    }
}
