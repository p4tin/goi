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

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/p4tin/goi/internal/appearance"
	"github.com/p4tin/goi/internal/applog"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/journal"
	"github.com/p4tin/goi/internal/paths"
	"github.com/p4tin/goi/internal/recent"
	"github.com/p4tin/goi/internal/runconfig"
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
	cwd, _ := os.Getwd()
	return parseArgsIn(args, cwd)
}

// parseArgsIn parses args with relative paths resolved against cwd.
func parseArgsIn(args []string, cwd string) (Options, error) {
	abs := func(p string) (string, error) {
		if !filepath.IsAbs(p) && cwd != "" {
			p = filepath.Join(cwd, p)
		}
		return filepath.Abs(p)
	}
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
		abs, err := abs(*project)
		if err != nil {
			return Options{}, fmt.Errorf("project path: %w", err)
		}
		opts.ProjectPath = abs
	}
	for _, p := range positional {
		abs, err := abs(p)
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

	ctx    context.Context
	appBus events.Bus // app-wide topics, delivered to every window
	recent *recent.Store

	winMu      sync.Mutex
	windows    map[uint]*window
	byNative   map[uint]*window // Wails window ID → window
	nextWindow uint

	// wapp is the Wails application (nil in tests); icon is the Linux
	// window icon.
	wapp *application.App
	icon []byte
	// makeWindow creates a native window and its bus; tests replace it.
	makeWindow func() (windowHandle, events.Bus)
	// quitFn quits the application; tests replace it.
	quitFn func()

	menuMu sync.Mutex
	menu   []menuTop // the menu bar, for windows opened later

	quitRequested atomic.Bool // a quit is closing the windows one by one
	quitting      atomic.Bool // every window closed; let the quit through

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
	debugSvc     *DebugService
	termSvc      *TerminalService
	gitSvc       *GitService
	lintSvc      *LintService
	modSvc       *ModService
	agentSvc     *AgentService
	mcp          *mcpHost

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
		appBus:    events.Nop{},
		windows:   map[uint]*window{},
		byNative:  map[uint]*window{},
	}
	a.quitFn = func() {
		if a.wapp != nil {
			a.wapp.Quit()
		}
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
	a.debugSvc = &DebugService{app: a}
	a.termSvc = &TerminalService{app: a}
	a.gitSvc = &GitService{app: a}
	a.lintSvc = &LintService{app: a}
	a.modSvc = &ModService{app: a}
	a.agentSvc = &AgentService{app: a}
	a.mcp = &mcpHost{app: a}
	a.debugLaunch = a.startDebug
	return a, nil
}

// services returns the services to bind to the frontend.
func (a *App) services() []application.Service {
	return []application.Service{
		application.NewService(a.appSvc),
		application.NewService(a.workspaceSvc),
		application.NewService(a.fileSvc),
		application.NewService(a.sdkSvc),
		application.NewService(a.settingsSvc),
		application.NewService(a.recentSvc),
		application.NewService(a.searchSvc),
		application.NewService(a.editorSvc),
		application.NewService(a.menuSvc),
		application.NewService(a.langSvc),
		application.NewService(a.runSvc),
		application.NewService(a.debugSvc),
		application.NewService(a.termSvc),
		application.NewService(a.gitSvc),
		application.NewService(a.lintSvc),
		application.NewService(a.modSvc),
		application.NewService(a.agentSvc),
	}
}

// Shutdown releases every window; it runs when the application exits.
func (a *App) Shutdown(context.Context) {
	for _, w := range a.allWindows() {
		a.removeWindow(w)
	}
	a.mcp.close()
	a.log.Info("shutdown")
	a.closer.Close()
}

var errNoProject = errors.New("no project is open")

// AppInfo describes the running application.
type AppInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	GoVersion   string `json:"goVersion"`
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
func (s *AppService) Info(ctx context.Context) AppInfo {
	info := AppInfo{
		Name:      "Goi",
		Version:   paths.Version,
		Build:     paths.BuildVersion,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		LogPath:   s.app.logPath,
	}
	w, err := s.app.windowFor(ctx)
	if err != nil {
		info.ProjectPath = s.app.opts.ProjectPath
	} else if sess, err := w.current(); err == nil {
		info.ProjectPath = sess.root
	}
	return info
}

// OpenFolderDialog shows the native folder picker and returns the chosen
// path, or "" if cancelled. The dialog is attached to the calling window.
func (s *AppService) OpenFolderDialog(ctx context.Context) (string, error) {
	if s.app.wapp == nil {
		return "", errNoWindow
	}
	d := s.app.wapp.Dialog.OpenFile().
		SetTitle("Open Folder").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true)
	if w, err := s.app.windowFor(ctx); err == nil {
		if h, ok := w.handle.(*nativeHandle); ok {
			d = d.AttachToWindow(h.w)
		}
	}
	return d.PromptForSingleSelection()
}

// OpenProject opens path. With newWindow, or when this window already has a
// project, it focuses the window that has path open or launches a new one.
// Otherwise it opens path in this window.
func (s *AppService) OpenProject(ctx context.Context, path string, newWindow bool) error {
	w, _ := s.app.windowFor(ctx) // no calling window: open in a new one
	if newWindow {
		return s.app.openProjectWindow(path, nil)
	}
	return s.app.openProjectWindow(path, w)
}

// CloseProject closes the project in this window and returns to Welcome.
func (s *AppService) CloseProject(ctx context.Context) error {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return err
	}
	return w.closeProject()
}

// PendingRestore returns unsaved buffers journaled before a crash of the
// previous session for the open project.
func (s *AppService) PendingRestore(ctx context.Context) ([]journal.Entry, error) {
	sess, err := s.app.sessionFor(ctx)
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
func (s *AppService) SaveLayout(ctx context.Context, layout string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.app.layoutPath(sess), []byte(layout))
}

// LoadLayout returns the saved layout JSON for the open project, or "".
func (s *AppService) LoadLayout(ctx context.Context) (string, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return "", err
	}
	return readFileOrEmpty(s.app.layoutPath(sess))
}

// Quit closes every window (each through its close flow) and quits.
func (s *AppService) Quit() {
	if s.app.wapp != nil {
		s.app.wapp.Quit()
		return
	}
	s.app.shouldQuit()
}

// NewWindow opens a Welcome window.
func (s *AppService) NewWindow() error { return s.app.openWindow("") }

// WindowID returns the calling window's id once its startup project is open,
// or 0 while it is not ready (the frontend waits for a non-zero id before it
// boots).
func (s *AppService) WindowID(ctx context.Context) uint {
	w, err := s.app.windowFor(ctx)
	if err != nil || !w.ready.Load() {
		return 0
	}
	return w.id
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
