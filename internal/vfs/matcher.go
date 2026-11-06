package vfs

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher reports whether a root-relative, slash-separated path is excluded.
type Matcher interface {
	Match(rel string, isDir bool) bool
}

type matchNone struct{}

func (matchNone) Match(string, bool) bool { return false }

// MatchNone excludes nothing.
var MatchNone Matcher = matchNone{}

type rule struct {
	pattern  string
	anchored bool // contains "/": matched against path prefixes from the root
	dirOnly  bool // trailing "/": only directories match
}

type globMatcher struct {
	rules []rule
}

// NewMatcher builds a gitignore-style Matcher. A glob without "/" matches any
// single path segment at any depth; a glob containing "/" is anchored at the
// root. A trailing "/" restricts a glob to directories. When a directory
// matches, every path beneath it matches too.
func NewMatcher(globs []string) (Matcher, error) {
	m := &globMatcher{}
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		r := rule{}
		if strings.HasSuffix(g, "/") {
			r.dirOnly = true
			g = strings.TrimRight(g, "/")
		}
		if strings.Contains(g, "/") {
			r.anchored = true
			g = strings.TrimPrefix(g, "/")
		}
		if !doublestar.ValidatePattern(g) {
			return nil, fmt.Errorf("vfs: invalid exclude pattern %q", g)
		}
		r.pattern = g
		m.rules = append(m.rules, r)
	}
	if len(m.rules) == 0 {
		return MatchNone, nil
	}
	return m, nil
}

func (m *globMatcher) Match(rel string, isDir bool) bool {
	rel = strings.Trim(rel, "/")
	if rel == "" || rel == "." {
		return false
	}
	segs := strings.Split(rel, "/")
	for _, r := range m.rules {
		for i := range segs {
			last := i == len(segs)-1
			// Every proper prefix is a directory; the full path is a dir only if isDir.
			if r.dirOnly && last && !isDir {
				continue
			}
			var subject string
			if r.anchored {
				subject = strings.Join(segs[:i+1], "/")
			} else {
				subject = segs[i]
			}
			if ok, _ := doublestar.Match(r.pattern, subject); ok {
				return true
			}
		}
	}
	return false
}
