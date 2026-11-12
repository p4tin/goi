package main

import (
	"embed"
	"io/fs"
	"log"
	"os"

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
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("goi: %v", err)
	}
	if err := app.Run(rpc.ShellOptions{Assets: dist, Icon: icon}); err != nil {
		log.Fatalf("goi: %v", err)
	}
}
