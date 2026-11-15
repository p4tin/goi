package main

import "github.com/p4tin/goi/internal/pack"

// Pack is Goi: the Go application pack.
func Pack() pack.Pack {
	return pack.Pack{
		Identity: pack.Identity{
			Name:        "Goi",
			Program:     "goi",
			Description: "A cross-platform Go IDE",
			BundleID:    "dev.goi.goi",
			// Kept from before the app packs, so an older Goi and this one
			// still share the single-instance lock.
			InstanceID:       "dev.goi.app",
			DirName:          "Goi",
			DataVersion:      "0.1",
			HomeEnv:          "GOI_HOME",
			ProjectDir:       ".goi",
			FileAssociations: []string{".go", ".mod", ".work"},
		},
	}
}
