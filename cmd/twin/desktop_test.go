package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"reflect"
	"strings"
	"testing"
)

func TestParseDesktop(t *testing.T) {
	e := parseDesktop(`# comment
[Desktop Entry]
Type=Application
Name=Visual Studio Code
Name[de]=Visual Studio Code DE
Exec=/usr/share/code/code --unity-launch %F
Icon=vscode
Categories=Development;IDE;
NoDisplay=false

[Desktop Action new-empty-window]
Name=New Empty Window
Exec=/usr/share/code/code --new-window %F
`)
	want := desktopEntry{Name: "Visual Studio Code", Exec: "/usr/share/code/code --unity-launch %F", Icon: "vscode",
		Categories: "Development;IDE;", IsApp: true}
	if e != want {
		t.Errorf("got %+v\nwant %+v", e, want)
	}
	if !parseDesktop("[Desktop Entry]\nType=Application\nNoDisplay=true\n").Hidden {
		t.Error("NoDisplay=true should hide the entry")
	}
}

func TestExecArgs(t *testing.T) {
	cases := map[string][]string{
		`discord %U`: {"discord", "%U"},
		`/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=com.discordapp.Discord com.discordapp.Discord @@u %U @@`: {
			"/usr/bin/flatpak", "run", "--branch=stable", "--arch=x86_64", "--command=com.discordapp.Discord", "com.discordapp.Discord", "@@u", "%U", "@@"},
		`"/opt/My App/bin/app" --flag "a \"quoted\" word" 100%%`: {"/opt/My App/bin/app", "--flag", `a "quoted" word`, "100%"},
		`sh -c "echo \$HOME"`: {"sh", "-c", "echo $HOME"},
	}
	for in, want := range cases {
		if got := execArgs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("execArgs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinExecRoundTrip(t *testing.T) {
	args := []string{"/home/a b/.local/share/twin/twin-x.sh", "%U", `it's "50%" $off`}
	if got := execArgs(joinExec(args)); !reflect.DeepEqual(got, args) {
		t.Errorf("round trip: %q → %q", args, got)
	}
}

func TestSplitFieldCodes(t *testing.T) {
	cmd, codes := splitFieldCodes([]string{"app", "--x", "%i", "%U", "%c", "%k"})
	if !reflect.DeepEqual(cmd, []string{"app", "--x"}) || !reflect.DeepEqual(codes, []string{"%U"}) {
		t.Errorf("cmd=%q codes=%q", cmd, codes)
	}
}

func TestFlatpakExec(t *testing.T) {
	got := flatpakExec(execArgs("flatpak run com.slack.Slack @@u %U @@"), "/home/a/.app_profiles/Slack-Work")
	joined := strings.Join(got, " ")
	for _, want := range []string{"run --filesystem=/home/a/.app_profiles/Slack-Work",
		"--env=XDG_CONFIG_HOME=/home/a/.app_profiles/Slack-Work/.config", "com.slack.Slack @@u %U @@"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
}

func TestLauncherScript(t *testing.T) {
	s := launcherScript("Slack", "/home/a/.app_profiles/Slack-Work", []string{"/usr/bin/slack", "--it's"})
	for _, want := range []string{
		"P='/home/a/.app_profiles/Slack-Work'",
		`export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"`,
		`export HOME="$P" XDG_CONFIG_HOME="$P/.config"`,
		`exec '/usr/bin/slack' '--it'\''s' "$@"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("launcher missing %q:\n%s", want, s)
		}
	}
}

func TestDesktopTwinRoundTrip(t *testing.T) {
	exec := joinExec([]string{"/home/a/.local/share/twin/twin-slack-work.sh", "%U"})
	e := parseDesktop(desktopTwin("Slack-Work", "Slack with its own Work profile", exec, "/i.png", "Network;", "/usr/share/applications/slack.desktop", "/p", false))
	if e.Name != "Slack-Work" || e.TwinSource != "/usr/share/applications/slack.desktop" || !e.IsApp || e.Hidden {
		t.Errorf("parsed back %+v", e)
	}
	if got := execArgs(e.Exec); !reflect.DeepEqual(got, []string{"/home/a/.local/share/twin/twin-slack-work.sh", "%U"}) {
		t.Errorf("exec round trip: %q", got)
	}
}

func TestSlugifyAndSnap(t *testing.T) {
	if got := slugify("Visual Studio Code-Work"); got != "visual-studio-code-work" {
		t.Errorf("slugify = %q", got)
	}
	if !isSnap("env BAMF_DESKTOP_FILE_HINT=/var/lib/snapd/desktop/applications/x.desktop /snap/bin/x %U") || isSnap("discord %U") {
		t.Error("isSnap misclassified")
	}
}

func TestWinLauncherArgs(t *testing.T) {
	got := winLauncherArgs(`C:\Users\a\.app_profiles\Discord-Work`, `C:\Users\a\AppData\Local\Discord\Update.exe`, `--processStart Discord.exe`)
	want := `/c set "USERPROFILE=C:\Users\a\.app_profiles\Discord-Work" & set "HOME=C:\Users\a\.app_profiles\Discord-Work" & ` +
		`set "APPDATA=C:\Users\a\.app_profiles\Discord-Work\AppData\Roaming" & set "LOCALAPPDATA=C:\Users\a\.app_profiles\Discord-Work\AppData\Local" & ` +
		`start "" "C:\Users\a\AppData\Local\Discord\Update.exe" --processStart Discord.exe`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestPSQuoteAndIconLocation(t *testing.T) {
	if got := psQuote(`C:\it's here`); got != `'C:\it''s here'` {
		t.Errorf("psQuote = %s", got)
	}
	if got := iconLocation(`C:\Program Files\App, Inc\app.exe,0`); got != `C:\Program Files\App, Inc\app.exe` {
		t.Errorf("iconLocation = %q", got)
	}
}

func TestEncodeICO(t *testing.T) {
	var b bytes.Buffer
	imgs := []image.Image{image.NewNRGBA(image.Rect(0, 0, 256, 256)), image.NewNRGBA(image.Rect(0, 0, 16, 16))}
	if err := encodeICO(&b, imgs); err != nil {
		t.Fatal(err)
	}
	d := b.Bytes()
	if binary.LittleEndian.Uint16(d[2:]) != 1 || binary.LittleEndian.Uint16(d[4:]) != 2 {
		t.Fatalf("bad header % x", d[:6])
	}
	if d[6] != 0 || d[6+16] != 16 {
		t.Errorf("sizes: %d, %d (256 must be stored as 0)", d[6], d[6+16])
	}
	off := binary.LittleEndian.Uint32(d[6+12:])
	if !bytes.HasPrefix(d[off:], []byte("\x89PNG")) {
		t.Error("first entry isn't a PNG")
	}
}
