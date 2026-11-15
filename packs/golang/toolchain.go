package golang

import (
	"context"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/golang/gosdk"
)

// toolchain is the Go SDK as a pack.Toolchain.
type toolchain struct{}

// Env is the go command environment for the project's SDK and go.* settings.
func (toolchain) Env(ctx context.Context, p pack.ProjectEnv) ([]string, error) {
	r, err := gosdk.ForProject(ctx, p.Settings, p.Root)
	if err != nil {
		return nil, err
	}
	return r.Environ(), nil
}
