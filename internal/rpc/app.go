package rpc

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/logger"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/p4tin/goi/internal/appearance"
	"github.com/p4tin/goi/internal/applog"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/journal"
	"github.com/p4tin/goi/internal/paths"
	"github.com/p4tin/goi/internal/recent"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

// Options are the parsed command-line options.
type Options struct {
	ProjectPath string
	// Files are files to open (positional arguments, e.g. from an OS file
	// association on Windows/Linux).
	Files []string
}

// ParseArgs parses Goi's command-line arguments. Unknown flags (for example
// ones injected by the OS launcher) are ignored rather than fatal.
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("goi", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "project folder to open")
	var known, positional []string
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
		} else if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
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
	for _, p := range positional {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			if opts.ProjectPath == "" {
				opts.ProjectPath = abs
			}
		} else if err == nil {
			opts.Files = append(opts.Files, abs)
		}
	}
	return opts, nil
}

// App is the core lifecycle owner. It is not bound to the frontend; the
// services it returns from Services are.
type App struct {
	startedUp    atomic.Bool
	pendingMu    sync.Mutex
	pendingFiles []string
	opts         Options
	log          *slog.Logger
	closer       io.Closer
	logPath      string
	configDir    string
	cacheDir     string

	ctx      context.Context
	bus      events.Bus
	settings *settings.Store
	recent   *recent.Store
	cfgWatch vfs.Watcher // watches settings files; no content roots

	mu      sync.Mutex
	session *session

	// focusFn brings this window forward when another process asks; set in
	// Startup (tests override it).
	focusFn func()
	// titleFn sets the window title; set in Startup (tests override it).
	titleFn func(string)

	closing closeState
	// quitAfterStart is set when the requested project is already open in
	// another window, which was focused instead.
	quitAfterStart bool

	appSvc       *AppService
	workspaceSvc *WorkspaceService
	fileSvc      *FileService
	sdkSvc       *SDKService
	settingsSvc  *SettingsService
	recentSvc    *RecentService
	searchSvc    *SearchService
	editorSvc    *EditorService
	menuSvc      *MenuService
	langSvc      *LangService
	runSvc       *RunService

	// debugLaunch starts a debug session for a run whose before-launch
	// tasks finished (set by the debugger integration).
	debugLaunch func(r *runInstance, pctx runconfig.Context)
}

// NewApp initialises logging and constructs the bound services. It runs
// before the Wails window exists.
func NewApp(opts Options) (*App, error) {
	configDir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	cacheDir, err := paths.CacheDir()
	if err != nil {
		return nil, err
	}
	logDir := filepath.Join(cacheDir, "logs")
	log, closer, err := applog.New(logDir)
	if err != nil {
		return nil, err
	}
	a := &App{
		opts:      opts,
		log:       log,
		closer:    closer,
		logPath:   filepath.Join(logDir, applog.FileName),
		configDir: configDir,
		cacheDir:  cacheDir,
		ctx:       context.Background(),
		bus:       events.Nop{},
	}
	a.appSvc = &AppService{app: a}
	a.workspaceSvc = &WorkspaceService{app: a}
	a.fileSvc = &FileService{app: a}
	a.sdkSvc = &SDKService{app: a}
	a.settingsSvc = &SettingsService{app: a}
	a.recentSvc = &RecentService{app: a}
	a.searchSvc = &SearchService{app: a}
	a.editorSvc = &EditorService{app: a}
	a.menuSvc = &MenuService{app: a}
	a.langSvc = &LangService{app: a}
	a.runSvc = &RunService{app: a}
	return a, nil
}

// Logger returns the Wails logger adapter.
func (a *App) Logger() logger.Logger { return wailsLogger{log: a.log} }

// Services returns the structs to bind to the frontend.
func (a *App) Services() []any {
	return []any{a.appSvc, a.workspaceSvc, a.fileSvc, a.sdkSvc, a.settingsSvc, a.recentSvc, a.searchSvc, a.editorSvc, a.menuSvc, a.langSvc, a.runSvc}
}

// Startup is the Wails OnStartup hook.
func (a *App) Startup(ctx context.Context) {
	a.focusFn = func() {
		wruntime.WindowUnminimise(ctx)
		wruntime.WindowShow(ctx)
	}
	a.titleFn = func(title string) { wruntime.WindowSetTitle(ctx, title) }
	if err := a.start(ctx, wailsBus{ctx: ctx}); err != nil {
		a.log.Error("startup", "err", err)
	}
	if a.quitAfterStart {
		a.log.Info("project already open in another window; focused it and quitting")
		wruntime.Quit(ctx)
	}
}

