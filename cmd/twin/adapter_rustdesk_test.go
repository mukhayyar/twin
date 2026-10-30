package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A made-up machine and its twin, as libtwinisolate would show it.
const (
	testMachineID = "4f1e2d3c4b5a69788796a5b4c3d2e1f0"
	testTwinName  = "RustDesk-Alice"
)

var (
	realKey = rustdeskKey([]byte(testMachineID))
	twinKey = rustdeskKey([]byte(twinMachineID(testMachineID, testTwinName)))
)

// enc encrypts like RustDesk's encrypt_str_or_original: "00" + base64.
func enc(s string, key [32]byte, v1 bool) string {
	return "00" + base64.StdEncoding.EncodeToString(rustdeskSeal([]byte(s), key, v1))
}

func decWith(t *testing.T, s string, key [32]byte) (string, bool) {
	t.Helper()
	if len(s) <= 2 {
		return "", false
	}
	box, err := base64.StdEncoding.DecodeString(s[2:])
	if err != nil {
		t.Fatalf("not base64: %q", s)
	}
	plain, _, ok := rustdeskOpen(box, key)
	return string(plain), ok
}

func TestRustDeskSealOpen(t *testing.T) {
	for _, v1 := range []bool{true, false} {
		box := rustdeskSeal([]byte("secret"), realKey, v1)
		if (box[0] == formatV1 && len(box) == 1+24+16+6) != v1 {
			t.Errorf("v1=%v: wrong layout, %d bytes", v1, len(box))
		}
		plain, gotV1, ok := rustdeskOpen(box, realKey)
		if !ok || string(plain) != "secret" || gotV1 != v1 {
			t.Errorf("v1=%v: open = %q, %v, %v", v1, plain, gotV1, ok)
		}
		if _, _, ok := rustdeskOpen(box, twinKey); ok {
			t.Errorf("v1=%v: opened with the wrong key", v1)
		}
	}
	// RustDesk pads a short key with zeros.
	if k := rustdeskKey([]byte("ab")); k[0] != 'a' || k[1] != 'b' || k[2] != 0 {
		t.Errorf("rustdeskKey = %v", k)
	}
}

func TestRustDeskPasswordStorage(t *testing.T) {
	salt := "abcdefghijkmnpqrstuvwxyz23456789"
	s := rustdeskPasswordStorage("hunter2", salt, twinKey, true)
	if !strings.HasPrefix(s, "01") {
		t.Fatalf("hashed storage %q doesn't start with 01", s)
	}
	inner, ok := decWith(t, s, twinKey)
	h1 := sha256.Sum256([]byte("hunter2" + salt))
	if !ok || inner != "00"+base64.StdEncoding.EncodeToString(h1[:]) {
		t.Errorf("inner storage = %q, %v", inner, ok)
	}
	legacy := rustdeskPasswordStorage("hunter2", salt, twinKey, false)
	if got, ok := decWith(t, legacy, twinKey); !ok || got != "hunter2" || !strings.HasPrefix(legacy, "00") {
		t.Errorf("legacy storage decrypts to %q, %v", got, ok)
	}
	if salt := newRustDeskSalt(); len(salt) != 32 || strings.Trim(salt, rustdeskSaltChars) != "" {
		t.Errorf("salt %q", salt)
	}
}

// writeFixture lays out a RustDesk config folder as RustDesk 1.5 writes it,
// encrypted for testMachineID, and returns the hashed password storage.
func writeFixture(t *testing.T, dir string) (password string) {
	t.Helper()
	salt := "k7mz2p9qrs3tuvw4xy5abc6defgh8ijn"
	password = rustdeskPasswordStorage("hunter2", salt, realKey, true)
	twoFA, _ := json.Marshal(map[string]any{
		"name": "RustDesk", "digits": 6, "created_at": 1761800000,
		"secret": []int(nil),
	})
	var obj map[string]json.RawMessage
	json.Unmarshal(twoFA, &obj)
	obj["secret"] = marshalBytes([]byte(enc("JBSWY3DPEHPK3PXP", realKey, true)))
	twoFA, _ = json.Marshal(obj)

	files := map[string]string{
		"RustDesk.toml": `enc_id = '` + enc("1762635493", realKey, true) + `'
password = '` + password + `'
salt = '` + salt + `'
key_pair = [
    [1, 2, 3],
    [9, 9, 9],
]
key_confirmed = true

[keys_confirmed]
rs-ny = true
`,
		"RustDesk2.toml": `rendezvous_server = 'rs-ny.rustdesk.com:21116'
nat_type = 1
serial = 0
unlock_pin = '` + enc("4321", realKey, false) + `'
trusted_devices = '` + enc(`[{"hwid":[1],"id":"9"}]`, realKey, true) + `'

[socks]
proxy = 'proxy.local:1080'
username = 'me'
password = '` + enc("proxy-pw", realKey, true) + `'

[options]
custom-rendezvous-server = 'rd.example.com'
key = 'PUBKEY='
verification-method = 'use-permanent-password'
2fa = ` + quoteTOML(string(twoFA)) + `
`,
		"RustDesk_local.toml":   "remote_id = '555'\n",
		"RustDesk_default.toml": "[options]\nview_style = 'adaptive'\n",
		"peers/555.toml":        "password = " + byteArrayTOML([]byte(enc("peer-pw", realKey, true))) + "\n\n[options]\nos-password = '" + enc("os-pw", realKey, true) + "'\n",
	}
	for name, data := range files {
		write(t, filepath.Join(dir, name), data)
	}
	write(t, filepath.Join(dir, "RustDesk_ab"), string(rustdeskSeal([]byte("address book"), realKey, true)))
	return password
}

