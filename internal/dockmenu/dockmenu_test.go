package dockmenu

import (
	"reflect"
	"testing"
)

func TestFlatten(t *testing.T) {
	got := flatten([]Item{
		{ID: "win:1", Title: "goi", Checked: true},
		{ID: "win:2", Title: "Welcome"},
		{Separator: true},
		{ID: "new", Title: "New Window"},
		{ID: "recent", Title: "Open Recent", Children: []Item{
			{ID: "recent:/a", Title: "a"},
			{ID: "recent:/b", Title: "b", Disabled: true},
		}},
	})
	want := []flatItem{
		{Parent: -1, ID: "win:1", Title: "goi", Checked: true},
		{Parent: -1, ID: "win:2", Title: "Welcome"},
		{Parent: -1, Separator: true},
		{Parent: -1, ID: "new", Title: "New Window"},
		{Parent: -1, ID: "recent", Title: "Open Recent", Submenu: true},
		{Parent: 4, ID: "recent:/a", Title: "a"},
		{Parent: 4, ID: "recent:/b", Title: "b", Disabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flatten:\n got %+v\nwant %+v", got, want)
	}
}

func TestTagRoundTrip(t *testing.T) {
	for _, c := range []struct {
		gen   uint32
		index int
	}{{1, 0}, {7, 12}, {1 << 30, 1<<20 - 1}} {
		gen, index := untag(tag(c.gen, c.index))
		if gen != c.gen || index != c.index {
			t.Errorf("untag(tag(%d, %d)) = %d, %d", c.gen, c.index, gen, index)
		}
	}
}

func TestSelectedIgnoresStaleSnapshot(t *testing.T) {
	picked := make(chan string, 4)
	OnSelect(func(id string) { picked <- id })
	defer OnSelect(nil)

	Set([]Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}})
	state.mu.Lock()
	old := state.gen
	state.mu.Unlock()
	selected(tag(old, 1))
	if id := <-picked; id != "b" {
		t.Fatalf("picked %q, want b", id)
	}

	Set([]Item{{ID: "c", Title: "C"}})
	selected(tag(old, 0)) // menu from the previous snapshot
	selected(tag(old+1, 5))
	selected(tag(old+1, 0))
	if id := <-picked; id != "c" {
		t.Fatalf("picked %q, want c (stale clicks must be ignored)", id)
	}
	select {
	case id := <-picked:
		t.Fatalf("unexpected pick %q", id)
	default:
	}
}
