// Command goi is Goi, the Go IDE: the shared core plus the Go pack.
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/p4tin/goi/internal/boot"
)

// dist is the frontend built with `vite build --mode goi`.
//
//go:embed all:dist
var assets embed.FS

// icon is the Linux window icon.
//
//go:embed build/appicon.png
var icon []byte

// buildConfig holds info.version, the product version shown in About and
// Welcome (release builds also stamp it into paths.BuildVersion).
//
//go:embed build/config.yml
var buildConfig []byte

func main() {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		log.Fatalf("goi: %v", err)
	}
	boot.Main(boot.App{Pack: Pack(), Assets: dist, Icon: icon, BuildConfig: buildConfig})
}
