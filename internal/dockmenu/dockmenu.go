// Package dockmenu provides the macOS Dock icon menu (right-click on the
// Dock tile). Other platforms have no equivalent and the calls are no-ops.
//
// The menu is a snapshot: Set replaces it, and the native side builds the
// NSMenu from the last snapshot. Selecting an item calls the OnSelect
// callback with the item's ID.
package dockmenu

import "sync"

// Item is one menu entry. A Separator ignores the other fields. An item with
// Children becomes a submenu.
type Item struct {
	ID        string
	Title     string
	Checked   bool
	Disabled  bool
	Separator bool
	Children  []Item
}

// flatItem is an Item placed in the flattened list sent to the native side.
// Parent is the index of the submenu owner, or -1 for the top level.
type flatItem struct {
	Parent    int
	ID        string
	Title     string
	Checked   bool
	Disabled  bool
	Separator bool
	Submenu   bool
}

// flatten lists items depth-first; parents always precede their children.
func flatten(items []Item) []flatItem {
	var out []flatItem
	var walk func(items []Item, parent int)
	walk = func(items []Item, parent int) {
		for _, it := range items {
			i := len(out)
			out = append(out, flatItem{
				Parent:    parent,
				ID:        it.ID,
				Title:     it.Title,
				Checked:   it.Checked,
				Disabled:  it.Disabled,
				Separator: it.Separator,
				Submenu:   len(it.Children) > 0 && !it.Separator,
			})
			if out[i].Submenu {
				walk(it.Children, i)
			}
		}
	}
	walk(items, -1)
	return out
}

// tag packs a snapshot generation and an item index into the native item's
// tag, so a click on a menu built from an older snapshot is ignored.
func tag(gen uint32, index int) int64 { return int64(gen)<<20 | int64(index) }

func untag(t int64) (gen uint32, index int) { return uint32(t >> 20), int(t & (1<<20 - 1)) }

var state struct {
	mu       sync.Mutex
	gen      uint32
	ids      []string
	onSelect func(id string)
}

// OnSelect sets the callback run when the user picks an item. It runs on a
// new goroutine, never on the main thread.
func OnSelect(fn func(id string)) {
	state.mu.Lock()
	state.onSelect = fn
	state.mu.Unlock()
}

// Set replaces the menu. Safe to call from any goroutine.
func Set(items []Item) {
	flat := flatten(items)
	ids := make([]string, len(flat))
	for i, f := range flat {
		ids[i] = f.ID
	}
	state.mu.Lock()
	state.gen++
	gen := state.gen
	state.ids = ids
	state.mu.Unlock()
	apply(gen, flat)
}

// selected resolves a native tag to an item ID and runs the callback.
func selected(t int64) {
	gen, index := untag(t)
	state.mu.Lock()
	fn := state.onSelect
	var id string
	ok := gen == state.gen && index >= 0 && index < len(state.ids)
	if ok {
		id = state.ids[index]
	}
	state.mu.Unlock()
	if ok && fn != nil && id != "" {
		go fn(id)
	}
}
