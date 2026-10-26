## ✦ Twin {{VERSION}}: one app, many lives

Clone an app into a twin with its own isolated profile (Work, Personal, Hobby), a recolored icon and a label badge.

### What's new

{{CHANGES}}

### Download

| | File | |
|---|---|---|
| 🖥️ **Mac app** | `Twin-{{VERSION}}.dmg` | Open it and drag **Twin** to **Applications** |
| 🐧 **Linux** | `twin-{{VERSION}}-x86_64.AppImage` · `…-aarch64.AppImage` | `chmod +x` it, then run it in a terminal |
| 🪟 **Windows** | `twin-{{VERSION}}-windows-amd64.exe` · `…-windows-arm64.exe` | Run it in a terminal. Experimental |

Or with Homebrew:

```bash
brew install mukhayyar/tap/twin          # CLI, macOS and Linux
brew install --cask mukhayyar/tap/twin   # Twin.app
```

**Other downloads:** `Twin-{{VERSION}}.zip` (Mac app) · CLI tarballs `twin_{{VERSION}}_darwin_universal.tar.gz` (Apple silicon + Intel), `twin_{{VERSION}}_linux_amd64.tar.gz`, `twin_{{VERSION}}_linux_arm64.tar.gz` · `twin_{{VERSION}}_windows_amd64.zip`, `twin_{{VERSION}}_windows_arm64.zip`

> [!NOTE]
> Twin is ad-hoc signed, not notarized. On first launch, right-click **Twin.app → Open**, or run
> `xattr -dr com.apple.quarantine /Applications/Twin.app`.

Verify downloads with `checksums.txt` (`shasum -a 256 -c checksums.txt`).
