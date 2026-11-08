package runconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		err  bool
	}{
		{"", []string{}, false},
		{"a b  c", []string{"a", "b", "c"}, false},
		{`-name "hello world" 'x y'`, []string{"-name", "hello world", "x y"}, false},
		{`a\ b "q\"uote" 'it''s'`, []string{"a b", `q"uote`, "its"}, false},
		{`"$HOME" 'lit $x'`, []string{"$HOME", "lit $x"}, false},
		{`"" x`, []string{"", "x"}, false},
		{`"open`, nil, true},
		{`'open`, nil, true},
	}
	for _, tt := range tests {
		got, err := SplitArgs(tt.in)
		if (err != nil) != tt.err {
			t.Fatalf("SplitArgs(%q) err = %v", tt.in, err)
		}
		if !tt.err && !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitArgs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseEnv(t *testing.T) {
	src := `# comment
A=1
export B = two
C="line\nnext \"q\""
D='single # not comment'
E=plain # trailing comment

F=
`
	got, err := ParseEnv(bufio.NewScanner(strings.NewReader(src)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two", "C": "line\nnext \"q\"", "D": "single # not comment", "E": "plain", "F": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseEnv = %#v", got)
	}
	if _, err := ParseEnv(bufio.NewScanner(strings.NewReader("not a pair"))); err == nil {
		t.Fatal("want error for invalid line")
	}
}

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

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Run server": "Run_server", "go test ./...": "go_test_._" /* trailing dots are invalid on Windows */, "": "config", "..": "config", "a/b:c": "a_b_c"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	base := t.TempDir()
	shared, local := filepath.Join(base, "proj", ".goi", "runConfigurations"), filepath.Join(base, "local")
	s, err := Open(shared, local)
	if err != nil {
		t.Fatal(err)
	}
	f := false
	c := Config{Name: "Server", Type: TypeApp, Kind: KindPackage, Package: "./cmd/server", Args: "-port 8080", Env: map[string]string{"A": "1"}, RunAfterBuild: &f, Shared: true,
		BeforeLaunch: []Task{{Type: TaskGenerate, Package: "./..."}}}
	if _, err := s.Save(c, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(shared, "Server.toml")); err != nil {
		t.Fatalf("shared file not written: %v", err)
	}
	if _, err := s.Save(Config{Name: "server", Type: TypeTest}, ""); err == nil {
		t.Fatal("want duplicate-name error (case-insensitive)")
	}
	// Reload from disk.
	s2, err := Open(shared, local)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("SERVER")
	if !ok || !got.Shared || got.Package != "./cmd/server" || got.RunsAfterBuild() || got.Env["A"] != "1" || len(got.BeforeLaunch) != 1 {
		t.Fatalf("reloaded = %+v", got)
	}
	// Rename and move to local.
	got.Name, got.Shared = "API", false
	if _, err := s2.Save(got, "Server"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(shared, "Server.toml")); !os.IsNotExist(err) {
		t.Fatal("old shared file should be removed")
	}
	if _, err := os.Stat(filepath.Join(local, "API.toml")); err != nil {
		t.Fatal("local file not written")
	}
	if err := s2.Delete("api"); err != nil {
		t.Fatal(err)
	}
	if len(s2.List()) != 0 {
		t.Fatalf("List after delete = %v", s2.List())
	}
}

func TestStoreBadFileSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("name = "), 0o644)
	os.WriteFile(filepath.Join(dir, "ok.toml"), []byte("type = \"test\"\n"), 0o644)
	s, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.LoadErrors()) != 1 {
		t.Fatalf("LoadErrors = %v", s.LoadErrors())
	}
	if c, ok := s.Get("ok"); !ok || c.Type != TypeTest {
		t.Fatalf("name defaults to file name: %+v %v", c, ok)
	}
}

