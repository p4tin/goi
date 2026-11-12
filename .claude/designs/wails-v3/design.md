# Wails v3: one process, many windows — Design

Implements `requirements.md`. The target is Wails **v3.1.x** (`github.com/wailsapp/wails/v3`, latest stable at time of writing: v3.1.0).

## 1. What v3 gives us (checked in the v3.1.0 source)

| Need | v3.1.0 API | Notes |
|---|---|---|
| Many windows in one process | `app.Window.NewWithOptions(application.WebviewWindowOptions{…})` | Each window gets its own webview and JS context. |
| Which window made a bound call | `ctx.Value(application.WindowKey).(application.Window)` | Set by `messageprocessor_call.go` when a bound method's first parameter is a `context.Context`. The generator leaves `ctx` out of the TS signature. |
| Event to one window only | `window.DispatchWailsEvent(&application.CustomEvent{Name, Data})` | `app.Event.Emit` / `WebviewWindow.EmitEvent` broadcast to every window, so they're only used for app-wide topics. |
| Single instance + handoff | `Options.SingleInstance{UniqueID, OnSecondInstanceLaunch(SecondInstanceData{Args, WorkingDir})}` | Lock is OS-level (released when the process dies). |
| Quit interception | `Options.ShouldQuit func() bool` | |
| Last-window behavior | `Options.Mac.ApplicationShouldTerminateAfterLastWindowClosed` | `false` on macOS. On Windows/Linux we quit ourselves when the registry empties. |
| Window close interception | `window.RegisterHook(events.Common.WindowClosing, func(e){ e.Cancel() })` | |
| Dock click with no windows | `events.Mac.ApplicationShouldHandleReopen` | |
| Window menu | `application.WindowMenu` role | Spike S1 checks that it sets `NSApp.windowsMenu`, so Cocoa lists the windows automatically. |
| Files from the OS | `events.Common.ApplicationOpenedWithFile`, `Options.FileAssociations` | |
| Browser clients (for E2E) | build tag `server`: browser tabs become `BrowserWindow`s | Spike S3. |
| **Dock menu** | **none** | We add it ourselves (§5). |

## 2. Process and window model

```
                    ┌──────────────── one Goi process (per GOI_HOME) ───────────────┐
 2nd launch ──IPC──▶│ SingleInstance → App.handoff(args)                             │
 (Finder, CLI)      │                                                                 │
                    │ rpc.App  (process-wide)                                         │
                    │   log, configDir, cacheDir, recent, toolbin, appBus (broadcast) │
                    │   windows: registry  id → *window                               │
                    │                                                                 │
                    │   *window (one per native window)                               │
                    │     id, handle (application.Window), bus (window-scoped)        │
                    │     settings *settings.Store  (user layer + this project layer) │
                    │     session *session  (nil on Welcome)                           │
                    │     closing closeState                                           │
                    │                                                                 │
                    │ Bound services (one instance each, shared by all windows):     │
                    │   method(ctx, …) → s.app.windowFor(ctx) → w.session / w.bus     │
                    └─────────────────────────────────────────────────────────────────┘
```

### 2.1 `rpc.App` becomes process-wide; `window` holds per-window state

New `internal/rpc/window.go` (the current file is replaced; instance locks go away):

```go
type window struct {
    id       uint
    app      *App
    handle   windowHandle   // focus/title/close; a fake in tests
    bus      events.Bus     // window-scoped: DispatchWailsEvent to this window
    settings *settings.Store
    mu       sync.Mutex
    session  *session
    closing  closeState
}

type windowHandle interface {
    Focus()               // unminimise, show, focus
    SetTitle(string)
    Close()               // after the close flow said yes
    ID() uint
}
```

`App` keeps: `log`, `closer`, `logPath`, `configDir`, `cacheDir`, `recent`, `appBus` (broadcast), `windows` (`map[uint]*window` + mutex), and the service instances. These move from `App` to `window`: `bus`, `settings`, `cfgWatch` (one watcher per window store), `session`, `focusFn`, `titleFn`, `closing` and `quitAfterStart` (deleted; the handoff replaces it).

Lookup:

```go
// windowFor returns the window that made the bound call. Calls without a window (Go tests that pass context.Background())
// fall back to the only window when exactly one exists.
func (a *App) windowFor(ctx context.Context) (*window, error)
func (a *App) sessionFor(ctx context.Context) (*session, error) // errNoProject when the window has none
```

