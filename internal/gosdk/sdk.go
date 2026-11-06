package gosdk

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sources of a discovered SDK.
const (
	SourcePath      = "path"      // the go binary found on $PATH
	SourceStandard  = "standard"  // a well-known install location
	SourceToolchain = "toolchain" // a golang.org/toolchain module in the module cache
	SourceSDK       = "sdk"       // ~/sdk/goX.Y.Z (golang.org/dl)
	SourceManual    = "manual"    // added by the user
)

// SDK is a Go installation.
type SDK struct {
	GOROOT  string `json:"goroot"`
	Version string `json:"version"`
	GoBin   string `json:"goBin"`
	Source  string `json:"source"`
}

// DiscoverOptions controls where Discover looks. Zero-valued fields are
// skipped; use DefaultDiscoverOptions for the real locations.
type DiscoverOptions struct {
	// LookPath resolves the go binary on $PATH (exec.LookPath).
	LookPath func(file string) (string, error)
	// Home expands a leading "~/" in Globs.
	Home string
	// Roots are fixed GOROOT candidates.
	Roots []string
	// Globs are glob patterns matching GOROOT directories.
	Globs []string
}

// DefaultDiscoverOptions returns the standard search locations for this OS.
func DefaultDiscoverOptions() DiscoverOptions {
	home, _ := os.UserHomeDir()
	return DiscoverOptions{
		LookPath: exec.LookPath,
		Home:     home,
		Roots: []string{
			"/usr/local/go",
			"/opt/homebrew/opt/go/libexec",
			"/usr/local/opt/go/libexec",
			"/usr/lib/go",
			"/usr/local/lib/go",
			`C:\Program Files\Go`,
			`C:\Go`,
		},
		Globs: []string{
			"~/sdk/go*",
			"~/go/pkg/mod/golang.org/toolchain@*",
		},
	}
}

// Discover returns the valid SDKs found in the default locations.
func Discover(ctx context.Context) []SDK {
	return DiscoverWith(ctx, DefaultDiscoverOptions())
}

// DiscoverWith returns valid SDKs from opts, deduplicated by real path (first
// source wins). The $PATH SDK sorts first, the rest by version descending.
func DiscoverWith(ctx context.Context, opts DiscoverOptions) []SDK {
	type cand struct{ root, source string }
	var cands []cand
	if opts.LookPath != nil {
		if bin, err := opts.LookPath("go"); err == nil {
			if root := gorootOf(ctx, bin); root != "" {
				cands = append(cands, cand{root, SourcePath})
			}
		}
	}
	for _, r := range opts.Roots {
		cands = append(cands, cand{r, SourceStandard})
	}
	for _, g := range opts.Globs {
		if strings.HasPrefix(g, "~/") {
			if opts.Home == "" {
				continue
			}
			g = filepath.Join(opts.Home, g[2:])
		}
		matches, _ := filepath.Glob(g)
		src := SourceSDK
		if strings.Contains(g, "toolchain@") {
			src = SourceToolchain
		}
		for _, m := range matches {
			cands = append(cands, cand{m, src})
		}
	}

	seen := map[string]bool{}
	var out []SDK
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		real, err := filepath.EvalSymlinks(c.root)
		if err != nil || seen[real] {
			continue
		}
		sdk, err := Validate(c.root)
		if err != nil {
			continue
		}
		seen[real] = true
		sdk.Source = c.source
		out = append(out, sdk)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := out[i].Source == SourcePath, out[j].Source == SourcePath
		if pi != pj {
			return pi
		}
		return CompareVersions(out[i].Version, out[j].Version) > 0
	})
	return out
}

// goEnvGOROOT runs `<bin> env GOROOT`. Replaced in tests.
var goEnvGOROOT = func(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gorootOf finds the GOROOT of a go binary: `go env GOROOT`, falling back to
// the parent of the binary's (symlink-resolved) bin directory.
func gorootOf(ctx context.Context, bin string) string {
	if root, err := goEnvGOROOT(ctx, bin); err == nil && root != "" {
		return root
	}
	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return ""
	}
	return filepath.Dir(filepath.Dir(real))
}

func goExe() string {
	if runtime.GOOS == "windows" {
		return "go.exe"
	}
	return "go"
}

// runGoVersion runs `<bin> version` and returns the version token. Replaced in tests.
var runGoVersion = func(bin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return "", err
	}
	// "go version go1.26.0 darwin/arm64"
	f := strings.Fields(string(out))
	if len(f) < 3 || !strings.HasPrefix(f[2], "go") {
		return "", fmt.Errorf("unexpected output %q", strings.TrimSpace(string(out)))
	}
	return f[2], nil
}

type versionKey struct {
	goroot string
	mtime  int64
	size   int64
}

var versionCache sync.Map // versionKey -> string

// Validate checks that goroot contains a go binary and determines its version
// from <goroot>/VERSION, falling back to `go version`.
func Validate(goroot string) (SDK, error) {
	if goroot == "" {
		return SDK{}, fmt.Errorf("gosdk: empty GOROOT")
	}
	abs, err := filepath.Abs(goroot)
	if err != nil {
		return SDK{}, fmt.Errorf("gosdk: %w", err)
	}
	bin := filepath.Join(abs, "bin", goExe())
	st, err := os.Stat(bin)
	if err != nil || !st.Mode().IsRegular() {
		return SDK{}, fmt.Errorf("gosdk: no go binary in %s", filepath.Join(abs, "bin"))
	}
	key := versionKey{abs, st.ModTime().UnixNano(), st.Size()}
	if v, ok := versionCache.Load(key); ok {
		return SDK{GOROOT: abs, Version: v.(string), GoBin: bin}, nil
	}
	v, err := readVersionFile(filepath.Join(abs, "VERSION"))
	if err != nil {
		v, err = runGoVersion(bin)
		if err != nil {
			return SDK{}, fmt.Errorf("gosdk: cannot determine version of %s: %w", abs, err)
		}
	}
	versionCache.Store(key, v)
	return SDK{GOROOT: abs, Version: v, GoBin: bin}, nil
}

func readVersionFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return "", fmt.Errorf("empty VERSION file")
	}
	v := strings.TrimSpace(sc.Text())
	if !strings.HasPrefix(v, "go") {
		return "", fmt.Errorf("unexpected VERSION %q", v)
	}
	return v, nil
}
