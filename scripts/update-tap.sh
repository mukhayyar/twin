#!/usr/bin/env bash
# Points the Homebrew tap (github.com/mukhayyar/homebrew-tap) at a release:
# writes Formula/twin.rb (the CLI) and Casks/twin.rb (Twin.app) from the
# release's checksums.txt, then commits and pushes.
#   scripts/update-tap.sh v0.3.0
set -euo pipefail

TAG="${1:?usage: update-tap.sh vX.Y.Z}"
V="${TAG#v}"
REPO=mukhayyar/twin
TAP=mukhayyar/homebrew-tap
URL="https://github.com/$REPO/releases/download/$TAG"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
gh release download "$TAG" --repo "$REPO" --pattern checksums.txt --dir "$work"
sum() { awk -v f="$1" '$2 == f { print $1 }' "$work/checksums.txt"; }

gh repo clone "$TAP" "$work/tap" -- --quiet
mkdir -p "$work/tap/Formula" "$work/tap/Casks"

cat > "$work/tap/Formula/twin.rb" <<EOF
class Twin < Formula
  desc "Clone any app into a twin with its own isolated profile"
  homepage "https://github.com/$REPO"
  license "MIT"

  # One universal binary serves both Mac architectures.
  on_macos do
    on_arm do
      url "$URL/twin_${V}_darwin_universal.tar.gz"
      sha256 "$(sum "twin_${V}_darwin_universal.tar.gz")"
    end
    on_intel do
      url "$URL/twin_${V}_darwin_universal.tar.gz"
      sha256 "$(sum "twin_${V}_darwin_universal.tar.gz")"
    end
  end

  on_linux do
    on_intel do
      url "$URL/twin_${V}_linux_amd64.tar.gz"
      sha256 "$(sum "twin_${V}_linux_amd64.tar.gz")"
    end
    on_arm do
      url "$URL/twin_${V}_linux_arm64.tar.gz"
      sha256 "$(sum "twin_${V}_linux_arm64.tar.gz")"
    end
  end

  def install
    bin.install "twin"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/twin --version")
  end
end
EOF

cat > "$work/tap/Casks/twin.rb" <<EOF
cask "twin" do
  version "$V"
  sha256 "$(sum "Twin-${V}.zip")"

  url "https://github.com/$REPO/releases/download/v#{version}/Twin-#{version}.zip"
  name "Twin"
  desc "Clone any app into a twin with its own isolated profile"
  homepage "https://github.com/$REPO"

  depends_on macos: :sonoma

  app "Twin.app"

  # Twin is ad-hoc signed rather than notarized; without this, Gatekeeper
  # refuses to open it on first launch.
  postflight_steps do
    run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{appdir}}/Twin.app"]
  end

  zap trash: [
    "~/Library/Preferences/com.mukhayyar.twin.plist",
    "~/Library/Saved Application State/com.mukhayyar.twin.savedState",
  ]
end
EOF

cat > "$work/tap/README.md" <<EOF
# mukhayyar/tap

Homebrew tap for [Twin](https://github.com/$REPO): one app, many lives.

\`\`\`bash
brew install mukhayyar/tap/twin          # the CLI (macOS and Linux)
brew install --cask mukhayyar/tap/twin   # Twin.app
\`\`\`
EOF

cd "$work/tap"
git add -A
if git diff --cached --quiet; then
  echo "tap already at $TAG"
else
  git commit -q -m "twin $V"
  git push -q
  echo "tap updated to $TAG"
fi
