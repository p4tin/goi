package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/paths"
	"github.com/p4tin/goi/internal/recent"
)

// start performs startup with an explicit bus (tests use a Recorder) and
// creates the first window, which delivers its events through the same bus.
func (a *App) start(ctx context.Context, bus events.Bus, handle windowHandle) error {
	return a.startWith(ctx, bus, handle, bus)
}

// startWith performs startup: it loads the shared stores and opens the first
// window (handle/winBus) on the startup target. A nil winBus (server mode)
// defers the first window until the first browser client calls in.
func (a *App) startWith(ctx context.Context, appBus events.Bus, handle windowHandle, winBus events.Bus) error {
	a.ctx, a.appBus = ctx, appBus
	a.log.Info("startup", "version", paths.Version, "build", paths.BuildVersion, "os", runtime.GOOS, "arch", runtime.GOARCH, "project", a.opts.ProjectPath)

	var err error
	if a.recent, err = recent.Load(filepath.Join(a.configDir, "recent.json")); err != nil {
		return fmt.Errorf("recent: %w", err)
	}
	if winBus == nil {
		// Server mode: OpenFile works from now on; the first tab opens the
		// startup project.
		a.startedUp.Store(true)
		return nil
	}
	w, err := a.addWindow(handle, winBus)
	if err != nil {
		return err
	}
	a.openStartup(w)
	return nil
}

// openStartup opens the startup projects: the command-line target, else the
// projects open when Goi last quit (startup.reopenLastProject). The first goes
// into w, the others into windows of their own. Then it opens queued files.
func (a *App) openStartup(w *window) {
	var targets []string
	if t := a.opts.ProjectPath; t != "" {
		targets = []string{t}
	} else if len(a.opts.Files) > 0 {
		targets = []string{ProjectRootFor(a.opts.Files[0])}
	} else if w.settings.Effective().Startup.ReopenLastProject {
		targets = a.reopenTargets()
	}
	for i, t := range targets {
		var err error
		if i == 0 {
			err = w.openProject(t)
		} else {
			err = a.openWindow(t)
		}
		if err != nil {
			a.log.Warn("open project at startup", "path", t, "err", err)
			w.notify("error", "Cannot open project", err.Error())
		}
	}
	w.ready.Store(true)
	a.startedUp.Store(true)
	for _, f := range append(a.opts.Files, a.drainPendingFiles()...) {
		a.OpenFile(f)
	}
}

// reopenTargets returns the existing projects that were open at the last
// quit, falling back to the most recent project.
func (a *App) reopenTargets() []string {
	var out []string
	for _, p := range a.recent.OpenSet() {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		if list, err := a.recent.List(); err == nil && len(list) > 0 && list[0].Exists {
			out = []string{list[0].Path}
		}
	}
	return out
}

// normalizeProject returns the absolute, symlink-resolved project folder.
func normalizeProject(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("open project: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("open project: %s is not a folder", abs)
	}
	return abs, nil
}

// windowWithProject returns the window that has root open.
func (a *App) windowWithProject(root string) *window {
	real, _ := filepath.EvalSymlinks(root)
	for _, w := range a.allWindows() {
		s, err := w.current()
		if err != nil {
			continue
		}
		if s.root == root {
			return w
		}
		if r, err := filepath.EvalSymlinks(s.root); err == nil && real != "" && r == real {
			return w
		}
	}
	return nil
}

// openProjectWindow shows path: it focuses the window that has it open,
// opens it in from when from shows Welcome, or opens a new window.
func (a *App) openProjectWindow(path string, from *window) error {
	abs, err := normalizeProject(path)
	if err != nil {
		return err
	}
	if w := a.windowWithProject(abs); w != nil {
		w.focus()
		return nil
	}
	if from != nil {
		if _, err := from.current(); errors.Is(err, errNoProject) {
			err := from.openProject(abs)
			a.refreshDock()
			return err
		}
	}
	return a.openWindow(abs)
}

// openWindow opens a new window on project ("" for Welcome). The project is
// opened before the window reports ready, so its frontend boots into it.
func (a *App) openWindow(project string) error {
	if a.makeWindow == nil {
		return errNoWindow
	}
	h, bus := a.makeWindow()
	w, err := a.addWindow(h, bus)
	if err != nil {
		return err
	}
	if project != "" {
		err = w.openProject(project)
	}
	w.ready.Store(true)
	w.focus()
	a.reapplyMenu()
	a.refreshDock()
	return err
}

// windowClosed runs after a window's close was confirmed: it releases the
// window and continues a pending quit. On Windows and Linux closing the last
// window quits.
func (a *App) windowClosed(w *window) {
	a.removeWindow(w)
	a.refreshDock()
	if a.quitRequested.Load() {
		a.closeNextForQuit()
		return
	}
	if !isMac() && len(a.allWindows()) == 0 {
		a.quitNow()
	}
}

// shouldQuit is the Wails ShouldQuit hook. The first request saves the open
// projects and closes the windows one at a time through their close flow;
// the app quits when none are left. Cancelling any close stops the quit.
func (a *App) shouldQuit() bool {
	if a.quitting.Load() || len(a.allWindows()) == 0 {
		return true
	}
	if !a.quitRequested.Swap(true) {
		a.saveOpenSet()
	}
	a.closeNextForQuit()
	return false
}

// closeNextForQuit asks the focused window to close, or quits when no window
// is left.
func (a *App) closeNextForQuit() {
	w := a.focusedWindow()
	if w == nil {
		a.quitNow()
		return
	}
	w.requestClose()
}

// cancelQuit stops a pending quit (the user kept a window open).
func (a *App) cancelQuit() { a.quitRequested.Store(false) }

func (a *App) quitNow() {
	a.quitting.Store(true)
	if a.quitFn != nil {
		a.quitFn()
	}
}

// saveOpenSet records the projects open now, for reopening at next startup.
func (a *App) saveOpenSet() {
	var open []string
	for _, w := range a.allWindows() {
		if s, err := w.current(); err == nil {
			open = append(open, s.root)
		}
	}
	if err := a.recent.SetOpenSet(open); err != nil {
		a.log.Warn("save open projects", "err", err)
	}
}

// secondInstance handles a later launch of Goi (CLI, Finder, file
// association): its arguments open projects or files in this process.
func (a *App) secondInstance(args []string, cwd string) {
	if len(args) > 0 {
		args = args[1:] // argv[0]
	}
	opts, err := parseArgsIn(args, cwd)
	if err != nil {
		a.log.Warn("second instance args", "args", args, "err", err)
	}
	a.log.Info("second instance", "project", opts.ProjectPath, "files", len(opts.Files))
	switch {
	case opts.ProjectPath != "":
		if err := a.openProjectWindow(opts.ProjectPath, a.welcomeWindow()); err != nil {
			a.log.Warn("open project from second instance", "err", err)
		}
	case len(opts.Files) > 0:
	default:
		if w := a.focusedWindow(); w != nil {
			w.focus()
		} else if err := a.openWindow(""); err != nil {
			a.log.Warn("open window", "err", err)
		}
	}
	for _, f := range opts.Files {
		a.OpenFile(f)
	}
}

// welcomeWindow returns a window without a project, if any.
func (a *App) welcomeWindow() *window {
	for _, w := range a.allWindows() {
		if _, err := w.current(); errors.Is(err, errNoProject) {
			return w
		}
	}
	return nil
}

// reopen handles a click on the Dock icon: with no windows, open Welcome.
func (a *App) reopen() {
	if len(a.allWindows()) > 0 {
		return
	}
	if err := a.openWindow(""); err != nil {
		a.log.Warn("reopen", "err", err)
	}
}
