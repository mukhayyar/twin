#!/usr/bin/env bash
# Creates stand-in app bundles for recording demos without exposing the
# recorder's real /Applications.  Usage: make-fixtures.sh <dir>
set -euo pipefail
dir="$1"
rm -rf "$dir" && mkdir -p "$dir"

mk() { # name executable bundle-id version
  local c="$dir/$1.app/Contents"
  mkdir -p "$c/MacOS"
  printf '#!/bin/sh\n' > "$c/MacOS/$2" && chmod +x "$c/MacOS/$2"
  plutil -create xml1 "$c/Info.plist"
  plutil -insert CFBundleExecutable -string "$2" "$c/Info.plist"
  plutil -insert CFBundleIdentifier -string "$3" "$c/Info.plist"
  plutil -insert CFBundleShortVersionString -string "$4" "$c/Info.plist"
}

mk "Arc" "Arc" "company.thebrowser.Browser" "1.89.1"
mk "Discord" "Discord" "com.hnc.Discord" "0.0.352"
mk "Figma" "Figma" "com.figma.Desktop" "125.9.6"
mk "Firefox" "firefox" "org.mozilla.firefox" "144.0"
mk "Google Chrome" "Google Chrome" "com.google.Chrome" "141.0.7390.108"
mk "Linear" "Linear" "com.linear" "1.31.2"
mk "Notion" "Notion" "notion.id" "5.2.0"
mk "Obsidian" "Obsidian" "md.obsidian" "1.9.14"
mk "Raycast" "Raycast" "com.raycast.macos" "1.103.6"
mk "Slack" "Slack" "com.tinyspeck.slackmacgap" "4.46.101"
mk "Spotify" "Spotify" "com.spotify.client" "1.2.74"
mk "Telegram" "Telegram" "ru.keepcoder.Telegram" "12.1"
mk "Visual Studio Code" "Electron" "com.microsoft.VSCode" "1.105.1"
mk "WhatsApp" "WhatsApp" "net.whatsapp.WhatsApp" "25.29.77"
mk "Zoom" "zoom.us" "us.zoom.xos" "6.6.2"
