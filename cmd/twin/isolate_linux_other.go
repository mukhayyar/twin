//go:build linux && !amd64 && !arm64

package main

// isolateSO is empty where no prebuilt library exists; twins then get
// HOME/XDG isolation only.
var isolateSO []byte
