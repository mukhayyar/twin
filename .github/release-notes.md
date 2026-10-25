## ✦ Twin {{VERSION}}: one app, many lives

Clone any Mac app into a twin with its own isolated profile (Work, Personal, Hobby), a recolored icon, and a label badge.

### Download

| | File | |
|---|---|---|
| 🖥️ **App** | `Twin-{{VERSION}}.zip` | Unzip and drag **Twin.app** to Applications |
| ⌨️ **CLI** | `twin_{{VERSION}}_darwin_universal.tar.gz` | Universal binary (Apple silicon + Intel) |

Or install the CLI in one line:

```bash
curl -fsSL https://raw.githubusercontent.com/mukhayyar/twin/main/install.sh | bash
```

> [!NOTE]
> Twin is ad-hoc signed, not notarized. On first launch, right-click **Twin.app → Open**, or run
> `xattr -dr com.apple.quarantine /Applications/Twin.app`.

Verify downloads with `checksums.txt` (`shasum -a 256 -c checksums.txt`).
