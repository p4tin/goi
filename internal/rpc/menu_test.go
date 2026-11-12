package rpc

import (
	"testing"
)

var testMenus = []MenuSpec{
	{Label: "File", Items: []MenuItemSpec{
		{Label: "Open Folder…", Command: "project.openFolder", Accel: "cmdorctrl+o"},
		{Separator: true},
		{Label: "Close Project", Command: "project.close"},
	}},
	{Label: "View", Items: []MenuItemSpec{
		{Label: "Toggle Left Panel", Command: "view.toggleLeft", Accel: "cmdorctrl+shift+1"},
	}},
}

func TestPlanMenuOther(t *testing.T) {
	plan := planMenu(testMenus, "linux")
	if len(plan) != 2 || plan[0].Label != "File" || plan[1].Label != "View" || plan[0].Role != roleNone {
		t.Fatalf("plan %+v", plan)
	}
	if len(plan[0].Items) != 3 || !plan[0].Items[1].Separator || plan[0].Items[0].Accel != "cmdorctrl+o" {
		t.Errorf("file items %+v", plan[0].Items)
	}
}

func TestPlanMenuMac(t *testing.T) {
	plan := planMenu(testMenus, "darwin")
	want := []menuRole{roleApp, roleNone, roleEdit, roleNone, roleWindow}
	if len(plan) != len(want) {
		t.Fatalf("got %d top-level items", len(plan))
	}
	for i, r := range want {
		if plan[i].Role != r {
			t.Errorf("item %d role %v, want %v", i, plan[i].Role, r)
		}
	}
	if plan[1].Label != "File" || plan[3].Label != "View" {
		t.Errorf("plan %+v", plan)
	}
	if one := planMenu(testMenus[:1], "darwin"); len(one) != 4 || one[2].Role != roleEdit || one[3].Role != roleWindow {
		t.Errorf("single menu plan %+v", one)
	}
}

func TestSetMenuStoresPlan(t *testing.T) {
	a, _ := started(t, "")
	var got []menuTop
	applyMenuHook = func(_ *App, p []menuTop) { got = p }
	t.Cleanup(func() { applyMenuHook = nil })
	(&MenuService{app: a}).SetMenu(testMenus[:1])
	if len(got) == 0 {
		t.Fatal("menu not applied")
	}
	a.menuMu.Lock()
	stored := a.menu
	a.menuMu.Unlock()
	if len(stored) != len(got) {
		t.Errorf("stored plan %+v", stored)
	}
}