func TestRekeyRustDeskConfig(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	local, _ := os.ReadFile(filepath.Join(dir, "RustDesk_local.toml"))
	keys := rustdeskKeys{From: rustdeskOriginalKeys(dir, testMachineID), To: twinKey}
	if id := rustdeskID(dir, keys); id != "1762635493" {
		t.Fatalf("original ID = %q", id)
	}

	notes, err := rekeyRustDeskConfig(dir, keys, true)
	if err != nil || len(notes) != 0 {
		t.Fatalf("rekey: %v, notes %q", err, notes)
	}
	one, _ := readTOML(filepath.Join(dir, "RustDesk.toml"))
	two, _ := readTOML(filepath.Join(dir, "RustDesk2.toml"))

	// Secrets now open with the twin's key, and not with the real one.
	pw, _ := one.get("", "password")
	h1 := sha256.Sum256([]byte("hunter2k7mz2p9qrs3tuvw4xy5abc6defgh8ijn"))
	if got, ok := decWith(t, pw, twinKey); !ok || got != "00"+base64.StdEncoding.EncodeToString(h1[:]) || !strings.HasPrefix(pw, "01") {
		t.Errorf("password under the twin's key = %q, %v", got, ok)
	}
	if _, ok := decWith(t, pw, realKey); ok {
		t.Error("password still opens with the real key")
	}
	for _, f := range [][3]string{{"", "unlock_pin", "4321"}, {"socks", "password", "proxy-pw"}} {
		v, _ := two.get(f[0], f[1])
		if got, ok := decWith(t, v, twinKey); !ok || got != f[2] {
			t.Errorf("%s = %q, %v", fieldName(f[0], f[1]), got, ok)
		}
	}
	if v, _ := two.get("", "unlock_pin"); v[2] == base64.StdEncoding.EncodeToString([]byte{formatV1})[0] {
		t.Error("unlock_pin should keep the legacy format it came in")
	}
	var twoFA map[string]json.RawMessage
	raw, _ := two.get("options", "2fa")
	var secret []byte
	if json.Unmarshal([]byte(raw), &twoFA) != nil || !unmarshalBytes(twoFA["secret"], &secret) {
		t.Fatalf("2fa option = %q", raw)
	}
	if got, ok := decWith(t, string(secret), twinKey); !ok || got != "JBSWY3DPEHPK3PXP" || string(twoFA["digits"]) != "6" {
		t.Errorf("2fa secret = %q, %v", got, ok)
	}

	// The original's identity is gone; its settings stay.
	for _, key := range []string{"enc_id", "id", "key_pair", "key_confirmed"} {
		if _, _, ok := one.find("", key); ok {
			t.Errorf("RustDesk.toml still has %s", key)
		}
	}
	if strings.Contains(one.String(), "keys_confirmed") || strings.Contains(two.String(), "trusted_devices") {
		t.Errorf("confirmations or trusted devices left:\n%s\n%s", one, two)
	}
	if salt, _ := one.get("", "salt"); salt != "k7mz2p9qrs3tuvw4xy5abc6defgh8ijn" {
		t.Error("salt changed")
	}
	for _, kv := range [][3]string{{"options", "custom-rendezvous-server", "rd.example.com"}, {"options", "key", "PUBKEY="},
		{"options", "verification-method", "use-permanent-password"}, {"", "rendezvous_server", "rs-ny.rustdesk.com:21116"}, {"socks", "username", "me"}} {
		if got, _ := two.get(kv[0], kv[1]); got != kv[2] {
			t.Errorf("%s = %q, want %q", fieldName(kv[0], kv[1]), got, kv[2])
		}
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "RustDesk_local.toml")); string(after) != string(local) {
		t.Error("RustDesk_local.toml changed")
	}
	ab, _ := os.ReadFile(filepath.Join(dir, "RustDesk_ab"))
	if plain, _, ok := rustdeskOpen(ab, twinKey); !ok || string(plain) != "address book" {
		t.Error("address book not moved to the twin's key")
	}
	peer, _ := readTOML(filepath.Join(dir, "peers", "555.toml"))
	praw, _ := peer.raw("", "password")
	if b, _ := parseByteArray(praw); func() bool { got, ok := decWith(t, string(b), twinKey); return !ok || got != "peer-pw" }() {
		t.Error("saved peer password not moved")
	}
	for _, name := range []string{"RustDesk.toml", "RustDesk2.toml"} {
		if info, _ := os.Stat(filepath.Join(dir, name)); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", name, info.Mode().Perm())
		}
	}

	// Doing it again changes nothing.
	before, _ := os.ReadFile(filepath.Join(dir, "RustDesk2.toml"))
	if _, err := rekeyRustDeskConfig(dir, keys, true); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "RustDesk2.toml")); string(again) != string(before) {
		t.Error("a second run changed RustDesk2.toml")
	}
}

