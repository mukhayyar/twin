#!/usr/bin/env bash
# Installs the latest twin CLI release on macOS or Linux.
#   curl -fsSL https://raw.githubusercontent.com/mukhayyar/twin/main/install.sh | bash
# Set BIN_DIR to choose where it goes (default: ~/.local/bin).
set -euo pipefail

REPO=mukhayyar/twin
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) asset="darwin_universal" ;;
  Linux)
    case "$(uname -m)" in
      x86_64 | amd64) asset="linux_amd64" ;;
      aarch64 | arm64) asset="linux_arm64" ;;
      *) echo "twin: no build for $(uname -m)" >&2; exit 1 ;;
    esac ;;
  *) echo "twin: on Windows, download twin_*_windows_*.zip from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac

url=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
  grep -o "\"browser_download_url\": *\"[^\"]*_${asset}\.tar\.gz\"" | cut -d'"' -f4)
if [ -z "$url" ]; then
  echo "twin: couldn't find a $asset release to download" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" | tar -xz -C "$tmp"
mkdir -p "$BIN_DIR"
install -m 755 "$tmp/twin" "$BIN_DIR/twin"

echo "✦ twin installed to $BIN_DIR/twin"
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "  run: twin" ;;
  *) echo "  add $BIN_DIR to your PATH, then run: twin" ;;
esac
