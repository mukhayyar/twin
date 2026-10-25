#!/usr/bin/env bash
# Installs the latest twin CLI release.
#   curl -fsSL https://raw.githubusercontent.com/mukhayyar/twin/main/install.sh | bash
# Set BIN_DIR to choose where it goes (default: ~/.local/bin).
set -euo pipefail

REPO=mukhayyar/twin
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

url=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
  grep -o '"browser_download_url": *"[^"]*darwin_universal\.tar\.gz"' | cut -d'"' -f4)
if [ -z "$url" ]; then
  echo "twin: couldn't find a release to download" >&2
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
