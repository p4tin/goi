package rpc

import (
	"os"
	"path/filepath"

	"github.com/p4tin/goi/internal/instance"
	"github.com/p4tin/goi/internal/workspace"
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

// OpenFile handles an OS request to open a file (macOS file association or
// dock drop). Before startup it is queued. With no project open, the file's
// project is opened here; a file outside this window's project opens its
// project in another window.
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
	sess, err := a.current()
	if err != nil {
		if err := a.openProject(ProjectRootFor(abs)); err != nil {
			a.log.Warn("open file", "path", abs, "err", err)
			a.notify("error", "Cannot open file", err.Error())
			return
		}
		sess, _ = a.current()
	}
	if sess == nil {
		return
	}
	if _, ok := sess.contains(abs); ok {
		a.bus.Emit(TopicOpenFile, OpenFileEvent{Path: abs})
		return
	}
	root := ProjectRootFor(abs)
	if err := instance.Focus(a.cacheDir, workspace.Hash(root)); err == nil {
		return
	}
	if err := instance.LaunchNewWindow(root); err != nil {
		a.log.Warn("open file in new window", "path", abs, "err", err)
	}
}

func (a *App) drainPendingFiles() []string {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	files := a.pendingFiles
	a.pendingFiles = nil
	return files
}
