package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/p4tin/goi/internal/mcp"
	"github.com/p4tin/goi/internal/paths"
	"github.com/p4tin/goi/internal/rpc"
	"github.com/p4tin/goi/internal/shellenv"
)

//go:embed all:frontend/dist
var assets embed.FS

// icon is the Linux window icon (macOS and Windows take theirs from the
// bundle's .icns and the exe's embedded icon.ico).
//
//go:embed build/appicon.png
var icon []byte

// buildConfig holds info.version, the product version shown in About and
// Welcome (release builds also stamp it into paths.BuildVersion).
//
//go:embed build/config.yml
var buildConfig []byte

func main() {
	if v := paths.ConfigVersion(buildConfig); v != "" {
		paths.ProductVersion = v
	}
	// `goi mcp-bridge` relays MCP over stdio for agents; no GUI.
	if len(os.Args) > 1 && os.Args[1] == "mcp-bridge" {
		os.Exit(mcp.RunBridge(os.Args[2:]))
	}
	// Started from the Dock/Finder (or a desktop launcher), Goi has launchd's
	// minimal PATH: take the login shell's, so npm, node and the user's tools
	// are found by Goi and by the programs it runs.
	if shellenv.Needed(runtime.GOOS) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := shellenv.Fix(ctx, shellenv.Shell(runtime.GOOS)); err != nil {
			log.Printf("goi: keeping the launch PATH: %v", err)
		}
		cancel()
	}
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
