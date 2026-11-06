package vfs

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
)

func eventually(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

type collector struct {
	mu      sync.Mutex
	batches [][]Event
}

func (c *collector) add(evs []Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batches = append(c.batches, evs)
}

func (c *collector) all() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Event
	for _, b := range c.batches {
		out = append(out, b...)
	}
	return out
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.batches)
}

func (c *collector) has(path string, op Op) bool {
	for _, e := range c.all() {
		if e.Path == path && e.Op == op {
			return true
		}
	}
	return false
}

func (c *collector) seen(path string) bool {
	for _, e := range c.all() {
		if e.Path == path {
			return true
		}
	}
	return false
}

func startWatcher(t *testing.T, roots []string, globs []string, window time.Duration, bus events.Bus) (*watcher, *collector) {
	t.Helper()
	m, err := NewMatcher(globs)
	if err != nil {
		t.Fatal(err)
	}
	w, err := newWatcher(roots, m, bus, window)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	c := &collector{}
	w.Subscribe(c.add)
	return w, c
}

func mustWrite(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherBatchesEvents(t *testing.T) {
	root := t.TempDir()
	gone := filepath.Join(root, "gone.txt")
	mustWrite(t, gone, "x")
	rec := &events.Recorder{}
	_, c := startWatcher(t, []string{root}, nil, 300*time.Millisecond, rec)

	a, b := filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")
	mustWrite(t, a, "1")
	mustWrite(t, a, "2")
	mustWrite(t, b, "1")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	eventually(t, time.Second, func() bool {
		return c.has(a, OpCreate) && c.has(b, OpCreate) && c.has(gone, OpRemove)
	})
	if n := c.count(); n != 1 {
		t.Errorf("batches = %d, want 1: %+v", n, c.batches)
	}
	for _, e := range c.all() {
		if e.Path == a && e.Op != OpCreate {
			t.Errorf("create+write should merge to create, got %+v", e)
		}
	}
	if len(rec.Events(events.TopicFSChanged)) != 1 {
		t.Errorf("bus events = %+v", rec.Events(""))
	}
}

func TestWatcherCreateThenRemoveDropped(t *testing.T) {
	root := t.TempDir()
	_, c := startWatcher(t, []string{root}, nil, 300*time.Millisecond, nil)
	tmp := filepath.Join(root, "flash.txt")
	mustWrite(t, tmp, "x")
	os.Remove(tmp)
	marker := filepath.Join(root, "marker.txt")
	mustWrite(t, marker, "m")
	eventually(t, time.Second, func() bool { return c.seen(marker) })
	if c.seen(tmp) {
		t.Errorf("create+remove should be dropped: %+v", c.all())
	}
}

func TestWatcherNewDirAutoWatched(t *testing.T) {
	root := t.TempDir()
	_, c := startWatcher(t, []string{root}, nil, 20*time.Millisecond, nil)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool { return c.has(sub, OpCreate) })
	f := filepath.Join(sub, "f.txt")
	mustWrite(t, f, "x")
	eventually(t, time.Second, func() bool { return c.seen(f) })
}

func TestWatcherSynthesizesMovedInTree(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	tree := filepath.Join(outside, "tree")
	deep := filepath.Join(tree, "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(deep, "x.txt"), "x")
	_, c := startWatcher(t, []string{root}, nil, 20*time.Millisecond, nil)

	moved := filepath.Join(root, "tree")
	if err := os.Rename(tree, moved); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(moved, "deep", "x.txt")
	eventually(t, time.Second, func() bool {
		return c.has(moved, OpCreate) && c.has(filepath.Join(moved, "deep"), OpCreate) && c.has(want, OpCreate)
	})
	// The moved-in subdirectory is watched too.
	later := filepath.Join(moved, "deep", "later.txt")
	mustWrite(t, later, "y")
	eventually(t, time.Second, func() bool { return c.seen(later) })
}

func TestWatcherIgnoresExcludedAndTemp(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	_, c := startWatcher(t, []string{root}, []string{"node_modules"}, 20*time.Millisecond, nil)
	mustWrite(t, filepath.Join(nm, "a.js"), "x")
	mustWrite(t, filepath.Join(root, ".goi-tmp-123"), "x")
	if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "pkg", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "marker.txt")
	mustWrite(t, marker, "m")
	eventually(t, time.Second, func() bool { return c.seen(marker) && c.seen(filepath.Join(root, "pkg")) })
	time.Sleep(100 * time.Millisecond)
	for _, e := range c.all() {
		if filepath.Base(e.Path) == "a.js" || filepath.Base(e.Path) == "node_modules" || filepath.Base(e.Path) == ".goi-tmp-123" {
			t.Errorf("unexpected event %+v", e)
		}
	}
}

func TestWatcherAddRemoveRoot(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	w, c := startWatcher(t, []string{r1}, nil, 20*time.Millisecond, nil)
	if err := w.AddRoot(r2); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(r2, "a.txt")
	mustWrite(t, a, "x")
	eventually(t, time.Second, func() bool { return c.seen(a) })

	if err := w.RemoveRoot(r2); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(r2, "b.txt")
	mustWrite(t, b, "x")
	marker := filepath.Join(r1, "marker.txt")
	mustWrite(t, marker, "m")
	eventually(t, time.Second, func() bool { return c.seen(marker) })
	time.Sleep(100 * time.Millisecond)
	if c.seen(b) {
		t.Errorf("event after RemoveRoot: %+v", c.all())
	}
}

func TestWatchFileAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.toml")
	mustWrite(t, target, "a = 1\n")
	w, _ := startWatcher(t, nil, nil, 20*time.Millisecond, nil)

	var calls atomic.Int32
	cancel, err := w.WatchFile(target, func() { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".goi-tmp-1")
	mustWrite(t, tmp, "a = 2\n")
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool { return calls.Load() >= 1 })

	// Unrelated files in the same directory don't fire.
	n := calls.Load()
	mustWrite(t, filepath.Join(dir, "other.txt"), "x")
	time.Sleep(250 * time.Millisecond)
	if calls.Load() != n {
		t.Errorf("unrelated file fired the watch")
	}

	cancel()
	mustWrite(t, target, "a = 3\n")
	time.Sleep(250 * time.Millisecond)
	if calls.Load() != n {
		t.Errorf("fired after cancel")
	}
}

func TestWatcherCloseStopsDelivery(t *testing.T) {
	root := t.TempDir()
	w, c := startWatcher(t, []string{root}, nil, 20*time.Millisecond, nil)
	mustWrite(t, filepath.Join(root, "a.txt"), "x")
	eventually(t, time.Second, func() bool { return c.count() > 0 })
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	n := c.count()
	mustWrite(t, filepath.Join(root, "b.txt"), "x")
	time.Sleep(150 * time.Millisecond)
	if c.count() != n {
		t.Errorf("delivery after Close")
	}
}

func TestLimitErrNotifiesOnce(t *testing.T) {
	rec := &events.Recorder{}
	w := &watcher{bus: rec}
	err := &os.PathError{Op: "add", Path: "/x", Err: syscall.ENOSPC}
	w.limitCheckLocked(err)
	w.limitCheckLocked(err)
	w.limitCheckLocked(errors.New("other"))
	if n := len(rec.Events(events.TopicNotify)); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}
}
