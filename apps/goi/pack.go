package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/golang"
	"github.com/p4tin/goi/packs/golang/services"
)

// Pack is Goi: Goi's identity plus the Go language pack.
func Pack() pack.Pack {
	p := golang.Pack(pack.Identity{
		Name:        "Goi",
		Program:     "goi",
		Description: "A cross-platform Go IDE",
		BundleID:    "dev.goi.goi",
		// Kept from before the app packs, so an older Goi and this one still
		// share the single-instance lock.
		InstanceID:       "dev.goi.app",
		DirName:          "Goi",
		DataVersion:      "0.1",
		HomeEnv:          "GOI_HOME",
		ProjectDir:       ".goi",
		FileAssociations: []string{".go", ".mod", ".work"},
	})
	// The Go pack's own services, bound here because packs don't import
	// Wails (the bindings generator needs the concrete types).
	p.Services = func(h pack.Host) []any {
		return []any{
			application.NewService(services.NewSDKService(h)),
			application.NewService(services.NewModService(h)),
			application.NewService(services.NewProjectService(h)),
		}
	}
	return p
}
