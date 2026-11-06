package rpc

import (
	"context"
	"errors"

	"github.com/p4tin/goi/internal/gosdk"
)

// SDKService discovers and selects Go SDKs.
type SDKService struct {
	app *App
}

// Discover lists installed Go SDKs.
func (s *SDKService) Discover() []gosdk.SDK {
	sdks := gosdk.Discover(s.app.ctx)
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
func (s *SDKService) Select(goroot string) error {
	if goroot != "" {
		if _, err := gosdk.Validate(goroot); err != nil {
			return err
		}
	}
	var v any = goroot
	if goroot == "" {
		v = nil
	}
	return s.app.settings.Set(s.app.settingsLayerForProject(), "go.goroot", v)
}

var errNoSDK = errors.New("no Go SDK found; install Go or choose an SDK in settings")

func (a *App) resolveSDK(ctx context.Context) (gosdk.SDK, error) {
	if root := a.settings.Effective().Go.GOROOT; root != "" {
		return gosdk.Validate(root)
	}
	sdks := gosdk.Discover(ctx)
	if len(sdks) == 0 {
		return gosdk.SDK{}, errNoSDK
	}
	// Discover sorts the PATH SDK first, then newest.
	return sdks[0], nil
}

func (a *App) runner(ctx context.Context) (gosdk.Runner, error) {
	sdk, err := a.resolveSDK(ctx)
	if err != nil {
		return gosdk.Runner{}, err
	}
	g := a.settings.Effective().Go
	r := gosdk.Runner{
		SDK: sdk,
		Env: gosdk.Env{Vars: g.Env, Tags: g.Tags, GOOS: g.GOOS, GOARCH: g.GOARCH, CGO: g.CGO, Vendor: g.Vendor},
	}
	if sess, err := a.current(); err == nil {
		r.Dir = sess.root
	}
	return r, nil
}
