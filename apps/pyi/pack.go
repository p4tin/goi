package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/python"
	"github.com/p4tin/goi/packs/python/services"
)

// Pack is Pyi: Pyi's identity plus the Python language pack.
func Pack() pack.Pack {
	p := python.Pack(pack.Identity{
		Name:             "Pyi",
		Program:          "pyi",
		Description:      "A cross-platform Python IDE",
		BundleID:         "dev.goi.pyi",
		DirName:          "Pyi",
		DataVersion:      "0.1",
		HomeEnv:          "PYI_HOME",
		ProjectDir:       ".pyi",
		FileAssociations: []string{".py", ".pyi"},
	})
	// The Python pack's own services, bound here because packs don't import
	// Wails (the bindings generator needs the concrete types).
	p.Services = func(h pack.Host) []any {
		return []any{
			application.NewService(services.NewInterpreterService(h)),
			application.NewService(services.NewProjectService(h)),
		}
	}
	return p
}
