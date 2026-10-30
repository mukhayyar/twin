package main

import (
	"strings"
	"testing"
)

const tomlSample = `enc_id = '00abc=='
password = "00p\"w=="
key_pair = [
    [1, 2, 3],
    [4, 5],
]
salt = 'XyZ12' # kept
key_confirmed = true

[options]
password = 'not top level'
`

func TestTOMLEdit(t *testing.T) {
	d := parseTOML(tomlSample)
	for key, want := range map[string]string{"enc_id": "00abc==", "password": `00p"w==`, "salt": "XyZ12"} {
		if got, ok := d.get("", key); !ok || got != want {
			t.Errorf("get(%s) = %q, %v; want %q", key, got, ok, want)
		}
	}
	if _, ok := d.get("", "key_pair"); ok {
		t.Error("key_pair isn't a string")
	}
	if _, ok := d.get("", "missing"); ok {
		t.Error("missing key found")
	}
	if got, _ := d.get("options", "password"); got != "not top level" {
		t.Errorf("options.password = %q", got)
	}
	raw, _ := d.raw("", "key_pair")
	if parts := splitArray(raw); len(parts) != 2 {
		t.Errorf("splitArray(%q) = %q", raw, parts)
	} else if b, ok := parseByteArray(parts[1]); !ok || string(b) != "\x04\x05" {
		t.Errorf("parseByteArray(%q) = %v, %v", parts[1], b, ok)
	}

	d.set("", "password", "00new==")
	if !d.del("", "key_pair") || !d.del("", "key_confirmed") || d.del("", "nope") {
		t.Error("del reported wrongly")
	}
	d.set("", "unlock_pin", "it's")
	want := `enc_id = '00abc=='
password = '00new=='
salt = 'XyZ12' # kept
unlock_pin = "it's"

[options]
password = 'not top level'
`
	if got := d.String(); got != want {
		t.Errorf("edited:\n%s\nwant:\n%s", got, want)
	}
	if got, _ := parseTOML(d.String()).get("", "unlock_pin"); got != "it's" {
		t.Errorf("round trip = %q", got)
	}
}

func TestTOMLNestedArrayIsNotATable(t *testing.T) {
	// A line of a multi-line array that starts with "[" isn't a table header.
	d := parseTOML("key_pair = [\n[1],\n[2],\n]\nid = '7'\n")
	if got, ok := d.get("", "id"); !ok || got != "7" {
		t.Errorf("get(id) after a nested array = %q, %v", got, ok)
	}
	d.del("", "key_pair")
	if strings.Contains(d.String(), "[") {
		t.Errorf("array not fully removed:\n%s", d)
	}
}

func TestTOMLTables(t *testing.T) {
	d := parseTOML(`id = '1'

[keys_confirmed]
rs-ny = true

[[trusted_devices_v2]]
hwid = 'x'

[trusted_devices_v2.extra]
a = 1

[options]
2fa = '{}'
`)
	if !d.delTable("keys_confirmed") || !d.delTable("trusted_devices_v2") || d.delTable("nope") {
		t.Error("delTable reported wrongly")
	}
	d.set("options", "2fa", `{"a":1}`)
	d.set("options", "key with space", "v")
	d.set("socks", "password", "p")
	want := `id = '1'

[options]
2fa = '{"a":1}'
'key with space' = 'v'

[socks]
password = 'p'
`
	if got := d.String(); got != want {
		t.Errorf("tables:\n%s\nwant:\n%s", got, want)
	}
	empty := parseTOML("")
	empty.set("", "a", "b")
	if empty.String() != "a = 'b'\n" {
		t.Errorf("set on empty = %q", empty.String())
	}
}

func TestTwinMachineID(t *testing.T) {
	// Golden values from the C loops in isolate/isolate_linux.c.
	for _, c := range []struct{ name, real, want string }{
		{"RustDesk-Alice", "0123456789abcdef0123456789abcdef", "d5e191300de90c288b521ca7f19328f4"},
		{"RustDesk-Test", "4f1e2d3c4b5a69788796a5b4c3d2e1f0\n", "956036e9d9795e8582b86a1d6851b7a1"},
	} {
		if got := twinMachineID(c.real, c.name); got != c.want {
			t.Errorf("twinMachineID(%s) = %s, want %s", c.name, got, c.want)
		}
	}
	if twinRoot(1000, "RustDesk-Alice") != "/tmp/twin.1000."+twinHash("RustDesk-Alice") {
		t.Error("twinRoot doesn't match the launcher's TWIN_ROOT")
	}
}
