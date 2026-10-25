package main

import _ "embed"

// isolateSO is libtwinisolate for Linux (arm64); see isolate/isolate_linux.c.
//
//go:embed isolate/libtwinisolate-linux-arm64.so
var isolateSO []byte
