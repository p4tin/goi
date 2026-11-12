package rpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"

	"github.com/p4tin/goi/internal/events"
)

// ShellOptions are the parts of the native shell that come from main.
type ShellOptions struct {
	Assets fs.FS
	// Icon is the Linux window icon (macOS and Windows take theirs from the
	// bundle's .icns and the exe's embedded icon.ico).
	Icon []byte
}

// Run builds the Wails application, opens the first window and runs until
// the application quits.
func (a *App) Run(opts ShellOptions) error {
	a.icon = opts.Icon
	a.wapp = application.New(application.Options{
		Name:        "Goi",
		Description: "A cross-platform Go IDE",
		Logger:      a.log,
		LogLevel:    slog.LevelWarn,
		Services:    a.services(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(opts.Assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               a.instanceID(),
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) { a.secondInstance(d.Args, d.WorkingDir) },
		},
		ShouldQuit: a.shouldQuit,
		OnShutdown: func() { a.Shutdown(context.Background()) },
		Mac: application.MacOptions{
			// The app stays running with no windows, like other Mac apps; the
			// Dock icon reopens a Welcome window.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			// Matches goi.desktop (StartupWMClass / Wayland app id), so
			// docks group the windows under the launcher's icon.
			ProgramName: "goi",
		},
		FileAssociations: []string{".go", ".mod", ".work"},
	})
	a.wapp.Event.OnApplicationEvent(wevents.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		if f := e.Context().Filename(); f != "" {
			a.OpenFile(f)
		}
	})
	a.wapp.Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.appStarted()
	})
	a.wapp.Event.OnApplicationEvent(wevents.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		a.reopen()
	})

	if serverMode {
		// Browser tabs become windows when they first call in (lazyWindow).
		if err := a.startWith(context.Background(), appBus{app: a.wapp}, nil, nil); err != nil {
			a.log.Error("startup", "err", err)
		}
		return a.wapp.Run()
	}
	a.makeWindow = func() (windowHandle, events.Bus) { return a.newNativeWindow() }
	h, bus := a.newNativeWindow()
	if err := a.startWith(context.Background(), appBus{app: a.wapp}, h, bus); err != nil {
		a.log.Error("startup", "err", err)
	}
	return a.wapp.Run()
}

// callerWindow returns the window of a bound call's Wails context.
func (a *App) callerWindow(ctx context.Context) *window {
	if nw, ok := ctx.Value(application.WindowKey).(application.Window); ok && nw != nil {
		return a.nativeWindow(nw)
	}
	return nil
}

// instanceID identifies the single Goi process for this config directory, so
// separate GOI_HOME profiles run separately.
func (a *App) instanceID() string {
	sum := sha256.Sum256([]byte(a.configDir))
	return "dev.goi.app." + hex.EncodeToString(sum[:6])
}

// cascade is the offset of a new window from the focused one.
const cascade = 28

// newNativeWindow creates a Wails window and its handle and bus. The window
// is registered with the app by addWindow, which binds its native hooks.
func (a *App) newNativeWindow() (windowHandle, events.Bus) {
	opts := application.WebviewWindowOptions{
		Title:            "Goi",
		Width:            1280,
		Height:           800,
		MinWidth:         640,
		MinHeight:        400,
		BackgroundColour: application.NewRGB(30, 31, 34),
		URL:              "/",
		Linux:            application.LinuxWindow{Icon: a.icon},
	}
	if cur := a.wapp.Window.Current(); cur != nil {
		x, y := cur.Position()
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = x+cascade, y+cascade
	}
	ww := a.wapp.Window.NewWithOptions(opts)
	h := &nativeHandle{w: ww}
	return h, newWindowBus(ww)
}

// bindNative connects a registered window to its native window's events.
func (a *App) bindNative(w *window, h *nativeHandle) {
	if a.wapp == nil {
		return
	}
	a.winMu.Lock()
	a.byNative[h.w.ID()] = w
	a.winMu.Unlock()
	h.w.RegisterHook(wevents.Common.WindowClosing, func(e *application.WindowEvent) {
		if w.beforeClose() {
			e.Cancel()
			return
		}
		a.windowClosed(w)
	})
	h.w.OnWindowEvent(wevents.Common.WindowFocus, func(*application.WindowEvent) { a.refreshDock() })
}

// nativeWindow returns the registered window for a Wails window, registering
// browser clients in server mode on first use.
func (a *App) nativeWindow(nw application.Window) *window {
	a.winMu.Lock()
	w := a.byNative[nw.ID()]
	a.winMu.Unlock()
	if w != nil {
		return w
	}
	return lazyWindow(a, nw)
}

// focusedWindow returns the window the user is working in: the native
// focused window, else the most recently created one.
func (a *App) focusedWindow() *window {
	if a.wapp != nil {
		if cur := a.wapp.Window.Current(); cur != nil {
			if w := a.nativeWindow(cur); w != nil {
				return w
			}
		}
	}
	ws := a.allWindows()
	if len(ws) == 0 {
		return nil
	}
	return ws[len(ws)-1]
}

// nativeHandle is the windowHandle of a Wails window.
type nativeHandle struct {
	w *application.WebviewWindow
}

func (h *nativeHandle) Focus() {
	if h.w.IsMinimised() {
		h.w.UnMinimise()
	}
	h.w.Show()
	h.w.Focus()
}

func (h *nativeHandle) SetTitle(title string) { h.w.SetTitle(title) }

func (h *nativeHandle) Close() { h.w.Close() }

func (h *nativeHandle) Window() application.Window { return h.w }

// appBus delivers app-wide topics to every window.
type appBus struct {
	app *application.App
}

func (b appBus) Emit(topic string, payload any) {
	b.app.Event.Emit(topic, payload)
}

// isMac reports whether the platform keeps apps running without windows.
func isMac() bool { return runtime.GOOS == "darwin" }