func TestTemporaryCap(t *testing.T) {
	s, _ := Open("", t.TempDir())
	if _, err := s.Save(Config{Name: "TestA", Type: TypeTest}, ""); err != nil {
		t.Fatal(err)
	}
	c, err := s.AddTemporary(Config{Name: "TestA", Type: TypeTest})
	if err != nil || c.Name != "TestA (2)" || !c.Temporary {
		t.Fatalf("conflict with permanent: %+v %v", c, err)
	}
	for _, n := range []string{"T1", "T2", "T3", "T4", "T5", "T1"} {
		if _, err := s.AddTemporary(Config{Name: n, Type: TypeTest}); err != nil {
			t.Fatal(err)
		}
	}
	var temps []string
	for _, c := range s.List() {
		if c.Temporary {
			temps = append(temps, c.Name)
		}
	}
	if len(temps) != MaxTemporary || temps[0] != "T1" {
		t.Fatalf("temporaries = %v (want 5, newest T1 first)", temps)
	}
	for _, n := range temps {
		if n == "TestA (2)" {
			t.Fatal("oldest temporary should be pruned")
		}
	}
	if s.List()[0].Name != "TestA" {
		t.Fatal("permanent configurations sort first")
	}
}

func testCtx(t *testing.T) Context {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "cmd", "srv"), 0o755)
	os.WriteFile(filepath.Join(root, "x_test.go"), []byte("package x\nimport \"testing\"\nfunc TestOne(t *testing.T) {}\nfunc TestTwo(t *testing.T) {}\nfunc helper() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("FROM_FILE=1\nOVER=file\n"), 0o644)
	return Context{Root: root, GoBin: "go", Env: []string{"PATH=/bin"}, Tags: []string{"integration"}, ModFlag: "-mod=mod", TempDir: "/tmp/goi"}
}

func TestPlanApp(t *testing.T) {
	ctx := testCtx(t)
	c := Config{Name: "srv", Type: TypeApp, Kind: KindPackage, Package: "./cmd/srv", Args: `-a "b c"`, Tags: []string{"dev", "integration"},
		BuildFlags: "-ldflags=-s", EnvFile: ".env", Env: map[string]string{"OVER": "cfg"}, InputFile: "in.txt"}
	p, err := c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantBuild := []string{"build", "-o", filepath.Join("/tmp/goi", "goi_srv"), "-tags=integration,dev", "-mod=mod", "-ldflags=-s", "."}
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
	ctx.PackageDir = func(string) (string, error) { return "/src/x", nil }
	c = Config{Name: "x", Type: TypeApp, Package: "example.com/x"}
	if p, _ := c.MakePlan(ctx); p.Build.Dir != "/src/x" {
		t.Fatalf("import path dir = %q", p.Build.Dir)
	}
}

func TestPlanTest(t *testing.T) {
	ctx := testCtx(t)
	c := Config{Name: "t", Type: TypeTest, Kind: KindDirectory, Directory: ".", Race: true, Count: 1, Short: true, Timeout: "30s", TestFlags: "-v", Args: "-update"}
	p, err := c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test", "-json", "-tags=integration", "-mod=mod", "-race", "-count=1", "-short", "-timeout=30s", "-v", "./...", "-args", "-update"}
	if !p.Test || !reflect.DeepEqual(p.Exec.Args, want) {
		t.Fatalf("args = %q", p.Exec.Args)
	}
	c = Config{Name: "f", Type: TypeTest, Kind: KindFile, Files: []string{"x_test.go"}}
	p, err = c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Exec.Args, " "); !strings.Contains(got, "-run=^(TestOne|TestTwo)$ .") {
		t.Fatalf("file kind args = %s", got)
	}
	c = Config{Name: "b", Type: TypeTest, Kind: KindFunction, Package: ".", Pattern: "^$", Bench: "^BenchmarkX$", Benchmem: true}
	p, _ = c.MakePlan(ctx)
	if got := strings.Join(p.Exec.Args, " "); !strings.Contains(got, "-run=^$ -bench=^BenchmarkX$ -benchmem .") {
		t.Fatalf("bench args = %s", got)
	}
}

