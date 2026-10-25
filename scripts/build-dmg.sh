#!/usr/bin/env bash
# Packs dist/Twin.app into dist/Twin-<v>.dmg: a drag-to-install disk image with
# Twin.app next to an Applications shortcut, volume name "Twin <v>".
# Only uses stock macOS tools (hdiutil, ditto), so it runs headless in CI.
# Usage: scripts/build-app.sh && scripts/build-dmg.sh
#   VERSION defaults to the app's CFBundleShortVersionString.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/Twin.app"
[ -d "$APP" ] || { echo "build-dmg: $APP not found; run scripts/build-app.sh first" >&2; exit 1; }
VERSION="${VERSION:-$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")}"
VERSION="${VERSION#v}"
DMG="dist/Twin-$VERSION.dmg"

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
chmod 755 "$stage" # becomes the volume root; mktemp makes it owner-only
ditto "$APP" "$stage/Twin.app"
ln -s /Applications "$stage/Applications"

rm -f "$DMG"
# hdiutil sometimes fails with "Resource busy" on CI runners; a retry clears it.
for attempt in 1 2 3; do
  if hdiutil create -volname "Twin $VERSION" -srcfolder "$stage" -fs HFS+ \
    -format UDZO -imagekey zlib-level=9 -ov "$DMG"; then
    break
  fi
  [ "$attempt" = 3 ] && exit 1
  echo "build-dmg: hdiutil create failed, retrying" >&2
  sleep 5
done
hdiutil verify "$DMG"
echo "==> $DMG"
