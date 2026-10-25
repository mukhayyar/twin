package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"time"
)

// icon.js paints with AppKit and Core Image, so macOS icons get the system
// font and color management; other platforms use paint.go.
//
//go:embed icon.js
var iconScript []byte

func paintIcon(p Plan) (string, error) {
	if !p.Tint && !p.Badge {
		return "", nil
	}
	dir, err := os.MkdirTemp("", "twin-icon-")
	if err != nil {
		return err.Error(), nil
	}
	defer os.RemoveAll(dir)

	script := filepath.Join(dir, "icon.js")
	iconset := filepath.Join(dir, "TwinIcon.iconset")
	if err := os.WriteFile(script, iconScript, 0o644); err != nil {
		return err.Error(), nil
	}
	if err := os.Mkdir(iconset, 0o755); err != nil {
		return err.Error(), nil
	}
	// Resolve symlinks, or the icon comes back with an alias arrow.
	src, err := filepath.EvalSymlinks(p.Src.Path)
	if err != nil {
		return err.Error(), nil
	}
	if err := run("osascript", "-l", "JavaScript", script, src, p.Label, p.Color, flag01(p.Tint), flag01(p.Badge), iconset); err != nil {
		return err.Error(), nil
	}

	res := filepath.Join(p.Dest, "Contents", "Resources")
	info := filepath.Join(p.Dest, "Contents", "Info.plist")
	if err := os.MkdirAll(res, 0o755); err != nil {
		return err.Error(), nil
	}
	if err := run("iconutil", "-c", "icns", iconset, "-o", filepath.Join(res, "TwinIcon.icns")); err != nil {
		return err.Error(), nil
	}
	if err := run("plutil", "-replace", "CFBundleIconFile", "-string", "TwinIcon", info); err != nil {
		return err.Error(), nil
	}
	// Without this, macOS keeps showing the asset-catalog icon.
	_ = run("plutil", "-remove", "CFBundleIconName", info)
	now := time.Now()
	_ = os.Chtimes(p.Dest, now, now)
	return "", nil
}

func flag01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
