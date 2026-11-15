package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/p4tin/goi/internal/lint"
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/toolbin"
	"github.com/p4tin/goi/internal/workspace"
	"github.com/p4tin/goi/packs/golang/golangci"
	"github.com/p4tin/goi/packs/golang/gosdk"
)

// linter is golangci-lint as a pack.Linter.
type linter struct{}

func (linter) Name() string                     { return golangci.Name }
func (linter) Enabled(s settings.Settings) bool { return s.Lint.Enabled }
func (linter) InstallHint() string              { return "go install " + golangci.Module }

// goEnvPaths returns GOBIN and GOPATH of the project's go env.
func goEnvPaths(ctx context.Context, p pack.ProjectEnv) (gobin, gopath string) {
	if r, err := gosdk.ForProject(ctx, p.Settings, p.Root); err == nil {
		if ge, err := r.GoEnv(ctx); err == nil {
			return ge["GOBIN"], ge["GOPATH"]
		}
	}
	return "", ""
}

// Resolve finds golangci-lint: lint.golangciPath, PATH, GOBIN, GOPATH/bin,
// then the tools dir.
func (linter) Resolve(ctx context.Context, p pack.ProjectEnv, toolsDir string) (string, error) {
	opts := toolbin.Options{Setting: p.Settings.Lint.GolangciPath, ToolsDir: toolsDir}
	opts.GOBIN, opts.GOPATH = goEnvPaths(ctx, p)
	return golangci.Resolve(opts)
}

// Version reports "v1.x.y" / "v2.x.y".
func (linter) Version(ctx context.Context, bin string) (string, error) {
	_, v, err := golangci.Version(ctx, bin)
	return v, err
}

// Jobs lints every module ("project") or the package containing path.
func (linter) Jobs(scope, path string, p workspace.Project) ([]pack.LintJob, error) {
	var jobs []pack.LintJob
	if scope == "project" {
		for _, m := range p.Modules {
			jobs = append(jobs, pack.LintJob{Dir: m.Dir, Targets: []string{"./..."}, Covered: m.Dir, Recursive: true})
		}
		if len(jobs) == 0 {
			if md := moduleDirFor(p.Root, p.Root); md != "" {
				jobs = append(jobs, pack.LintJob{Dir: md, Targets: []string{"./..."}, Covered: md, Recursive: true})
			}
		}
		if len(jobs) == 0 {
			return nil, errors.New("no Go module in the project")
		}
		return jobs, nil
	}
	pkgDir := path
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		pkgDir = filepath.Dir(path)
	}
	md := moduleDirFor(pkgDir, p.Root)
	if md == "" {
		return nil, errors.New("no go.mod above " + pkgDir)
	}
	rel, _ := filepath.Rel(md, pkgDir)
	target := "./" + filepath.ToSlash(rel)
	if rel == "." {
		target = "."
	}
	return []pack.LintJob{{Dir: md, Targets: []string{target}, Covered: pkgDir}}, nil
}

// Run runs golangci-lint for one job with the lint.args setting.
func (linter) Run(ctx context.Context, bin string, job pack.LintJob, env []string, s settings.Settings) ([]lint.Issue, error) {
	major, _, err := golangci.Version(ctx, bin)
	if err != nil {
		return nil, err
	}
	return golangci.Run(ctx, golangci.Spec{Bin: bin, Major: major, Dir: job.Dir, Env: env, Targets: job.Targets, Extra: s.Lint.Args})
}

// Install runs `go install` for golangci-lint with the project's SDK.
func (linter) Install(ctx context.Context, p pack.ProjectEnv, toolsDir string) (string, error) {
	r, err := gosdk.ForProject(ctx, p.Settings, p.Root)
	if err != nil {
		return "", err
	}
	return toolbin.Install(ctx, r.SDK.GoBin, r.Environ(), toolsDir, golangci.Module, golangci.Name)
}

// moduleDirFor finds the module directory containing path (nearest go.mod).
func moduleDirFor(path, root string) string {
	d := path
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		d = filepath.Dir(path)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d || !within(root, parent) && parent != root {
			return ""
		}
		d = parent
	}
}
