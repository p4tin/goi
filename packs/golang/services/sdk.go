// Package services holds Goi's own bound services: Go SDK selection and
// go env, go.mod completion and new Go projects. They are plain structs on
// the core's pack.Host; apps/goi binds them (packs don't import Wails).
package services

import (
	"context"
	"time"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/golang/gosdk"
)

// SDKService discovers and selects Go SDKs and reports the go env.
type SDKService struct {
	h pack.Host
}

// NewSDKService returns the SDK service.
func NewSDKService(h pack.Host) *SDKService { return &SDKService{h: h} }

// Discover lists installed Go SDKs.
func (s *SDKService) Discover() []gosdk.SDK {
	sdks := gosdk.Discover(s.h.Background())
	if sdks == nil {
		return []gosdk.SDK{}
	}
	return sdks
}

// Validate checks that goroot holds a Go SDK.
func (s *SDKService) Validate(goroot string) (gosdk.SDK, error) {
	return gosdk.Validate(goroot)
}

// Select stores goroot as the SDK: in project settings when a project is
// open, otherwise in user settings. An empty goroot clears the choice.
func (s *SDKService) Select(ctx context.Context, goroot string) error {
	if goroot != "" {
		if _, err := gosdk.Validate(goroot); err != nil {
			return err
		}
	}
	var v any = goroot
	if goroot == "" {
		v = nil
	}
	return s.h.SetSetting(ctx, "go.goroot", v)
}

// SDKStatus reports which Go SDK the project uses.
type SDKStatus struct {
	OK      bool      `json:"ok"`
	SDK     gosdk.SDK `json:"sdk"`
	Problem string    `json:"problem"`
}

// Status resolves the configured SDK: go.goroot when set, otherwise the
// SDK on PATH (or the newest discovered).
func (s *SDKService) Status(ctx context.Context) SDKStatus {
	st, err := s.h.Settings(ctx)
	if err != nil {
		return SDKStatus{Problem: err.Error()}
	}
	sdk, err := gosdk.Resolve(s.h.Background(), st)
	if err != nil {
		return SDKStatus{Problem: err.Error()}
	}
	return SDKStatus{OK: true, SDK: sdk}
}

// runner is the go command runner of the calling window.
func (s *SDKService) runner(ctx context.Context) (gosdk.Runner, error) {
	st, err := s.h.Settings(ctx)
	if err != nil {
		return gosdk.Runner{}, err
	}
	root := ""
	if sess, err := s.h.Session(ctx); err == nil {
		root = sess.Root()
	}
	return gosdk.ForProject(s.h.Background(), st, root)
}

// GoEnv runs `go env -json` with the project environment.
func (s *SDKService) GoEnv(ctx context.Context) (map[string]string, error) {
	r, err := s.runner(ctx)
	if err != nil {
		return nil, err
	}
	tctx, cancel := context.WithTimeout(s.h.Background(), 10*time.Second)
	defer cancel()
	return r.GoEnv(tctx)
}

// GoEnvSummary is the effective target shown in the status bar.
type GoEnvSummary struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GOVERSION string `json:"goversion"`
	Cross     bool   `json:"cross"`
}

// GoEnvSummary returns GOOS/GOARCH/GOVERSION for the status bar.
func (s *SDKService) GoEnvSummary(ctx context.Context) (GoEnvSummary, error) {
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
