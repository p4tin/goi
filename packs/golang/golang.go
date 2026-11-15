// Package golang is the Go language pack: project detection, the Go SDK,
// gopls, gofmt/goimports, golangci-lint, Go run configurations and Delve.
// apps/goi combines it with Goi's identity.
package golang

import (
	"os"
	"path/filepath"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/golang/godetect"
	"github.com/p4tin/goi/packs/golang/gopls"
)

// Pack returns the Go capabilities with identity id.
func Pack(id pack.Identity) pack.Pack {
	return pack.Pack{
		Identity:  id,
		Project:   godetect.Detector{GOPATH: filepath.SplitList(os.Getenv("GOPATH"))},
		Toolchain: toolchain{},
		Server:    gopls.Server{},
		Formatter: formatter{},
		Linter:    linter{},
		Run:       run{},
		Debug:     delve{},
	}
}
