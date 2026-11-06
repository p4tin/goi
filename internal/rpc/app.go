package rpc

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/logger"

	"github.com/p4tin/goi/internal/applog"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/paths"
)

// Options are the parsed command-line options.
type Options struct {
	ProjectPath string
}

// ParseArgs parses Goi's command-line arguments. Unknown flags (for example
// ones injected by the OS launcher) are ignored rather than fatal.
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("goi", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "project folder to open")
	var known []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--project" || a == "-project" {
			known = append(known, a)
			if i+1 < len(args) {
				known = append(known, args[i+1])
				i++
			}
		} else if strings.HasPrefix(a, "--project=") || strings.HasPrefix(a, "-project=") {
			known = append(known, a)
		}
	}
	if err := fs.Parse(known); err != nil {
		return Options{}, fmt.Errorf("parse args: %w", err)
	}
	opts := Options{}
	if *project != "" {
		abs, err := filepath.Abs(*project)
		if err != nil {
			return Options{}, fmt.Errorf("project path: %w", err)
		}
		opts.ProjectPath = abs
	}
	return opts, nil
}

// App is the core lifecycle owner. It is not bound to the frontend; the
// services it returns from Services are.
type App struct {
	opts    Options
	log     *slog.Logger
	closer  io.Closer
	logPath string
	bus     events.Bus

	appSvc *AppService
}

// NewApp initialises logging and constructs the bound services. It runs
// before the Wails window exists.
func NewApp(opts Options) (*App, error) {
	dir, err := paths.LogsDir()
	if err != nil {
		return nil, err
	}
	log, closer, err := applog.New(dir)
	if err != nil {
		return nil, err
	}
	a := &App{
		opts:    opts,
		log:     log,
		closer:  closer,
		logPath: filepath.Join(dir, applog.FileName),
		bus:     events.Nop{},
	}
	a.appSvc = &AppService{app: a}
	return a, nil
}

// Logger returns the Wails logger adapter.
func (a *App) Logger() logger.Logger { return wailsLogger{log: a.log} }

// Services returns the structs to bind to the frontend.
func (a *App) Services() []any { return []any{a.appSvc} }

// Startup is the Wails OnStartup hook.
func (a *App) Startup(ctx context.Context) {
	a.bus = wailsBus{ctx: ctx}
	a.log.Info("startup", "version", paths.Version, "os", runtime.GOOS, "arch", runtime.GOARCH, "project", a.opts.ProjectPath)
}

// Shutdown is the Wails OnShutdown hook.
func (a *App) Shutdown(context.Context) {
	a.log.Info("shutdown")
	a.closer.Close()
}

// AppInfo describes the running application.
type AppInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	ProjectPath string `json:"projectPath"`
	LogPath     string `json:"logPath"`
}

// AppService is the frontend-bound application service.
type AppService struct {
	app *App
}

// Info returns static application information.
func (s *AppService) Info() AppInfo {
	return AppInfo{
		Name:        "Goi",
		Version:     paths.Version,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		ProjectPath: s.app.opts.ProjectPath,
		LogPath:     s.app.logPath,
	}
}
