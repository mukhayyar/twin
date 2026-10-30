package main

import (
	"fmt"
	"strings"
)

// On Linux, libtwinisolate shows each twin its own machine-id, derived from
// the real one and the twin's name (prepareHardware in
// isolate/isolate_linux.c). These functions compute the same value, for apps
// whose settings are encrypted with the machine-id. Keep the two in step.

// twinRoot is a twin's TWIN_ROOT, where libtwinisolate keeps its /tmp and the
// twin's hardware files. It matches the launcher's
// /tmp/twin.$(id -u).<hash>.
func twinRoot(uid int, name string) string {
	return fmt.Sprintf("/tmp/twin.%d.%s", uid, twinHash(name))
}

// twinSalt is libtwinisolate's per-twin salt: FNV-1a over the name,
// stretched to 32 bytes.
func twinSalt(name string) [32]byte {
	var salt [32]byte
	h := uint64(1469598103934665603)
	for i := range salt {
		for _, c := range []byte(name) {
			h = (h ^ uint64(c)) * 1099511628211
		}
		h = (h ^ uint64(i)) * 1099511628211
		salt[i] = byte(h >> 32)
	}
	return salt
}

// twinMachineID is the machine-id a twin named name sees: each of the first
// 32 hex digits of the real one mixed with the salt. Like the C code, it
// assumes lowercase hex, which is what systemd writes.
func twinMachineID(real, name string) string {
	real = strings.TrimSpace(real)
	salt := twinSalt(name)
	out := []byte(real)
	for i := 0; i < len(out) && i < 32; i++ {
		c := int(out[i])
		v := c - '0'
		if c >= 'a' {
			v = c - 'a' + 10
		}
		out[i] = "0123456789abcdef"[(v^int(salt[i%32]&0xF))&0xF]
	}
	return string(out)
}