`ctx` in tests: `withWindow(ctx, w)` puts a `*window` on the context directly, so tests with several windows don't need Wails. `windowFor` checks our own key first, then `application.WindowKey` (mapping the handle's ID through the registry).

### 2.2 Services get `ctx context.Context` as the first parameter

All ~170 bound methods that touch a session, settings, a window bus or a dialog get `ctx context.Context` as their first parameter and replace `s.app.current()` with `s.app.sessionFor(ctx)` (`a.settings` becomes `w.settings`, `a.bus` becomes `w.bus`). The generated TS signatures don't change, because v3 leaves `ctx` out. Process-wide methods (`AppService.Info` for version fields, `RecentService.List`) may skip the param but still get it for consistency where they read per-window data (`Info().ProjectPath`).

Session goroutines (gopls callbacks, watchers, run output, PTYs, git watcher) already capture the session. They switch from `s.app.bus` to `s.bus`, set to the owning window's bus when the session is created. That is what keeps events in the right window (R3.2). Each `session` gets a `bus events.Bus` field and `startLang`, `runManager`, `debugManager`, `termSession`, `gitSession`, `lintSession` and `modSession` are passed `s.bus` instead of reading `app.bus`.

### 2.3 Event routing

| Topic class | Bus | Delivery |
|---|---|---|
| Project topics (fs, lang, run, debug, term, git, lint, mod, journal, project-level notify) | `session.bus` = `window.bus` | `handle.DispatchWailsEvent` to that window |
| Window topics (`menu.command`, window-level notify, settings changes of that window's store) | `window.bus` | same |
| App topics (recent changed, appearance/theme change, tool install progress) | `App.appBus` | `app.Event.Emit` (all windows) |

`events.Bus` is unchanged: `internal/*` still emits through the interface, and `internal/events` gets no new dependencies. A `windowBus{h application.Window}` and an `appBus{app *application.App}` live in `internal/rpc/bus.go`.

### 2.4 Settings

Each window owns a `settings.Store` loaded with the user file plus its project's `.goi/settings.toml` (`settings.Load(user, project, w.bus)`), plus its own config watcher. Writing a user setting in one window changes the file. Every other window's watcher reloads it and emits `settings.changed` to its own window (R3.4). No changes to `internal/settings`. A Welcome window's store has no project layer, and opening a project in that window calls `SetProjectPath` as today.

### 2.5 Shared stores

- `recent` already uses flock and re-reads, so it's safe for concurrent windows. It gets one addition, `recent.Store.SetOpenSet([]string)`, which saves the list of currently open projects to `recent.json` (field `open`) for R6.3.
- `journal` is per session already (keyed by workspace hash).
- `toolbin` installs: two windows could start "Install gopls" at once. Add an app-level `singleflight.Group` keyed by tool in `rpc` (not in `toolbin`).
- `instance` package: project locks and `LaunchNewWindow` are deleted. The window registry is the "is it open?" check, and v3 `SingleInstance` does the process-level part. The package itself is removed. Two different `GOI_HOME`s have different caches and UniqueIDs, so they never contend.

## 3. Opening projects and windows

`App.openProjectWindow(path string, from *window)`:
1. Normalize the path (abs, `EvalSymlinks`). If a registered window has that root, `Focus()` it and return (R2.2).
2. If `from` is a Welcome window (no session), open the session in `from` (R2.1).
3. Otherwise create a window: `w := a.newWindow()`, open the session into it **before** the webview loads (so `AppService.Info` and `WorkspaceService.Current` return the project when the frontend boots, as today), then show it. Position: focused window's position + (28, 28) (R2.5).

Callers change from `openInOtherWindow`/`launchNewWindow` to `openProjectWindow`: `WorkspaceService.Open*`, Recent, New Project, the Dock menu, handoff and file opening.

**Handoff (R1):** `SingleInstance.UniqueID = "dev.goi.app." + shortHash(configDir)`. `OnSecondInstanceLaunch` gets the second process's `Args` and `WorkingDir`, resolves relative paths against `WorkingDir`, runs `ParseArgs`, and then:
- a project path → `openProjectWindow`;
- files → `routeFiles` (below);
- nothing → focus the frontmost window, or open a Welcome window if there are none.

On macOS, Finder/Dock relaunches reuse the running app through LaunchServices anyway (no more `open -n`), so the handoff mainly serves the CLI and Windows/Linux.

**Files (R2.6):** `routeFiles(files)` groups the files by `ProjectRootFor(file)` and sends each group to the window whose root contains them, or to `openProjectWindow(root)`, and then emits `editor.openFiles` on that window's bus (the existing pending-files logic, kept per window until its frontend is ready).

## 4. Closing and quitting

- **Window close (R3.6):** the `WindowClosing` hook cancels the native close and emits `app.closeRequested` on that window's bus. The frontend runs the existing close flow (`app/closeChecks.ts`) and calls `AppService.ConfirmClose(ctx)`. That closes the session (`session.close(false)`: gopls, watchers, processes, PTYs, journal flush), removes the window from the registry and calls `handle.Close()`. This is the same `closeState` handshake as `close.go` today, moved per window.
- **Last window:** on macOS the app keeps running (`ApplicationShouldTerminateAfterLastWindowClosed: false`), and `ApplicationShouldHandleReopen` with no windows opens a Welcome window (R4.5). On Windows/Linux, removing the last window calls `app.Quit()` (R6.1).
- **Quit (R6.2):** `ShouldQuit` returns false the first time and starts `quitSequence`. Before closing anything, it saves the open set (`recent.SetOpenSet`). Then it closes the windows one at a time through the same close flow, frontmost first. If one is cancelled, it stops and resets. When the registry is empty it sets `quitting = true` and calls `app.Quit()`, which `ShouldQuit` now allows.
- **Startup reopen (R6.3):** if `startup.reopenLastProject` is set and there's no CLI target, Goi opens every existing path in `recent.OpenSet()` in its own window (the first one in the initial window). Otherwise it opens one Welcome or target window.
- **Isolation (R3.7):** each bound call is already recovered by v3 (`bindings_panic` handling). Session goroutines started in `rpc` get a `defer a.recoverSession(s)` that logs the panic and notifies that window. gopls restart backoff is already per server.

## 5. macOS Dock menu and Window menu

New package **`internal/dockmenu`** (no Wails import, so architecture rule 1 holds):

```go
type Item struct { ID string; Title string; Checked bool; Separator bool; Children []Item }
func Set(items []Item)                // replaces the menu; safe from any goroutine
func OnSelect(fn func(id string))     // called on the main thread's behalf, from a goroutine
func Install() error                  // darwin: adds -applicationDockMenu: to [NSApp delegate]'s class
```

- `dockmenu_darwin.go` + `dockmenu_darwin.m` (cgo, `-framework Cocoa`): `Install` uses `class_addMethod` on `object_getClass([NSApp delegate])` for `applicationDockMenu:` (v3.1.0's delegate doesn't implement it; `Install` fails rather than replacing an existing implementation, so a future v3 that adds one is detected). The method builds an `NSMenu` from the last `Set` snapshot. Each item's target/action calls the exported `goiDockSelect(char *id)`.
- `dockmenu_other.go`: no-ops. Windows and Linux have no equivalent; their taskbar/dock already lists windows.
- Called from `rpc` (`internal/rpc/dock.go`) after `events.Common.ApplicationStarted`. `App.refreshDock()` runs whenever the registry, a window's project or focus changes, and builds: one item per window (project name, or "Welcome"; `Checked` for the focused window), a separator, **New Window**, **Open Recent ▸** (from `recent`). Selecting an item: `win:<id>` focuses it, `new` opens Welcome, `recent:<path>` calls `openProjectWindow`.
- `menu.go`: on darwin, add a `Window` menu with the `WindowMenu` role after the existing menus (R4.4). Menu clicks find the target with `app.Window.Current()` and emit `menu.command` on that window's bus (R5.1). On Windows/Linux every window gets the same application menu (`UseApplicationMenu`), and the same lookup routes the click (R5.2).

## 6. Frontend

The frontend is per window and almost unchanged, because everything goes through `lib/api.ts`.

- `lib/api.ts`: import bindings from v3's generated `frontend/bindings/github.com/p4tin/goi/internal/rpc/…` and models from their packages, and the runtime from `@wailsio/runtime` (`Events.On`, `Browser.OpenURL`). `onEvent` unwraps v3's `WailsEvent` (`ev.data`). The exported wrapper signatures stay the same, so feature code doesn't change.
- v3 returns `CancellablePromise`, which is a subtype of `Promise`, so wrapper return types still hold.
- Model types: v3 generates classes/interfaces per Go package. `api.ts` re-exports aliases under the old names (`rpc.AppInfo` etc.) so features don't change their imports.
- Close flow: subscribe to `app.closeRequested` (the topic is added to `Topics`). The existing `closeChecks` and `ConfirmClose` path is reused.
- `frontend/wailsjs/` is deleted, and `frontend/bindings/` takes its place as the generated directory (still never hand-edited).

## 7. Build, dev, tooling

- v3 uses `wails3` + Taskfile. Generate the v3 build scaffold (`wails3 init` template → `build/config.yml`, `Taskfile.yml`, `build/{darwin,windows,linux}/Taskfile.yml`), then fold in what we have:
  - Info.plist keys (bundle id, file associations, `Info.goland.plist` equivalent);
  - `build/windows` icon and NSIS;
  - `build/package/macos/make-dmg.sh`, `build/package/linux/nfpm.yaml`;
  - the `-ldflags -X …paths.BuildVersion`.
- Version source moves from `wails.json` `info.productVersion` to `build/config.yml` `info.version`. `scripts/version.sh` reads it. `wails.json` is deleted.
- Commands (documented in CLAUDE.md/tech.md in Phase 5):

  | Was | Becomes |
  |---|---|
  | `wails dev` | `wails3 dev` |
  | `wails build` | `wails3 build` / `wails3 package` |
  | `wails generate module` | `wails3 generate bindings` |

  The GoLand `.run/` configs follow.
- E2E: build or run with `-tags server`. `cdp.mjs` points at the server URL, and each Chrome tab is a `BrowserWindow`, so a step list can open a second tab to test two windows (R8.4). Spike S3 confirms this; if it fails, the fallback is keeping v2-style single-window E2E through `wails3 dev` with the webview inspector, and the multi-window checks become a manual script.
- CI/release: install `wails3` pinned (`WAILS_VERSION=v3.1.0`), keep the same artifact names, and keep the tag check.

## 8. Testing strategy

| Area | Tests |
|---|---|
| Window registry/routing | `rpc` unit tests with two fake windows (`fakeHandle`, `events.Recorder` per window): calls with `withWindow(ctx, wA)` only touch A's session, A's gopls/run/git events only reach A's recorder, and settings written in A reload in B. |
| Open/focus/reuse | `openProjectWindow`: open-same focuses, Welcome reuse, cascade position, symlinked paths dedupe. |
| Handoff | `handoff(args, cwd)` table tests (project, files, nothing, relative paths). |
| Close/quit | quit sequence with a cancel in the 2nd window, last-window per-OS behavior (OS injected), `OpenSet` saved before closing. |
| dockmenu | item model and menu snapshot are pure Go and tested. The darwin `Install` gets a smoke test behind `GOI_DOCK_TEST=1` (needs an NSApp). |
| Frontend | existing vitest. `api.ts` event unwrap test. |
| E2E | existing four suites on v3, plus `multiwindow.json` (two tabs, two projects, no cross-talk). |
| Platform | CI matrix builds. A manual mac checklist (Dock count, Dock menu, Cmd-Tab, Window menu, file association, open/save sheets) goes in `acceptance.md`. |

## 9. Risks

| Risk | Mitigation |
|---|---|
| Per-window event delivery or ctx window injection behaves differently from what the source reading suggests | Spike S1 before any refactor. |
| `class_addMethod` on Wails' delegate breaks in a later v3 | `Install` refuses if the selector exists; the dockmenu smoke test runs in CI on macOS. |
| E2E harness can't drive v3 | Spike S3, with the fallback in §7. |
| Large mechanical change in `internal/rpc` (~170 methods) | Split by file group into parallel tasks after the core lands; each group's tests must pass on its own. |
| Packaging regressions (dmg, NSIS, nfpm) | Release workflow dry run (`workflow_dispatch` without a tag) before merge. |
| v3 binding generator types differ (null slices, int64, maps) | `tsc --noEmit` across the frontend after generation; aliases in `api.ts`. |
