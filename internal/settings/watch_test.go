package settings

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/vfs"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestStoreWatchReloadsOnExternalEdit(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user", "settings.toml")
	writeFile(t, user, "[editor]\nfontSize = 14\n")
	s, err := Load(user, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := vfs.NewWatcher(nil, vfs.MatchNone, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	cancel, err := s.Watch(w)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	start := time.Now()
	writeFile(t, user, "[editor]\nfontSize = 20\n")
	waitFor(t, func() bool { return s.Effective().Editor.FontSize == 20 })
	if d := time.Since(start); d > time.Second {
		t.Errorf("reload took %v, want < 1s", d)
	}

	// The project watch follows SetProjectPath.
	project := filepath.Join(dir, "proj", ".goi", "settings.toml")
	writeFile(t, project, "[editor]\ntabSize = 2\n")
	if err := s.SetProjectPath(project); err != nil {
		t.Fatal(err)
	}
	if s.Effective().Editor.TabSize != 2 {
		t.Fatalf("tabSize = %d", s.Effective().Editor.TabSize)
	}
	writeFile(t, project, "[editor]\ntabSize = 8\n")
	waitFor(t, func() bool { return s.Effective().Editor.TabSize == 8 })

	// After cancel, edits are no longer picked up.
	cancel()
	writeFile(t, user, "[editor]\nfontSize = 30\n")
	time.Sleep(300 * time.Millisecond)
	if s.Effective().Editor.FontSize != 20 {
		t.Errorf("reloaded after cancel")
	}
	os.Remove(user)
}
