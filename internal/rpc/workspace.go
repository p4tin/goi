package rpc

import (
	"context"
	"time"

	"github.com/p4tin/goi/internal/gosdk"
	"github.com/p4tin/goi/internal/workspace"
)

// WorkspaceService exposes the open project model.
type WorkspaceService struct {
	app *App
}

// Project returns the open project, or nil on the Welcome screen.
func (s *WorkspaceService) Project(ctx context.Context) *workspace.Project {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil
	}
	p := sess.manager.Project()
	return &p
}

// AddContentRoot adds an extra top-level folder to the project.
func (s *WorkspaceService) AddContentRoot(ctx context.Context, path string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.AddContentRoot(path)
}

// RemoveContentRoot removes an extra top-level folder.
func (s *WorkspaceService) RemoveContentRoot(ctx context.Context, path string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.RemoveContentRoot(path)
}

// SetExcludes replaces the project's exclude globs.
func (s *WorkspaceService) SetExcludes(ctx context.Context, globs []string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.SetExcludes(globs)
}

// SDKStatus reports which Go SDK the project uses.
type SDKStatus struct {
	OK      bool      `json:"ok"`
	SDK     gosdk.SDK `json:"sdk"`
	Problem string    `json:"problem"`
}

// SDKStatus resolves the configured SDK: go.goroot when set, otherwise the
// only discovered SDK, otherwise the one on PATH.
func (s *WorkspaceService) SDKStatus(ctx context.Context) SDKStatus {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return SDKStatus{Problem: err.Error()}
	}
	sdk, err := w.resolveSDK(s.app.ctx)
	if err != nil {
		return SDKStatus{Problem: err.Error()}
	}
	return SDKStatus{OK: true, SDK: sdk}
}

// GoEnvSummary is the effective target shown in the status bar.
type GoEnvSummary struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GOVERSION string `json:"goversion"`
	Cross     bool   `json:"cross"`
}

// GoEnv runs `go env -json` with the project environment.
func (s *WorkspaceService) GoEnv(ctx context.Context) (map[string]string, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return nil, err
	}
	r, err := w.runner(s.app.ctx)
	if err != nil {
		return nil, err
	}
	tctx, cancel := context.WithTimeout(s.app.ctx, 10*time.Second)
	defer cancel()
	return r.GoEnv(tctx)
}

// GoEnvSummary returns GOOS/GOARCH/GOVERSION for the status bar.
func (s *WorkspaceService) GoEnvSummary(ctx context.Context) (GoEnvSummary, error) {
	env, err := s.GoEnv(ctx)
	if err != nil {
		return GoEnvSummary{}, err
	}
	return GoEnvSummary{
		GOOS:      env["GOOS"],
		GOARCH:    env["GOARCH"],
		GOVERSION: env["GOVERSION"],
		Cross:     gosdk.IsCrossTarget(env),
	}, nil
}
