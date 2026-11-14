package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/p4tin/goi/internal/events"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, user, project string) (*Store, string, string, *events.Recorder) {
	t.Helper()
	dir := t.TempDir()
	up := filepath.Join(dir, "user", "settings.toml")
	pp := filepath.Join(dir, "proj", ".goi", "settings.toml")
	if user != "" {
		writeFile(t, up, user)
	}
	if project != "" {
		writeFile(t, pp, project)
	}
	rec := &events.Recorder{}
	s, err := Load(up, pp, rec)
	if err != nil {
		t.Fatal(err)
	}
	return s, up, pp, rec
}

func TestEffectiveLayering(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		project string
		check   func(t *testing.T, s Settings)
	}{
		{
			name: "defaults only",
			check: func(t *testing.T, s Settings) {
				if !s.Startup.ReopenLastProject || s.Editor.FontSize != 13 || s.Editor.TabSize != 4 ||
					s.Editor.MaxHighlightSizeKB != 2500 || s.Editor.MaxOpenSizeKB != 20000 ||
					s.Go.Vendor != "auto" || s.Save.Format != "goimports" || s.UI.Theme != "auto" ||
					s.UI.Scheme != "auto" || s.UI.Keymap != "goi-default" || s.Markdown.PreviewMode != "split" ||
					s.JSON.FormatOnSave || s.YAML.FormatOnSave {
					t.Errorf("unexpected defaults: %+v", s)
				}
				want := []string{".git", "node_modules", ".idea", ".goi/cache"}
				if !reflect.DeepEqual(s.Project.Excludes, want) {
					t.Errorf("excludes = %v", s.Project.Excludes)
				}
				if s.Go.Env == nil || s.Keys == nil {
					t.Errorf("maps must be non-nil")
				}
			},
		},
		{
			name: "user overrides defaults",
			user: "[editor]\nfontSize = 15\n[ui]\ntheme = \"dark\"\n",
			check: func(t *testing.T, s Settings) {
				if s.Editor.FontSize != 15 || s.UI.Theme != "dark" || s.Editor.TabSize != 4 {
					t.Errorf("got %+v %+v", s.Editor, s.UI)
				}
			},
		},
		{
			name:    "project overrides user",
			user:    "[save]\nformat = \"gofmt\"\n",
			project: "[save]\nformat = \"none\"\n",
			check: func(t *testing.T, s Settings) {
				if s.Save.Format != "none" {
					t.Errorf("format = %q", s.Save.Format)
				}
			},
		},
		{
			name:    "markdown and formatting tables",
			user:    "[markdown]\npreviewMode = \"preview\"\n[json]\nformatOnSave = true\n",
			project: "[yaml]\nformatOnSave = true\n",
			check: func(t *testing.T, s Settings) {
				if s.Markdown.PreviewMode != "preview" || !s.JSON.FormatOnSave || !s.YAML.FormatOnSave {
					t.Errorf("got %+v %+v %+v", s.Markdown, s.JSON, s.YAML)
				}
			},
		},
		{
			name:    "maps merge per key",
			user:    "[go.env]\nGOPROXY = \"direct\"\nGOPRIVATE = \"x\"\n",
			project: "[go.env]\nGOPRIVATE = \"y\"\n",
			check: func(t *testing.T, s Settings) {
				want := map[string]string{"GOPROXY": "direct", "GOPRIVATE": "y"}
				if !reflect.DeepEqual(s.Go.Env, want) {
					t.Errorf("env = %v", s.Go.Env)
				}
			},
		},
		{
			name:    "slices replace",
			user:    "[project]\nexcludes = [\"a\", \"b\"]\n",
			project: "[project]\nexcludes = [\"c\"]\n",
			check: func(t *testing.T, s Settings) {
				if !reflect.DeepEqual(s.Project.Excludes, []string{"c"}) {
					t.Errorf("excludes = %v", s.Project.Excludes)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _, _ := load(t, tt.user, tt.project)
			tt.check(t, s.Effective())
		})
	}
}

