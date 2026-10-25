#!/usr/bin/env bash
# Builds Linux AppImages of the twin CLI into dist/:
#   twin-<v>-x86_64.AppImage
#   twin-<v>-aarch64.AppImage
# Needs a Linux host (x86_64 or aarch64) with Go, curl and file. One host builds
# both: each AppImage gets the runtime for its own architecture. On a Mac:
#   docker run --rm -v "$PWD":/src -w /src golang:1.26.4 sh -c \
#     'apt-get update -qq && apt-get install -qq -y file >/dev/null && VERSION=v1.2.3 scripts/build-appimage.sh'
# Usage: VERSION=v1.2.3 scripts/build-appimage.sh [x86_64] [aarch64]
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-dev}"
V="${VERSION#v}"
LDFLAGS="-s -w -X main.version=$VERSION" # same as build-cli.sh
archs=("$@")
[ ${#archs[@]} -gt 0 ] || archs=(x86_64 aarch64)

die() { echo "build-appimage: $*" >&2; exit 1; }
[ "$(uname -s)" = Linux ] || die "run this on Linux (see the docker line at the top)"
host=$(uname -m)
case $host in x86_64 | aarch64) ;; *) die "no appimagetool for $host" ;; esac
for cmd in go curl file sha256sum; do # appimagetool refuses to run without file
  command -v "$cmd" >/dev/null || die "$cmd is missing, please install it"
done

# Pinned: appimagetool (github.com/AppImage/appimagetool) and the runtime each
# AppImage starts with (github.com/AppImage/type2-runtime).
TOOL_URL=https://github.com/AppImage/appimagetool/releases/download/1.9.1
RUNTIME_URL=https://github.com/AppImage/type2-runtime/releases/download/20251108
pinned() {
  case $1 in
    appimagetool-x86_64.AppImage) echo ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0 ;;
    appimagetool-aarch64.AppImage) echo f0837e7448a0c1e4e650a93bb3e85802546e60654ef287576f46c71c126a9158 ;;
    runtime-x86_64) echo 2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d ;;
    runtime-aarch64) echo 00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444 ;;
  esac
}
fetch() { # base-url name -> $work/name, checksum verified
  curl -fsSL --retry 3 -o "$work/$2" "$1/$2"
  echo "$(pinned "$2")  $work/$2" | sha256sum -c --quiet - || die "checksum mismatch for $2"
}
# ELF e_machine, the 2 bytes at offset 18: 3e00 is x86_64, b700 is aarch64.
elf_machine() { od -An -tx1 -j18 -N2 "$1" | tr -d ' \n'; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p dist

# Containers have no FUSE, so run appimagetool extracted rather than mounted.
fetch "$TOOL_URL" "appimagetool-$host.AppImage"
chmod +x "$work/appimagetool-$host.AppImage"
(cd "$work" && "./appimagetool-$host.AppImage" --appimage-extract >/dev/null)
appimagetool="$work/squashfs-root/AppRun"

for arch in "${archs[@]}"; do
  case $arch in
    x86_64) goarch=amd64 machine=3e00 ;;
    aarch64) goarch=arm64 machine=b700 ;;
    *) die "unknown architecture $arch (want x86_64 or aarch64)" ;;
  esac
  echo "==> twin-$V-$arch.AppImage"
  appdir="$work/Twin-$arch.AppDir"
  mkdir -p "$appdir/usr/bin"
  GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$appdir/usr/bin/twin" ./cmd/twin
  ln -s usr/bin/twin "$appdir/AppRun"
  cp assets/icon.png "$appdir/twin.png"
  cat > "$appdir/twin.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Twin
GenericName=App Cloner
Comment=Clone any app into a twin with its own isolated profile
Exec=twin
Icon=twin
Terminal=true
Categories=Utility;
Keywords=clone;profile;twin;
EOF

  fetch "$RUNTIME_URL" "runtime-$arch"
  [ "$(elf_machine "$work/runtime-$arch")" = "$machine" ] || die "runtime-$arch is not an $arch binary"
  out="dist/twin-$V-$arch.AppImage"
  rm -f "$out"
  # ARCH is required: the AppDir has no .so files for appimagetool to guess from.
  ARCH=$arch VERSION=$V "$appimagetool" --no-appstream --runtime-file "$work/runtime-$arch" "$appdir" "$out"
  [ "$(elf_machine "$out")" = "$machine" ] || die "$out does not start with the $arch runtime"
done
ls -1 dist/twin-"$V"-*.AppImage
