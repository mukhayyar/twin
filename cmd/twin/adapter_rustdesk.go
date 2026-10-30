package main

// RustDesk keeps its secrets in ~/.config/rustdesk encrypted with the
// machine-id (hbb_common's password_security.rs and config.rs). A twin sees
// its own machine-id, so it can't read secrets copied from the original.
// This file moves them to the twin's key and strips what makes a config the
// original's device, so the twin registers as a new one. It's plain Go with
// no platform code, and only ever used for RustDesk
// (adapter_rustdesk_linux.go).

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/nacl/secretbox"
)

// rustdeskKey is the secretbox key RustDesk makes from a machine-id (its
// ASCII bytes) or, failing that, from its public key: zero-padded or cut to
// 32 bytes.
func rustdeskKey(material []byte) [32]byte {
	var k [32]byte
	copy(k[:], material)
	return k
}

// formatV1 marks ciphertext from RustDesk 1.4.7 on: a random nonce follows.
// Earlier versions used an all-zero nonce and no marker; RustDesk still
// reads both.
const formatV1 = 1

func rustdeskSeal(plain []byte, key [32]byte, v1 bool) []byte {
	var nonce [24]byte
	if !v1 {
		return secretbox.Seal(nil, plain, &nonce, &key)
	}
	rand.Read(nonce[:])
	return secretbox.Seal(append([]byte{formatV1}, nonce[:]...), plain, &nonce, &key)
}

// rustdeskOpen decrypts either format and reports which one it was.
func rustdeskOpen(box []byte, key [32]byte) (plain []byte, v1, ok bool) {
	if len(box) >= 1+24+secretbox.Overhead && box[0] == formatV1 {
		var nonce [24]byte
		copy(nonce[:], box[1:25])
		if plain, ok := secretbox.Open(nil, box[25:], &nonce, &key); ok {
			return plain, true, true
		}
	}
	var zero [24]byte
	plain, ok = secretbox.Open(nil, box, &zero, &key)
	return plain, false, ok
}

// rustdeskKeys are the keys a copied config moves between.
type rustdeskKeys struct {
	From [][32]byte // the original's: its machine-id, then its public key
	To   [32]byte   // the twin's
}

var errNoDecrypt = errors.New("doesn't decrypt with this computer's key")

// rekeyBox re-encrypts raw ciphertext for the twin in the same format.
// Ciphertext that already opens with the twin's key stays as it is.
func (k rustdeskKeys) rekeyBox(box []byte) ([]byte, error) {
	if _, _, ok := rustdeskOpen(box, k.To); ok {
		return box, nil
	}
	for _, from := range k.From {
		if plain, v1, ok := rustdeskOpen(box, from); ok {
			return rustdeskSeal(plain, k.To, v1), nil
		}
	}
	return nil, errNoDecrypt
}

// rekeyString re-encrypts a stored value: one of versions ("00", or "01"
// for a hashed permanent password) followed by standard base64. Like
// RustDesk, it takes anything else, or anything too short to be
// ciphertext, as plaintext and leaves it alone.
func (k rustdeskKeys) rekeyString(s string, versions ...string) (string, error) {
	if len(s) <= 2 || !contains(versions, s[:2]) {
		return s, nil
	}
	box, err := base64.StdEncoding.DecodeString(s[2:])
	if err != nil || len(box) < secretbox.Overhead {
		return s, nil
	}
	out, err := k.rekeyBox(box)
	if err != nil {
		return "", err
	}
	return s[:2] + base64.StdEncoding.EncodeToString(out), nil
}

