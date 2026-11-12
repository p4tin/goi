package rpc

import (
	"os"
	"path/filepath"

)

// TopicOpenFile asks the frontend to open a file in the editor.
const TopicOpenFile = "app.openFile"

// OpenFileEvent is the payload of TopicOpenFile.
type OpenFileEvent struct {
	Path string `json:"path"`
}

// ProjectRootFor returns the folder to open as a project for file: the
// nearest ancestor holding go.work, else go.mod, else the file's directory.
func ProjectRootFor(file string) string {
	dir := filepath.Dir(file)
	var mod string
	for d := dir; ; d = filepath.Dir(d) {
		if isFile(filepath.Join(d, "go.work")) {
			return d
		}
		if mod == "" && isFile(filepath.Join(d, "go.mod")) {
			mod = d
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	if mod != "" {
		return mod
	}
	return dir
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// OpenFile handles an OS request to open a file (file association, Dock
// drop, a later launch with a file argument). Before startup it is queued.
// The file opens in the window whose project contains it; otherwise its
// project opens (in a Welcome window if there is one, else a new window).
func (a *App) OpenFile(path string) {
	if !a.startedUp.Load() {
		a.pendingMu.Lock()
		a.pendingFiles = append(a.pendingFiles, path)
		a.pendingMu.Unlock()
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	w := a.windowContaining(abs)
	if w == nil {
		root := ProjectRootFor(abs)
		if err := a.openProjectWindow(root, a.welcomeWindow()); err != nil {
			a.log.Warn("open file", "path", abs, "err", err)
			if fw := a.focusedWindow(); fw != nil {
				fw.notify("error", "Cannot open file", err.Error())
			}
			return
		}
		if w = a.windowContaining(abs); w == nil {
			return
		}
	}
	w.focus()
	w.bus.Emit(TopicOpenFile, OpenFileEvent{Path: abs})
}

// windowContaining returns the window whose project contains path.
func (a *App) windowContaining(path string) *window {
	for _, w := range a.allWindows() {
		if s, err := w.current(); err == nil {
			if _, ok := s.contains(path); ok {
				return w
			}
		}
	}
	return nil
}

func (a *App) drainPendingFiles() []string {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	files := a.pendingFiles
	a.pendingFiles = nil
	return files
}
