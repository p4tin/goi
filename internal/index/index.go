package index

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/p4tin/goi/internal/vfs"
)

type entry struct {
	abs       string
	rel       string // slash-separated, relative to its root
	lower     string // ASCII-lowercased rel (same byte length)
	baseStart int    // byte offset of the basename in rel
	root      int
}

// Index is an in-memory list of the files under a set of roots. It is safe
// for concurrent use.
type Index struct {
	mu      sync.RWMutex
	roots   []string
	m       vfs.Matcher
	entries []entry
	pos     map[string]int // abs -> index into entries
}

// Match is one search result. Positions are byte offsets into Rel of the
// matched query characters.
type Match struct {
	Path      string `json:"path"`
	Rel       string `json:"rel"`
	Root      string `json:"root"`
	Score     int    `json:"score"`
	Positions []int  `json:"positions"`
}

// Build walks roots, skipping paths excluded by m, and indexes every file.
func Build(ctx context.Context, roots []string, m vfs.Matcher) (*Index, error) {
	if m == nil {
		m = vfs.MatchNone
	}
	ix := &Index{roots: append([]string(nil), roots...), m: m, pos: map[string]int{}}
	for ri, root := range roots {
		n := 0
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if p == root {
					return err
				}
				return nil // unreadable subtree: skip it
			}
			if n++; n%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if p == root {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if m.Match(rel, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() {
				ix.add(ri, p, rel)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("index: walk %s: %w", root, err)
		}
	}
	return ix, nil
}

func (ix *Index) add(root int, abs, rel string) {
	if _, ok := ix.pos[abs]; ok {
		return
	}
	ix.pos[abs] = len(ix.entries)
	ix.entries = append(ix.entries, entry{
		abs:       abs,
		rel:       rel,
		lower:     asciiLower(rel),
		baseStart: strings.LastIndexByte(rel, '/') + 1,
		root:      root,
	})
}

// Len returns the number of indexed files.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.entries)
}

// Paths returns the absolute paths of all indexed files, sorted.
func (ix *Index) Paths() []string {
	ix.mu.RLock()
	out := make([]string, len(ix.entries))
	for i, e := range ix.entries {
		out[i] = e.abs
	}
	ix.mu.RUnlock()
	sort.Strings(out)
	return out
}

// Add indexes a file if it lies under a root and isn't excluded. Paths that
// are directories, missing, or already indexed are ignored.
func (ix *Index) Add(abs string) {
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ri, rel := ix.locate(abs)
	if ri < 0 || ix.m.Match(rel, false) || excludedAncestor(ix.m, rel) {
		return
	}
	ix.add(ri, abs, rel)
}

// Remove drops abs and, if it was a directory, everything under it.
func (ix *Index) Remove(abs string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	prefix := abs + string(filepath.Separator)
	for i := 0; i < len(ix.entries); {
		e := ix.entries[i]
		if e.abs == abs || strings.HasPrefix(e.abs, prefix) {
			last := len(ix.entries) - 1
			ix.entries[i] = ix.entries[last]
			ix.pos[ix.entries[i].abs] = i
			ix.entries = ix.entries[:last]
			delete(ix.pos, e.abs)
			continue
		}
		i++
	}
}

// locate returns the index of the deepest root containing abs and the
// slash-separated path relative to it, or -1.
func (ix *Index) locate(abs string) (int, string) {
	best, bestLen, bestRel := -1, -1, ""
	for i, r := range ix.roots {
		rel, err := filepath.Rel(r, abs)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		if len(r) > bestLen {
			best, bestLen, bestRel = i, len(r), filepath.ToSlash(rel)
		}
	}
	return best, bestRel
}

func excludedAncestor(m vfs.Matcher, rel string) bool {
	for i := strings.IndexByte(rel, '/'); i >= 0; {
		if m.Match(rel[:i], true) {
			return true
		}
		j := strings.IndexByte(rel[i+1:], '/')
		if j < 0 {
			break
		}
		i += j + 1
	}
	return false
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func (ix *Index) match(e entry, score int, pos []int) Match {
	return Match{Path: e.abs, Rel: e.rel, Root: ix.roots[e.root], Score: score, Positions: pos}
}

func sortMatches(ms []Match) {
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].Score != ms[j].Score {
			return ms[i].Score > ms[j].Score
		}
		if len(ms[i].Rel) != len(ms[j].Rel) {
			return len(ms[i].Rel) < len(ms[j].Rel)
		}
		return ms[i].Rel < ms[j].Rel
	})
}
