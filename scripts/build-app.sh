#!/usr/bin/env bash
# Builds dist/Twin.app (universal when possible), ad-hoc signs it and zips it.
# Usage: VERSION=1.2.3 scripts/build-app.sh
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${VERSION:-0.0.0-dev}"
VERSION="${VERSION#v}"
APP="dist/Twin.app"
ICON="app/Resources/AppIcon.icns"

[ -f "$ICON" ] || swift scripts/make-icon.swift
scripts/build-isolate.sh

# Copy each slice out right away: newer SwiftPM writes every triple to the same bin path.
SLICES="dist/.slices"
rm -rf "$SLICES" && mkdir -p "$SLICES"
bins=()
for arch in arm64 x86_64; do
  triple="$arch-apple-macosx14.0"
  echo "==> Building $triple"
  if swift build -c release --package-path app --triple "$triple"; then
    cp "$(swift build -c release --package-path app --triple "$triple" --show-bin-path)/Twin" "$SLICES/Twin-$arch"
    bins+=("$SLICES/Twin-$arch")
  elif [ "$arch" = x86_64 ]; then
    echo "warning: x86_64 build failed; Twin.app will be arm64-only" >&2
  else
    exit 1
  fi
done

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
if [ ${#bins[@]} -gt 1 ]; then
  lipo -create "${bins[@]}" -output "$APP/Contents/MacOS/Twin"
else
  cp "${bins[0]}" "$APP/Contents/MacOS/Twin"
fi
rm -rf "$SLICES"
cp "$ICON" "$APP/Contents/Resources/AppIcon.icns"
# Loaded into every twin to keep it isolated (see cmd/twin/isolate).
cp cmd/twin/isolate/libtwinisolate.dylib "$APP/Contents/Resources/"

# CFBundleDocumentTypes lets users drop an app on Twin's Dock icon or use Open With → Twin.
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key><string>en</string>
  <key>CFBundleExecutable</key><string>Twin</string>
  <key>CFBundleIdentifier</key><string>com.mukhayyar.twin</string>
  <key>CFBundleName</key><string>Twin</string>
  <key>CFBundleDisplayName</key><string>Twin</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.utilities</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSPrincipalClass</key><string>NSApplication</string>
  <key>NSHumanReadableCopyright</key><string>One app. Many lives.</string>
  <key>CFBundleDocumentTypes</key>
  <array>
    <dict>
      <key>CFBundleTypeName</key><string>Application</string>
      <key>CFBundleTypeRole</key><string>Viewer</string>
      <key>LSHandlerRank</key><string>None</string>
      <key>LSItemContentTypes</key>
      <array><string>com.apple.application-bundle</string></array>
    </dict>
  </array>
</dict>
</plist>
EOF

codesign --force --deep --sign - "$APP"
rm -f "dist/Twin-$VERSION.zip"
ditto -c -k --keepParent "$APP" "dist/Twin-$VERSION.zip"

echo "==> $(lipo -info "$APP/Contents/MacOS/Twin")"
echo "==> dist/Twin-$VERSION.zip"
