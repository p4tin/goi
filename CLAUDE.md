# CLAUDE.md — Goi

Goi is a cross-platform Go IDE (Wails v3 + CodeMirror 6). One process owns every project window. Product spec: `SPEC.md`. Steering: `.claude/steering/{product,structure,tech}.md`. Feature specs: `.claude/designs/<feature>/` (requirements, design, tasks, acceptance, retro).

## Commands

```
wails3 dev                       # hot reload (uses GOI_HOME if set)
wails3 build                     # host OS binary in bin/; `wails3 package` for the bundle/installer
wails3 generate bindings -ts -i  # regenerate frontend/bindings after changing bound Go methods
go test ./internal/...           # backend tests (add -race for rpc/workspace/vfs/lsp/gopls)
cd frontend && npm test          # vitest
cd frontend && npx tsc --noEmit  # type-check
go build -tags server -o bin/goi-server .     # E2E server build: each browser tab is a Goi window
node tools/e2e/cdp.mjs tools/e2e/multiwindow.json  # two windows, one process (sample: tools/e2e/multiwindow-sample.sh /tmp/goi-mw)
node tools/e2e/cdp.mjs tools/e2e/smoke.json   # headless E2E; add a "server" block (see cdp.mjs header) or point "url" at a running server
node tools/e2e/cdp.mjs tools/e2e/m2.json      # M2 code intelligence E2E (project: tools/e2e/m2-sample.sh <dir>)
node tools/e2e/cdp.mjs tools/e2e/m3.json      # M3 run/test/debug/terminal E2E (project: tools/e2e/m3-sample.sh <dir>)
GOI_DLV=/path/to/dlv go test -race ./internal/debugger ./internal/rpc  # debugger tests skip without Delve
GOI_GOLANGCI=/path/to/golangci-lint go test ./internal/lint ./internal/rpc  # lint integration tests
node tools/e2e/cdp.mjs tools/e2e/m4.json      # M4 git/lint/go.mod E2E (sample: tools/e2e/m4-sample.sh <dir>; run Goi with GIT_CONFIG_GLOBAL=<dir>/gitconfig)
go build -o bin/fakeagent ./tools/fakeagent && tools/e2e/agent-sample.sh /private/tmp/goi-e2e/agentsample $PWD/bin/fakeagent
node tools/e2e/cdp.mjs tools/e2e/agent.json   # AI agent session against the scripted fake ACP agent (server build)
GOI_REAL_AGENT="gemini --acp" go test -run RealAgent -v ./internal/agent  # protocol check with a real agent (no model call; add GOI_REAL_AGENT_PROMPT=1 for one prompt)
scripts/version.sh [tag]                      # product version from build/config.yml (checks a release tag)
scripts/icons.sh                              # regenerate build/appicon.png and build/windows/icon.ico from build/appicon.svg (macOS, needs Chrome)
GOI_GOPLS_BENCH=1 go test -run CompletionLatency -v ./internal/gopls   # completion p95 on this repo
```

gopls is needed for code intelligence (`go install golang.org/x/tools/gopls@latest`). Goi resolves it from the `gopls.path` setting, then `$PATH`, `$GOBIN`, `$(go env GOPATH)/bin`, then `<config>/tools/bin` (where "Install gopls" puts it). Tests that need it skip when it's missing.

Wails CLI on this machine: `/Users/p4tin/.gvm/pkgsets/go1.25.6/global/bin/wails3` (v3.1.0; install with `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`). `wails3 task …` runs the Taskfiles (`Taskfile.yml`, `build/*/Taskfile.yml`); the build config is `build/config.yml`. `go.mod` needs Go 1.26; `GOTOOLCHAIN=auto` fetches it.

GoLand run configurations are shared in `.run/` (Goi, wails3 dev, wails3 build, Go tests, frontend build/tests). A plain `go build`/`go run` of the app needs `-tags production` and `frontend/dist` built first. On macOS also add `CGO_LDFLAGS=-Wl,-sectcreate,__TEXT,__info_plist,build/darwin/Info.goland.plist` (from the repo root) to embed a bundle identifier. Without one, native open/save sheets close by themselves about 0.5 s after opening (the system log shows `masquerading-service-lacks-host-bundle-identifier`). The `Goi` run config does this.

To read the macOS system log, call `/usr/bin/log show …`. In zsh, a bare `log` is a shell builtin and silently prints nothing.

