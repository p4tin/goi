package gosdk

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fakeGOROOT creates dir with bin/go[.exe] and, if version != "", a VERSION file.
func fakeGOROOT(t *testing.T, dir, version string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", goExe()), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if version != "" {
		if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\ntime 2026-01-01\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// noExec disables running real binaries for the duration of a test.
func noExec(t *testing.T) {
	t.Helper()
	oldV, oldR := runGoVersion, goEnvGOROOT
	runGoVersion = func(string) (string, error) { return "", errors.New("exec disabled") }
	goEnvGOROOT = func(context.Context, string) (string, error) { return "", errors.New("exec disabled") }
	t.Cleanup(func() { runGoVersion, goEnvGOROOT = oldV, oldR })
}

func TestValidate(t *testing.T) {
	noExec(t)
	base := t.TempDir()
	ok := fakeGOROOT(t, filepath.Join(base, "ok"), "go1.26.0")
	noVersion := fakeGOROOT(t, filepath.Join(base, "nover"), "")
	empty := filepath.Join(base, "empty")
	os.MkdirAll(empty, 0o755)

	tests := []struct {
		name    string
		root    string
		want    string
		wantErr string
	}{
		{"valid", ok, "go1.26.0", ""},
		{"missing bin", empty, "", "no go binary"},
		{"missing VERSION and exec fails", noVersion, "", "cannot determine version"},
		{"empty", "", "", "empty GOROOT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdk, err := Validate(tt.root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sdk.Version != tt.want || sdk.GoBin != filepath.Join(tt.root, "bin", goExe()) {
				t.Errorf("sdk = %+v", sdk)
			}
		})
	}
}

func TestValidateFallsBackToGoVersion(t *testing.T) {
	noExec(t)
	runGoVersion = func(string) (string, error) { return "go1.24.3", nil }
	root := fakeGOROOT(t, filepath.Join(t.TempDir(), "r"), "")
	sdk, err := Validate(root)
	if err != nil || sdk.Version != "go1.24.3" {
		t.Fatalf("sdk=%+v err=%v", sdk, err)
	}
}