// start performs startup with an explicit bus (tests use a Recorder).
func (a *App) start(ctx context.Context, bus events.Bus) error {
	a.ctx, a.bus = ctx, bus
	a.log.Info("startup", "version", paths.Version, "os", runtime.GOOS, "arch", runtime.GOARCH, "project", a.opts.ProjectPath)

	st, err := settings.Load(filepath.Join(a.configDir, "settings.toml"), "", bus)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	a.settings = st
	if a.recent, err = recent.Load(filepath.Join(a.configDir, "recent.json")); err != nil {
		return fmt.Errorf("recent: %w", err)
	}
	if a.cfgWatch, err = vfs.NewWatcher(nil, vfs.MatchNone, events.Nop{}); err != nil {
		a.log.Warn("config watcher", "err", err)
	} else if _, err := st.Watch(a.cfgWatch); err != nil {
		a.log.Warn("watch settings", "err", err)
	}

	target := a.opts.ProjectPath
	if target == "" && len(a.opts.Files) > 0 {
		target = ProjectRootFor(a.opts.Files[0])
	}
	if target == "" && st.Effective().Startup.ReopenLastProject {
		if list, err := a.recent.List(); err == nil && len(list) > 0 && list[0].Exists {
			target = list[0].Path
		}
	}
	if target != "" {
		if err := a.openProject(target); errors.Is(err, ErrOpenElsewhere) {
			a.quitAfterStart = a.opts.ProjectPath != ""
		} else if err != nil {
			a.log.Warn("open project at startup", "path", target, "err", err)
			a.notify("error", "Cannot open project", err.Error())
		}
	}
	a.startedUp.Store(true)
	for _, f := range append(a.opts.Files, a.drainPendingFiles()...) {
		a.OpenFile(f)
	}
	return nil
}

// Shutdown is the Wails OnShutdown hook.
func (a *App) Shutdown(context.Context) {
	a.mu.Lock()
	s := a.session
	a.session = nil
	a.mu.Unlock()
	if s != nil {
		s.close(true)
	}
	if a.cfgWatch != nil {
		a.cfgWatch.Close()
	}
	a.log.Info("shutdown")
	a.closer.Close()
}

// current returns the open session or an error when none is open.
func (a *App) current() (*session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == nil {
		return nil, errNoProject
	}
	return a.session, nil
}

var errNoProject = errors.New("no project is open")

func (a *App) notify(level, title, body string) {
	a.bus.Emit(events.TopicNotify, events.Notification{ID: title, Level: level, Title: title, Body: body})
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

// Info returns static application information. ProjectPath is the project
// open in this window, if any.
func (s *AppService) Info() AppInfo {
	info := AppInfo{
		Name:    "Goi",
		Version: paths.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		LogPath: s.app.logPath,
	}
	if sess, err := s.app.current(); err == nil {
		info.ProjectPath = sess.root
	} else {
		info.ProjectPath = s.app.opts.ProjectPath
	}
	return info
}

// OpenFolderDialog shows the native folder picker and returns the chosen
// path, or "" if cancelled.
func (s *AppService) OpenFolderDialog() (string, error) {
	return wruntime.OpenDirectoryDialog(s.app.ctx, wruntime.OpenDialogOptions{
		Title:                "Open Folder",
		CanCreateDirectories: true,
	})
}

// OpenProject opens path. With newWindow, or when this window already has a
// project, it focuses the window that has path open or launches a new one.
// Otherwise it opens path in this window.
func (s *AppService) OpenProject(path string, newWindow bool) error {
	_, err := s.app.current()
	if newWindow || err == nil {
		return s.app.openInOtherWindow(path)
	}
	return s.app.openProject(path)
}

// CloseProject closes the project in this window and returns to Welcome.
func (s *AppService) CloseProject() error {
	return s.app.closeProject()
}

// PendingRestore returns unsaved buffers journaled before a crash of the
// previous session for the open project.
func (s *AppService) PendingRestore() ([]journal.Entry, error) {
	sess, err := s.app.current()
	if err != nil || !sess.crashed {
		return []journal.Entry{}, nil
	}
	entries, err := sess.journal.Pending()
	if entries == nil {
		entries = []journal.Entry{}
	}
	return entries, err
}

// SaveLayout persists the window layout JSON for the open project.
func (s *AppService) SaveLayout(layout string) error {
	sess, err := s.app.current()
	if err != nil {
		return err
	}
	return writeFileAtomic(s.app.layoutPath(sess), []byte(layout))
}

// LoadLayout returns the saved layout JSON for the open project, or "".
func (s *AppService) LoadLayout() (string, error) {
	sess, err := s.app.current()
	if err != nil {
		return "", err
	}
	return readFileOrEmpty(s.app.layoutPath(sess))
}

// Quit closes the application.
func (s *AppService) Quit() {
	wruntime.Quit(s.app.ctx)
}

// setAppearance applies the native window appearance; tests replace it.
var setAppearance = appearance.Set

// SetAppearance matches the native window chrome to the UI theme setting
// ("auto", "light", "dark", "hc"). Wails' WindowSet*Theme are no-ops on macOS.
func (s *AppService) SetAppearance(theme string) {
	switch theme {
	case "light":
		setAppearance(appearance.Light)
	case "dark", "hc":
		setAppearance(appearance.Dark)
	default:
		setAppearance(appearance.System)
	}
}
