package main

import (
	"reflect"
	"testing"
)

func TestShellWords(t *testing.T) {
	cases := map[string][]string{
		`/Applications/Visual\ Studio\ Code.app`:        {"/Applications/Visual Studio Code.app"},
		`'/Applications/Google Chrome.app' `:            {"/Applications/Google Chrome.app"},
		`"/Applications/Slack.app" /Applications/A.app`: {"/Applications/Slack.app", "/Applications/A.app"},
		"  /Applications/Zoom.app\n":                    {"/Applications/Zoom.app"},
	}
	for in, want := range cases {
		if got := shellWords(in); !reflect.DeepEqual(got, want) {
			t.Errorf("shellWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeLabel(t *testing.T) {
	cases := map[string]string{
		"Work":          "Work",
		"  my work  ":   "my-work",
		"Client A/B!":   "Client-AB",
		"":              "Clone",
		"!!!":           "Clone",
		"Personal 2026": "Personal-2026",
	}
	for in, want := range cases {
		if got := sanitizeLabel(in); got != want {
			t.Errorf("sanitizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikePath(t *testing.T) {
	for in, want := range map[string]bool{
		"/Applications/Slack.app": true,
		"'/Applications/A B.app'": true,
		"~/Downloads/X.app":       true,
		"slack":                   false,
		"Work":                    false,
	} {
		if got := looksLikePath(in); got != want {
			t.Errorf("looksLikePath(%q) = %v, want %v", in, got, want)
		}
	}
}
