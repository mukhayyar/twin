// Command probe is a stand-in app for the Windows CI smoke test: it records
// the folders it was started with into $TWIN_PROBE_OUT.
package main

import (
	"fmt"
	"os"
)

func main() {
	out := os.Getenv("TWIN_PROBE_OUT")
	if out == "" {
		return
	}
	var s string
	for _, k := range []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "HOME"} {
		s += fmt.Sprintf("%s=%s\n", k, os.Getenv(k))
	}
	os.WriteFile(out, []byte(s), 0o644)
}