func TestRekeyKeepsTwinsOwnID(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "RustDesk.toml"), "enc_id = '"+enc("299612344", twinKey, true)+"'\nkey_pair = [[1], [2]]\n")
	keys := rustdeskKeys{From: []([32]byte){realKey}, To: twinKey}
	if _, err := rekeyRustDeskConfig(dir, keys, false); err != nil {
		t.Fatal(err)
	}
	if id := rustdeskID(dir, keys); id != "299612344" {
		t.Errorf("the twin's own ID went: %q", id)
	}
}

func TestRekeyRefusesUndecryptable(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	other := rustdeskKey([]byte("ffffffffffffffffffffffffffffffff"))
	pin := "unlock_pin = '" + enc("4321", other, true) + "'\n"
	write(t, filepath.Join(dir, "RustDesk2.toml"), pin)
	before, _ := os.ReadFile(filepath.Join(dir, "RustDesk.toml"))

	_, err := rekeyRustDeskConfig(dir, rustdeskKeys{From: []([32]byte){realKey}, To: twinKey}, true)
	if err == nil || !strings.Contains(err.Error(), "RustDesk2.toml unlock_pin") {
		t.Fatalf("err = %v, want one naming RustDesk2.toml unlock_pin", err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "RustDesk.toml")); string(after) != string(before) {
		t.Error("RustDesk.toml was written although the re-key failed")
	}
}

func TestRekeyFallsBackToPublicKey(t *testing.T) {
	// When RustDesk can't read the machine-id it encrypts with its public key.
	dir := t.TempDir()
	pk := []byte("0123456789abcdef0123456789abcdef")
	write(t, filepath.Join(dir, "RustDesk.toml"), "password = '"+enc("legacy-pw", rustdeskKey(pk), false)+"'\nkey_pair = [[7], "+byteArrayTOML(pk)+"]\n")
	keys := rustdeskKeys{From: rustdeskOriginalKeys(dir, testMachineID), To: twinKey}
	if _, err := rekeyRustDeskConfig(dir, keys, true); err != nil {
		t.Fatal(err)
	}
	one, _ := readTOML(filepath.Join(dir, "RustDesk.toml"))
	pw, _ := one.get("", "password")
	if got, ok := decWith(t, pw, twinKey); !ok || got != "legacy-pw" {
		t.Errorf("password = %q, %v", got, ok)
	}
}

func TestRekeyLeavesPlaintext(t *testing.T) {
	k := rustdeskKeys{From: []([32]byte){realKey}, To: twinKey}
	for _, s := range []string{"", "hunter2", "00abc", "0", "01plain text"} {
		if got, err := k.rekeyString(s, "00", "01"); err != nil || got != s {
			t.Errorf("rekeyString(%q) = %q, %v", s, got, err)
		}
	}
}

func TestCompareRustDeskIDs(t *testing.T) {
	own := Plan{Name: testTwinName, OwnHardware: true}
	if _, err := compareRustDeskIDs(own, "299612344", "1762635493"); err != nil {
		t.Errorf("different IDs refused: %v", err)
	}
	if _, err := compareRustDeskIDs(own, "1762635493", "1762635493"); err == nil {
		t.Error("the original's ID was accepted")
	}
	shared := Plan{Name: testTwinName}
	if _, err := compareRustDeskIDs(shared, "1762635493", "1762635493"); err == nil || !strings.Contains(err.Error(), "--share-hardware") {
		t.Errorf("shared hardware: %v", err)
	}
	if note, err := compareRustDeskIDs(own, "299612344", ""); err != nil || note == "" {
		t.Error("an unknown original ID should pass with a note")
	}
}
