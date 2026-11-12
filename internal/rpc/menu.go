package rpc

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TopicMenuCommand asks the frontend to run a command chosen from the
// native menu bar.
const TopicMenuCommand = "menu.command"

// MenuCommandEvent is the payload of TopicMenuCommand.
type MenuCommandEvent struct {
	Command string `json:"command"`
}

// MenuItemSpec is one entry of a menu built by the frontend. An item with
// Separator set ignores the other fields.
type MenuItemSpec struct {
	Label     string `json:"label"`
	Command   string `json:"command"`
	Accel     string `json:"accel"` // Wails accelerator, e.g. "cmdorctrl+shift+1"; may be empty
	Separator bool   `json:"separator"`
}

// MenuSpec is a top-level menu (File, View, …).
type MenuSpec struct {
	Label string         `json:"label"`
	Items []MenuItemSpec `json:"items"`
}

// MenuService lets the frontend define the native menu bar from its
// command registry and keymap.
type MenuService struct{ app *App }

// menuRole is a standard menu supplied by the platform.
type menuRole int

const (
	roleNone menuRole = iota
	roleApp
	roleEdit
	roleWindow
)

// menuTop is one top-level entry of a planned menu bar: a standard menu, or
// one of the frontend's menus.
type menuTop struct {
	Role  menuRole
	Label string
	Items []MenuItemSpec
}

// planMenu lays out the menu bar. On macOS it adds the standard app, Edit
// (clipboard) and Window (open windows) menus around the frontend's menus.
func planMenu(menus []MenuSpec, goos string) []menuTop {
	var out []menuTop
	mac := goos == "darwin"
	if mac {
		out = append(out, menuTop{Role: roleApp})
	}
	for i, m := range menus {
		if mac && i == 1 {
			out = append(out, menuTop{Role: roleEdit})
		}
		out = append(out, menuTop{Label: m.Label, Items: m.Items})
	}
	if mac {
		if len(menus) < 2 {
			out = append(out, menuTop{Role: roleEdit})
		}
		out = append(out, menuTop{Role: roleWindow})
	}
	return out
}

// toWailsMenu builds a Wails menu from a plan; run executes a command.
func toWailsMenu(plan []menuTop, run func(cmd string)) *application.Menu {
	root := application.NewMenu()
	for _, top := range plan {
		switch top.Role {
		case roleApp:
			root.AddRole(application.AppMenu)
			continue
		case roleEdit:
			root.AddRole(application.EditMenu)
			continue
		case roleWindow:
			root.AddRole(application.WindowMenu)
			continue
		}
		sub := root.AddSubmenu(top.Label)
		for _, it := range top.Items {
			if it.Separator {
				sub.AddSeparator()
				continue
			}
			cmd := it.Command
			item := sub.Add(it.Label).OnClick(func(*application.Context) { run(cmd) })
			if it.Accel != "" {
				// An invalid accelerator is logged and the item keeps no shortcut.
				item.SetAccelerator(it.Accel)
			}
		}
	}
	return root
}

// applyMenuHook replaces installing the menu bar in tests.
var applyMenuHook func(a *App, plan []menuTop)

// applyMenu installs the menu bar. On macOS the menu is global and a click
// runs in the focused window. On Windows and Linux each window gets its own
// menu, so a click runs in that window.
func applyMenu(a *App, plan []menuTop) {
	if applyMenuHook != nil {
		applyMenuHook(a, plan)
		return
	}
	if a.wapp == nil {
		return
	}
	if isMac() {
		a.wapp.Menu.Set(toWailsMenu(plan, func(cmd string) {
			if w := a.focusedWindow(); w != nil {
				w.bus.Emit(TopicMenuCommand, MenuCommandEvent{Command: cmd})
			}
		}))
		return
	}
	for _, w := range a.allWindows() {
		h, ok := w.handle.(*nativeHandle)
		if !ok {
			continue
		}
		w := w
		h.w.SetMenu(toWailsMenu(plan, func(cmd string) {
			w.bus.Emit(TopicMenuCommand, MenuCommandEvent{Command: cmd})
		}))
	}
}

// SetMenu replaces the menu bar. Items with an invalid accelerator keep their
// label but get no shortcut.
func (s *MenuService) SetMenu(menus []MenuSpec) {
	s.app.menuMu.Lock()
	s.app.menu = planMenu(menus, runtime.GOOS)
	plan := s.app.menu
	s.app.menuMu.Unlock()
	applyMenu(s.app, plan)
}

// reapplyMenu gives a new window the current menu bar (Windows and Linux,
// where menus are per window).
func (a *App) reapplyMenu() {
	a.menuMu.Lock()
	plan := a.menu
	a.menuMu.Unlock()
	if plan != nil && !isMac() {
		applyMenu(a, plan)
	}
}
