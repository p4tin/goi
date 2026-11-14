# Structure — Goi

## Repository layout
```
goi/
├── SPEC.md                  # Product spec (source of truth for scope)
├── CLAUDE.md                # Agent guidance (created at Phase 5)
├── .claude/
│   ├── steering/            # product.md, structure.md, tech.md
│   └── designs/<feature>/   # requirements.md, design.md, tasks.md, retro.md
├── main.go                  # entrypoint: parses args, rpc.App.Run (the Wails v3 shell lives in internal/rpc)
├── Taskfile.yml             # wails3 task entry (build/*/Taskfile.yml per OS; build/config.yml = product info)
├── tools/e2e/               # headless-Chrome CDP harness (drives the -tags server build)
├── tools/fakeagent/         # scripted ACP agent for E2E (wraps internal/acp/acptest)
├── go.mod
├── internal/                # goi-core: all backend logic, UI-agnostic
│   ├── events/              # Bus interface + topic constants
│   ├── applog/              # rotating JSON log
│   ├── paths/               # version-scoped config/cache dirs (GOI_HOME override)
│   ├── workspace/           # project detection + Manager (roots, excludes, re-detect)
│   ├── vfs/                 # text I/O (EOL/BOM), tree ops, matcher, watcher, trash, reveal
│   ├── gosdk/               # SDK discovery/validation, go command Runner
│   ├── settings/            # layered TOML settings + hot reload
│   ├── format/              # gofmt / goimports
│   ├── gosyntax/            # go/parser run markers
│   ├── index/               # file index + fuzzy search
│   ├── journal/             # crash journal for unsaved buffers
│   ├── recent/              # recent projects (flock-shared)
│   ├── dockmenu/            # macOS Dock icon menu (cgo; no-op elsewhere)
│   ├── lsp/                 # LSP JSON-RPC client + protocol types (no Goi knowledge)
│   ├── gopls/               # gopls resolve/install, supervised server, sync, requests, UTF-16 edits
│   ├── textsearch/          # Find/Replace in Path engine (line-based Go regexp)
│   ├── toolbin/             # resolve/install Go tools (gopls, dlv): setting → PATH → GOBIN → GOPATH/bin → tools dir
│   ├── runconfig/           # run configurations: model, TOML store (shared/local/temporary), validate, plans, dlv launch args
│   ├── process/             # user processes: process groups, stdin, interrupt-then-kill stop
│   ├── dap/                 # Debug Adapter Protocol client + types (no Goi knowledge)
│   ├── debugger/            # dlv dap sessions: launch/attach, breakpoints, stepping, evaluation, run to cursor, path mapping
│   ├── terminal/            # PTY terminals (go-pty) + zsh/bash shell integration
│   ├── shellenv/            # login-shell PATH for GUI launches (macOS/Linux), applied first thing in main.go
│   ├── git/                 # git CLI wrapper: status, diffs/hunk patches, branches, stash, log, blame, conflicts
│   ├── lint/                # golangci-lint v1/v2 runner and JSON → issues/fixes
│   ├── gomod/               # module proxy versions, known modules (module cache, go.sum)
│   ├── acp/                 # Agent Client Protocol v1 client + types (no Goi knowledge); acptest/ = scripted fake agent
│   ├── mcp/                 # MCP server (Streamable HTTP, tools) + stdio bridge (no Goi knowledge)
│   ├── agent/               # coding agents: catalogue, resolve, npm install, ACP client process, agent terminals
│   └── rpc/                 # Wails-bound services; App (process) / window (per-window) / session (per-project);
│                            # shell.go (v3 app + windows), lifecycle.go, dock.go, menu.go
├── frontend/                # Wails frontend (TypeScript + Vite)
│   ├── src/
│   │   ├── features.ts      # installer list (one entry per feature)
│   │   ├── app/             # state, workbench shell, docks, layout, status, dialogs, context menus,
│   │   │                    # native menu bar definition (menuBar.ts)
│   │   ├── editor/          # manager, panes/splits, tabs, languages, save flow, disk sync,
│   │   │                    # close guard, journal, run gutter
│   │   ├── lang/            # gopls features: completion, hover/docs, signature, diagnostics + Problems,
│   │   │                    # navigation/history, usages, rename, context actions, structure/breadcrumbs,
│   │   │                    # palette symbols, Find/Replace in Path, gopls status
│   │   ├── run/             # run configurations, widget, Run window, ANSI console, test tree, benchmarks, context runs
│   │   ├── debug/           # breakpoints, Debug window (goroutines, frames, variables, watches), inline values, evaluate
│   │   ├── terminal/        # xterm.js terminal tabs
│   │   ├── git/             # status, tree colors, gutter, commit panel, diff, branches, stash, log, blame, merge editor
│   │   ├── lint/            # golangci-lint diagnostics source, lint on save, fixes
│   │   ├── mod/             # go.mod/go.work completion, links, module actions
│   │   ├── agent/           # AI Agent window, transcript model, Markdown, IDE tools for agents (ACP fs + MCP), change log
│   │   ├── markdown/        # Markdown preview: renderer (@lezer/markdown + GFM), code highlighting, Mermaid, links/images, scroll sync
│   │   ├── format/          # JSON (hand-written) and YAML (`yaml` library) formatters for Reformat File / format on save
│   │   ├── palette/         # Search Everywhere (files, symbols, actions, settings), ranking, double-Shift, recent files
│   │   ├── keymap/          # keymaps, chords, dispatcher, rebinding
│   │   ├── theme/           # tokens, themes, editor schemes, theme commands (dark mode toggle)
│   │   ├── views/           # welcome, project tree, SDK/status, settings UI
│   │   └── lib/             # api.ts (binding wrappers), store, dom, commands
│   └── bindings/            # GENERATED by `wails3 generate bindings` — never hand-edit
├── scripts/version.sh       # product version from wails.json (release tag check)
├── .github/workflows/       # ci.yml (tests + builds), release.yml (installers, draft release)
└── build/                   # Wails platform assets (icons, plist, manifests, NSIS); build/package/ (dmg, nfpm)
```

