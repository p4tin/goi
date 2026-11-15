// Package gomod helps edit go.mod/go.work files: module versions from the
// module proxy, module paths known locally (module cache and go.sum), and
// parsing the module on a go.mod line.
package gomod

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// CacheTTL is how long proxy answers are reused.
const CacheTTL = 10 * time.Minute

// ProxyURLs parses GOPROXY ("https://a,https://b|direct"); "direct" and
// "off" are not URLs and are skipped. "off" first means no network.
func ProxyURLs(goproxy string) []string {
	if goproxy == "" {
		goproxy = "https://proxy.golang.org,direct"
	}
	var out []string
	for _, p := range strings.FieldsFunc(goproxy, func(r rune) bool { return r == ',' || r == '|' }) {
		p = strings.TrimSpace(p)
		switch p {
		case "", "direct":
			continue
		case "off":
			return out
		}
		out = append(out, strings.TrimRight(p, "/"))
	}
	return out
}

// Proxy queries module proxies with caching.
type Proxy struct {
	URLs   []string
	Client *http.Client

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	versions []string
	at       time.Time
}

// NewProxy returns a proxy client for GOPROXY.
func NewProxy(goproxy string) *Proxy {
	return &Proxy{URLs: ProxyURLs(goproxy), Client: &http.Client{Timeout: 5 * time.Second}, cache: map[string]cached{}}
}

// ErrNoProxy means GOPROXY disables network lookups.
var ErrNoProxy = errors.New("gomod: no module proxy (GOPROXY is off or direct)")

// Versions lists released versions of mod, newest first (pseudo-versions
// excluded). An empty list falls back to @latest.
func (p *Proxy) Versions(ctx context.Context, mod string) ([]string, error) {
	if len(p.URLs) == 0 {
		return nil, ErrNoProxy
	}
	p.mu.Lock()
	if c, ok := p.cache[mod]; ok && time.Since(c.at) < CacheTTL {
		p.mu.Unlock()
		return c.versions, nil
	}
	p.mu.Unlock()
	esc, err := module.EscapePath(mod)
	if err != nil {
		return nil, fmt.Errorf("gomod: %w", err)
	}
	var lastErr error
	for _, base := range p.URLs {
		body, err := p.get(ctx, base+"/"+esc+"/@v/list")
		if err != nil {
			lastErr = err
			continue
		}
		vs := SortVersions(strings.Fields(body))
		if len(vs) == 0 {
			if latest, err := p.get(ctx, base+"/"+esc+"/@latest"); err == nil {
				var info struct{ Version string }
				if json.Unmarshal([]byte(latest), &info) == nil && info.Version != "" {
					vs = []string{info.Version}
				}
			}
		}
		p.mu.Lock()
		p.cache[mod] = cached{versions: vs, at: time.Now()}
		p.mu.Unlock()
		return vs, nil
	}
	return nil, lastErr
}

func (p *Proxy) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gomod: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gomod: %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

// SortVersions keeps valid semver (no pseudo-versions), newest first.
func SortVersions(vs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range vs {
		if !semver.IsValid(v) || module.IsPseudoVersion(v) || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return semver.Compare(out[i], out[j]) > 0 })
	return out
}

// KnownModules returns module paths from the module download cache
// (modcache/cache/download) and the given go.sum files, sorted.
func KnownModules(ctx context.Context, modcache string, sums []string) []string {
	set := map[string]bool{}
	if modcache != "" {
		root := filepath.Join(modcache, "cache", "download")
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || ctx.Err() != nil {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				return nil
			}
			if d.Name() == "@v" {
				rel, err := filepath.Rel(root, filepath.Dir(p))
				if err == nil {
					if mod, err := module.UnescapePath(filepath.ToSlash(rel)); err == nil {
						set[mod] = true
					}
				}
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), "sumdb") && filepath.Dir(p) == root {
				return filepath.SkipDir
			}
			return nil
		})
	}
	for _, s := range sums {
		for _, m := range sumModules(s) {
			set[m] = true
		}
	}
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func sumModules(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) >= 2 {
			out = append(out, fields[0])
		}
	}
	return out
}
