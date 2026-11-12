package rpc

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/p4tin/goi/internal/dockmenu"
)

// maxDockRecent is how many recent projects the Dock menu's Open Recent lists.
const maxDockRecent = 10

// appStarted runs once the native application is up: it installs the macOS
// Dock menu.
func (a *App) appStarted() {
	if serverMode {
		return
	}
	if err := dockmenu.Install(); err != nil {
		a.log.Warn("dock menu", "err", err)
		return
	}
	dockmenu.OnSelect(a.dockSelect)
	a.refreshDock()
}

// refreshDock rebuilds the Dock menu: the open windows (the focused one
// checked), New Window and Open Recent.
func (a *App) refreshDock() {
	if a.recent == nil {
		return
	}
	dockmenu.Set(a.dockItems())
}

func (a *App) dockItems() []dockmenu.Item {
	focused := a.focusedWindow()
	var items []dockmenu.Item
	for _, w := range a.allWindows() {
		title := "Welcome"
		if s, err := w.current(); err == nil {
			title = s.manager.Project().Name
		}
		items = append(items, dockmenu.Item{ID: "win:" + strconv.FormatUint(uint64(w.id), 10), Title: title, Checked: w == focused})
	}
	if len(items) > 0 {
		items = append(items, dockmenu.Item{Separator: true})
	}
	items = append(items, dockmenu.Item{ID: "new", Title: "New Window"})
	if list, err := a.recent.List(); err == nil && len(list) > 0 {
		var recent []dockmenu.Item
		for _, e := range list {
			if !e.Exists {
				continue
			}
			name := e.Name
			if name == "" {
				name = filepath.Base(e.Path)
			}
			recent = append(recent, dockmenu.Item{ID: "recent:" + e.Path, Title: name})
			if len(recent) == maxDockRecent {
				break
			}
		}
		if len(recent) > 0 {
			items = append(items, dockmenu.Item{ID: "recent", Title: "Open Recent", Children: recent})
		}
	}
	return items
}

// dockSelect runs a Dock menu choice.
func (a *App) dockSelect(id string) {
	switch {
	case id == "new":
		if err := a.openWindow(""); err != nil {
			a.log.Warn("dock: new window", "err", err)
		}
	case strings.HasPrefix(id, "win:"):
		n, _ := strconv.ParseUint(strings.TrimPrefix(id, "win:"), 10, 64)
		a.winMu.Lock()
		w := a.windows[uint(n)]
		a.winMu.Unlock()
		if w != nil {
			w.focus()
		}
	case strings.HasPrefix(id, "recent:"):
		if err := a.openProjectWindow(strings.TrimPrefix(id, "recent:"), a.welcomeWindow()); err != nil {
			a.log.Warn("dock: open recent", "err", err)
		}
	}
}