func TestOrigins(t *testing.T) {
	s, _, _, _ := load(t,
		"[editor]\nfontSize = 15\n[go.env]\nGOPROXY = \"direct\"\n",
		"[project]\nexcludes = [\"c\"]\n[go.env]\nGOFLAGS = \"-mod=mod\"\n")
	tests := []struct {
		key  string
		want Layer
	}{
		{"editor.fontSize", LayerUser},
		{"editor.tabSize", LayerDefault},
		{"project.excludes", LayerProject},
		{"go.env.GOPROXY", LayerUser},
		{"go.env.GOFLAGS", LayerProject},
		{"go.env", LayerProject}, // highest among children
		{"editor", LayerUser},    // highest among children
		{"keys", LayerDefault},   // empty default table
		{"keys.unset", LayerDefault},
		{"nope.at.all", LayerDefault},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := s.Origin(tt.key); got != tt.want {
				t.Errorf("Origin(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
	if o := s.Origins(); o["editor.fontSize"] != LayerUser {
		t.Errorf("Origins()[editor.fontSize] = %q", o["editor.fontSize"])
	}
}

func TestInvalidTOMLKeepsLastGood(t *testing.T) {
	s, up, _, rec := load(t, "[editor]\nfontSize = 15\n", "")
	writeFile(t, up, "[editor]\nfontSize = 16\n\nthis is = = broken\n")
	if err := s.Reload(LayerUser); err == nil {
		t.Fatal("expected error")
	}
	if got := s.Effective().Editor.FontSize; got != 15 {
		t.Errorf("fontSize = %d, want last good 15", got)
	}
	n := rec.Events(events.TopicNotify)
	if len(n) != 1 {
		t.Fatalf("notify events = %d", len(n))
	}
	note := n[0].Payload.(events.Notification)
	if note.Level != "error" || note.File != up || note.Line != 4 {
		t.Errorf("notification = %+v", note)
	}
	if len(rec.Events(events.TopicSettingsChanged)) != 0 {
		t.Error("settings.changed must not fire on invalid reload")
	}
}

func TestInvalidValueTypeRejectedOnReload(t *testing.T) {
	s, up, _, rec := load(t, "", "")
	writeFile(t, up, "[editor]\nfontSize = \"big\"\n")
	if err := s.Reload(LayerUser); err == nil {
		t.Fatal("expected error")
	}
	if s.Effective().Editor.FontSize != 13 {
		t.Error("should keep defaults")
	}
	if len(rec.Events(events.TopicNotify)) != 1 {
		t.Error("expected notify")
	}
}

func TestInvalidFileOnLoadIsNotFatal(t *testing.T) {
	s, _, _, rec := load(t, "garbage = = =", "")
	if s.Effective().Editor.FontSize != 13 {
		t.Error("defaults expected")
	}
	if len(rec.Events(events.TopicNotify)) != 1 {
		t.Error("expected notify")
	}
}

func TestReloadPicksUpExternalEdit(t *testing.T) {
	s, _, pp, rec := load(t, "", "")
	writeFile(t, pp, "[ui]\nkeymap = \"vscode\"\n")
	if err := s.Reload(LayerProject); err != nil {
		t.Fatal(err)
	}
	if s.Effective().UI.Keymap != "vscode" || s.Origin("ui.keymap") != LayerProject {
		t.Errorf("keymap = %q origin %q", s.Effective().UI.Keymap, s.Origin("ui.keymap"))
	}
	ev := rec.Events(events.TopicSettingsChanged)
	if len(ev) != 1 || ev[0].Payload.(ChangedEvent).Layer != LayerProject {
		t.Errorf("changed events = %+v", ev)
	}
}

func TestSetRoundTrip(t *testing.T) {
	s, _, pp, rec := load(t, "", "")
	if err := s.Set(LayerProject, "go.goroot", "/usr/local/go"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(LayerProject, "go.tags", []string{"integration", "e2e"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(LayerProject, "editor.fontSize", float64(14)); err != nil { // JSON-style number
		t.Fatal(err)
	}
	if err := s.Set(LayerProject, "go.env.GOPROXY", "off"); err != nil {
		t.Fatal(err)
	}
	eff := s.Effective()
	if eff.Go.GOROOT != "/usr/local/go" || !reflect.DeepEqual(eff.Go.Tags, []string{"integration", "e2e"}) ||
		eff.Editor.FontSize != 14 || eff.Go.Env["GOPROXY"] != "off" {
		t.Errorf("effective = %+v", eff)
	}
	if s.Origin("go.goroot") != LayerProject {
		t.Errorf("origin = %q", s.Origin("go.goroot"))
	}
	b, err := os.ReadFile(pp)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := toml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written file not TOML: %v\n%s", err, b)
	}
	if !strings.Contains(string(b), "fontSize = 14") {
		t.Errorf("int not preserved:\n%s", b)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(filepath.Dir(pp))
	if len(entries) != 1 {
		t.Errorf("dir entries = %d", len(entries))
	}
	// A fresh load sees the persisted values.
	s2, err := Load("", pp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Effective().Go.GOROOT != "/usr/local/go" {
		t.Error("not persisted")
	}
	if len(rec.Events(events.TopicSettingsChanged)) != 4 {
		t.Errorf("changed events = %d", len(rec.Events(events.TopicSettingsChanged)))
	}
	// nil removes the key and falls back to the lower layer.
	if err := s.Set(LayerProject, "editor.fontSize", nil); err != nil {
		t.Fatal(err)
	}
	if s.Effective().Editor.FontSize != 13 || s.Origin("editor.fontSize") != LayerDefault {
		t.Error("nil should reset to default")
	}
}

func TestSetValidation(t *testing.T) {
	s, _, _, _ := load(t, "", "")
	tests := []struct {
		name  string
		layer Layer
		key   string
		value any
	}{
		{"default layer", LayerDefault, "ui.theme", "dark"},
		{"keys on project", LayerProject, "keys.file.save", []string{"Mod-s"}},
		{"keys root on project", LayerProject, "keys", map[string]any{}},
		{"bad type", LayerUser, "editor.fontSize", "huge"},
		{"empty key", LayerUser, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.Set(tt.layer, tt.key, tt.value); err == nil {
				t.Error("expected error")
			}
		})
	}
	if s.Effective().Editor.FontSize != 13 {
		t.Error("failed Set must not change effective settings")
	}
	if err := s.Set(LayerUser, "keys.file.save", []string{"Mod-s"}); err != nil {
		t.Fatalf("keys on user: %v", err)
	}
	if got := s.Effective().Keys["file.save"]; !reflect.DeepEqual(got, []string{"Mod-s"}) {
		t.Errorf("keys = %v", got)
	}
}

func TestSetWithoutProjectPath(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "u.toml"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(LayerProject, "ui.theme", "dark"); err == nil {
		t.Error("expected error without project path")
	}
}

func TestSetProjectPath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a", "settings.toml")
	b := filepath.Join(dir, "b", "settings.toml")
	writeFile(t, a, "[ui]\ntheme = \"light\"\n")
	writeFile(t, b, "[ui]\ntheme = \"hc\"\n")
	s, err := Load("", a, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Effective().UI.Theme != "light" {
		t.Fatal("a not loaded")
	}
	if err := s.SetProjectPath(b); err != nil {
		t.Fatal(err)
	}
	if s.Effective().UI.Theme != "hc" || s.Path(LayerProject) != b {
		t.Errorf("theme = %q", s.Effective().UI.Theme)
	}
	if err := s.SetProjectPath(""); err != nil {
		t.Fatal(err)
	}
	if s.Effective().UI.Theme != "auto" {
		t.Errorf("cleared theme = %q", s.Effective().UI.Theme)
	}
}

func TestSubscribe(t *testing.T) {
	s, _, _, _ := load(t, "", "")
	var got []string
	cancel := s.Subscribe(func(st Settings) { got = append(got, st.UI.Theme) })
	if err := s.Set(LayerUser, "ui.theme", "dark"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := s.Set(LayerUser, "ui.theme", "light"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"dark"}) {
		t.Errorf("got %v", got)
	}
}
