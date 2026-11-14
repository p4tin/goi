// Package paths resolves Goi's per-user configuration and cache directories.
//
// Directories are version-scoped (Goi/<Version>). Setting GOI_HOME redirects
// both to $GOI_HOME/config and $GOI_HOME/cache, which tests and dev builds use
// to stay isolated from the real user profile.
package paths

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Version is the major.minor used to scope user data directories.
const Version = "0.1"

// BuildVersion is the full product version of a release build, set with
// -ldflags "-X github.com/p4tin/goi/internal/paths.BuildVersion=0.1.0";
// "dev" otherwise.
var BuildVersion = "dev"

// ProductVersion is the full product version (info.version in
// build/config.yml). main sets it from the embedded build config, so dev
// builds know it too; it stays Version when that isn't available (tests).
var ProductVersion = Version

// ConfigDir returns the user config directory, e.g.
// ~/Library/Application Support/Goi/0.1 on macOS.
func ConfigDir() (string, error) {
	if h := goiHome(); h != "" {
		return filepath.Join(h, "config"), nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("paths: user config dir: %w", err)
	}
	return filepath.Join(d, "Goi", Version), nil
}

// CacheDir returns the user cache directory, e.g. ~/Library/Caches/Goi/0.1.
func CacheDir() (string, error) {
	if h := goiHome(); h != "" {
		return filepath.Join(h, "cache"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("paths: user cache dir: %w", err)
	}
	return filepath.Join(d, "Goi", Version), nil
}

// goiHome returns $GOI_HOME made absolute (relative values, as in a run
// configuration, resolve against the working directory). Tools such as
// `go install` reject relative paths derived from it.
func goiHome() string {
	h := os.Getenv("GOI_HOME")
	if h == "" {
		return ""
	}
	if abs, err := filepath.Abs(h); err == nil {
		return abs
	}
	return h
}

// LogsDir returns the directory holding goi.log.
func LogsDir() (string, error) {
	c, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "logs"), nil
}

// ConfigVersion returns `version:` inside the top-level `info:` block of a
// Wails build config (build/config.yml), or "" if there is none. It reads the
// file the same way scripts/version.sh does.
func ConfigVersion(cfg []byte) string {
	inInfo := false
	sc := bufio.NewScanner(bytes.NewReader(cfg))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			inInfo = strings.TrimSpace(line) == "info:"
			continue
		}
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "version:")
		if !inInfo || !ok {
			continue
		}
		if i := strings.Index(v, " #"); i >= 0 {
			v = v[:i]
		}
		return strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return ""
}
