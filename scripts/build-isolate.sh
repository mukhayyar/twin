#!/usr/bin/env bash
# Rebuilds cmd/twin/isolate/libtwinisolate.dylib (universal) from isolate.m.
# The built library is committed so `go install` works without a compiler;
# release builds call this first so what ships always matches the source.
set -euo pipefail
cd "$(dirname "$0")/../cmd/twin/isolate"
clang -arch arm64 -arch x86_64 -mmacosx-version-min=11.0 -dynamiclib -O2 -Wall -Werror -fobjc-arc \
  -framework Foundation -framework Security -framework IOKit -o libtwinisolate.dylib isolate.m
lipo -info libtwinisolate.dylib
