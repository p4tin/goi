package rpc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/index"
	"github.com/p4tin/goi/internal/textsearch"
	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

// searchIndex keeps the session's file index current: built in the
// background on open, updated from watcher events, rebuilt when content
// roots or excludes change.
type searchIndex struct {
	app *App

	mu       sync.RWMutex
	ix       *index.Index
	roots    []string
	excludes []string
	cancel   context.CancelFunc
	gen      int

	cancels []func()
}

// IndexStatus is the payload of index.ready / index.updated.
type IndexStatus struct {
	Ready bool `json:"ready"`
	Count int  `json:"count"`
}

func startSearchIndex(a *App, m *workspace.Manager, w vfs.Watcher) *searchIndex {
	s := &searchIndex{app: a}
	s.rebuild(m.Project(), m.Matcher())
	s.cancels = append(s.cancels,
		w.Subscribe(s.apply),
		m.Subscribe(func(p workspace.Project) {
			s.mu.RLock()
			same := slices.Equal(p.ContentRoots, s.roots) && slices.Equal(p.Excludes, s.excludes)
			s.mu.RUnlock()
			if !same {
				s.rebuild(p, m.Matcher())
			}
		}),
	)
	return s
}

func (s *searchIndex) rebuild(p workspace.Project, matcher vfs.Matcher) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.gen++
	gen := s.gen
	s.roots = slices.Clone(p.ContentRoots)
	s.excludes = slices.Clone(p.Excludes)
	s.mu.Unlock()

	go func() {
		ix, err := index.Build(ctx, p.ContentRoots, matcher)
		if err != nil {
			if ctx.Err() == nil {
				s.app.log.Warn("index build", "err", err)
			}
			return
		}
		s.mu.Lock()
		if gen != s.gen {
			s.mu.Unlock()
			return
		}
		s.ix = ix
		s.mu.Unlock()
		s.app.log.Info("index ready", "files", ix.Len())
		s.app.bus.Emit(events.TopicIndexReady, IndexStatus{Ready: true, Count: ix.Len()})
	}()
}

func (s *searchIndex) apply(evs []vfs.Event) {
	s.mu.RLock()
	ix := s.ix
	s.mu.RUnlock()
	if ix == nil {
		return
	}
	for _, e := range evs {
		switch e.Op {
		case vfs.OpCreate, vfs.OpWrite:
			if !e.IsDir {
				ix.Add(e.Path)
			}
		case vfs.OpRemove, vfs.OpRename:
			ix.Remove(e.Path)
		}
	}
	s.app.bus.Emit(events.TopicIndexUpdated, IndexStatus{Ready: true, Count: ix.Len()})
}

func (s *searchIndex) search(query string, limit int, recent []string) ([]index.Match, IndexStatus) {
	s.mu.RLock()
	ix := s.ix
	s.mu.RUnlock()
	if ix == nil {
		return []index.Match{}, IndexStatus{}
	}
	return ix.Search(query, limit, recent), IndexStatus{Ready: true, Count: ix.Len()}
}

func (s *searchIndex) paths() ([]string, bool) {
	s.mu.RLock()
	ix := s.ix
	s.mu.RUnlock()
	if ix == nil {
		return nil, false
	}
	return ix.Paths(), true
}

func (s *searchIndex) close() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.gen++
	cancels := s.cancels
	s.cancels = nil
	s.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// SearchService searches project files.
type SearchService struct {
	app *App
}

// FileSearchResult is a ranked list of files plus the index state.
type FileSearchResult struct {
	Matches []index.Match `json:"matches"`
	Index   IndexStatus   `json:"index"`
}

// Files fuzzy-matches query against project file paths. recent lists
// absolute paths, most recent first, to boost; an empty query returns them.
func (s *SearchService) Files(query string, limit int, recent []string) (FileSearchResult, error) {
	sess, err := s.app.current()
	if err != nil {
		return FileSearchResult{Matches: []index.Match{}}, err
	}
	m, st := sess.search.search(query, limit, recent)
	if m == nil {
		m = []index.Match{}
	}
	return FileSearchResult{Matches: m, Index: st}, nil
}

// MaxFindResults caps Find in Path results.
const MaxFindResults = 2000

// BufferText is the unsaved content of an open editor buffer.
type BufferText struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// FindQuery is a Find/Replace in Path request.
type FindQuery struct {
	Pattern       string `json:"pattern"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	Mask          string `json:"mask"`
	Replace       string `json:"replace"`
	// Scope limits the search to a directory ("" = whole project).
	Scope string `json:"scope"`
	// Buffers override file contents with unsaved editor text.
	Buffers []BufferText `json:"buffers"`
}

func (q FindQuery) query() textsearch.Query {
	return textsearch.Query{Pattern: q.Pattern, Regex: q.Regex, CaseSensitive: q.CaseSensitive, WholeWord: q.WholeWord, Mask: q.Mask, Replace: q.Replace}
}

// FindResult lists matches in path order.
type FindResult struct {
	Matches   []textsearch.Match `json:"matches"`
	Truncated bool               `json:"truncated"`
}

var errIndexNotReady = errors.New("the file index is still being built; try again in a moment")

// FindInPath searches project files (honoring excludes) for q.
func (s *SearchService) FindInPath(q FindQuery) (FindResult, error) {
	empty := FindResult{Matches: []textsearch.Match{}}
	sess, err := s.app.current()
	if err != nil {
		return empty, err
	}
	all, ok := sess.search.paths()
	if !ok {
		return empty, errIndexNotReady
	}
	paths := all[:0:0]
	for _, p := range all {
		if q.Scope == "" || within(q.Scope, p) {
			paths = append(paths, p)
		}
	}
	buffers := map[string]string{}
	for _, b := range q.Buffers {
		buffers[b.Path] = b.Text
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m, truncated, err := textsearch.SearchWith(ctx, paths, q.query(), MaxFindResults, buffers)
	if m == nil {
		m = []textsearch.Match{}
	}
	return FindResult{Matches: m, Truncated: truncated}, err
}

// ReplaceInPath replaces q's matches in paths (files not open in the
// editor; the frontend edits open buffers itself). It returns the number of
// replacements.
func (s *SearchService) ReplaceInPath(q FindQuery, paths []string) (int, error) {
	sess, err := s.app.current()
	if err != nil {
		return 0, err
	}
	tq := q.query()
	re, err := textsearch.Compile(tq)
	if err != nil {
		return 0, err
	}
	tmpl := textsearch.Template(tq)
	total := 0
	var errs []error
	for _, p := range paths {
		if _, ok := sess.contains(p); !ok {
			errs = append(errs, fmt.Errorf("%s is outside the project", p))
			continue
		}
		meta, text, err := vfs.ReadText(p, vfs.Limits{})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if meta.Binary {
			continue
		}
		out, n := textsearch.ReplaceText(text, re, tmpl)
		if n == 0 {
			continue
		}
		if _, err := vfs.WriteText(p, out, meta); err != nil {
			errs = append(errs, err)
			continue
		}
		total += n
	}
	return total, errors.Join(errs...)
}
