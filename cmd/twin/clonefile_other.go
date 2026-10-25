//go:build !darwin

package main

import "errors"

// cloneFile is macOS only; elsewhere io.Copy already uses copy_file_range,
// which reflinks on file systems that support it.
func cloneFile(src, dst string) error { return errors.ErrUnsupported }
