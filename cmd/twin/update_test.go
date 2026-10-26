package main

import "testing"

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		candidate, current string
		want               bool
	}{
		{"v0.5.0", "v0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.0", "v0.10.0", false},
		{"v0.4.1", "0.4.0", true},
		{"v0.4.0", "dev", true},
		{"v0.4.0", "v0.4.0-rc1", true},
	} {
		if got := newerVersion(c.candidate, c.current); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.candidate, c.current, got)
		}
	}
	if checksumFor("abc  twin_1_linux_amd64.tar.gz\ndef  checksums.txt\n", "twin_1_linux_amd64.tar.gz") != "abc" {
		t.Error("checksum lookup")
	}
}
