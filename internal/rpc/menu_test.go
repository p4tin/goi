package rpc

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

var testMenus = []MenuSpec{
	{Label: "File", Items: []MenuItemSpec{
		{Label: "Open Folder…", Command: "project.openFolder", Accel: "cmdorctrl+o"},
		{Separator: true},
		{Label: "Close Project", Command: "project.close"},
	}},
	{Label: "View", Items: []MenuItemSpec{
		{Label: "Toggle Left Panel", Command: "view.toggleLeft", Accel: "cmdorctrl+shift+1"},
		{Label: "Bad", Command: "x.bad", Accel: "hyper+q"},
	}},
}

func labels(m *menu.Menu) []string {
	var out []string
	for _, it := range m.Items {
		out = append(out, it.Label)
	}
	return out
}

func TestBuildMenuOther(t *testing.T) {
	var ran []string
	m := buildMenu(testMenus, "linux", func(c string) { ran = append(ran, c) })
	if got := labels(m); len(got) != 2 || got[0] != "File" || got[1] != "View" {
		t.Fatalf("top level %v", got)
	}
	file := m.Items[0].SubMenu.Items
	if len(file) != 3 || !file[1].IsSeparator() {
		t.Fatalf("file items %+v", file)
	}
	if acc := file[0].Accelerator; acc == nil || acc.Key != "o" || len(acc.Modifiers) != 1 || acc.Modifiers[0] != keys.CmdOrCtrlKey {
		t.Errorf("accelerator %+v", acc)
	}
	view := m.Items[1].SubMenu.Items
	if view[1].Accelerator != nil {
		t.Errorf("invalid accelerator kept: %+v", view[1].Accelerator)
	}
	file[2].Click(nil)
	view[0].Click(nil)
	if len(ran) != 2 || ran[0] != "project.close" || ran[1] != "view.toggleLeft" {
		t.Errorf("ran %v", ran)
	}
}

func TestBuildMenuMac(t *testing.T) {
	m := buildMenu(testMenus, "darwin", func(string) {})
	if len(m.Items) != 5 {
		t.Fatalf("got %d top-level items", len(m.Items))
	}
	want := []menu.Role{menu.AppMenuRole, 0, menu.EditMenuRole, 0, menu.WindowMenuRole}
	for i, r := range want {
		if m.Items[i].Role != r {
			t.Errorf("item %d role %v, want %v", i, m.Items[i].Role, r)
		}
	}
	if m.Items[1].Label != "File" || m.Items[3].Label != "View" {
		t.Errorf("labels %v", labels(m))
	}
}

func TestSetMenuEmitsCommand(t *testing.T) {
	a, rec := started(t, "")
	var got *menu.Menu
	old := applyMenu
	applyMenu = func(_ context.Context, m *menu.Menu) { got = m }
	t.Cleanup(func() { applyMenu = old })
	(&MenuService{app: a}).SetMenu(testMenus[:1])
	if got == nil {
		t.Fatal("menu not applied")
	}
	var file *menu.Menu
	for _, it := range got.Items {
		if it.Label == "File" {
			file = it.SubMenu
		}
	}
	file.Items[0].Click(nil)
	evs := rec.Events(TopicMenuCommand)
	if len(evs) != 1 || evs[0].Payload.(MenuCommandEvent).Command != "project.openFolder" {
		t.Errorf("events %+v", evs)
	}
}
