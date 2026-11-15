# Application Packs — Design

Implements `requirements.md` (approved 2026-11-15).

## 1. Shape

```
goi/                                   one Go module, one frontend package
├── apps/
│   ├── goi/                           ── Go pack + Goi app
│   │   ├── main.go                    package main: embeds dist/, build/config.yml, icon; calls boot.Main(pack)
│   │   ├── pack.go                    func Pack() pack.Pack (identity + capabilities)
│   │   ├── internal/                  Go-only packages (compiler-enforced: only apps/goi/... may import them)
│   │   │   ├── gosdk/  gosyntax/  gomod/  gofmt/ (was internal/format)
│   │   │   ├── gopls/                 gopls resolve/install, settings, Go-specific requests
│   │   │   ├── golangci/              (was internal/lint)
│   │   │   ├── dlv/                   Delve adapter: resolve, launch/attach args (from debugger + runconfig/debug.go)
│   │   │   ├── gorun/                 Go Build/Test/Remote run types, test2json, benchmarks
│   │   │   └── services/              pack-bound services: SdkService, ModService (plain structs, no Wails import)
│   │   ├── build/                     config.yml, appicon.svg/png, darwin/, windows/, linux/, package/
│   │   └── dist/                      GENERATED: Vite output for this app (embedded)
│   └── pyi/                           ── Python pack + Pyi app (same layout)
│       └── internal/ pyenv/ pyright/ pyrun/
├── internal/                          core (no Go- or Python-specific code)
│   ├── pack/                          NEW: Pack, Identity, capability interfaces
│   ├── boot/                          NEW: the old main.go body: mcp-bridge, shellenv, args, App.Run
│   ├── langserver/                    was internal/gopls minus Go specifics: supervision, sync, requests, edits, uri
│   ├── debugger/                      generic DAP session driver; adapter comes from the pack
│   ├── runconfig/                     model + store + type registry; Shell/Makefile types stay here
│   ├── workspace/                     Manager + generic Project; detection is delegated to the pack
│   └── … (vfs, settings, git, terminal, lsp, dap, acp, mcp, agent, rpc, paths, …)
├── frontend/
│   ├── src/                           core features (unchanged location)
│   │   ├── packs/go/                  Go frontend pack: index.ts (features, settings, languages), mod/, sdk, go run bits
│   │   ├── packs/python/              Python frontend pack
│   │   └── pack.ts                    `import pack from 'virtual:pack'`, resolved by Vite per app
│   └── vite.config.ts                 `--mode goi|pyi` → alias + outDir ../apps/<app>/dist
├── build/                             SHARED build machinery only: Taskfile.yml, darwin|windows|linux/Taskfile.yml,
│                                      package/macos/make-dmg.sh. All per-app paths go through {{.APP_DIR}}.
└── Taskfile.yml                       includes goi: and pyi: (apps/<app>/Taskfile.yml); bare `build`/`dev`/… = goi
```

Placing the Go-only packages under `apps/goi/internal/` lets the compiler enforce R1.2: core can't import them, and neither can Pyi. `TestPackBoundaries` (R8.2) also checks that nothing in `internal/` imports `apps/...`, and that `frontend/src` outside `packs/` doesn't import `packs/*`.

## 2. Core contracts: `internal/pack`

```go
type Identity struct {
    Name       string // "Goi"   display name; from build/config.yml productName
    Program    string // "goi"   binary, ProgramName, log prefix, flag set, MCP server name
    BundleID   string // "dev.goi.goi"; SingleInstance id is derived from it (+ home hash, as today)
    Version    string // info.version from the embedded build/config.yml
    DirName    string // "Goi"   <UserConfigDir>/<DirName>/<major.minor>
    DataVersion string // "0.1"  major.minor scoping user dirs (Goi keeps "0.1": R3.2)
    HomeEnv    string // "GOI_HOME"
    ProjectDir string // ".goi"
    Icon       []byte // Linux window icon (png)
}

type Pack struct {
    Identity  Identity
    Project   ProjectDetector          // R5.1
    SDK       SDKProvider              // R5.2  (nil = no SDK concept)
    Servers   []LanguageServerSpec     // R5.3
    Formatter Formatter                // R5.4  (nil = none)
    Linter    Linter                   // R5.5  (nil = none)
    Tools     []toolbin.Tool           // R5.6
    RunTypes  []runconfig.Type         // R5.7  (+ markers)
    Tests     TestParser               // R5.8
    Debug     DebugAdapter             // R5.9  (nil = no debugging)
    Agent     AgentContext             // R5.10 (nil = none)
    Services  func(Host) []any         // extra bound services (Goi: SdkService, ModService)
    Settings  []SettingsSection        // pack settings sections and their defaults
}
```

