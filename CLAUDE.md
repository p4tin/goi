# CLAUDE.md — Goi

Goi is a cross-platform Go IDE (Wails v2 + CodeMirror 6). Product spec: `SPEC.md`. Steering: `.claude/steering/{product,structure,tech}.md`. Feature specs: `.claude/designs/<feature>/` (requirements, design, tasks, acceptance, retro).

## Commands

```
wails dev                        # hot reload (uses GOI_HOME if set)
wails build                      # macOS: build/bin/Goi.app
wails generate module            # regenerate frontend/wailsjs bindings after changing bound Go methods
go test ./internal/...           # backend tests (add -race for rpc/workspace/vfs/lsp/gopls)
cd frontend && npm test          # vitest
cd frontend && npx tsc --noEmit  # type-check
node tools/e2e/cdp.mjs tools/e2e/smoke.json   # headless E2E against a running `wails dev`
node tools/e2e/cdp.mjs tools/e2e/m2.json      # M2 code intelligence E2E (project: tools/e2e/m2-sample.sh <dir>)
node tools/e2e/cdp.mjs tools/e2e/m3.json      # M3 run/test/debug/terminal E2E (project: tools/e2e/m3-sample.sh <dir>)
GOI_DLV=/path/to/dlv go test -race ./internal/debugger ./internal/rpc  # debugger tests skip without Delve
GOI_GOPLS_BENCH=1 go test -run CompletionLatency -v ./internal/gopls   # completion p95 on this repo
```

gopls is needed for code intelligence (`go install golang.org/x/tools/gopls@latest`). Goi resolves it from the `gopls.path` setting, then `$PATH`, `$GOBIN`, `$(go env GOPATH)/bin`, then `<config>/tools/bin` (where "Install gopls" puts it). Tests that need it skip when it's missing.

Wails CLI on this machine: `/Users/p4tin/.gvm/pkgsets/go1.25.6/global/bin/wails` (v2.17.1). `go.mod` needs Go 1.26; `GOTOOLCHAIN=auto` fetches it.

GoLand run configurations are shared in `.run/` (Goi, wails dev, wails build, Go tests, frontend build/tests). A plain `go build`/`go run` of the app needs `-tags desktop,production`, `frontend/dist` built first, and on macOS `CGO_LDFLAGS="-framework UniformTypeIdentifiers"`; without the flag it fails to link (`_OBJC_CLASS_$_UTType`). Also add `-Wl,-sectcreate,__TEXT,__info_plist,build/darwin/Info.goland.plist` (from the repo root) to embed a bundle identifier. Without one, native open/save sheets close by themselves about 0.5 s after opening (the system log shows `masquerading-service-lacks-host-bundle-identifier`). The `Goi` run config does both.

To read the macOS system log, call `/usr/bin/log show …`. In zsh, a bare `log` is a shell builtin and silently prints nothing.

Debugging needs Delve. Goi resolves it like gopls: the `debug.dlvPath` setting, `$PATH`, `$GOBIN`, `$(go env GOPATH)/bin`, then `<config>/tools/bin` (where "Install Delve" puts it). Delve tests skip unless dlv is found or `GOI_DLV` is set.

`GOI_HOME=<dir>` redirects user config/cache to `<dir>/config` and `<dir>/cache`. Use it for dev runs and tests so the real profile is untouched.

## Architecture rules

