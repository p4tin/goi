package golang

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/packs/golang/gorun"
	"github.com/p4tin/goi/packs/golang/gosdk"
	"github.com/p4tin/goi/packs/golang/gosyntax"
)

// run is the Go run configuration support.
type run struct{}

func (run) Types() runconfig.Types                  { return gorun.Types() }
func (run) Tasks() map[string]runconfig.TaskHandler { return gorun.Tasks() }

// IsTest reports Go Test configurations (their output is go test -json).
func (run) IsTest(c runconfig.Config) bool { return c.Type == gorun.TypeTest }

// Context resolves the SDK and fills the go binary, environment, build tags,
// -mod flag and import path resolution.
func (run) Context(ctx context.Context, p pack.ProjectEnv, tempDir string) (runconfig.Context, error) {
	r, err := gosdk.ForProject(ctx, p.Settings, p.Root)
	if err != nil {
		return runconfig.Context{}, fmt.Errorf("Go SDK: %w", err)
	}
	g := p.Settings.Go
	return runconfig.Context{
		Root:    p.Root,
		Tool:    r.SDK.GoBin,
		Env:     r.Environ(),
		TempDir: tempDir,
		Types:   gorun.Types(),
		Extra: gorun.GoContext{
			Tags:    g.Tags,
			ModFlag: gosdk.ModFlag(g.Vendor),
			PackageDir: func(pkg string) (string, error) {
				c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				out, err := r.Command(c, "list", "-f", "{{.Dir}}", pkg).Output()
				if err != nil {
					return "", err
				}
				return strings.TrimSpace(string(out)), nil
			},
		},
	}, nil
}

// Markers returns the runnable functions (main, tests, benchmarks, fuzz
// targets, examples) of a Go file.
func (run) Markers(path, content string) []pack.RunMarker {
	if !strings.HasSuffix(path, ".go") {
		return nil
	}
	ms := gosyntax.RunMarkers(path, content)
	out := make([]pack.RunMarker, len(ms))
	for i, m := range ms {
		out[i] = pack.RunMarker{Kind: string(m.Kind), Name: m.Name, Line: m.Line}
	}
	return out
}

// ContextConfig builds the temporary Go Build / Go Test configuration for a
// gutter marker, a file or a directory.
func (run) ContextConfig(req pack.ContextRequest, rel func(string) string) (runconfig.Config, error) {
	path := req.Path
	if path == "" {
		return runconfig.Config{}, errors.New("no file")
	}
	dir := path
	if req.Kind != "dir" {
		dir = filepath.Dir(path)
	}
	relDir := rel(dir)
	c := runconfig.Config{Type: gorun.TypeTest, Kind: gorun.KindFunction, Directory: relDir}
	switch req.Kind {
	case "main":
		c = runconfig.Config{Name: "go build " + relDir, Type: gorun.TypeApp, Kind: gorun.KindPackage, Package: relDir}
	case "test", "example", "fuzz":
		c.Name = req.Name + " in " + relDir
		c.Pattern = "^" + req.Name + "$"
	case "bench":
		c.Name = req.Name + " in " + relDir
		c.Pattern = "^$"
		c.Bench = "^" + req.Name + "$"
	case "file":
		c.Name = filepath.Base(path)
		c.Kind = gorun.KindFile
		c.Files = []string{rel(path)}
		c.Directory = ""
	case "dir":
		c.Name = "go test " + relDir
		c.Kind = gorun.KindDirectory
	default:
		return runconfig.Config{}, fmt.Errorf("unknown context kind %q", req.Kind)
	}
	return c, nil
}