The interfaces, with only the methods core calls:

| Interface | Methods | Go pack | Python pack (Part B) |
|---|---|---|---|
| `ProjectDetector` | `Detect(ctx, root, matcher) (workspace.Project, error)`; `Excludes() []string` | today's `workspace/detect.go` (go.work/go.mod/GOPATH) | pyproject/setup.py/requirements/.venv |
| `SDKProvider` | `Current(ctx, root) (SDKInfo, error)`; `Env(ctx, root) []string` (environment for processes, servers, agents) | `gosdk` | `.venv/bin/python`, then `python3` on `PATH` |
| `LanguageServerSpec` | `ID`, `Languages []string`, `Resolve(ctx, settings) (cmd, args, error)`, `InitOptions/Settings(settings) any`, `Install` (tool id) | gopls | basedpyright (`basedpyright-langserver --stdio`) |
| `Formatter` | `Languages()`, `Format(ctx, path, src) ([]byte, error)` | gofmt/goimports | none in Part B |
| `Linter` | `Run(ctx, root, files) ([]lint.Issue, error)` (core keeps the `Issue`/`Fix` types) | golangci-lint | none in Part B |
| `runconfig.Type` | `ID`, `Title`, `Validate`, `Plan(cfg, env) (Plan, error)`, `Markers(path, src) []Marker` | Go Build/Test/Remote | Python File |
| `TestParser` | `Parse(line []byte) ([]TestEvent, bool)`. `TestEvent` is the core event and has test2json's shape | test2json passthrough | n/a (python-app spec: pytest plugin emits the same shape) |
| `DebugAdapter` | `Start(ctx, env) (dap transport)`, `LaunchArgs(cfg, plan) map[string]any`, `AttachArgs(...)` | `dlv dap` (incl. the `output` temp path and symlink rules) | n/a (python-app: debugpy) |
| `AgentContext` | `Env(root) []string` | GOFLAGS etc. (whatever the agent env carries today) | venv activation vars |

`Host` is the core API that pack services use instead of the unexported rpc internals. rpc implements it:

```go
type Host interface {
    Session(ctx) (ProjectSession, error) // root, workspace.Project, settings, bus, SDK env
    AppBus() events.Bus
    Log() *slog.Logger
    ToolsDir() string
}
```

Pack services are plain structs whose methods take `ctx` first (the same convention as today). rpc wraps them with `application.NewService`, so `apps/*/internal` never imports Wails. This is the rule "rpc is the only bridge", applied to packs.

## 3. Identity plumbing (R2)

| Today (hard-coded) | Becomes |
|---|---|
| `paths`: `"Goi"`, `Version="0.1"`, `GOI_HOME` | `paths.Configure(DirName, DataVersion, HomeEnv)`, called first thing in `boot.Main`. Package state is set once by main; tests call `Configure` in `TestMain` |
| `rpc/shell.go` Name/Title/ProgramName/single-instance id; `session.go` titles; `app.go` flag set, About info | read from `App.id pack.Identity` |
| `workspace.SettingsPath` `.goi`; `run.go` `.goi/runConfigurations` | `ProjectDir` passed in `workspace.Options` and to the run store |
| `gopls` clientInfo, `debugger` ClientID/Name, `agent` ACP clientInfo, `mcp` Implementation, MCP server key `"goi"`, `GOI_MCP_TOKEN`, `TERM_PROGRAM` | taken from Identity (`Program`, `Name`; the token variable becomes `strings.ToUpper(Program)+"_MCP_TOKEN"`) |
| Messages like "already open in another Goi window" | formatted with `Name` |
| Frontend: `document.title`, About, Welcome, `goi-icon.svg`, "Goi MCP Server" | `pack.ts` exports `product` (name, icon). Vite reads `name` from `apps/<app>/build/config.yml` (`define: __PRODUCT_NAME__`), so each app has one source of truth |
| Theme and keymap display names | "Light/Dark/Ember/Paper", "Default"; ids unchanged (Q4) |
| `-ldflags -X …/internal/paths.BuildVersion` | unchanged (core path) |

