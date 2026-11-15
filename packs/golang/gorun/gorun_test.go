package gorun

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/p4tin/goi/internal/runconfig"
)

func TestTestRunPattern(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"TestA"}, "^TestA$"},
		{[]string{"TestA", "TestB"}, "^(TestA|TestB)$"},
		{[]string{"TestA/sub_1"}, "^TestA$/^sub_1$"},
		{[]string{"TestA/case_(1)", "TestA/x.y"}, `^TestA$/^(case_\(1\)|x\.y)$`},
		{[]string{"TestA/a/deep", "TestB/b/deep"}, "^(TestA|TestB)$/^(a|b)$/^deep$"},
		// A whole top-level test keeps all its subtests.
		{[]string{"TestA", "TestB/sub"}, "^(TestA|TestB)$"},
	}
	for _, tt := range tests {
		if got := TestRunPattern(tt.names); got != tt.want {
			t.Errorf("TestRunPattern(%q) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func testCtx(t *testing.T) runconfig.Context {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "cmd", "srv"), 0o755)
	os.WriteFile(filepath.Join(root, "x_test.go"), []byte("package x\nimport \"testing\"\nfunc TestOne(t *testing.T) {}\nfunc TestTwo(t *testing.T) {}\nfunc helper() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("FROM_FILE=1\nOVER=file\n"), 0o644)
	return runconfig.Context{Root: root, Tool: "go", Env: []string{"PATH=/bin"}, TempDir: "/tmp/goi", Types: Types(),
		Extra: GoContext{Tags: []string{"integration"}, ModFlag: "-mod=mod"}}
}

func TestPlanApp(t *testing.T) {
	ctx := testCtx(t)
	c := runconfig.Config{Name: "srv", Type: TypeApp, Kind: KindPackage, Package: "./cmd/srv", Args: `-a "b c"`, Tags: []string{"dev", "integration"},
		BuildFlags: "-ldflags=-s", EnvFile: ".env", Env: map[string]string{"OVER": "cfg"}, InputFile: "in.txt"}
	p, err := c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	wantBuild := []string{"build", "-o", filepath.Join("/tmp/goi", "goi_srv"+exe), "-tags=integration,dev", "-mod=mod", "-ldflags=-s", "."}
	if !reflect.DeepEqual(p.Build.Args, wantBuild) || p.Build.Dir != filepath.Join(ctx.Root, "cmd", "srv") {
		t.Fatalf("build = %+v", p.Build)
	}
	if p.Exec == nil || !reflect.DeepEqual(p.Exec.Args, []string{"-a", "b c"}) || p.Exec.Dir != filepath.Join(ctx.Root, "cmd", "srv") {
		t.Fatalf("exec = %+v", p.Exec)
	}
	env := strings.Join(p.Exec.Env, ";")
	if !strings.Contains(env, "FROM_FILE=1") || !strings.Contains(env, "OVER=cfg") || strings.Contains(env, "OVER=file") {
		t.Fatalf("env = %s", env)
	}
	if p.Exec.Stdin != filepath.Join(ctx.Root, "in.txt") {
		t.Fatalf("stdin = %q", p.Exec.Stdin)
	}
	off := false
	c.RunAfterBuild = &off
	if p, _ := c.MakePlan(ctx); p.Exec != nil {
		t.Fatal("build-only plan must not exec")
	}
	// Import paths resolve through PackageDir.
	ctx.Extra = GoContext{Tags: []string{"integration"}, ModFlag: "-mod=mod", PackageDir: func(string) (string, error) { return "/src/x", nil }}
	c = runconfig.Config{Name: "x", Type: TypeApp, Package: "example.com/x"}
	if p, _ := c.MakePlan(ctx); p.Build.Dir != "/src/x" {
		t.Fatalf("import path dir = %q", p.Build.Dir)
	}
}

func TestPlanTest(t *testing.T) {
	ctx := testCtx(t)
	c := runconfig.Config{Name: "t", Type: TypeTest, Kind: KindDirectory, Directory: ".", Race: true, Count: 1, Short: true, Timeout: "30s", TestFlags: "-v", Args: "-update"}
	p, err := c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test", "-json", "-tags=integration", "-mod=mod", "-race", "-count=1", "-short", "-timeout=30s", "-v", "./...", "-args", "-update"}
	if !p.Test || !reflect.DeepEqual(p.Exec.Args, want) {
		t.Fatalf("args = %q", p.Exec.Args)
	}
	c = runconfig.Config{Name: "f", Type: TypeTest, Kind: KindFile, Files: []string{"x_test.go"}}
	p, err = c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Exec.Args, " "); !strings.Contains(got, "-run=^(TestOne|TestTwo)$ .") {
		t.Fatalf("file kind args = %s", got)
	}
	c = runconfig.Config{Name: "p", Type: TypeTest, Kind: KindFunction, Packages: []string{"example.com/a", "example.com/b"}, Pattern: "^TestA$"}
	p, _ = c.MakePlan(ctx)
	if got := strings.Join(p.Exec.Args, " "); !strings.HasSuffix(got, "-run=^TestA$ example.com/a example.com/b") || p.Exec.Dir != ctx.Root {
		t.Fatalf("packages args = %s (dir %s)", got, p.Exec.Dir)
	}
	if _, err := DebugLaunch(c, ctx, "b"); err == nil {
		t.Fatal("several packages can't be debugged")
	}
	c = runconfig.Config{Name: "b", Type: TypeTest, Kind: KindFunction, Package: ".", Pattern: "^$", Bench: "^BenchmarkX$", Benchmem: true}
	p, _ = c.MakePlan(ctx)
	if got := strings.Join(p.Exec.Args, " "); !strings.Contains(got, "-run=^$ -bench=^BenchmarkX$ -benchmem .") {
		t.Fatalf("bench args = %s", got)
	}
}

func TestDebugLaunch(t *testing.T) {
	ctx := testCtx(t)
	c := runconfig.Config{Name: "t", Type: TypeTest, Kind: KindFunction, Package: ".", Pattern: "^TestOne$/^sub$", Count: 1, Args: "-x"}
	l, err := DebugLaunch(c, ctx, "/tmp/bin")
	if err != nil {
		t.Fatal(err)
	}
	if l["mode"] != "test" || l["program"] != ctx.Root || l["output"] != "/tmp/bin" || l["outputMode"] != "remote" || l["buildFlags"] != "-tags=integration -mod=mod" {
		t.Fatalf("launch = %v", l)
	}
	if got := l["args"].([]string); !reflect.DeepEqual(got, []string{"-test.count=1", "-test.run=^TestOne$/^sub$", "-test.v", "-x"}) {
		t.Fatalf("args = %q", got)
	}
	if _, err := DebugLaunch(runconfig.Config{Type: TypeTest, Kind: KindDirectory, Directory: "."}, ctx, "b"); err == nil {
		t.Fatal("recursive directory can't be debugged")
	}
	a := runconfig.Config{Type: TypeApp, Kind: KindFile, Files: []string{"main.go"}}
	if l, _ := DebugLaunch(a, ctx, "b"); l["program"] != filepath.Join(ctx.Root, "main.go") || l["mode"] != "debug" {
		t.Fatalf("file launch = %v", l)
	}
}

func TestValidate(t *testing.T) {
	ctx := testCtx(t)
	tests := []struct {
		c    runconfig.Config
		want string // substring of the first error; "" = valid
	}{
		{runconfig.Config{Name: "a", Type: TypeApp, Package: "./cmd/srv"}, ""},
		{runconfig.Config{Name: "a", Type: TypeApp}, "Package is required"},
		{runconfig.Config{Name: "", Type: TypeTest}, "Name is required"},
		{runconfig.Config{Name: "f", Type: TypeTest, Kind: KindFile, Files: []string{"nope_test.go"}}, "does not exist"},
		{runconfig.Config{Name: "f", Type: TypeTest, Kind: KindFunction}, "Test pattern is required"},
		{runconfig.Config{Name: "r", Type: TypeRemote, Host: "localhost", Port: 70000}, "Port must be"},
		{runconfig.Config{Name: "s", Type: runconfig.TypeShell}, "script"},
		{runconfig.Config{Name: "m", Type: runconfig.TypeMake}, "No Makefile"},
		{runconfig.Config{Name: "x", Type: TypeTest, Args: `"open`}, "Program arguments"},
		{runconfig.Config{Name: "x", Type: TypeTest, BeforeLaunch: []runconfig.Task{{Type: runconfig.TaskConfig, Config: "X"}}}, "itself"},
		{runconfig.Config{Name: "x", Type: TypeTest, WorkingDir: "missing"}, "Working directory"},
	}
	for _, tt := range tests {
		errs := tt.c.Validate(ctx, Tasks())
		if tt.want == "" {
			if len(errs) != 0 {
				t.Errorf("%+v: unexpected errors %v", tt.c, errs)
			}
			continue
		}
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, "|"), tt.want) {
			t.Errorf("%+v: errors %v, want %q", tt.c, errs, tt.want)
		}
	}
}
