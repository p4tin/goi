// Package paths resolves the app's per-user configuration and cache
// directories.
//
// Directories are version-scoped (<DirName>/<DataVersion>, from the product
// identity: Goi/0.1 for Goi). Setting the identity's home variable (GOI_HOME
// for Goi) redirects both to $HOME_VAR/config and $HOME_VAR/cache, which
// tests and dev builds use to stay isolated from the real user profile.
package paths

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/p4tin/goi/internal/product"
)

// BuildVersion is the full product version of a release build, set with
// -ldflags "-X github.com/p4tin/goi/internal/paths.BuildVersion=0.1.0";
// "dev" otherwise.
var BuildVersion = "dev"

// errNoIdentity guards against resolving user directories before main set
// the product identity (which would write into an unnamed directory).
var errNoIdentity = errors.New("paths: product identity not set")

// ConfigDir returns the user config directory, e.g.
// ~/Library/Application Support/Goi/0.1 on macOS.
func ConfigDir() (string, error) {
	id := product.Current()
	if h := home(id); h != "" {
		return filepath.Join(h, "config"), nil
	}
	if id.DirName == "" || id.DataVersion == "" {
		return "", errNoIdentity
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("paths: user config dir: %w", err)
	}
	return filepath.Join(d, id.DirName, id.DataVersion), nil
}

// CacheDir returns the user cache directory, e.g. ~/Library/Caches/Goi/0.1.
func CacheDir() (string, error) {
	id := product.Current()
	if h := home(id); h != "" {
		return filepath.Join(h, "cache"), nil
	}
	if id.DirName == "" || id.DataVersion == "" {
		return "", errNoIdentity
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("paths: user cache dir: %w", err)
	}
	return filepath.Join(d, id.DirName, id.DataVersion), nil
}

// home returns the identity's home override ($GOI_HOME for Goi) made
// absolute (relative values, as in a run configuration, resolve against the
// working directory). Tools such as `go install` reject relative paths
// derived from it.
func home(id product.Identity) string {
	if id.HomeEnv == "" {
		return ""
	}
	h := os.Getenv(id.HomeEnv)
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
