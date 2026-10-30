package main

import (
	"fmt"
	"strconv"
	"strings"
)

// tomlDoc edits a TOML file line by line, so whatever twin doesn't touch
// stays exactly as the app wrote it. It understands enough TOML for app
// config files: bare and quoted keys, basic and literal strings, arrays and
// inline tables that span lines, and [table] / [[array]] headers.
type tomlDoc struct{ lines []string }

func parseTOML(data string) *tomlDoc {
	data = strings.TrimSuffix(data, "\n")
	if data == "" {
		return &tomlDoc{}
	}
	return &tomlDoc{lines: strings.Split(data, "\n")}
}

func (d *tomlDoc) String() string {
	if len(d.lines) == 0 {
		return ""
	}
	return strings.Join(d.lines, "\n") + "\n"
}

// tomlKey is a key and the lines its value spans.
type tomlKey struct {
	name        string
	first, last int
}

// tomlTable is a table's header line (-1 for the top level) and its keys.
type tomlTable struct {
	name   string // "" for the top level
	header int
	end    int // the line after its last key
	keys   []tomlKey
}

func (d *tomlDoc) tables() []tomlTable {
	cur := tomlTable{header: -1}
	var out []tomlTable
	for i := 0; i < len(d.lines); i++ {
		line := strings.TrimSpace(d.lines[i])
		if strings.HasPrefix(line, "[") {
			out = append(out, cur)
			cur = tomlTable{name: tableName(line), header: i, end: i + 1}
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found || strings.HasPrefix(line, "#") {
			continue
		}
		end := i // values that span lines end where their brackets close
		for depth := nesting(v); depth > 0 && end+1 < len(d.lines); {
			end++
			depth += nesting(d.lines[end])
		}
		cur.keys = append(cur.keys, tomlKey{unquoteKey(strings.TrimSpace(k)), i, end})
		cur.end = end + 1
		i = end
	}
	return append(out, cur)
}

// tableName reads "[a.b]" or "[[a]]" as a.b or a.
func tableName(header string) string {
	header = strings.TrimSpace(strings.SplitN(header, "#", 2)[0])
	header = strings.Trim(header, "[]")
	return strings.TrimSpace(header)
}

// find returns the first and last line of key's value in table ("" for the
// top level).
func (d *tomlDoc) find(table, key string) (first, last int, ok bool) {
	for _, t := range d.tables() {
		if t.name != table {
			continue
		}
		for _, k := range t.keys {
			if k.name == key {
				return k.first, k.last, true
			}
		}
	}
	return 0, 0, false
}

func unquoteKey(k string) string {
	if len(k) >= 2 && (k[0] == '"' || k[0] == '\'') && k[len(k)-1] == k[0] {
		return k[1 : len(k)-1]
	}
	return k
}

// nesting counts how many brackets and braces a line opens but doesn't
// close, outside strings and comments.
func nesting(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return depth
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		}
	}
	return depth
}

// raw returns the text of a value, joined into one line.
func (d *tomlDoc) raw(table, key string) (string, bool) {
	first, last, ok := d.find(table, key)
	if !ok {
		return "", false
	}
	_, v, _ := strings.Cut(d.lines[first], "=")
	parts := []string{strings.TrimSpace(v)}
	for _, l := range d.lines[first+1 : last+1] {
		parts = append(parts, strings.TrimSpace(l))
	}
	return strings.Join(parts, " "), true
}

// get returns a string value. ok is false when the key is missing or isn't
// a string.
func (d *tomlDoc) get(table, key string) (value string, ok bool) {
	v, found := d.raw(table, key)
	if !found {
		return "", false
	}
	return parseTOMLString(v)
}

// set writes a string value, in place when the key exists, else after the
// table's last key; a missing table is added at the end.
func (d *tomlDoc) set(table, key, value string) {
	d.setRaw(table, key, quoteTOML(value))
}

