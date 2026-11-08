// Package paths resolves Goi's per-user configuration and cache directories.
//
// Directories are version-scoped (Goi/<Version>). Setting GOI_HOME redirects
// both to $GOI_HOME/config and $GOI_HOME/cache, which tests and dev builds use
// to stay isolated from the real user profile.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Version is the major.minor used to scope user data directories.
const Version = "0.1"

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