`boot.Main(p pack.Pack, assets fs.FS)` runs today's `main.go` body in the same order: mcp-bridge first, then shellenv, args, `rpc.NewApp(opts, p)`, then Run. `apps/goi/main.go` is about 20 lines: its embeds, `ConfigVersion`, then `boot.Main(goi.Pack(), dist)`.

## 4. Backend: what moves where

| Today | After | Notes |
|---|---|---|
| `internal/gopls` (1.2k) | `internal/langserver` (supervisor, sync, versions, UTF-16 edits, URIs, requests) + `apps/goi/internal/gopls` (resolve/install, gopls settings, `gopls.*` commands) | `langSession` (rpc/lang.go) keeps one `langserver.Server` per `LanguageServerSpec` and routes by language id. Positions and paths stay as they are |
| `internal/gosdk`, `gosyntax`, `gomod`, `format` | `apps/goi/internal/...` | `rpc/sdk.go`, `rpc/mod.go` → `apps/goi/internal/services` |
| `internal/lint` | core keeps `Issue`/`Fix` + `rpc/lint.go` (generic LintService over `pack.Linter`); golangci runner → `apps/goi/internal/golangci` | LintService is hidden when `Linter == nil` |
| `internal/debugger` | core: DAP session, breakpoints, stepping, evaluation, path mapping; `apps/goi/internal/dlv`: resolve, `dlv dap` start, launch/attach args (from `runconfig/debug.go`) | `AdapterID` comes from the adapter |
| `internal/runconfig` | core: model, TOML store, validation framework, type registry, plans for Shell/Makefile, before-launch tasks; Go types + args → `apps/goi/internal/gorun` | TOML type ids unchanged, so saved configurations load as before (R3.2) |
| `rpc/markers.go` (gosyntax) | asks the `runconfig.Type`s for markers | |
| `workspace/detect.go` | Goi's detector moves to `apps/goi`; `workspace.Project` becomes generic (`Kind string`, `Modules` → `Units []Unit{Path, Name, Meta map[string]string}`) | JSON field names Goi's frontend already uses are kept, or are mapped in `packs/go` |
| `toolbin` | core: resolve order + tools dir + install runner; a `Tool` has an `Install(ctx, env) *exec.Cmd` | Go tools: `go install`; Pyi: venv + pip |
| `settings.Settings` (`Go`, `Gopls`, `Debug.DlvPath`, `Lint.GolangciPath`) | core struct keeps the generic sections; pack sections are decoded from the same TOML tables through `settings.Section[T](eff, "gopls")`, with defaults from `pack.Settings` | TOML keys and files stay the same (R3.2). Core sections that mix in Go fields (`debug.dlvPath`) become pack sections of the same name, merged on load |

### Bound services and bindings

- Core services stay bound in every app with the same type names: App, Workspace, File, Settings, Recent, Search, Editor, Menu, Lang, Run, Debug, Terminal, Git, Lint, Agent.
- `AppService.Product()` returns Identity (minus the icon) plus `Capabilities{lsp, format, lint, debug, sdk, tests}`. Core features use it to hide UI (R1.4).
- Bindings: `wails3 generate bindings -ts -i ./apps/goi ./apps/pyi` generates one `frontend/bindings` tree covering every app's services. Core TS imports only core service bindings; `packs/go/api.ts` imports Goi's services. The CLAUDE.md rule becomes: only `lib/api.ts` and `packs/*/api.ts` import `bindings/`. **Spike in T1** to confirm the generator accepts several main packages. Fallback: one output dir per app plus a `frontend/bindings` alias chosen per mode.

## 5. Frontend

- `vite.config.ts`: `mode` is `goi` or `pyi`. It sets `resolve.alias['virtual:pack'] = src/packs/<pack>/index.ts`, `build.outDir = ../apps/<app>/dist`, `define.__PRODUCT_NAME__`, and `__PRODUCT_ICON__` (svg path).
- `packs/<p>/index.ts` exports `{ product, features: Feature[], settings: SettingsSection[], languages: LanguageSupport[] }`.
- `features.ts` builds the list as core features + `pack.features`. Pack features are inserted at named anchors (`after: 'lang'`, `after: 'projectTree'`…) so the dock-default ordering rule still holds. A feature list test (R6.2) pins the order.
- Moves to `packs/go/`: `mod/`, `views/goStatus.ts`, `views/sdkPicker.ts`, `views/newProject.ts` (Go templates), the Go parts of `run/` (`bench.ts`, Go test-name parsing in `testTree.ts`, Go context runs), and the Go entries in `editor/languages.ts`, `views/settings/schema.ts` and `lang/` code actions/keywords.
- Stays core and generic: `lang/` (LSP features keyed by server id; the status item shows the server name from `Product().capabilities`), `run/` console/window/test tree (fed core `TestEvent`s), `debug/`, `lint/` (hidden without a linter).
- `editor/languages.ts` gets `registerLanguage(spec)`. Go registers `lang-go`, Python registers `@codemirror/lang-python` (new dependency).
- Settings UI: `schema.ts` keeps core sections; packs add theirs with `registerSettingsSection`.