// open decrypts a "00" value with the first key that fits; ok is false for
// an empty value or one that no key opens.
func (k rustdeskKeys) open(s string) (string, bool) {
	if len(s) <= 2 || s[:2] != "00" {
		return "", false
	}
	box, err := base64.StdEncoding.DecodeString(s[2:])
	if err != nil {
		return "", false
	}
	for _, key := range append([][32]byte{k.To}, k.From...) {
		if plain, _, ok := rustdeskOpen(box, key); ok {
			return string(plain), true
		}
	}
	return "", false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// rustdeskPublicKey reads the public key from RustDesk.toml's key_pair
// (secret key, public key; two arrays of bytes), the key RustDesk falls back
// to when it can't read the machine-id.
func rustdeskPublicKey(doc *tomlDoc) []byte {
	raw, ok := doc.raw("", "key_pair")
	if !ok {
		return nil
	}
	parts := splitArray(raw)
	if len(parts) != 2 {
		return nil
	}
	pk, ok := parseByteArray(parts[1])
	if !ok || len(pk) == 0 {
		return nil
	}
	return pk
}

// rustdeskID reads the ID a RustDesk config answers to, or "" if it has
// none it can read.
func rustdeskID(dir string, k rustdeskKeys) string {
	data, err := os.ReadFile(filepath.Join(dir, "RustDesk.toml"))
	if err != nil {
		return ""
	}
	doc := parseTOML(string(data))
	if enc, _ := doc.get("", "enc_id"); enc != "" {
		id, _ := k.open(enc)
		return id
	}
	id, _ := doc.get("", "id")
	return id
}

// compareRustDeskIDs refuses a twin that answers to the original's RustDesk
// ID: anyone connecting to that ID could reach either of them.
func compareRustDeskIDs(p Plan, twin, original string) (note string, err error) {
	switch {
	case original == "":
		return "couldn't read RustDesk's own ID to compare; the twin's is " + twin, nil
	case twin != original:
		return "", nil
	case !p.OwnHardware:
		return "", fmt.Errorf("%s answers to RustDesk's own ID %s: RustDesk makes its ID from the hardware, so a twin needs its own (leave out --share-hardware)", p.Name, twin)
	}
	return "", fmt.Errorf("%s answers to RustDesk's own ID %s; not finishing it", p.Name, twin)
}

// rustdeskOriginalKeys are the keys the original's config may be encrypted
// with: the machine-id, then the public key in its key_pair.
func rustdeskOriginalKeys(dir, machineID string) [][32]byte {
	keys := [][32]byte{rustdeskKey([]byte(machineID))}
	if data, err := os.ReadFile(filepath.Join(dir, "RustDesk.toml")); err == nil {
		if pk := rustdeskPublicKey(parseTOML(string(data))); pk != nil {
			keys = append(keys, rustdeskKey(pk))
		}
	}
	return keys
}

// rekeyRustDeskConfig moves a copied RustDesk config folder to the twin's
// key. The permanent password (with its salt), unlock PIN, proxy password,
// two-factor secret, saved logins and address book come along. With
// stripIdentity, the ID, key pair, the rendezvous server's confirmation of
// them and the devices that trust the original don't, so the twin gets an
// ID and keys of its own. Nothing is written unless every secret of the
// twin's own decrypts; the error names the one that didn't. moved lists the
// secrets (field names only) that now open with the twin's key.
func rekeyRustDeskConfig(dir string, k rustdeskKeys, stripIdentity bool) (moved, notes []string, err error) {
	writes := map[string][]byte{}
	var removes []string
	field := func(doc *tomlDoc, table, key, file string, versions ...string) error {
		had, err := rekeyField(doc, table, key, k, file, versions...)
		if had {
			moved = append(moved, fieldName(table, key))
		}
		return err
	}

	if doc, err := readTOML(filepath.Join(dir, "RustDesk.toml")); err != nil {
		return nil, nil, err
	} else if doc != nil {
		if err := field(doc, "", "password", "RustDesk.toml", "00", "01"); err != nil {
			return nil, nil, err
		}
		if stripIdentity {
			for _, key := range []string{"id", "enc_id", "key_pair", "key_confirmed", "keys_confirmed"} {
				doc.del("", key)
			}
			doc.delTable("keys_confirmed")
		}
		writes["RustDesk.toml"] = []byte(doc.String())
	}

	if doc, err := readTOML(filepath.Join(dir, "RustDesk2.toml")); err != nil {
		return nil, nil, err
	} else if doc != nil {
		for _, f := range [][2]string{{"", "unlock_pin"}, {"socks", "password"}} {
			if err := field(doc, f[0], f[1], "RustDesk2.toml", "00"); err != nil {
				return nil, nil, err
			}
		}
		for _, f := range [][2]string{{"2fa", "secret"}, {"bot", "token"}} {
			had, err := rekeyOptionJSON(doc, f[0], f[1], k)
			if err != nil {
				return nil, nil, err
			}
			if had {
				moved = append(moved, fieldName("options", f[0]))
			}
		}
		if stripIdentity {
			doc.del("", "trusted_devices")
			doc.del("", "trusted_devices_v2")
			doc.delTable("trusted_devices_v2")
		}
		writes["RustDesk2.toml"] = []byte(doc.String())
	}

	// The address book caches: RustDesk deletes them when they don't decrypt
	// and fetches them again, so a failure here only costs a download.
	for _, name := range []string{"RustDesk_ab", "RustDesk_group"} {
		box, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if out, err := k.rekeyBox(box); err == nil {
			writes[name] = out
			moved = append(moved, name)
		} else {
			removes = append(removes, name)
		}
	}

	// Logins saved for other computers. One that doesn't decrypt is asked
	// for again on the next connection, so it doesn't stop the copy.
	peers, _ := filepath.Glob(filepath.Join(dir, "peers", "*.toml"))
	stale, logins := 0, 0
	for _, path := range peers {
		doc, err := readTOML(path)
		if err != nil || doc == nil {
			continue
		}
		changed, failed := rekeyPeer(doc, k)
		stale += failed
		if changed {
			writes[filepath.Join("peers", filepath.Base(path))] = []byte(doc.String())
			logins++
		}
	}
	if logins > 0 {
		moved = append(moved, fmt.Sprintf("peers (%d)", logins))
	}
	if stale > 0 {
		notes = append(notes, fmt.Sprintf("%d saved login(s) for other computers will ask for their password again", stale))
	}

	for name, data := range writes {
		if err := writeSecretFile(filepath.Join(dir, name), data); err != nil {
			return moved, notes, err
		}
	}
	for _, name := range removes {
		os.Remove(filepath.Join(dir, name))
	}
	return moved, notes, nil
}

// readTOML reads a TOML file, or returns nil if it doesn't exist.
func readTOML(path string) (*tomlDoc, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseTOML(string(data)), nil
}

// rekeyField re-encrypts one field; had reports whether it held a secret.
func rekeyField(doc *tomlDoc, table, key string, k rustdeskKeys, file string, versions ...string) (had bool, err error) {
	v, ok := doc.get(table, key)
	if !ok || v == "" {
		return false, nil
	}
	out, err := k.rekeyString(v, versions...)
	if err != nil {
		return true, fmt.Errorf("%s %s: %w", file, fieldName(table, key), err)
	}
	doc.set(table, key, out)
	return true, nil
}

func fieldName(table, key string) string {
	if table == "" {
		return key
	}
	return "[" + table + "] " + key
}

// rekeyOptionJSON re-encrypts a secret inside a JSON option, such as the
// two-factor secret: {"secret": [bytes of "00…"], …}.
func rekeyOptionJSON(doc *tomlDoc, option, field string, k rustdeskKeys) (had bool, err error) {
	v, ok := doc.get("options", option)
	if !ok || v == "" {
		return false, nil
	}
	var obj map[string]json.RawMessage
	var secret []byte
	if json.Unmarshal([]byte(v), &obj) != nil || obj[field] == nil || !unmarshalBytes(obj[field], &secret) || len(secret) == 0 {
		return false, nil // not the shape RustDesk writes; leave it
	}
	out, err := k.rekeyString(string(secret), "00")
	if err != nil {
		return true, fmt.Errorf("RustDesk2.toml [options] %s: %w", option, err)
	}
	obj[field] = marshalBytes([]byte(out))
	data, _ := json.Marshal(obj)
	doc.set("options", option, string(data))
	return true, nil
}

// unmarshalBytes reads a JSON array of numbers, which is how serde writes
// a Vec<u8>.
func unmarshalBytes(raw json.RawMessage, out *[]byte) bool {
	var nums []int
	if json.Unmarshal(raw, &nums) != nil {
		return false
	}
	*out = make([]byte, len(nums))
	for i, n := range nums {
		if n < 0 || n > 255 {
			return false
		}
		(*out)[i] = byte(n)
	}
	return true
}

func marshalBytes(b []byte) json.RawMessage {
	nums := make([]int, len(b))
	for i, c := range b {
		nums[i] = int(c)
	}
	data, _ := json.Marshal(nums)
	return data
}

// rekeyPeer re-encrypts a saved login: password (bytes of "00…") and the
// rdp_password, os-username and os-password options. failed counts the
// values that didn't decrypt, which are left as they were.
func rekeyPeer(doc *tomlDoc, k rustdeskKeys) (changed bool, failed int) {
	if raw, ok := doc.raw("", "password"); ok {
		if b, ok := parseByteArray(raw); ok && len(b) > 0 {
			if out, err := k.rekeyString(string(b), "00"); err != nil {
				failed++
			} else if out != string(b) {
				doc.setRaw("", "password", byteArrayTOML([]byte(out)))
				changed = true
			}
		}
	}
	for _, opt := range []string{"rdp_password", "os-username", "os-password"} {
		v, ok := doc.get("options", opt)
		if !ok || v == "" {
			continue
		}
		if out, err := k.rekeyString(v, "00"); err != nil {
			failed++
		} else if out != v {
			doc.set("options", opt, out)
			changed = true
		}
	}
	return changed, failed
}

func byteArrayTOML(b []byte) string {
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = fmt.Sprint(c)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// writeSecretFile replaces a file atomically, readable only by the user.
func writeSecretFile(path string, data []byte) error {
	tmp := path + ".twin-partial"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil { // WriteFile keeps an existing file's mode
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// rustdeskSaltChars are the characters RustDesk makes salts from (CHARS in
// hbb_common's config.rs).
const rustdeskSaltChars = "23456789abcdefghijkmnpqrstuvwxyz"

func newRustDeskSalt() string {
	b := make([]byte, 32)
	rand.Read(b)
	for i := range b {
		b[i] = rustdeskSaltChars[int(b[i])%len(rustdeskSaltChars)]
	}
	return string(b)
}

// rustdeskPasswordStorage is how RustDesk stores a permanent password. From
// 1.4.7 on (hashed): "01" + base64(encrypt("00" + base64(sha256(password +
// salt)))), the same hash peers prove they know. Before (legacy): "00" +
// base64(encrypt(password)).
func rustdeskPasswordStorage(password, salt string, key [32]byte, hashed bool) string {
	if !hashed {
		return "00" + base64.StdEncoding.EncodeToString(rustdeskSeal([]byte(password), key, false))
	}
	h1 := sha256.Sum256([]byte(password + salt))
	inner := "00" + base64.StdEncoding.EncodeToString(h1[:])
	return "01" + base64.StdEncoding.EncodeToString(rustdeskSeal([]byte(inner), key, true))
}
