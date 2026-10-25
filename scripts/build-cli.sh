#!/usr/bin/env bash
# Builds a universal (arm64 + x86_64) twin binary and its release tarball:
#   dist/twin
#   dist/twin_<version>_darwin_universal.tar.gz
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-dev}"
mkdir -p dist

for arch in arm64 amd64; do
  GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "dist/twin-$arch" ./cmd/twin
done
lipo -create -output dist/twin dist/twin-arm64 dist/twin-amd64
rm dist/twin-arm64 dist/twin-amd64
codesign --force --sign - dist/twin

tar -czf "dist/twin_${VERSION#v}_darwin_universal.tar.gz" -C dist twin -C .. README.md LICENSE
lipo -info dist/twin