func TestDiscoverWith(t *testing.T) {
	noExec(t)
	home := t.TempDir()
	std := fakeGOROOT(t, filepath.Join(home, "std"), "go1.24.0")
	link := filepath.Join(home, "std-link")
	if err := os.Symlink(std, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	fakeGOROOT(t, filepath.Join(home, "sdk", "go1.26.0"), "go1.26.0")
	fakeGOROOT(t, filepath.Join(home, "sdk", "go1.26rc1"), "go1.26rc1")
	os.MkdirAll(filepath.Join(home, "sdk", "gobroken"), 0o755) // invalid: no bin/go
	tc := fakeGOROOT(t, filepath.Join(home, "go", "pkg", "mod", "golang.org", "toolchain@v0.0.1-go1.25.6.x"), "go1.25.6")
	onPath := fakeGOROOT(t, filepath.Join(home, "pathgo"), "go1.20.1")

	got := DiscoverWith(context.Background(), DiscoverOptions{
		LookPath: func(string) (string, error) { return filepath.Join(onPath, "bin", goExe()), nil },
		Home:     home,
		Roots:    []string{std, link, filepath.Join(home, "missing")},
		Globs:    []string{"~/sdk/go*", "~/go/pkg/mod/golang.org/toolchain@*"},
	})

	var versions, sources []string
	for _, s := range got {
		versions = append(versions, s.Version)
		sources = append(sources, s.Source)
	}
	wantV := []string{"go1.20.1", "go1.26.0", "go1.26rc1", "go1.25.6", "go1.24.0"}
	wantS := []string{SourcePath, SourceSDK, SourceSDK, SourceToolchain, SourceStandard}
	if !slices.Equal(versions, wantV) {
		t.Errorf("versions = %v, want %v", versions, wantV)
	}
	if !slices.Equal(sources, wantS) {
		t.Errorf("sources = %v, want %v", sources, wantS)
	}
	if got[3].GOROOT != tc {
		t.Errorf("toolchain GOROOT = %s", got[3].GOROOT)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"go1.26.0", "go1.26.0", 0},
		{"go1.26", "go1.26.0", 0},
		{"go1.26.1", "go1.26.0", 1},
		{"go1.9", "go1.10", -1},
		{"go1.26rc1", "go1.26.0", -1},
		{"go1.26beta1", "go1.26rc1", -1},
		{"go1.26rc2", "go1.26rc1", 1},
		{"go2.0.0", "go1.99.9", 1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRunnerCommand(t *testing.T) {
	sdk := SDK{GOROOT: "/sdk", GoBin: "/sdk/bin/go"}
	tests := []struct {
		name     string
		env      Env
		args     []string
		wantArgs []string
		wantEnv  []string
	}{
		{
			name:     "tags and vendor on build",
			env:      Env{Tags: []string{"a", "b"}, Vendor: "on"},
			args:     []string{"build", "./..."},
			wantArgs: []string{"/sdk/bin/go", "build", "-tags=a,b", "-mod=vendor", "./..."},
		},
		{
			name:     "vendor off on test",
			env:      Env{Vendor: "off"},
			args:     []string{"test", "-run", "X"},
			wantArgs: []string{"/sdk/bin/go", "test", "-mod=mod", "-run", "X"},
		},
		{
			name:     "no flags for env",
			env:      Env{Tags: []string{"a"}, Vendor: "on"},
			args:     []string{"env", "-json"},
			wantArgs: []string{"/sdk/bin/go", "env", "-json"},
		},
		{
			name:     "auto vendor adds nothing",
			env:      Env{Vendor: "auto"},
			args:     []string{"list", "-m"},
			wantArgs: []string{"/sdk/bin/go", "list", "-m"},
		},
		{
			name:     "env vars and target",
			env:      Env{Vars: map[string]string{"GOPROXY": "off", "GOI_TEST_X": "1"}, GOOS: "linux", GOARCH: "arm64", CGO: "0"},
			args:     []string{"version"},
			wantArgs: []string{"/sdk/bin/go", "version"},
			wantEnv:  []string{"GOPROXY=off", "GOI_TEST_X=1", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "GOROOT=/sdk"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Runner{SDK: sdk, Env: tt.env, Dir: "/proj"}.Command(context.Background(), tt.args...)
			if !slices.Equal(cmd.Args, tt.wantArgs) {
				t.Errorf("Args = %v, want %v", cmd.Args, tt.wantArgs)
			}
			if cmd.Dir != "/proj" {
				t.Errorf("Dir = %s", cmd.Dir)
			}
			for _, kv := range tt.wantEnv {
				if !slices.Contains(cmd.Env, kv) {
					t.Errorf("Env missing %s", kv)
				}
			}
		})
	}
}

func TestRunnerOverridesExistingEnv(t *testing.T) {
	t.Setenv("GOFLAGS", "-x")
	cmd := Runner{SDK: SDK{GoBin: "go"}, Env: Env{Vars: map[string]string{"GOFLAGS": "-v"}}}.Command(context.Background(), "version")
	n := 0
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "GOFLAGS=") {
			n++
			if kv != "GOFLAGS=-v" {
				t.Errorf("GOFLAGS = %s", kv)
			}
		}
	}
	if n != 1 {
		t.Errorf("GOFLAGS entries = %d, want 1", n)
	}
}

func TestIsCrossTarget(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{"GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH}, false},
		{map[string]string{}, false},
		{map[string]string{"GOOS": "plan9", "GOARCH": runtime.GOARCH}, true},
		{map[string]string{"GOOS": runtime.GOOS, "GOARCH": "mips"}, true},
	}
	for _, tt := range tests {
		if got := IsCrossTarget(tt.env); got != tt.want {
			t.Errorf("IsCrossTarget(%v) = %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestGoEnvIntegration(t *testing.T) {
	bin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
	}
	out, err := exec.Command(bin, "env", "GOROOT").Output()
	if err != nil {
		t.Skip("go env failed:", err)
	}
	sdk, err := Validate(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	env, err := Runner{SDK: sdk, Dir: t.TempDir()}.GoEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if env["GOVERSION"] == "" {
		t.Errorf("GOVERSION missing from go env: %v", env)
	}
}
