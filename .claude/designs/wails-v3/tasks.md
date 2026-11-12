# Wails v3: one process, many windows — Tasks

Tasks are grouped in **waves**. Tasks in the same wave don't depend on each other, and each one owns a disjoint set of files, so they can run concurrently (one git worktree / branch per task, merged at the end of the wave). A wave starts only when the previous wave is merged and green. Each wave ends with a review gate.

**Definition of done (every task):** `go build ./...`, `go vet ./...`, `go test -race` for the packages touched, `npm test` + `npx tsc --noEmit` if frontend files changed, and no new imports of Wails outside `internal/rpc` and `main.go`.

Work happens on branch `feat/wails-v3`. Task branches are `feat/wails-v3-<id>`.

---

## Wave 0 — Spikes (parallel, throwaway code; answers feed back into design.md)

| Id | Task | Output | Owns |
|---|---|---|---|
| S1 | Minimal v3.1.0 app in `scratch/v3spike/`: two windows, a bound method `Who(ctx)` that returns the calling window's ID, a per-window event via `DispatchWailsEvent` (check the other window doesn't receive it), `SingleInstance` (second launch delivers args), `ShouldQuit` cancel, `WindowClosing` hook cancel, `WindowMenu` role lists windows on macOS, `ApplicationShouldHandleReopen`. | Short findings note appended to design.md §1 (confirmed / differs). | `scratch/v3spike/` |
| S2 | Dock menu prototype: add `applicationDockMenu:` via `class_addMethod` to the v3 app delegate class in the S1 app. Item click calls back into Go. Check it doesn't break v3's own delegate methods (open file, reopen). | Findings plus the working `.m` snippet. | `scratch/dockspike/` |
| S3 | E2E feasibility: build the S1 app with `-tags server`, connect headless Chrome over CDP, call a binding and receive an event. Two tabs count as two windows. | Findings: E2E path chosen (server mode or fallback, design §7). | `scratch/e2espike/` |

**Gate 0:** design.md updated with the findings, and you approve any design changes they cause. `scratch/` is deleted.

---

## Wave 1 — Foundations (parallel)

| Id | Task | Owns | Verify |
|---|---|---|---|
| T1 | **`internal/dockmenu` package** (design §5): `Item`, `Set`, `OnSelect`, `Install`. Darwin cgo + `.m`; no-op stub for other OSes. Unit tests for the item snapshot/ID mapping. Darwin smoke test gated by `GOI_DOCK_TEST=1`. | `internal/dockmenu/**` | `go test ./internal/dockmenu`; `GOOS=windows go vet ./internal/dockmenu`; `GOOS=linux go vet ./internal/dockmenu` |
| T2 | **rpc window model, still on Wails v2** (design §2.1, 2.3, 2.4): add `window`, `windowHandle`, the registry on `App`, `windowFor`/`sessionFor`/`withWindow`; move `bus`/`settings`/`cfgWatch`/`session`/`closing`/`focusFn`/`titleFn` into `window`; `session` gets its own `bus` and the session-side managers (`lang`, `runs`, `debug`, `terms`, `git`, `lint`, `mod`, `search`) emit through `s.bus`. Under v2 there's exactly one window. `App.current()` stays as a shim returning the single window's session, so **bound method signatures don't change yet** (v2 can't take `ctx`). Add test helpers: `fakeHandle`, `newTestWindow(t)`. | `internal/rpc/{app,session,window,close,bus}.go`, the session-manager constructors in `internal/rpc/{lang,run,debug,terminal,git,lint,mod,search}.go` (constructor/bus lines only), `internal/rpc/*_test.go` helpers | `go test -race ./internal/rpc`; app runs under `wails dev`; smoke E2E passes |
| T3 | **v3 build scaffold, prepared but not wired**: generate the v3 template into `build-v3/` (staging dir); port Info.plist keys, file associations, icons, NSIS and nfpm settings, the dmg script and version stamping into it; rewrite `scripts/version.sh` to read `build-v3/config.yml` (keep the old path as a fallback until T5). | `build-v3/**`, `scripts/version.sh` | `scripts/version.sh` prints 0.1.2 from both sources |
| T4 | **E2E harness for v3** per S3's result: `cdp.mjs` gets a `--server-url` mode and a `{"tab": n}` / `{"newTab": url}` step for multi-window. The current v2 mode keeps working. | `tools/e2e/cdp.mjs` | `smoke.json` still passes against v2 `wails dev` |

