package rpc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/instance"
	"github.com/p4tin/goi/internal/journal"
	"github.com/p4tin/goi/internal/recent"
	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

// session is the state of the project open in a window.
type session struct {
	root    string
	manager *workspace.Manager
	watcher vfs.Watcher
	journal *journal.Journal
	lock    *instance.Lock
	crashed bool
	app     *App
	win     *window // the window that owns the project; its bus carries the project's events
	search  *searchIndex
	lang    *langSession
	runs    *runManager
	debug   *debugManager
	terms   *termSession
	git     *gitSession
	lint    *lintSession
	mod     *modSession
}

// startLangHook starts gopls for a new session; tests set it to nil.
var startLangHook = startLang

func (w *window) openProject(path string) error {
	a := w.app
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("open project: %s is not a folder", abs)
	}

	w.mu.Lock()
	open := w.session
	w.mu.Unlock()
	if open != nil {
		if open.root == abs {
			return nil
		}
		return fmt.Errorf("open project: %s is already open in this window", open.root)
	}

	lock, err := w.acquireInstance(abs)
	if err != nil {
		return err
	}
	opened := false
	defer func() {
		if !opened {
			lock.Release()
		}
	}()

	if err := w.settings.SetProjectPath(workspace.SettingsPath(abs)); err != nil {
		a.log.Warn("project settings", "err", err)
	}
	excludes := w.settings.Effective().Project.Excludes
	matcher, err := vfs.NewMatcher(excludes)
	if err != nil {
		matcher = vfs.MatchNone
	}
	fw, err := vfs.NewWatcher([]string{abs}, matcher, w.bus)
	if err != nil {
		return fmt.Errorf("open project: watcher: %w", err)
	}
	m, err := workspace.Open(a.ctx, workspace.Options{
		Root:     abs,
		GOPATH:   filepath.SplitList(os.Getenv("GOPATH")),
		Settings: w.settings,
		Watcher:  fw,
		Bus:      w.bus,
		Log:      a.log,
	})
	if err != nil {
		fw.Close()
		w.settings.SetProjectPath("")
		return fmt.Errorf("open project: %w", err)
	}
	p := m.Project()
	j, crashed, err := journal.Open(a.cacheDir, p.Hash)
	if errors.Is(err, journal.ErrInUse) {
		m.Close()
		fw.Close()
		w.settings.SetProjectPath("")
		return fmt.Errorf("open project: %s is already open in another Goi window", abs)
	}
	if err != nil {
		a.log.Warn("journal", "err", err)
	}

	s := &session{root: abs, manager: m, watcher: fw, journal: j, lock: lock, crashed: crashed, app: a, win: w}
	s.search = startSearchIndex(w, m, fw)
	s.runs = startRuns(w, s, fw)
	s.debug = newDebugManager(w)
	s.terms = newTermSession(w)
	s.git = startGit(w, abs, fw)
	s.lint = newLintSession()
	s.mod = &modSession{}
	if startLangHook != nil {
		s.lang = startLangHook(w, abs, fw)
	}
	opened = true
	w.mu.Lock()
	w.session = s
	w.mu.Unlock()

	if err := a.recent.Touch(recent.Entry{Path: abs, Name: p.Name, Kind: string(p.Kind)}); err != nil {
		a.log.Warn("recent", "err", err)
	} else if list, err := a.recent.List(); err == nil {
		a.appBus.Emit(events.TopicRecentChanged, list)
	}
	a.log.Info("project opened", "root", abs, "kind", p.Kind, "crashed", crashed)
	w.setTitle("Goi - " + p.Name)
	w.bus.Emit(events.TopicWorkspaceChanged, p)
	return nil
}

func (w *window) closeProject() error {
	w.mu.Lock()
	s := w.session
	w.session = nil
	w.mu.Unlock()
	if s == nil {
		return errNoProject
	}
	s.close(true)
	if err := w.settings.SetProjectPath(""); err != nil {
		w.app.log.Warn("clear project settings", "err", err)
	}
	w.setTitle("Goi")
	w.bus.Emit(events.TopicWorkspaceChanged, nil)
	return nil
}

// close releases the session. clean=true removes the crash journal.
func (s *session) close(clean bool) {
	if s.lint != nil {
		s.lint.close()
	}
	if s.git != nil {
		s.git.close()
	}
	if s.terms != nil {
		s.terms.close()
	}
	if s.debug != nil {
		s.debug.close()
	}
	if s.runs != nil {
		s.runs.close()
	}
	if s.lang != nil {
		s.lang.close()
	}
	if s.search != nil {
		s.search.close()
	}
	s.manager.Close()
	s.watcher.Close()
	if s.journal != nil && clean {
		if err := s.journal.CloseClean(); err != nil {
			s.app.log.Warn("journal close", "err", err)
		}
	}
	s.lock.Release()
}

// contains reports whether path lies inside one of the session's content
// roots, returning that root.
func (s *session) contains(path string) (string, bool) {
	for _, r := range s.manager.Project().ContentRoots {
		if within(r, path) {
			return r, true
		}
	}
	return "", false
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (a *App) layoutPath(s *session) string {
	return filepath.Join(a.configDir, "workspaces", s.manager.Project().Hash, "layout.json")
}

func writeFileAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".goi-tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func readFileOrEmpty(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
