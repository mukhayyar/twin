#!/usr/bin/env bash
# Builds the twin CLI release archives into dist/:
#   twin_<v>_darwin_universal.tar.gz    Apple silicon + Intel
#   twin_<v>_linux_{amd64,arm64}.tar.gz
#   twin_<v>_windows_{amd64,arm64}.zip  experimental
#   twin-<v>-windows-{amd64,arm64}.exe  the same twin.exe, to run without unzipping
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-dev}"
V="${VERSION#v}"
LDFLAGS="-s -w -X main.version=$VERSION"
mkdir -p dist

build() { # goos goarch output
  GOOS=$1 GOARCH=$2 CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$3" ./cmd/twin
}

# macOS: one universal binary.
scripts/build-isolate.sh
build darwin arm64 dist/twin-arm64
build darwin amd64 dist/twin-amd64
lipo -create -output dist/twin dist/twin-arm64 dist/twin-amd64
rm dist/twin-arm64 dist/twin-amd64
codesign --force --sign - dist/twin
tar -czf "dist/twin_${V}_darwin_universal.tar.gz" -C dist twin -C .. README.md LICENSE
lipo -info dist/twin

for arch in amd64 arm64; do
  stage="dist/linux-$arch" && rm -rf "$stage" && mkdir -p "$stage"
  build linux $arch "$stage/twin"
  tar -czf "dist/twin_${V}_linux_${arch}.tar.gz" -C "$stage" twin -C ../.. README.md LICENSE
  rm -rf "$stage"

  stage="dist/windows-$arch" && rm -rf "$stage" && mkdir -p "$stage"
  build windows $arch "$stage/twin.exe"
  cp README.md LICENSE "$stage/"
  (cd "$stage" && zip -q "../twin_${V}_windows_${arch}.zip" twin.exe README.md LICENSE)
  mv "$stage/twin.exe" "dist/twin-${V}-windows-${arch}.exe"
  rm -rf "$stage"
done
ls -1 dist/twin_"${V}"_* dist/twin-"${V}"-windows-*.exe
