package main

import "golang.org/x/sys/unix"

// cloneFile makes an APFS copy-on-write clone: instant, and it takes no
// extra space until one side changes. It fails on other file systems.
func cloneFile(src, dst string) error { return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW) }
