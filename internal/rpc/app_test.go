package rpc

import (
	"path/filepath"
	"testing"
)

func TestParseArgs(t *testing.T) {
	abs, _ := filepath.Abs("some/dir")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"none", nil, ""},
		{"space form", []string{"--project", "some/dir"}, abs},
		{"equals form", []string{"--project=some/dir"}, abs},
		{"single dash", []string{"-project", "some/dir"}, abs},
		{"unknown flags ignored", []string{"-psn_0_12345", "--project", "some/dir", "--weird"}, abs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.ProjectPath != tt.want {
				t.Errorf("ProjectPath = %q, want %q", got.ProjectPath, tt.want)
			}
		})
	}
}

func TestNewAppInfo(t *testing.T) {
	t.Setenv("GOI_HOME", t.TempDir())
	a, err := NewApp(Options{ProjectPath: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.closer.Close()
	info := a.appSvc.Info()
	if info.Name != "Goi" || info.ProjectPath != "/p" || info.LogPath == "" {
		t.Errorf("info = %+v", info)
	}
}