Debugging needs Delve. Goi resolves it like gopls: the `debug.dlvPath` setting, `$PATH`, `$GOBIN`, `$(go env GOPATH)/bin`, then `<config>/tools/bin` (where "Install Delve" puts it). Delve tests skip unless dlv is found or `GOI_DLV` is set.

golangci-lint is resolved the same way (`lint.golangciPath`, then the usual places, then `<config>/tools/bin`, where "Install golangci-lint" puts it). Git is the `git` CLI (`git.path`).

Releases: set `info.version` in `build/config.yml` (`main.go` embeds it, so About and Welcome show the full version even in dev builds: `paths.ProductVersion`), then tag `v<version>`. `.github/workflows/release.yml` builds the macOS universal dmg/zip, the Windows NSIS installer and zips, and Linux deb/rpm/tar.gz with `wails3 task …`. It checks the tag against `build/config.yml` and creates a draft GitHub Release. Linux builds use `-tags gtk3` (GTK3 + WebKitGTK 4.1, the `EXTRA_TAGS` default in `build/linux/Taskfile.yml`) so Ubuntu 22.04 and Debian 12 keep working. Packaging files live in `build/package/` (`macos/make-dmg.sh`, `linux/nfpm.yaml`); `paths.BuildVersion` is stamped with `-ldflags`. The app icon's source is `build/appicon.svg`; after editing it, run `scripts/icons.sh` and copy it to `frontend/src/assets/goi-icon.svg` (Welcome and About show it). `wails3 generate icons` builds `build/darwin/icons.icns` from `build/appicon.png`; Windows uses the committed `build/windows/icon.ico`.

After merging to `main`, watch CI (`gh run watch`) before starting new work. Windows-only failures have slipped through before.

`GOI_HOME=<dir>` redirects user config/cache to `<dir>/config` and `<dir>/cache`. Use it for dev runs and tests so the real profile is untouched.

## Architecture rules