**Gate 1:** merge T1–T4; full `go test -race ./internal/...`, `npm test` and the smoke E2E pass on v2.

---

## Wave 2 — Switch to v3, single window (one task: go.mod, main.go and the bridge must change together)

| Id | Task | Owns | Verify |
|---|---|---|---|
| T5 | **Port to Wails v3.1.x.** Replace `wails/v2` with `wails/v3` in go.mod. `main.go` → `application.New` (assets, `SingleInstance` with a stub handler that only focuses, `Mac`/`Linux`/`Windows` options, `ApplicationShouldTerminateAfterLastWindowClosed: false`) plus one window created through `App.newWindow`. `bus.go`: `windowBus` (DispatchWailsEvent) and `appBus`. `logger.go` → slog handler. Dialogs, clipboard and quit in `app.go`/`files.go`/`close.go` → v3 managers. `menu.go` → v3 menu builder (same `MENUS`, accelerators; click → `menu.command` on the current window's bus). Move `build-v3/` → `build/` (replacing the v2 assets) and delete `wails.json`. Run `wails3 generate bindings`, delete `frontend/wailsjs/`, update `lib/api.ts` imports, the runtime (`Events.On` unwrap, `Browser.OpenURL`) and model aliases. | `go.mod`, `go.sum`, `main.go`, `internal/rpc/{bus,logger,menu,app,files,close}.go`, `build/**`, `wails.json`, `frontend/wailsjs/**`, `frontend/bindings/**`, `frontend/src/lib/api.ts`, `frontend/package.json` | Everything works as on v2 in one window: all Go tests, `npm test`, `tsc`, E2E smoke/m2/m3/m4 through the T4 harness, manual macOS run (open/save sheets, file association, appearance). |

**Gate 2:** you check a v3 single-window build on macOS before the multi-window work starts.

---

## Wave 3 — Route every call by window (parallel by file group)

Each task: add `ctx context.Context` as the first parameter of every bound method in its files; replace `s.app.current()` with `s.app.sessionFor(ctx)` and `a.settings`/`a.bus` with the window's; update that file's tests to pass `withWindow(ctx, w)`; add **two-window tests** showing calls and events stay in their window; regenerate bindings (`wails3 generate bindings`; the TS signatures shouldn't change, so a non-empty diff in `frontend/bindings` other than comments is a bug to investigate).

Generated bindings are the one shared output. Each task regenerates them on its branch, and they're regenerated once more after the merge.

| Id | Task | Owns |
|---|---|---|
| T6a | Code intelligence: `LangService`, `SearchService`, `EditorService` | `internal/rpc/{lang,search,editor,markers,batch}.go` + tests |
| T6b | Run/debug/terminal: `RunService`, `DebugService`, `TerminalService` | `internal/rpc/{run,debug,terminal}.go` + tests |
| T6c | Git/lint/modules: `GitService`, `LintService`, `ModService` | `internal/rpc/{git,lint,mod}.go` + tests |
| T6d | Core services: `AppService`, `WorkspaceService`, `FileService`, `SettingsService`, `SDKService`, `RecentService`, journal, new project | `internal/rpc/{app,workspace,files,settings,sdk,journal,newproject}.go` + tests |
| T7 | **Menus and Dock:** `MenuService` (ctx), macOS `Window` menu role, `internal/rpc/dock.go` (install `dockmenu` on `ApplicationStarted`, `refreshDock` on registry/focus/project changes, handle `win:`/`new`/`recent:` selections; until T8 lands, `new` and `recent:` just call `newWindow`/`openProjectWindow` stubs). | `internal/rpc/{menu,dock}.go` + tests |

Once T6a–T6d merge, delete the `App.current()` shim (done in the merge commit for the wave).

**Gate 3:** all tests pass, the shim is gone (`grep -n "current()" internal/rpc` is empty), and the E2E suites pass in one window.

---

## Wave 4 — Multi-window behavior (parallel)

| Id | Task | Owns | Verify |
|---|---|---|---|
| T8 | **Open, focus, handoff, files** (design §3): `openProjectWindow` (dedupe via `EvalSymlinks`, Welcome reuse, cascade), switch all callers off `openInOtherWindow`/`launchNewWindow`, the `SingleInstance` handoff with `WorkingDir`-relative args, `routeFiles` with per-window pending files, `ApplicationOpenedWithFile`. Delete `internal/instance`. Tool-install `singleflight`. | `internal/rpc/{window,openfile,workspace_open,toolinstall}.go`, `internal/instance/**` (delete) + tests | Table tests from design §8; manual: `goi --project B` while A is open → second window, one process |
| T9 | **Close, quit, reopen** (design §4): per-window `WindowClosing` → `app.closeRequested` → `ConfirmClose(ctx)`; quit sequence with cancel; last-window behavior per OS; `ApplicationShouldHandleReopen` → Welcome; `recent.Store` `OpenSet`/`SetOpenSet`; startup reopens the open set; `recoverSession`. Frontend: subscribe to `app.closeRequested` in the close flow. | `internal/rpc/{close,lifecycle}.go`, `internal/recent/**`, `frontend/src/app/closeChecks.ts`, `frontend/src/lib/api.ts` (`Topics` entry only) + tests | Quit-with-cancel test; reopen test; manual: Cmd-Q with unsaved files in two windows |
| T10 | **Tooling and CI:** `ci.yml`/`release.yml` → `wails3` pinned, same artifacts; `.run/*.xml` updated; release workflow dry run via `workflow_dispatch`. | `.github/workflows/**`, `.run/**` | CI green on macOS/Windows/Linux; dry-run artifacts listed |

**Gate 4:** two projects open on macOS: one Dock icon, the Dock menu switches windows, Cmd-Tab shows one Goi.

---

## Wave 5 — Verification (parallel)

| Id | Task | Owns |
|---|---|---|
| T11 | E2E: all four suites on v3, plus a new `tools/e2e/multiwindow.json` + sample script (two tabs/windows, two projects: diagnostics, run output and git status stay in their own window; closing one leaves the other working). | `tools/e2e/**` |
| T12 | Platform pass: Windows and Linux builds from CI installed and checked (taskbar grouping, file associations, menus per window, last-window quit); macOS manual checklist (R4, R7.3). Results recorded in `acceptance.md`. | `.claude/designs/wails-v3/acceptance.md` |
| T13 | Performance (R8.5): cold start to first paint and time to open a second project window, compared with the v2 numbers measured at Gate 1. Results go in `acceptance.md` (a separate section from T12, so the merge is trivial). | `.claude/designs/wails-v3/acceptance.md` (perf section) |

**Gate 5:** acceptance.md is complete and every requirement is marked pass or has an explicit waiver from you.

---

## Wave 6 — Wrap-up (sequential)

| Id | Task |
|---|---|
| T14 | Phase 5 docs: CLAUDE.md (commands, the generated `frontend/bindings`, the window model, the routing rules and v3 gotchas), `.claude/steering/{tech,structure,product}.md` (Wails v3, `internal/dockmenu`, `internal/instance` removed). |
| T15 | Phase 6: `retro.md`. |

## Dependency summary

```
Wave 0:  S1 ─┬─ S2      S3
             │
Wave 1:  T1   T2   T3   T4        (all parallel)
              │    │    │
Wave 2:       └─ T5 ◀───┘ (uses T3 scaffold, T4 harness)
                 │
Wave 3:  T6a  T6b  T6c  T6d  T7   (parallel; T7 uses T1)
                 │
Wave 4:  T8   T9   T10            (parallel)
                 │
Wave 5:  T11  T12  T13            (parallel)
                 │
Wave 6:  T14 → T15
```
