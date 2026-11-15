// Package boot is the shared main of every app built on the core: it
// installs the pack's identity, handles `<program> mcp-bridge`, fixes the
// PATH of GUI launches and runs the application.
package boot

import (
	"context"
	"io/fs"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/p4tin/goi/internal/mcp"
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/paths"
	"github.com/p4tin/goi/internal/product"
	"github.com/p4tin/goi/internal/rpc"
	"github.com/p4tin/goi/internal/shellenv"
)

// App is what an app's main package passes to Main.
type App struct {
	Pack pack.Pack
	// Assets is the built frontend (the app's embedded dist directory).
	Assets fs.FS
	// Icon is the Linux window icon (macOS and Windows take theirs from the
	// bundle's .icns and the exe's embedded icon.ico).
	Icon []byte
	// BuildConfig is the app's build/config.yml; its info.version is the
	// product version shown in About and Welcome.
	BuildConfig []byte
}

// Main runs the app and never returns.
func Main(a App) {
	id := a.Pack.Identity
	if v := paths.ConfigVersion(a.BuildConfig); v != "" {
		id.Version = v
	}
	product.Set(id)
	a.Pack.Identity = id
	prefix := id.Program + ": "
	// `<program> mcp-bridge` relays MCP over stdio for agents; no GUI.
	if len(os.Args) > 1 && os.Args[1] == "mcp-bridge" {
		os.Exit(mcp.RunBridge(os.Args[2:]))
	}
	// Started from the Dock/Finder (or a desktop launcher), the app has
	// launchd's minimal PATH: take the login shell's, so npm, node and the
	// user's tools are found by the app and by the programs it runs.
	if shellenv.Needed(runtime.GOOS) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := shellenv.Fix(ctx, shellenv.Shell(runtime.GOOS)); err != nil {
			log.Printf("%skeeping the launch PATH: %v", prefix, err)
		}
		cancel()
	}
	opts, err := rpc.ParseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("%s%v", prefix, err)
	}
	app, err := rpc.NewApp(opts, a.Pack)
	if err != nil {
		log.Fatalf("%s%v", prefix, err)
	}
	if err := app.Run(rpc.ShellOptions{Assets: a.Assets, Icon: a.Icon}); err != nil {
		log.Fatalf("%s%v", prefix, err)
	}
	os.Exit(0)
}
