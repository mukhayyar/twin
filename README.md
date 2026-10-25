<div align="center">

<img src="assets/icon.png" width="136" alt="Twin app icon">

# Twin

### One app. Many lives.

Clone any Mac app into a **twin** with its own isolated profile, a recolored icon and a label badge.<br>
Slack for work, Slack for you, side by side, never sharing a login.

[![Latest release](https://img.shields.io/github/v/release/mukhayyar/twin?style=flat-square&color=7D56F4&label=release)](https://github.com/mukhayyar/twin/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/mukhayyar/twin/total?style=flat-square&color=FF5FAF)](https://github.com/mukhayyar/twin/releases)
[![macOS 14+](https://img.shields.io/badge/macOS-14%2B-111?style=flat-square&logo=apple&logoColor=white)](#install)
[![CI](https://img.shields.io/github/actions/workflow/status/mukhayyar/twin/ci.yml?style=flat-square&label=CI)](https://github.com/mukhayyar/twin/actions/workflows/ci.yml)
[![MIT license](https://img.shields.io/github/license/mukhayyar/twin?style=flat-square&color=04B575)](LICENSE)

**[Download Twin.app](https://github.com/mukhayyar/twin/releases/latest)** &nbsp;·&nbsp; **[Install the CLI](#install)** &nbsp;·&nbsp; **[▶ Watch the film](assets/twin-promo.mp4)**

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/app-dark.png">
  <img src="assets/app-light.png" width="900" alt="The Twin app: pick an app, name the profile, preview the recolored icon, create the clone">
</picture>

</div>

<br>

## ✦ Why Twin

Most Mac apps only let you be one person at a time. Signing in to a second Slack workspace, a second Discord account or a separate browser profile means logging out, juggling windows or giving up.

Twin makes a full copy of the app that **believes it's on its own machine**. Each twin keeps its own logins, settings, caches and window state, gets its own Dock icon, and runs alongside the original, sharing nothing with it:

| Kept separate per twin | |
|---|---|
| 📁 **Files & settings** | Home folder, `~/Library`, preferences |
| 🔐 **Secrets** | Keychain items (tokens, passwords, encryption keys) |
| 🔌 **Running state** | `/tmp` sockets and lock files, temp and cache folders |
| 🖥️ **Hardware fingerprint** | MAC addresses, hardware UUID and serial number |

> [!IMPORTANT]
> **Twins also change the hardware fingerprint.** Each twin sees its own stable MAC addresses, hardware UUID and serial number, so apps that identify a machine by its hardware treat every twin as a separate device. That's what gives a RustDesk twin its own ID. It also means apps licensed per device may count a twin as another device.

<div align="center">
<a href="assets/twin-promo.mp4"><img src="assets/twin-preview.gif" width="860" alt="Twin in action: drop Discord in, clone it as Work, make a Personal twin from the terminal, and run both side by side"></a>
<br><sub>▶ Click for the full 53-second film</sub>
<br><br>
<img src="assets/twins.png" width="860" alt="Discord cloned into Work, Personal and Hobby twins, each recolored with a matching badge">
</div>

<br>

## Two ways to use it

<table>
<tr>
<th width="50%">🖥️ &nbsp;The app</th>
<th width="50%">⌨️ &nbsp;The CLI</th>
</tr>
<tr>
<td><img src="assets/app-drop.png" alt="Dropping an app onto the Twin window"></td>
<td><img src="assets/demo-cli.gif" alt="The twin CLI: filter to Slack, type Work, clone"></td>
</tr>
<tr>
<td>A native SwiftUI app. Drop an app on the window or the Dock icon, pick a label, press <kbd>⌘</kbd> <kbd>↩</kbd>. Manage, open and trash your twins from the sidebar.</td>
<td>A <a href="https://github.com/charmbracelet/bubbletea">Bubble Tea</a> TUI. Filter, <kbd>tab</kbd> through quick picks, <kbd>enter</kbd>. Or skip the UI entirely: <code>twin Slack Work</code>.</td>
</tr>
</table>

## Install

### The app

1. Download **`Twin-x.y.z.zip`** from the [latest release](https://github.com/mukhayyar/twin/releases/latest) and drag **Twin.app** into Applications.
2. Twin is open source and ad-hoc signed rather than notarized, so the first launch needs a right-click → **Open**, or:

   ```bash
   xattr -dr com.apple.quarantine /Applications/Twin.app
   ```

### The CLI

```bash
curl -fsSL https://raw.githubusercontent.com/mukhayyar/twin/main/install.sh | bash
```

<details>
<summary>Other ways</summary>

<br>

**With Go**

```bash
go install github.com/mukhayyar/twin/cmd/twin@latest
```

**By hand.** Download the archive for your system from the [releases](https://github.com/mukhayyar/twin/releases/latest), extract it and put `twin` on your `PATH`. Verify it with `checksums.txt`.

</details>

### Linux and Windows

The CLI runs there too. The installer above works on Linux (x86-64 and ARM64); on Windows, download `twin_x.y.z_windows_*.zip`.

- **Linux:** a twin is a new app-menu entry whose files and settings live in their own profile (HOME and XDG folders). Flatpak apps work; Snap apps can't be twinned. The desktop keyring and hardware fingerprint are shared.
- **Windows (experimental):** a twin is a Start Menu shortcut that runs the app with its own user folders. Apps that only allow one running copy won't open twice.

## Usage

### In the app

| | |
|---|---|
| **Choose an app** | Drop it on the window or the Dock icon, pick it from the sidebar, or press <kbd>⌘</kbd> <kbd>O</kbd> |
| **Name the twin** | Type a label or click a quick pick: Work, Personal, Hobby, School |
| **Style the icon** | Auto color (or pick your own), badge on/off, tint on/off, previewed live |
| **Create** | <kbd>⌘</kbd> <kbd>↩</kbd>. Twin shows each step, then offers to open it |
| **Manage** | Right-click a twin in the sidebar to Open, Show in Finder, Copy Path or Move to Trash (<kbd>⌘</kbd> <kbd>Z</kbd> undoes it) |

| Shortcut | |
|---|---|
| <kbd>⌘</kbd> <kbd>O</kbd> | Choose App… |
| <kbd>⌘</kbd> <kbd>↩</kbd> | Create the twin |
| <kbd>⌘</kbd> <kbd>L</kbd> | Open the selected twin |
| <kbd>⌘</kbd> <kbd>R</kbd> | Reveal in Finder |
| <kbd>⌘</kbd> <kbd>⌫</kbd> | Move twin to Trash (<kbd>⌘</kbd> <kbd>Z</kbd> to undo) |
| <kbd>⌘</kbd> <kbd>F</kbd> | Search apps |
| <kbd>⇧</kbd> <kbd>⌘</kbd> <kbd>R</kbd> | Rescan Applications |

### In the terminal

```text
twin                        pick an app interactively
twin Slack Work             clone Slack as Slack-Work
twin ~/Downloads/Foo.app    clone an app from anywhere
twin Slack Hobby -c pink    clone with a pink icon and badge
twin -n                     try the whole flow without touching the disk
```

> [!TIP]
> Drag an app from Finder into the terminal while `twin` is open, and it's selected.

| Flag | |
|---|---|
| `-n`, `--dry-run` | Walk through everything without changing the disk |
| `-c`, `--color` | `auto` (default), `none`, a name (`blue` `green` `orange` `purple` `pink` `teal` `indigo` `yellow` `red` `mint` `brown` `cyan`) or `#RRGGBB` |
| `--no-badge` | Keep the label off the icon |
| `-v`, `--version` | Print the version |

| Key | |
|---|---|
| <kbd>↑</kbd> <kbd>↓</kbd> / <kbd>j</kbd> <kbd>k</kbd> | Move |
| <kbd>/</kbd> | Filter apps |
| <kbd>enter</kbd> | Choose / create |
| <kbd>tab</kbd> | Cycle quick-pick labels |
| <kbd>esc</kbd> | Back |
| <kbd>o</kbd> | Open the new twin |
| <kbd>q</kbd> | Quit |

## Icons & badges

Every twin is repainted so you can tell them apart at a glance in the Dock, Finder and ⌘-Tab:

- **Recolor:** the icon's dominant hue is rotated to the label's color, keeping its shading. Near-monochrome icons keep their colors.
- **Badge:** the label is stamped on a pill at the bottom of the icon.

| Label | Color |
|---|---|
| Work | ![Work](https://img.shields.io/badge/Work-0A84FF?style=for-the-badge) |
| Personal | ![Personal](https://img.shields.io/badge/Personal-30D158?style=for-the-badge) |
| Hobby | ![Hobby](https://img.shields.io/badge/Hobby-FF9F0A?style=for-the-badge) |
| School | ![School](https://img.shields.io/badge/School-BF5AF2?style=for-the-badge) |
| Anything else | Picked from ![](https://img.shields.io/badge/-FF375F?style=flat-square) ![](https://img.shields.io/badge/-40C8E0?style=flat-square) ![](https://img.shields.io/badge/-5E5CE6?style=flat-square) ![](https://img.shields.io/badge/-FFD60A?style=flat-square) ![](https://img.shields.io/badge/-FF453A?style=flat-square) ![](https://img.shields.io/badge/-63E6BE?style=flat-square) ![](https://img.shields.io/badge/-AC8E68?style=flat-square) ![](https://img.shields.io/badge/-64D2FF?style=flat-square) by the label's hash, so it's always the same |

## FAQ

<details>
<summary><b>Which apps work?</b></summary>

<br>

Most apps that keep their data under `~/Library`: Electron apps (Slack, Discord, VS Code, Notion…), Chromium browsers, and most native apps. Known exceptions:

- **Mac App Store apps** usually refuse to launch after re-signing (receipt checks).
- Apps that rely on **iCloud or Keychain sharing entitlements** lose them, because ad-hoc signing can't keep a developer's entitlements.
- Apps that enforce a **single instance** through a server-side session may still log one twin out.
- Apps that install **system-wide services** (privileged helpers, launch daemons) share those services between twins.

</details>

<details>
<summary><b>What happens when the original app updates?</b></summary>

<br>

Twins are independent copies, so they keep their version. Re-create the twin with the same label to refresh it. The profile in `~/.app_profiles` is kept, so you stay signed in.

</details>

<details>
<summary><b>How do I remove a twin?</b></summary>

<br>

In the app, right-click it in the sidebar → **Move to Trash**. From the terminal:

```bash
rm -rf "/Applications/Slack-Work.app" ~/.app_profiles/Slack-Work
```

</details>

<details>
<summary><b>Is it safe?</b></summary>

<br>

Twin never touches the original app or its data. Everything it creates is a separate `.app` in `/Applications` and a folder in `~/.app_profiles`. Use `twin --dry-run` to see exactly what would happen first.

</details>

## Build from source

```bash
git clone https://github.com/mukhayyar/twin && cd twin

go run ./cmd/twin --dry-run           # the CLI (Go 1.26+)
scripts/build-app.sh && open dist/Twin.app   # the app (Xcode or Command Line Tools)
```


## Roadmap

- [x] macOS app and CLI
- [x] Auto icon recolor and label badges
- [x] Full isolation on macOS, including keychain and hardware fingerprint
- [x] Linux CLI: `.desktop` twins with XDG profiles
- [x] Windows CLI (experimental): shortcut twins with their own user folders
- [ ] Homebrew tap
- [ ] Full isolation on Linux (keyring, `/tmp`)

## Credits

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss) and [VHS](https://github.com/charmbracelet/vhs) by Charm, plus SwiftUI. Started life as a 100-line shell script.

<div align="center">
<br>
<sub>MIT © Tsaqif Mukhayyar</sub>
</div>
