package recent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func paths(list []Entry) []string {
	var out []string
	for _, e := range list {
		out = append(out, e.Path)
	}
	return out
}

func TestTouchOrderDedupeAndCap(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "recent.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if err := s.Touch(Entry{Path: fmt.Sprintf("/p/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Touch(Entry{Path: "/p/10/"}); err != nil { // dedupe after Clean
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != MaxEntries {
		t.Fatalf("len = %d, want %d", len(list), MaxEntries)
	}
	want0 := filepath.Clean("/p/10")
	if list[0].Path != want0 || list[0].Name != "10" || list[1].Path != filepath.Clean("/p/24") {
		t.Errorf("order = %v", paths(list))
	}
	seen := map[string]bool{}
	for _, e := range list {
		if seen[e.Path] {
			t.Errorf("duplicate %s", e.Path)
		}
		seen[e.Path] = true
		if e.OpenedAt.IsZero() {
			t.Errorf("OpenedAt zero for %s", e.Path)
		}
	}
	if seen[filepath.Clean("/p/4")] {
		t.Error("oldest entries should be dropped")
	}
}

func TestRemoveExistsAndPersistence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "recent.json")
	live := filepath.Join(dir, "live")
	gone := filepath.Join(dir, "gone")
	os.Mkdir(live, 0o755)
	os.Mkdir(gone, 0o755)

	s, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{gone, live, "/nowhere"} {
		if err := s.Touch(Entry{Path: p, Kind: "module", OpenedAt: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Remove("/nowhere"); err != nil {
		t.Fatal(err)
	}
	os.Remove(gone)

	s2, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Path != live || list[1].Path != gone {
		t.Fatalf("list = %v", paths(list))
	}
	if !list[0].Exists || list[1].Exists {
		t.Errorf("exists = %v, %v", list[0].Exists, list[1].Exists)
	}
	if list[0].Kind != "module" || !list[0].OpenedAt.Equal(time.Unix(1, 0)) {
		t.Errorf("entry = %+v", list[0])
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "sub", "recent.json"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("list = %v, err = %v", list, err)
	}
}

func TestConcurrentStoresDoNotLoseWrites(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recent.json")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for g := 0; g < 2; g++ {
		s, err := Load(file)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(g int, s *Store) {
			defer wg.Done()
			// MaxEntries/2 touches each: nothing is evicted, so every write
			// must survive whatever the interleaving.
			for i := 0; i < MaxEntries/2; i++ {
				if err := s.Touch(Entry{Path: fmt.Sprintf("/g%d/%d", g, i)}); err != nil {
					errs <- err
					return
				}
			}
		}(g, s)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var raw []Entry
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("corrupt JSON: %v", err)
	}
	if len(raw) != MaxEntries {
		t.Fatalf("len = %d", len(raw))
	}
	// Each goroutine touches in increasing order, so within each goroutine's
	// entries the indices must be strictly decreasing, and both goroutines'
	// last writes must be present.
	last := map[byte]int{'0': 100, '1': 100}
	for _, e := range raw {
		var g, i int
		if _, err := fmt.Sscanf(filepath.ToSlash(e.Path), "/g%d/%d", &g, &i); err != nil {
			t.Fatalf("bad path %q", e.Path)
		}
		k := byte('0' + g)
		if i >= last[k] {
			t.Errorf("g%d out of order: %v", g, paths(raw))
		}
		last[k] = i
	}
	top := map[string]bool{}
	for _, e := range raw {
		top[filepath.ToSlash(e.Path)] = true
	}
	for g := 0; g < 2; g++ {
		for i := 0; i < MaxEntries/2; i++ {
			if p := fmt.Sprintf("/g%d/%d", g, i); !top[p] {
				t.Errorf("write %s lost: %v", p, paths(raw))
			}
		}
	}
}