- `internal/*` never imports Wails. `internal/rpc` is the only bridge: bound services are thin, and the domain logic lives in other packages.
- Windows: one Goi process per `GOI_HOME` (Wails `SingleInstance`; a later launch hands its args to `App.secondInstance`). `rpc.App` is process-wide (log, recent, `appBus`, window registry); `rpc.window` holds per-window state (event bus, settings store with that project's layer, `session`, close handshake). Bound methods that touch per-window state take `ctx context.Context` first and call `s.app.windowFor(ctx)` / `sessionFor(ctx)`; Wails v3 drops the ctx from the TS signature. Session managers emit on their window's bus (`.win.bus`); app-wide topics (recent list, tool installs) go on `appBus`. Lifecycle (open-or-focus, Welcome reuse, quit closing windows one by one, reopening the projects open at quit) is in `internal/rpc/lifecycle.go`; the native shell in `shell.go`; the macOS Dock menu in `internal/dockmenu` (wired by `rpc/dock.go`).
- Backend → UI updates go through `events.Bus` topics (`internal/events`). Frontend topic names are in `frontend/src/lib/api.ts` (`Topics`).
- The frontend never touches the filesystem; it goes through `lib/api.ts` wrappers and never imports `bindings/` or `@wailsio/runtime` directly. Boot waits for `waitForWindow()` before anything else.
- Frontend features are installers listed in `frontend/src/features.ts`. Features extend the editor through `editors.hooks` (`opened`, `closed`, `beforeSave`, `saved`, `viewCreated`, `activeChanged`) and `editors.addExtension(buf => ext)`; don't edit `editorManager.ts` for feature work.
- Every user action is a command (`lib/commands.ts`). Shortcuts live only in `keymap/keymaps.ts`.
- The native menu bar is a list of command ids in `app/menuBar.ts` (`MENUS`). Labels come from command titles and shortcuts from the active keymap; `internal/rpc/menu.go` builds it and emits `menu.command` to the window the click belongs to (macOS: the focused window; Windows/Linux: each window has its own menu). To add a menu entry, add its command id to `MENUS`.
- Colors come only from theme tokens (`var(--goi-*)`, source of truth `theme/tokens.ts`).
- Code intelligence: `internal/lsp` is a protocol-only LSP client; `internal/gopls` supervises one gopls per project (sync, requests, restart backoff) and reports through callbacks; `internal/rpc/lang.go` (`LangService`, `langSession`) bridges it. Positions crossing the bridge are LSP positions (0-based line, UTF-16 character), the same units as CodeMirror offsets within a line. Paths cross as OS paths; `file://` URIs stay inside `internal/gopls`.
- Run/test/debug: `internal/runconfig` models configurations (`.goi/runConfigurations/*.toml` shared, `<config>/workspaces/<hash>/runConfigurations` local) and turns them into command plans and dlv launch args. `internal/process` runs programs in process groups. `internal/dap` is a protocol-only DAP client; `internal/debugger` drives `dlv dap`. `internal/terminal` owns PTYs and shell integration. `internal/toolbin` resolves and installs Go tools (gopls, dlv). rpc: `RunService` (`run.go`, runManager), `DebugService` (`debug.go`), `TerminalService` (`terminal.go`). A debug session's id is its run id: Run and Debug share before-launch tasks, output batching and stop/rerun.
- Git / lint / modules: `internal/git` wraps the git CLI (porcelain v2 status, diffs, hunk patches, branches, stash, log, blame, conflicts). `internal/lint` runs golangci-lint v1/v2 and normalizes issues and fixes. `internal/gomod` covers proxy versions and known modules. rpc: `GitService` (`git.go`, gitSession: watcher plus `.git/index|HEAD|FETCH_HEAD|logs/HEAD` file watches, streamed fetch/pull/push ops), `LintService` (`lint.go`), `ModService` (`mod.go`). Frontend: `git/`, `lint/`, `mod/`.
- Extension hooks (use them instead of editing core modules): `treeDecorators` / `treeDecorationsVersion` and `treeMenuContributors` (`views/projectTree.ts`), `extraCompletionSources` (`lang/completion.ts`), `actionProviders` (`lang/codeActions.ts`), `setDiagnosticsSource` / `replaceDiagnosticsSource` (`lang/diagnostics.ts`, multi-source), `addCloseCheck` (`app/closeChecks.ts`), `runHooks` (`run/launch.ts`).
- Feature order in `features.ts` matters for dock defaults: the first panel registered in a dock becomes its default tab (Project must come before Commit).
- Frontend: `run/` (run window, console, test tree, configs dialog, widget, context runs), `debug/` (breakpoints, debug window, variables, inline values, evaluate), `terminal/` (xterm.js). Tool windows build their DOM and subscribe to events at install time; the panel's `render` only attaches the element, so events that arrive before the panel was ever shown aren't lost.
- AI agents: `internal/acp` is a protocol-only Agent Client Protocol v1 client (JSON-RPC, one message per line); `internal/mcp` a protocol-only MCP server (Streamable HTTP, stateless, JSON responses) plus the stdio bridge `goi mcp-bridge` (handled first thing in `main.go`, no GUI); `internal/agent` the catalogue (claude, gemini, codex), command resolution (`ai.agents.<id>` setting → `<tools>/agents/<id>` npm install → PATH), npm installs, the agent process (`Client`) and agent-run commands (`Terminals`). rpc: `AgentService` + `agentManager` per project session (`agent.go`), the window's `uiBroker` (`agentui.go`), the MCP host (`mcp.go`: internal server with a random port/token and per-session URLs `/mcp/<window>/<session>`; optional external server on `ai.mcpPort`), tool schemas (`agenttools.go`). Editor-aware work (agent `fs/*` requests, MCP tool calls, permission prompts) is sent to the window's frontend as `agent.request` and answered with `AgentService.Respond`; the backend checks paths are inside the project first. Frontend: `agent/` (`tools.ts` implements the tools behind ports, `ports.ts` the real ports, `model.ts` the transcript reducer, `view.ts` the AI Agent window, `changes.ts` the per-session change log for diff/revert).
- Frontend language features live in `frontend/src/lang/` (one installer, `installLang`). Every request first calls `client.flush(buf)` and drops responses whose buffer version changed (`fresh()`). Edits go through `lang/edits.ts`: open buffers are edited in CodeMirror, unopened files on disk via `LangService.ApplyFileEdits`.

## Gotchas

- Don't use `time.Time` (or other non-JSON-native types) in bound method parameters. Pass RFC 3339 strings.
- Return empty slices, not nil, from bound methods. Nil arrives as `null` in JS. The v3 generator types every slice and map as `T | null` anyway; `lib/api.ts` narrows them (`list()`, `dict()`, `NonNullFields`). Go string-constant types become TS enums; `api.ts` exposes their values as string unions.
- Never tie work that outlives a bound call (processes, installs, git ops, watchers) to the call's `ctx`: Wails cancels it when the call returns.
- `frontend/dist` is generated (Vite empties it). Run `npm run build` before `go vet ./...` or `wails3 generate bindings` in a fresh checkout or worktree.
- Test files must not end in `_windows_test.go` (or another `_<GOOS>_test.go`): Go treats the suffix as a build constraint and the tests silently don't run elsewhere. Multi-window tests are `*_multiwindow_test.go`.
- In the server build (`-tags server`, E2E only) Wails broadcasts every event to every tab; window events carry a `{_w, d}` envelope that `api.ts` filters. A tab only becomes a window once its WebSocket connects, so `windowFor` refuses calls without a window when a Wails app exists (the test fallback to "the only window" applies only without one).
- happy-dom tests: open editor views with `{ focus: false }`. happy-dom fires `selectionchange` synchronously, which re-enters CodeMirror's update.
- All windows share one process. Opening a project that a window already has focuses that window (`openProjectWindow`); there are no per-project process locks any more.
- rpc tests don't start gopls: `TestMain` in `internal/rpc/lang_test.go` sets `startLangHook = nil`; call `withLang(t)` in tests that need it. Use `twoWindows(t)` / `withWindows(a)` and `withWindow(ctx, w)` for multi-window tests; `a.mainWindow()` etc. exist only in tests.
- CodeMirror tooltips: the `TooltipView.dom` you return *is* the `.cm-tooltip` element. Put classes on it; its parent is the editor.
- gopls rejects a `didChange` whose version isn't higher than the last one. Re-opening a tracked document must send `didClose` + `didOpen` (`gopls.Server.Open` does).
- In the CDP harness, synthetic Cmd-A doesn't select all in text inputs. Set input values with `eval` and dispatch an `input` event instead.
- z-index scale: CodeMirror panels are 300 (CodeMirror's own value). The diff overlay is 320, popups 350–360, toasts 390, the palette 395, dialogs 400, menus 410. Anything drawn over editors must be above 300.
- Git in tests and E2E: isolate from the user's config (`HOME`/`XDG_CONFIG_HOME` in Go tests; `GIT_CONFIG_GLOBAL=<sample>/gitconfig` and `GIT_CONFIG_NOSYSTEM=1` for the app under test), or user hooks and signing settings leak in. `rpc` test modules from `module()` have an empty `.git`, so set `GOFLAGS=-buildvcs=false` before building them.
- CodeMirror `EditorView.domEventHandlers` only see events on the content DOM, not gutters. For clicks on line numbers, listen on `view.dom` (see `debug/breakpoints.ts`).
- Delve: always pass `output` (a temp path) in launch args or it writes `__debug_bin*` into the project; resolve symlinks in `program`/`cwd` (the go command calls symlinked package paths "outside main module"); after `terminated` the client must send `disconnect` or the connection stays open.
- go-pty's `Close` is not safe to call concurrently (wrap in `sync.Once`).
- The key dispatcher lets the integrated terminal keep most keys (Ctrl-R, Ctrl-D…); only `TERMINAL_PASSTHROUGH` commands, F-keys, and macOS Cmd shortcuts run while `.xterm` has focus.
- E2E with xterm: send Enter as a `key` step (an `insertText` "\r" doesn't press Enter), wait ~1 s after the prompt appears, and make steps idempotent (breakpoints and watches persist per project). Kill stray headless Chrome (`--remote-debugging-port`) if the harness says "chrome did not start".
- MCP tool names are defined twice: schemas in `internal/rpc/agenttools.go`, implementations in `frontend/src/agent/tools.ts` (`TOOL_NAMES`). `TestAgentToolNamesMatchFrontend` keeps them in sync; add a tool in both.
- Agent writes (`fs/write_text_file`, `replace_text`, `write_file`) replace the open buffer as one undoable change and save it, so a later `go build` by the agent sees them; the "before" snapshot for revert includes the user's unsaved edits.
- rpc and agent tests re-run the test binary as the fake agent (`GOI_FAKE_AGENT=1`, `acptest.Main`) and as `goi mcp-bridge`. `TestMain` must check the `mcp-bridge` argument before the env var: the bridge inherits the agent's environment. Fake agents must pass the MCP server's `env` to the commands they start (the token travels in `GOI_MCP_TOKEN`).
- Don't move the agent transcript element in the DOM (toggle `hidden` instead): re-attaching a scrolled element resets its scroll position.
- Don't rebuild DOM from `mousedown`/`pointerdown` handlers. If the pressed element is replaced, the browser sends `click`/`dblclick` to the common ancestor and row handlers never run. Update the existing nodes instead. CDP-dispatched synthetic events don't catch this; tests should dispatch to the common ancestor (see `test/projectTree.test.ts`).