## 6. Build and release (R4)

- **Taskfiles.**
  - `apps/<app>/Taskfile.yml` sets `APP_NAME`, `PRODUCT_NAME`, `APP_DIR=apps/<app>`, `MAIN_PKG=./apps/<app>` and `VITE_MODE`, then includes the shared `build/Taskfile.yml` and `build/<os>/Taskfile.yml` with those vars.
  - In the shared Taskfiles, `build/darwin/Info.plist` → `{{.APP_DIR}}/build/darwin/Info.plist`, `go build … -o` → `go build … -o … {{.MAIN_PKG}}`, and the frontend build → `npm run build -- --mode {{.VITE_MODE}}`. Artifacts go to `bin/<app>/`, so the two apps never overwrite each other.
  - The root `Taskfile.yml` includes `goi:` and `pyi:`. Its bare `build/package/run/dev` tasks delegate to `goi:`, so `wails3 build` and `wails3 dev` keep building Goi (R3.5).
- **Dev:** `apps/<app>/build/config.yml` `dev_mode.executes` calls `wails3 task <app>:build DEV=true` and `<app>:run`; `wails3 dev -config apps/<app>/build/config.yml`.
- **Server build:** `go build -tags server -o bin/goi-server ./apps/goi`. The cdp.mjs `server` blocks in `tools/e2e/*.json` get the package path; Pyi gets `tools/e2e/pyi-smoke.json`.
- **CI:** `go test ./...` covers core and both packs. vitest runs once, and the pack tests live under `frontend/test/packs/`. The build matrix becomes OS × app.
- **Release:** `release.yml` takes the app from the tag (`v*` or `goi-v*` → goi, `pyi-v*` → pyi). The job steps use `${APP}`/`${PRODUCT}` and read the version from `apps/${APP}/build/config.yml` (`scripts/version.sh <app> [tag]`). Packaging files (nfpm.yaml, .desktop, NSIS defines, dmg) are per app under `apps/<app>/build/package/`.
- **Running both apps at once (R4.6)** follows from Identity: different bundle ids give different single-instance ids, Dock entries and user dirs (so a different MCP token file).

## 7. Pyi (Part B)

- Identity: Pyi, `pyi`, `dev.goi.pyi`, `PYI_HOME`, `.pyi`, `Pyi/0.1`. An original icon (a new `appicon.svg`, not derived from the Python logo), then `scripts/icons.sh <app>`.
- `ProjectDetector`: root markers `pyproject.toml`, `setup.cfg`, `setup.py`, `requirements*.txt`, `.venv/`; excludes `.venv`, `__pycache__`, `.mypy_cache`, `.pytest_cache`, `.ruff_cache`, `*.egg-info`, `node_modules`.
- `SDKProvider`: the `python.interpreter` setting, then `<root>/.venv/bin/python` (`Scripts\python.exe` on Windows), then `python3`/`python` on `PATH`. `Env` sets `VIRTUAL_ENV` and prepends the venv bin dir to `PATH`.
- `LanguageServerSpec` basedpyright: resolved from the `python.lsp.path` setting, `PATH`, then `<config>/tools/bin`. "Install basedpyright" creates `<config>/tools/venvs/basedpyright` with the selected interpreter, runs `pip install basedpyright`, and links `basedpyright-langserver` into `<config>/tools/bin`. Settings: `python.pythonPath` = the selected interpreter.
- `runconfig.Type` "Python File": `python <file> <args>` in the project env. The gutter marker goes on `if __name__ == "__main__":`.
- No formatter, linter, tests or debug in Part B. Capabilities hide that UI.

## 8. Migration order (keeps Goi green after each step)

