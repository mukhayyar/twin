## ✦ Twin {{VERSION}}: one app, many lives

Clone an app into a twin with its own isolated profile (Work, Personal, Hobby), a recolored icon and a label badge.

### Download

| | File | |
|---|---|---|
| 🖥️ **Mac app** | `Twin-{{VERSION}}.zip` | Unzip and drag **Twin.app** to Applications |
| ⌨️ **CLI · macOS** | `twin_{{VERSION}}_darwin_universal.tar.gz` | Apple silicon + Intel |
| 🐧 **CLI · Linux** | `twin_{{VERSION}}_linux_amd64.tar.gz` · `…_linux_arm64.tar.gz` | `.desktop` twins, fully isolated |
| 🪟 **CLI · Windows** | `twin_{{VERSION}}_windows_amd64.zip` · `…_windows_arm64.zip` | Experimental |

Or with Homebrew:

```bash
brew install mukhayyar/tap/twin          # CLI, macOS and Linux
brew install --cask mukhayyar/tap/twin   # Twin.app
```

> [!NOTE]
> Twin is ad-hoc signed, not notarized. On first launch, right-click **Twin.app → Open**, or run
> `xattr -dr com.apple.quarantine /Applications/Twin.app`.

Verify downloads with `checksums.txt` (`shasum -a 256 -c checksums.txt`).
