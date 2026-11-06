package rpc

import (
	"context"
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

// applyMenu installs m as the application menu; tests replace it.
var applyMenu = func(ctx context.Context, m *menu.Menu) {
	wruntime.MenuSetApplicationMenu(ctx, m)
	wruntime.MenuUpdateApplicationMenu(ctx)
}

// SetMenu replaces the application menu. Items with an invalid accelerator
// keep their label but get no shortcut.
func (s *MenuService) SetMenu(menus []MenuSpec) {
	a := s.app
	applyMenu(a.ctx, buildMenu(menus, runtime.GOOS, func(cmd string) {
		a.bus.Emit(TopicMenuCommand, MenuCommandEvent{Command: cmd})
	}))
}

// buildMenu turns specs into a Wails menu. On macOS it adds the standard
// app, Edit (clipboard) and Window menus around the frontend's menus.
func buildMenu(menus []MenuSpec, goos string, run func(cmd string)) *menu.Menu {
	root := menu.NewMenu()
	mac := goos == "darwin"
	if mac {
		root.Append(menu.AppMenu())
	}
	for i, m := range menus {
		if mac && i == 1 {
			root.Append(menu.EditMenu())
		}
		sub := root.AddSubmenu(m.Label)
		for _, it := range m.Items {
			if it.Separator {
				sub.AddSeparator()
				continue
			}
			var acc *keys.Accelerator
			if it.Accel != "" {
				if a, err := keys.Parse(it.Accel); err == nil {
					acc = a
				}
			}
			cmd := it.Command
			sub.AddText(it.Label, acc, func(*menu.CallbackData) { run(cmd) })
		}
	}
	if mac {
		if len(menus) < 2 {
			root.Append(menu.EditMenu())
		}
		root.Append(menu.WindowMenu())
	}
	return root
}