1. Spikes: bindings for several mains; Task include vars; `wails3 dev` with a config outside `build/`.
2. Identity: `internal/pack.Identity`, `paths.Configure`, rpc and the protocol clients read Identity, `boot.Main`. Move `main.go` → `apps/goi/main.go` and the build assets → `apps/goi/build/`, rewire the Taskfiles, update the E2E server path. Goi now builds from `apps/goi`.
3. Frontend pack switch: `virtual:pack`, product name/icon, generic theme names, Vite modes and outDir.
4. Backend capabilities, one at a time, each with tests passing: settings sections → project detection → SDK → language servers (`gopls` → `langserver` + pack) → formatter → linter → tools → run types and tests → debug adapter → agent env → pack services (SDK, Mod).
5. Frontend Go moves to `packs/go`, with the capabilities-based hiding.
6. Boundary test, CI matrix, release workflow per app, docs.
7. Part B: the Pyi pack, icon, E2E smoke.

## 9. Risks

| Risk | Mitigation |
|---|---|
| The bindings generator only handles one main package | T1 spike; fallback is per-app output dirs (§4) |
| A big diff hides behavior changes | Small steps, each ending green on Go tests, vitest and the E2E suites (R3.4); no behavior edits inside move commits |
| Moving `main.go` breaks GoLand run configs and the `-sectcreate` plist path | Update `.run/*` and the CLAUDE.md commands in the same step |
| `workspace.Project` shape change breaks the frontend | Keep the JSON field names Goi uses; the Go pack maps any Go-only fields |
| Windows-only build or path breakage (history) | CI matrix covers both apps on Windows before merge |
| macOS keeps permissions per bundle id | Goi keeps `dev.goi.goi` (R3.2) |

## 10. Spike findings and deviations (as built)

**Spikes.**
- **S1:** `wails3 generate bindings -ts -i ./apps/...` takes several main packages and writes one tree: core services plus each app's own (20 services). Pack services must be wrapped with `application.NewService(&T)` where `T` is concrete, so the wrapping happens in `apps/<app>/pack.go` (main packages may import Wails) and reaches rpc as `[]any`.
- **S2:** Instead of per-app Taskfiles with include vars, the root Taskfile derives everything from one `APP` var (`APP_DIR`, `MAIN_PKG`, `BIN_DIR=bin/<app>`, `PRODUCT_NAME` read by `scripts/app-info.sh`). A task that calls `task: build` with different vars would not re-derive the global vars, so namespaced `goi:`/`pyi:` tasks were dropped. Use `wails3 build APP=pyi` and `wails3 task <task> APP=pyi`.
- **S3:** `wails3 dev -config apps/<app>/build/config.yml` works; `dev_mode.root_path` is relative to the working directory (the repo root), and each app's `dev_mode.executes` passes `APP=<app>`.

**Deviations from §1–§7.**
- **Pack code locations.** Language packs are `packs/golang` and `packs/python`, not `apps/<app>/internal`. Core rpc tests import the Go pack (in `_test.go` files only) to keep exercising Go projects. `TestPackBoundaries` enforces the boundary instead of the compiler. `apps/<app>` holds identity, assets and service binding.
- **Identity.** It lives in `internal/product`, a process-wide value set once by `boot.Main`, rather than `App.id`: protocol clients, the MCP bridge and terminal integration read it without new parameters. Goi keeps `InstanceID: dev.goi.app`, so single-instance locks still work across versions.
- **Language server.** There is one per app (`Pack.Server`), not a list.
- **Settings.** Goi's `go`/`gopls`/`debug.dlvPath`/`lint.*` stay typed fields of `settings.Settings`, and only Goi reads them. Pack sections the struct doesn't model (Pyi's `[python]`) are kept in `Settings.Extra` and read with `Value(key)`. Packs add defaults through `Pack.SettingsDefaults`. In the frontend, setting descriptors are per pack (`registerSettings`).
- **Workspace model.** `workspace.Project` keeps its fields, `Module` included. Pyi reports one module per `pyproject.toml`.
- **Go run configuration forms.** The Go Build/Test/Remote forms are still in `run/configDialog.ts`. Pyi never shows them: the add menu lists `capabilities.runTypes` plus Shell/Makefile.
- **Frontend product.** The product (name, tagline, icon) is `src/product.ts`, built from defines and `virtual:pack-icon`, not `pack.product`. Reading it from `pack.ts` at module load caused a TDZ error in the bundle: the pack imports core modules that read the product while loading.
- **Guards.** `vite build` refuses to run without `--mode goi|pyi`, and boot refuses to start when the bundle's product name differs from `AppInfo.name`.