## Architectural boundaries
1. **`internal/` must not import Wails.** It stays UI-agnostic so it can later run as a headless/remote `goi-core` daemon (SPEC §2.3).
2. **`internal/rpc/` is the only bridge.** It holds thin facades bound to Wails that translate calls and events to and from `internal/*` packages. No business logic.
3. **Backend → UI events** go through one event emitter interface (`events.Bus`) so the transport can later move from Wails events to JSON-RPC over WebSocket.
4. **Frontend never touches the filesystem or spawns processes.** Everything goes through bindings.
5. **`frontend/bindings/` is generated.** Wrap it in `frontend/src/lib/` and import the wrappers, not the generated files.
5a. **Features are installers** in `frontend/src/features.ts`; they extend the editor via `editors.hooks` / `editors.addExtension`, not by editing `editorManager.ts`.
6. Packages depend downward only: `rpc` → domain packages; `workspace` uses `settings` + `vfs`, `index` uses `vfs`, `gopls` uses `lsp` + `toolbin`, `debugger` uses `dap`, `runconfig` uses `gosyntax`, `lint` uses `toolbin`, `agent` uses `acp` + `process`. Domain packages don't depend on each other unless the design says so.

## Project-local state
- `.goi/settings.toml`: project settings (shareable).
- `.goi/runConfigurations/*.toml`: shared run configurations (local ones: `<config>/workspaces/<hash>/runConfigurations/`; benchmark history `<config>/workspaces/<hash>/bench.json`).
- User state: `os.UserConfigDir()/Goi/<major.minor>/` (settings, recent projects, the projects open at last quit (`open-projects.json`), window layouts keyed by workspace hash, the external MCP token `mcp-token`). Agents installed by Goi live in `<config>/tools/agents/<id>`.
- Caches: `os.UserCacheDir()/Goi/<major.minor>/`.
