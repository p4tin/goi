package gosdk

import (
	"context"
	"errors"

	"github.com/p4tin/goi/internal/settings"
)

// ErrNoSDK means no Go SDK was found.
var ErrNoSDK = errors.New("no Go SDK found; install Go or choose an SDK in settings")

// Resolve returns the SDK for settings s: go.goroot when set, otherwise the
// discovered SDK on PATH (or the newest).
func Resolve(ctx context.Context, s settings.Settings) (SDK, error) {
	if root := s.Go.GOROOT; root != "" {
		return Validate(root)
	}
	sdks := Discover(ctx)
	if len(sdks) == 0 {
		return SDK{}, ErrNoSDK
	}
	// Discover sorts the PATH SDK first, then newest.
	return sdks[0], nil
}

// ForProject returns the go command runner for settings s and project root
// ("" for none): the resolved SDK plus the go.* settings.
func ForProject(ctx context.Context, s settings.Settings, root string) (Runner, error) {
	sdk, err := Resolve(ctx, s)
	if err != nil {
		return Runner{}, err
	}
	g := s.Go
	return Runner{
		SDK: sdk,
		Env: Env{Vars: g.Env, Tags: g.Tags, GOOS: g.GOOS, GOARCH: g.GOARCH, CGO: g.CGO, Vendor: g.Vendor},
		Dir: root,
	}, nil
}

// ModFlag is the -mod flag for the go.vendor setting ("" for auto).
func ModFlag(vendor string) string {
	switch vendor {
	case "on":
		return "-mod=vendor"
	case "off":
		return "-mod=mod"
	}
	return ""
}
