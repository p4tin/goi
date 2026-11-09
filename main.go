package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/p4tin/goi/internal/rpc"
)

//go:embed all:frontend/dist
var assets embed.FS

// icon is the Linux window icon (macOS and Windows take theirs from the
// bundle's .icns and the exe's embedded icon.ico).
//
//go:embed build/appicon.png
var icon []byte

func main() {
	opts, err := rpc.ParseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("goi: %v", err)
	}
	app, err := rpc.NewApp(opts)
	if err != nil {
		log.Fatalf("goi: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "Goi",
		Width:     1280,
		Height:    800,
		MinWidth:  640,
		MinHeight: 400,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 31, B: 34, A: 1},
		Logger:           app.Logger(),
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		OnBeforeClose:    app.BeforeClose,
		Bind:             app.Services(),
		Mac: &mac.Options{
			OnFileOpen: app.OpenFile,
		},
		Linux: &linux.Options{
			Icon: icon,
			// Matches goi.desktop (StartupWMClass / Wayland app id), so
			// docks group the window under the launcher's icon.
			ProgramName: "goi",
		},
	})
	if err != nil {
		log.Fatalf("goi: %v", err)
	}
}
