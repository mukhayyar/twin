package main

import _ "embed"

// isolateSO is libtwinisolate for Linux (amd64); see isolate/isolate_linux.c.
//
//go:embed isolate/libtwinisolate-linux-amd64.so
var isolateSO []byte