func (d *tomlDoc) setRaw(table, key, value string) {
	line := quoteKey(key) + " = " + value
	if first, last, ok := d.find(table, key); ok {
		d.lines = append(d.lines[:first], append([]string{line}, d.lines[last+1:]...)...)
		return
	}
	for _, t := range d.tables() {
		if t.name == table && (t.header >= 0 || table == "") {
			at := t.end
			if t.header < 0 && len(t.keys) == 0 {
				at = 0
			}
			d.lines = append(d.lines[:at], append([]string{line}, d.lines[at:]...)...)
			return
		}
	}
	if len(d.lines) > 0 && strings.TrimSpace(d.lines[len(d.lines)-1]) != "" {
		d.lines = append(d.lines, "")
	}
	d.lines = append(d.lines, "["+table+"]", line)
}

// del removes a key and reports whether it was there.
func (d *tomlDoc) del(table, key string) bool {
	first, last, ok := d.find(table, key)
	if ok {
		d.lines = append(d.lines[:first], d.lines[last+1:]...)
	}
	return ok
}

// delTable removes [name], [[name]] and their sub-tables, header and keys,
// and reports whether there was any.
func (d *tomlDoc) delTable(name string) bool {
	removed := false
	for {
		var target *tomlTable
		ts := d.tables()
		for i := range ts {
			if ts[i].header >= 0 && (ts[i].name == name || strings.HasPrefix(ts[i].name, name+".")) {
				target = &ts[i]
				break
			}
		}
		if target == nil {
			return removed
		}
		end := len(d.lines) // up to the next header
		for i := target.header + 1; i < len(d.lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(d.lines[i]), "[") && !inValue(ts, i) {
				end = i
				break
			}
		}
		d.lines = append(d.lines[:target.header], d.lines[end:]...)
		for len(d.lines) > 0 && strings.TrimSpace(d.lines[len(d.lines)-1]) == "" {
			d.lines = d.lines[:len(d.lines)-1]
		}
		removed = true
	}
}

// inValue reports whether line i continues a value that spans lines.
func inValue(ts []tomlTable, i int) bool {
	for _, t := range ts {
		for _, k := range t.keys {
			if i > k.first && i <= k.last {
				return true
			}
		}
	}
	return false
}

// parseTOMLString reads a basic ("…") or literal ('…') string, ignoring a
// trailing comment.
func parseTOMLString(v string) (string, bool) {
	if len(v) < 2 {
		return "", false
	}
	switch v[0] {
	case '\'':
		end := strings.IndexByte(v[1:], '\'')
		if end < 0 || strings.HasPrefix(v, "'''") {
			return "", false
		}
		return v[1 : 1+end], true
	case '"':
		if strings.HasPrefix(v, `"""`) {
			return "", false
		}
		for i := 1; i < len(v); i++ {
			switch v[i] {
			case '\\':
				i++
			case '"':
				s, err := strconv.Unquote(v[:i+1])
				return s, err == nil
			}
		}
	}
	return "", false
}

// quoteKey leaves bare keys bare and quotes the rest.
func quoteKey(k string) string {
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return quoteTOML(k)
		}
	}
	if k == "" {
		return `""`
	}
	return k
}

// quoteTOML writes a literal string when it can, as RustDesk does, else a
// basic string.
func quoteTOML(s string) string {
	if !strings.ContainsAny(s, "'\n\r") && isPrintable(s) {
		return "'" + s + "'"
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}

// parseByteArray reads an array of small integers, such as a Vec<u8> that
// serde wrote: [1, 2, 3].
func parseByteArray(v string) ([]byte, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, false
	}
	var out []byte
	for _, f := range strings.Split(v[1:len(v)-1], ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.ParseUint(f, 10, 8)
		if err != nil {
			return nil, false
		}
		out = append(out, byte(n))
	}
	return out, true
}

// splitArray splits an array of arrays, [[1, 2], [3]], into its elements.
func splitArray(v string) []string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil
	}
	var out []string
	depth, start := 0, -1
	for i, c := range v[1 : len(v)-1] {
		switch c {
		case '[':
			if depth == 0 {
				start = i
			}
			depth++
		case ']':
			if depth--; depth == 0 && start >= 0 {
				out = append(out, v[1+start:1+i+1])
			}
		}
	}
	return out
}