func TestPlanShellAndMake(t *testing.T) {
	ctx := testCtx(t)
	p, err := Config{Name: "s", Type: TypeShell, ScriptText: "echo hi", Args: "a"}.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Exec.Dir != ctx.Root || p.Exec.Args[len(p.Exec.Args)-1] != "a" {
		t.Fatalf("shell = %+v", p.Exec)
	}
	p, err = Config{Name: "m", Type: TypeMake, Makefile: "build/Makefile", Targets: []string{"all", "lint"}}.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Exec.Path != "make" || p.Exec.Dir != filepath.Join(ctx.Root, "build") || strings.Join(p.Exec.Args, " ") != "-f "+filepath.Join(ctx.Root, "build", "Makefile")+" all lint" {
		t.Fatalf("make = %+v", p.Exec)
	}
	if _, err := (Config{Type: TypeRemote}).MakePlan(ctx); err == nil {
		t.Fatal("remote can't be run")
	}
}

func TestDebugLaunch(t *testing.T) {
	ctx := testCtx(t)
	c := Config{Name: "t", Type: TypeTest, Kind: KindFunction, Package: ".", Pattern: "^TestOne$/^sub$", Count: 1, Args: "-x"}
	l, err := c.DebugLaunch(ctx, "/tmp/bin")
	if err != nil {
		t.Fatal(err)
	}
	if l["mode"] != "test" || l["program"] != ctx.Root || l["output"] != "/tmp/bin" || l["outputMode"] != "remote" || l["buildFlags"] != "-tags=integration -mod=mod" {
		t.Fatalf("launch = %v", l)
	}
	if got := l["args"].([]string); !reflect.DeepEqual(got, []string{"-test.count=1", "-test.run=^TestOne$/^sub$", "-test.v", "-x"}) {
		t.Fatalf("args = %q", got)
	}
	if _, err := (Config{Type: TypeTest, Kind: KindDirectory, Directory: "."}).DebugLaunch(ctx, "b"); err == nil {
		t.Fatal("recursive directory can't be debugged")
	}
	a := Config{Type: TypeApp, Kind: KindFile, Files: []string{"main.go"}}
	if l, _ := a.DebugLaunch(ctx, "b"); l["program"] != filepath.Join(ctx.Root, "main.go") || l["mode"] != "debug" {
		t.Fatalf("file launch = %v", l)
	}
}

func TestValidate(t *testing.T) {
	ctx := testCtx(t)
	tests := []struct {
		c    Config
		want string // substring of the first error; "" = valid
	}{
		{Config{Name: "a", Type: TypeApp, Package: "./cmd/srv"}, ""},
		{Config{Name: "a", Type: TypeApp}, "Package is required"},
		{Config{Name: "", Type: TypeTest}, "Name is required"},
		{Config{Name: "f", Type: TypeTest, Kind: KindFile, Files: []string{"nope_test.go"}}, "does not exist"},
		{Config{Name: "f", Type: TypeTest, Kind: KindFunction}, "Test pattern is required"},
		{Config{Name: "r", Type: TypeRemote, Host: "localhost", Port: 70000}, "Port must be"},
		{Config{Name: "s", Type: TypeShell}, "script"},
		{Config{Name: "m", Type: TypeMake}, "No Makefile"},
		{Config{Name: "x", Type: TypeTest, Args: `"open`}, "Program arguments"},
		{Config{Name: "x", Type: TypeTest, BeforeLaunch: []Task{{Type: TaskConfig, Config: "X"}}}, "itself"},
		{Config{Name: "x", Type: TypeTest, WorkingDir: "missing"}, "Working directory"},
	}
	for _, tt := range tests {
		errs := tt.c.Validate(ctx.Root)
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
