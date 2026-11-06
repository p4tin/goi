package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/p4tin/goi/internal/rpc"
)

//go:embed all:frontend/dist
var assets embed.FS

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
	})
	if err != nil {
		log.Fatalf("goi: %v", err)
	}
}