- `internal/*` never imports Wails. `internal/rpc` is the only bridge: bound services are thin, and the domain logic lives in other packages.
- Backend → UI updates go through `events.Bus` topics (`internal/events`). Frontend topic names are in `frontend/src/lib/api.ts` (`Topics`).
- The frontend never touches the filesystem; it goes through `lib/api.ts` wrappers and never imports `wailsjs/` directly.
- Frontend features are installers listed in `frontend/src/features.ts`. Features extend the editor through `editors.hooks` (`opened`, `closed`, `beforeSave`, `saved`, `viewCreated`, `activeChanged`) and `editors.addExtension(buf => ext)`; don't edit `editorManager.ts` for feature work.
- Every user action is a command (`lib/commands.ts`). Shortcuts live only in `keymap/keymaps.ts`.
- The native menu bar is a list of command ids in `app/menuBar.ts` (`MENUS`). Labels come from command titles and shortcuts from the active keymap; `internal/rpc/menu.go` builds it and emits `menu.command`. To add a menu entry, add its command id to `MENUS`.
- Colors come only from theme tokens (`var(--goi-*)`, source of truth `theme/tokens.ts`).
- Code intelligence: `internal/lsp` is a protocol-only LSP client; `internal/gopls` supervises one gopls per project (sync, requests, restart backoff) and reports through callbacks; `internal/rpc/lang.go` (`LangService`, `langSession`) bridges it. Positions crossing the bridge are LSP positions (0-based line, UTF-16 character), the same units as CodeMirror offsets within a line. Paths cross as OS paths; `file://` URIs stay inside `internal/gopls`.
- Run/test/debug: `internal/runconfig` models configurations (`.goi/runConfigurations/*.toml` shared, `<config>/workspaces/<hash>/runConfigurations` local) and turns them into command plans and dlv launch args. `internal/process` runs programs in process groups. `internal/dap` is a protocol-only DAP client; `internal/debugger` drives `dlv dap`. `internal/terminal` owns PTYs and shell integration. `internal/toolbin` resolves and installs Go tools (gopls, dlv). rpc: `RunService` (`run.go`, runManager), `DebugService` (`debug.go`), `TerminalService` (`terminal.go`). A debug session's id is its run id: Run and Debug share before-launch tasks, output batching and stop/rerun.
- Frontend: `run/` (run window, console, test tree, configs dialog, widget, context runs), `debug/` (breakpoints, debug window, variables, inline values, evaluate), `terminal/` (xterm.js). Tool windows build their DOM and subscribe to events at install time; the panel's `render` only attaches the element, so events that arrive before the panel was ever shown aren't lost.
- Frontend language features live in `frontend/src/lang/` (one installer, `installLang`). Every request first calls `client.flush(buf)` and drops responses whose buffer version changed (`fresh()`). Edits go through `lang/edits.ts`: open buffers are edited in CodeMirror, unopened files on disk via `LangService.ApplyFileEdits`.

## Gotchas

- Don't use `time.Time` (or other non-JSON-native types) in bound method parameters: Wails then generates a `Time` class that retypes every timestamp in `models.ts`. Pass RFC 3339 strings.
- Return empty slices, not nil, from bound methods. Nil arrives as `null` in JS.
- `frontend/dist` is generated (Vite empties it). Run `npm run build` before `go vet ./...` or `wails generate module` in a fresh checkout or worktree.
- `wails generate module` may flip file modes under `frontend/wailsjs/runtime`; revert with `git checkout frontend/wailsjs/runtime`.
- `wails dev` regenerates bindings on Go changes. If its Go rebuild fails, the old backend keeps running; restart it.
- happy-dom tests: open editor views with `{ focus: false }`. happy-dom fires `selectionchange` synchronously, which re-enters CodeMirror's update.
- Each project window is its own process (Wails v2 is single-window). Instance locks live in `<cache>/instances/<hash>.lock` (`internal/instance`).
- rpc tests don't start gopls: `TestMain` in `internal/rpc/lang_test.go` sets `startLangHook = nil`; call `withLang(t)` in tests that need it.
- CodeMirror tooltips: the `TooltipView.dom` you return *is* the `.cm-tooltip` element. Put classes on it; its parent is the editor.
- gopls rejects a `didChange` whose version isn't higher than the last one. Re-opening a tracked document must send `didClose` + `didOpen` (`gopls.Server.Open` does).
- In the CDP harness, synthetic Cmd-A doesn't select all in text inputs. Set input values with `eval` and dispatch an `input` event instead.
- CodeMirror `EditorView.domEventHandlers` only see events on the content DOM, not gutters. For clicks on line numbers, listen on `view.dom` (see `debug/breakpoints.ts`).
- Delve: always pass `output` (a temp path) in launch args or it writes `__debug_bin*` into the project; resolve symlinks in `program`/`cwd` (the go command calls symlinked package paths "outside main module"); after `terminated` the client must send `disconnect` or the connection stays open.
- go-pty's `Close` is not safe to call concurrently (wrap in `sync.Once`).
- The key dispatcher lets the integrated terminal keep most keys (Ctrl-R, Ctrl-D…); only `TERMINAL_PASSTHROUGH` commands, F-keys, and macOS Cmd shortcuts run while `.xterm` has focus.
- E2E with xterm: send Enter as a `key` step (an `insertText` "\r" doesn't press Enter), wait ~1 s after the prompt appears, and make steps idempotent (breakpoints and watches persist per project). Kill stray headless Chrome (`--remote-debugging-port`) if the harness says "chrome did not start".
- Don't rebuild DOM from `mousedown`/`pointerdown` handlers. If the pressed element is replaced, the browser sends `click`/`dblclick` to the common ancestor and row handlers never run. Update the existing nodes instead. CDP-dispatched synthetic events don't catch this; tests should dispatch to the common ancestor (see `test/projectTree.test.ts`).
