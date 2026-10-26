package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const releasesAPI = "https://api.github.com/repos/mukhayyar/twin/releases/latest"

// selfUpdate replaces this binary with the latest release, once its archive
// matches the published checksum. Homebrew installs are left to Homebrew.
func selfUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if strings.Contains(exe, "/Cellar/") {
		return errors.New("twin was installed with Homebrew; update it with: brew upgrade twin")
	}

	tag, assets, err := latestRelease()
	if err != nil {
		return err
	}
	current := resolvedVersion()
	if !newerVersion(tag, current) {
		fmt.Printf("twin %s is the newest version\n", current)
		return nil
	}
	v := strings.TrimPrefix(tag, "v")
	name := fmt.Sprintf("twin_%s_%s_%s.tar.gz", v, runtime.GOOS, runtime.GOARCH)
	switch runtime.GOOS {
	case "darwin":
		name = fmt.Sprintf("twin_%s_darwin_universal.tar.gz", v)
	case "windows":
		name = fmt.Sprintf("twin_%s_windows_%s.zip", v, runtime.GOARCH)
	}
	if assets[name] == "" || assets["checksums.txt"] == "" {
		return fmt.Errorf("release %s has no %s for this system yet", tag, name)
	}

	fmt.Printf("Downloading twin %s…\n", tag)
	archive, err := fetch(assets[name])
	if err != nil {
		return err
	}
	sums, err := fetch(assets["checksums.txt"])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	if want := checksumFor(string(sums), name); want == "" || want != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("%s didn't match its published checksum; nothing was changed", name)
	}
	binary, err := extractBinary(archive, name)
	if err != nil {
		return err
	}
	if err := replaceExecutable(exe, binary); err != nil {
		return fmt.Errorf("couldn't replace %s: %w", exe, err)
	}
	fmt.Printf("Updated twin %s → %s\n", current, tag)
	return nil
}

func latestRelease() (tag string, assets map[string]string, err error) {
	req, _ := http.NewRequest("GET", releasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "twin/"+resolvedVersion())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("GitHub answered %s; try again later", resp.Status)
	}
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", nil, err
	}
	assets = map[string]string{}
	for _, a := range release.Assets {
		assets[a.Name] = a.URL
	}
	return release.Tag, assets, nil
}

func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", filepath.Base(url), resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// newerVersion compares dotted versions numerically; "dev" and suffixed
// builds count as older than a release.
func newerVersion(candidate, current string) bool {
	parts := func(v string) []int {
		v, _, _ = strings.Cut(strings.TrimPrefix(v, "v"), "-")
		var out []int
		for _, p := range strings.Split(v, ".") {
			n, _ := strconv.Atoi(p)
			out = append(out, n)
		}
		return out
	}
	a, b := parts(candidate), parts(current)
	for i := range max(len(a), len(b)) {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return strings.Contains(current, "-") || current == "dev"
}

func checksumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			return f[0]
		}
	}
	return ""
}

// extractBinary pulls twin (twin.exe on Windows) out of a release archive.
func extractBinary(archive []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range r.File {
			if f.Name == "twin.exe" {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s has no twin.exe", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s has no twin binary", name)
		}
		if h.Name == "twin" {
			return io.ReadAll(tr)
		}
	}
}

// replaceExecutable swaps in the new binary next to the old one. A running
// program can be renamed but not overwritten on Windows, so the old one
// moves aside first.
func replaceExecutable(exe string, binary []byte) error {
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, binary, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
