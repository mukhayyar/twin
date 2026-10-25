# app-clone

Clone any macOS app into a separate copy with its own isolated profile, so you can run e.g. `Slack-Work` and `Slack-Personal` side by side with different accounts, settings and data.

## How it works

1. Copies `/Applications/<App>.app` to `/Applications/<App>-<Label>.app`
2. Gives the clone a unique bundle ID (`<original.id>.<Label>`) so macOS treats it as a separate app
3. Swaps the main executable for a small wrapper that points `HOME` and `CFFIXED_USER_HOME` at `~/.app_profiles/<App>-<Label>`, so the clone's `~/Library`, dotfiles and caches live there instead of your real home
4. Strips quarantine, ad-hoc re-signs the bundle and registers it with Launch Services

## Install

```bash
git clone git@github.com:mukhayyar/mac-app-clone.git
cd mac-app-clone
ln -sf "$PWD/app-clone" ~/.local/bin/app-clone   # or /usr/local/bin
```

Or without cloning:

```bash
curl -fsSL https://raw.githubusercontent.com/mukhayyar/mac-app-clone/main/app-clone -o ~/.local/bin/app-clone
chmod +x ~/.local/bin/app-clone
```

## Usage

```bash
app-clone
```

Pick an app from the numbered list, then enter a label (`Work`, `Personal`, `Hobby`, ...).

## Remove a clone

```bash
rm -rf "/Applications/<App>-<Label>.app" ~/.app_profiles/<App>-<Label>
```

## Caveats

- Ad-hoc re-signing drops the original developer signature and entitlements. Apps that depend on iCloud, Keychain sharing groups or other entitlements may misbehave in the clone.
- App updates replace only the original app. Re-run `app-clone` with the same label to refresh a clone; its profile data in `~/.app_profiles` is kept.
- Mac App Store apps often refuse to launch after re-signing (receipt and entitlement checks).
