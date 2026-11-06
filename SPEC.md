# Goi — Go IDE: Re-creation Specification

> Status: Draft v0.1 · 2026-11-05
> Source: reverse-engineering pass over `/Applications/GoLand.app` (GoLand 2026.2.1.1, build GO-262.9437.286, arm64) using **REA 6.8.0** plus static reading of the bundle's plugin descriptors (XML metadata only, no decompiled code).
> Goal: specify an **original** Go IDE ("Goi") with an equivalent feature set. Goi must not reuse JetBrains code, names, icons, color schemes, or branding.

---

## 0. How this spec was derived

| Step | Tool / command | What it established |
|---|---|---|
| Bundle identity | `rea inspect-plist` | Bundle ID, version, 110+ registered document extensions, `goland://` URL scheme (stack-trace links), macOS privacy usage strings |
| Bundle layout | `rea inspect-desktop-application` | 3,410 entries: thin native launcher + bundled JetBrains Runtime (JBR, Java 25) + ~420 platform jars + 88 bundled plugins |
| Launcher | `rea inspect-macho Contents/MacOS/goland` | 815 KB arm64 Mach-O, macOS 11+, SDK 26 — only a JVM bootstrapper |
| Debugger | `rea inspect-go-binary plugins/go-plugin/lib/dlv/macarm/dlv` | Bundled **Delve v1.27.0** (built with go1.27rc1, includes `google/go-dap v0.12.0`), shipped for 8 OS/arch targets |
| Feature inventory | `plugin.xml` + module XML descriptors in the Go plugin jars | 12 languages, 223 inspection registrations (~126 unique), 118 intentions, 99 actions, 3 run-config types, 5 program runners, refactorings, generators |
| Config | `product-info.json`, `goland.vmoptions`, `idea.properties` | Data-dir naming, heap sizing, file-size limits for code insight |

### Key architectural findings (as-built)

1. **Host platform**: a JVM application (Kotlin/Java, Swing + Compose/"Jewel" UI) on a general IDE platform. Go support is a plugin (`org.jetbrains.plugins.go`, package `com.goide`) layered on platform services (PSI, indexing, VCS, execution, debugger, terminal).
2. **Own Go front-end**: Go parsing, resolution, type inference, inspections, and data-flow analysis are implemented **in-process** (it does *not* use `gopls`); module awareness via `go list` / `go env`. Pre-built **shared indexes** of the Go SDK (Go 1.25.7 / 1.26.0, all OSes) ship with the app to avoid indexing the stdlib on first open.
3. **Debugging**: shells out to a bundled **Delve** binary per platform; supports record/replay (`rr`), core dumps, goroutine filtering.
4. **Split front-end/back-end** ("frontend-split", RPC jars, `remote-dev-server`): the UI can run separately from the analysis back-end (remote dev / thin client).
5. **LSP/DAP clients** exist in the platform (lsp4j, `platform.lsp`, `platform.dap`) but are used for *other* languages, not Go.
6. **AI hooks**: `fullLine` (local ML line completion), `mcpserver` (exposes IDE as an MCP server), `platform-acp-plugin` (agent client protocol), ML-ranked completion for Go.

### Design decision for Goi (recommended)

Reimplementing GoLand's in-process Go analyzer is multi-year work. Goi gets ~80% of the value by **orchestrating the official Go toolchain**:

| GoLand (as-built) | Goi (to build) |
|---|---|
| In-process Go PSI, resolver, indexes | **gopls** over LSP (completion, nav, refs, rename, diagnostics, code actions, inlay hints, semantic tokens) |
| Custom inspections (~126) | gopls analyzers + `go vet` + `staticcheck` + **golangci-lint** integration; a small set of Goi-native checks |
| Bundled Delve via custom protocol | **Delve `dlv dap`** over DAP |
| Custom test runner UI | `go test -json` parser |
| JVM + Swing | See §2 stack |

---

## 1. Product scope

### 1.1 Personas
- Go backend developer working on modules / multi-module workspaces (`go.work`).
- Polyglot service developer touching YAML, Dockerfile, SQL, Terraform, protobuf, Markdown alongside Go.

### 1.2 Release tiers
- **MVP (v0.1)** — editor, project model, gopls intelligence, run/test/debug, terminal, git basics.
- **v0.2** — refactorings & generators, coverage, golangci-lint, go.mod tooling, search everywhere, HTTP client.
- **v0.3** — profiling (pprof), Docker/K8s/DB tooling, remote/split mode, AI/MCP.

Out of scope: JavaScript/TS IDE parity, Qodana-style CI inspection server, plugin marketplace (v1+).

---

## 2. Architecture

```
┌──────────────────────────────── Goi Desktop (UI) ────────────────────────────────┐
│ Editor (CodeMirror 6 / Monaco) · Project tree · Tool windows · Command palette   │
│ Settings UI · Run/Debug UI · Diff/VCS UI · Terminal (xterm.js)                    │
└───────────────▲──────────────────────────────────────────────────────────────────┘
                │  JSON-RPC 2.0 over WebSocket (local) or TLS (remote)  §2.3
┌───────────────┴──────────────── goi-core (Go daemon) ────────────────────────────┐
│ Workspace/VFS + fs watcher │ Project model (go env/list/work) │ Settings store   │
│ LSP client mux ──► gopls, yaml-ls, sql-ls, terraform-ls, … (per language)        │
│ DAP client ──► dlv dap │ Process runner (run configs) │ Test runner (go test -json)│
│ Lint runner (golangci-lint, vet, staticcheck) │ Git service │ PTY service        │
│ Local history store (SQLite) │ Search/index (ripgrep/trigram) │ Extension host  │
└───────────────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Stack (recommended)
- **Core**: Go 1.26+, single binary `goi-core`. Rationale: Go users can contribute; native process/PTY handling; easy cross-compile (GoLand ships 8 OS/arch Delve targets — Goi core should target the same: darwin/{amd64,arm64}, linux/{amd64,arm64}{,-musl}, windows/{amd64,arm64}).
- **UI shell**: Wails v2 (Go + system WebView) or Tauri. Front-end TypeScript + Solid/React; editor CodeMirror 6 (preferred for big-file perf) with LSP bridge; xterm.js for terminal.
- **Storage**: SQLite (local history, caches, recent projects); TOML/JSON for settings.
- Alternative: if a native-feeling multi-window app is required, the UI can be swapped without touching `goi-core` because of the RPC boundary.

### 2.2 Process model
- One `goi-core` per user session; one *workspace* per open project window.
- One `gopls` per workspace root (or per `go.work`), restarted on crash with exponential backoff; stderr captured to "Goi Log".
- Toolchain binaries resolved in order: user setting → `$PATH` → Goi-managed download dir (`~/Library/Application Support/Goi/tools/<tool>@<ver>`). Goi offers to install `gopls`, `dlv`, `golangci-lint`, `staticcheck` via `go install` (mirrors GoLand bundling Delve).

### 2.3 Core ⇄ UI protocol
JSON-RPC 2.0. Namespaces: `workspace/*`, `fs/*`, `editor/*` (LSP passthrough with doc versioning), `run/*`, `debug/*` (DAP-shaped), `test/*`, `vcs/*`, `term/*`, `settings/*`, `notify/*`. All long-running calls return a `taskId`; progress via `$/progress`. Remote mode = same protocol over TLS + token auth (parity with GoLand's split frontend / remote-dev-server).

### 2.4 Performance targets
- Cold start to interactive editor < 1.5 s (excluding gopls warm-up).
- Keystroke-to-render < 16 ms; completion popup < 150 ms p95 on a 500 kLOC repo.
- Files > 2.5 MB: syntax highlight only, no code insight (GoLand `idea.max.intellisense.filesize=2500` KB). Files > 20 MB: open read-only, paged (`idea.max.content.load.filesize=20000`).
- Memory: core + gopls < 1.5 GB on a 1 MLOC monorepo (GoLand defaults to `-Xmx2048m`).

---

## 3. Functional requirements

IDs: `F-<area>-<n>`. Priority: **M**=MVP, **2**=v0.2, **3**=v0.3.

### 3.1 Projects & SDK
| ID | Requirement | P |
|---|---|---|
| F-PRJ-1 | Open folder; detect `go.mod`, `go.work`, or GOPATH layout (GoLand has `GoOpenProjectFromGopath`) | M |
| F-PRJ-2 | GOROOT selection per project; list detected SDKs (`/usr/local/go`, `~/sdk/go*`, `~/go/pkg/mod/golang.org/toolchain*`); download SDK from go.dev | M |
| F-PRJ-3 | GOPATH settings (global + project); toggle "index entire GOPATH" | 2 |
| F-PRJ-4 | Go Modules settings: env vars (`GOPROXY`, `GOPRIVATE`, `GOFLAGS`), vendoring on/off, auto-sync on `go.mod` change | M |
| F-PRJ-5 | Build tags & GOOS/GOARCH/cgo per project; propagated to gopls `buildFlags` and `env` | M |
| F-PRJ-6 | New Project wizards: Go module, empty project, GOPATH project; welcome-screen "New…" Dockerfile / K8s resource / Terraform file | M/3 |
| F-PRJ-7 | Additional content roots; exclude dirs | M |
| F-PRJ-8 | Recent projects, welcome screen (non-modal: opens as a tab rather than a dialog) | M |
| F-PRJ-9 | `goi://open?file=…&line=…` URL scheme for stack-trace links (GoLand registers `goland://`) | 2 |

### 3.2 Editor
| ID | Requirement | P |
|---|---|---|
| F-ED-1 | Syntax highlighting for every language in §3.14 (TextMate grammars as fallback — GoLand bundles a TextMate plugin) + LSP semantic tokens for Go | M |
| F-ED-2 | Multi-caret, column select, code folding, bracket matching, soft-wrap, minimap optional | M |
| F-ED-3 | Inlay hints (param names, types), code vision lenses ("N usages", "N implementations", "run test") | 2 |
| F-ED-4 | Gutter markers: implements/implemented-by, overrides, recursion, run/debug icons on `main`, `TestX`, `BenchmarkX`, `FuzzX`, `ExampleX` | M |
| F-ED-5 | Live templates with variables + Surround-with; ship Go set (`for`, `forr`, `err`, `fmain`, `meth`, `test`, `bench`, `json` tag, …) and Go-template set (`if`, `ifel`, `elif`, `with`, `range`, `rangekv`, `def`, `templ`, `val`) | 2 |
| F-ED-6 | Postfix completion (`expr.if`, `.nn`, `.rr` return-err, `.for`, `.var`, `.sort`…) | 2 |
| F-ED-7 | Struct tag editing: completion of keys (`json`, `yaml`, `db`, `xml`…) and options (`omitempty`, `omitzero`), rename tag names, change naming style (camel/snake/kebab) | 2 |
| F-ED-8 | Language injection: SQL/JSON/regexp/HTML inside Go string literals (by comment `// language=sql` or call-site heuristics) | 3 |
| F-ED-9 | Spelling & grammar check in comments/strings | 3 |
| F-ED-10 | Actions on save: gofmt / goimports / gofumpt, optimize imports, golangci-lint `--fix`, run `go mod tidy` on go.mod save | M |
| F-ED-11 | EditorConfig support | 2 |
| F-ED-12 | Local history (every save + external change, diffable, restorable) | 2 |
| F-ED-13 | Bookmarks, TODO tool window (configurable patterns), recent files/locations | 2 |
| F-ED-14 | Markdown preview incl. Mermaid diagrams | 2 |

### 3.3 Code intelligence (via gopls unless noted)
| ID | Requirement | P |
|---|---|---|
| F-CI-1 | Completion: basic, smart/type-aware, unimported packages (auto-add import), postfix, member completion through embedding | M |
| F-CI-2 | Navigation: go to definition / type definition / implementation(s) / super-interface / test ⇄ subject; file structure popup; breadcrumbs; navbar | M |
| F-CI-3 | Find usages (grouped by read/write, package, test vs prod) and highlight usages in file | M |
| F-CI-4 | Call hierarchy (incoming/outgoing), type hierarchy | 2 |
| F-CI-5 | Quick documentation (hover, rendered godoc), parameter info, keyword docs for Go keywords (GoLand ships per-keyword doc XML) | M |
| F-CI-6 | Diagnostics streamed into editor + Problems tool window | M |
| F-CI-7 | Search Everywhere: files, symbols (workspace/symbol), actions, settings — fuzzy, ranked by recency | M |
| F-CI-8 | Find/replace in path with regex, scope filters, preview | M |
| F-CI-9 | Structural search for Go (AST pattern), v1 via `gogrep`-style matcher | 3 |

### 3.4 Inspections & quick-fixes
Goi exposes one unified **Inspections** profile (enable/disable, severity, scope) whose providers are gopls analyzers, `go vet`, staticcheck, golangci-lint and a few native checks. The table maps GoLand's observed inspection groups to providers; each must surface a quick-fix where the provider offers one.

| Group (as observed) | Count | Representative checks | Goi provider |
|---|---|---|---|
| Probable bugs | 32 | unhandled error result, loop closure capture, shadowed var, nilness, division by zero, printf misuse, copylocks, lostcancel, structtag, unmarshal, unsafeptr, defer in goroutine, irregular iota, missing switch cases for iota consts, `errors.As` misuse, comparing errors with `==` | gopls analyzers + vet + staticcheck; native: "switch missing iota cases" |
| Declaration redundancy | 21 | unused func/type/const/var/param/type-param, redundant conversion/alias/comma/semicolon/type in composite literal, self-assignment, unused go.mod dependency | gopls `unusedparams`, `unusedvariable`, staticcheck U1000, `simplifycompositelit`; native: unused module dep (`go mod why`) |
| Code style | 15 | comment start/leading space, exported element should have comment, receiver names consistent, snake_case, redundant else, unit-specific duration suffix, error string format, unsorted imports, struct literal without field names | staticcheck ST1xxx, revive (via golangci-lint) |
| Control flow | 4 | unreachable code, infinite for, defer in loop, assignment to receiver | vet/staticcheck |
| Data-flow analysis | 6 | constant condition, nil dereference, error may be non-nil, maybe-nil, resource leak (unclosed `io.Closer`) | gopls `nilness`; native lightweight DFA is **v0.3** |
| Modernize ("go fix") | 26 | `any`, atomic types, `range int`, `min/max`, `slices.Contains/Sort`, `maps` loop, `strings.Cut/CutPrefix/SplitSeq`, `strings.Builder`, `t.Context()`, `WaitGroup.Go`, `omitzero`, `//go:build` from `+build`, `new(expr)`, `errors.AsType`, `go:fix inline` | gopls `modernize` analyzer + `go fix` |
| go.mod / dependencies | 4 | update available, deprecated module, retracted version, mergeable require blocks | native: `go list -m -u -json all` |
| General tools | 18 | golangci-lint, cyclic imports, deprecation, `context.TODO`, struct field layout/padding, test name format, fuzzing support, unnecessarily exported identifiers | golangci-lint, gopls `deprecated`, native: struct layout (`fieldalignment`) |

Additional requirements:
- F-INS-1 (M) Run golangci-lint on save/on demand; read project `.golangci.yml`/`.toml`; map issues to editor diagnostics with linter name.
- F-INS-2 (2) "Inspect Code" batch run over scope → results tree, export to SARIF/JSON.
- F-INS-3 (2) Suppress via `//nolint:<linter>` / `//lint:ignore` quick-fix.

### 3.5 Intentions & code generation
Required (each is an `Alt+Enter` code action; prefer gopls code actions where they exist, otherwise native via `go/ast` rewrite in `goi-core`):

| Category | Items | P |
|---|---|---|
| Generate | constructor, getter/setter, implement interface (stub methods), add method to interface, generate method, fill struct fields (shallow & recursive), **generate struct from JSON**, struct fields from JSON | 2 |
| Declarations | `var` ⇄ `:=`, split/merge declarations, add/remove parens on decl groups, specify type, export/unexport identifier, introduce type | 2 |
| Expressions | invert `if`, `if`→`switch`, negate expression, flip binary operator, join concatenated strings, remove keys from struct literal, move to struct initialization | 2 |
| Imports | add/remove alias, add/remove dot import, optimize imports | M |
| Struct tags | add tag key, update value, change name style | 2 |
| go:generate | run `//go:generate` for line / file / package | M |
| fmt | add missing fmt argument; mark function as printf-like | 2 |
| go.mod | mass-update dependencies, merge directives / groups | 2 |

### 3.6 Refactorings
| ID | Requirement | P |
|---|---|---|
| F-RF-1 | Rename: identifier, method, field (incl. embedded/anonymous), receiver, type param, import alias, struct tag key, **package**, file, directory — with preview of all changes | M (via gopls) / 2 (package/dir) |
| F-RF-2 | Change signature (add/remove/reorder params & results, update call sites) | 2 |
| F-RF-3 | Extract function/method, extract variable/constant, introduce parameter | 2 |
| F-RF-4 | Inline function, inline var/const | 2 |
| F-RF-5 | Move declaration/file to another package (fix imports, export as needed) | 3 |
| F-RF-6 | Convert function parameter to method receiver | 3 |
| F-RF-7 | Suggested refactoring: after editing a signature/name inline, offer "update usages" | 3 |
| F-RF-8 | Safe delete (warn if referenced) | 2 |

### 3.7 Run configurations
Three types (matching observed `GoApplication`, `GoTest`, `GoRemoteDebug`), plus generic ones.

**Go Build / Application**
- Kind: package | file | directory; working dir; program args; env vars (+ `.env` file); `go build` flags; build tags; output path; run with sudo; "run after build"; redirect input from file; before-launch tasks (another config, `go generate`, external tool).
- Runners: *Run* (build then exec), *Debug* (via dlv), *Coverage*, *Profile* (pprof), *Record & Replay* (dlv + `rr`, Linux only).

**Go Test**
- Kind: directory | package | file | function (regex) ; test framework: `go test` (also gocheck, gotest.tools/testify suites recognized for gutter icons); flags (`-race`, `-count`, `-short`, `-tags`, `-timeout`); fuzz mode (`-fuzz`, `-fuzztime`); benchmarks (`-bench`, `-benchmem`).
- Created from context (gutter / right-click / Ctrl+Shift+R).

**Go Remote** — attach to `dlv --headless --listen=host:port` (incl. inside Docker/K8s).

Also: Shell script, Makefile target, Docker (compose/Dockerfile), HTTP request.

F-RUN-1 (M) Run tool window: multiple tabs, ANSI colors, clickable `file.go:line` and panic stack traces, rerun, stop (SIGINT then SIGKILL), soft-wrap, search in output.
F-RUN-2 (M) Persist configs in `.goi/runConfigurations/*.toml` (shareable via VCS) and user-local ones in app data.
F-RUN-3 (2) Services view: all running configs/containers in one tree.

### 3.8 Test runner
- F-TST-1 (M) Parse `go test -json` stream → tree (package → test → subtests), status, duration, output per node; rerun failed / rerun single; navigate to test source; diff view for `testify`-style "expected/actual" output.
- F-TST-2 (M) Benchmarks table (ns/op, B/op, allocs/op), compare with previous run.
- F-TST-3 (2) Fuzz: show corpus entries (`testdata/fuzz/FuzzX/*` — GoLand has a dedicated "Go Fuzz Corpus" file type with run markers per entry), run a single corpus entry.
- F-TST-4 (2) Coverage: `-coverprofile` → per-line gutter coloring (covered/partial/none), coverage % in project tree, coverage tool window; merge multiple runs.
- F-TST-5 (2) Auto-test: rerun on save (toggle).

### 3.9 Debugger (DAP → `dlv dap`)
| ID | Requirement | P |
|---|---|---|
| F-DBG-1 | Line / conditional / hit-count / log breakpoints; function breakpoints; "break on panic / error" (observed `DlvErrorBreakpointType`) | M |
| F-DBG-2 | Step over/into/out, run to cursor, force step into, set next statement | M |
| F-DBG-3 | Frames per goroutine; goroutine view with grouping by function/label, **filter goroutines**, hide/reset hidden; toggle show OS threads | M |
| F-DBG-4 | Variables tree with lazy loading; options: show types, show pointer addresses, string view for `[]byte`, show unreadable vars | M |
| F-DBG-5 | Evaluate expression, watches, inline values in editor, set value | M |
| F-DBG-6 | Attach to local process by PID; remote attach | 2 |
| F-DBG-7 | Core dump analysis (`dlv core`) | 3 |
| F-DBG-8 | Record & replay with reverse stepping (`rr`, Linux) | 3 |
| F-DBG-9 | Disassembly view (Plan 9 asm highlighting) | 3 |
| F-DBG-10 | Debug tests and benchmarks from gutter | M |
| F-DBG-11 | Replay `go tool trace` file (observed `GoReplayTraceAction`) | 3 |

### 3.10 Profiling & optimization
- F-PRF-1 (3) Profile run config with CPU / memory (alloc, inuse) / block / mutex profiles; flame graph, call tree, method list; import external `.pprof`.
- F-PRF-2 (3) "Go Optimization" tool window: escape analysis (`go build -gcflags=-m=2`) mapped to editor annotations (heap escapes, inlining decisions, bounds checks).
- F-PRF-3 (3) Struct layout/padding visualizer.

### 3.11 Modules (`go.mod` / `go.work`)
- F-MOD-1 (M) Dedicated editor for `go.mod`/`go.work`/`go.sum` with completion of module paths & versions (proxy query), clickable module paths.
- F-MOD-2 (M) Actions: `go mod tidy`, `go mod vendor`, sync dependencies, `go work use`, new go.mod / go.work file.
- F-MOD-3 (2) Dependency update inspections + bulk update; deprecated/retracted warnings; migrate `replace` directives to a workspace.
- F-MOD-4 (2) External libraries node in project tree (module cache, read-only), SDK sources.
- F-MOD-5 (2) Vulnerability check via `govulncheck` with results in Problems.

### 3.12 VCS
- F-VCS-1 (M) Git: status in tree/gutter, commit (non-modal commit panel with modal option), amend, push/pull/fetch, branches popup, stash, cherry-pick, revert, blame/annotate, partial (hunk) staging.
- F-VCS-2 (M) Diff viewer (side-by-side & unified, 3-way merge with conflict resolution).
- F-VCS-3 (2) Log graph with filters (branch, author, path, date).
- F-VCS-4 (3) GitHub & GitLab: clone, create/review PR/MR, comments inline.

### 3.13 Integrated tools
| ID | Tool | P |
|---|---|---|
| F-TL-1 | Terminal (PTY, multiple tabs, shell integration, clickable paths, `go` command completions) | M |
| F-TL-2 | HTTP client: `.http` files, environments, run & view responses; generate requests from Go `net/http`/gin/echo/chi routes (observed "microservices"/endpoints) | 2 |
| F-TL-3 | OpenAPI/Swagger editor + preview | 3 |
| F-TL-4 | Database tool: connections (Postgres, MySQL, SQLite, Mongo…), query console, grid editor, SQL completion against schema; run via `sql-ls` or native | 3 |
| F-TL-5 | Docker: images/containers/compose in Services, Dockerfile run configs, run Go configs inside container | 3 |
| F-TL-6 | Kubernetes: contexts, resources, logs, port-forward, YAML schema validation | 3 |
| F-TL-7 | Terraform/OpenTofu editing (terraform-ls) | 3 |
| F-TL-8 | Protobuf editing (buf / protols) and go_package navigation | 2 |
| F-TL-9 | SSH: remote hosts, SFTP deployment, remote interpreters | 3 |
| F-TL-10 | File watchers (run a tool on file change) | 3 |
| F-TL-11 | Go Playground: open/share snippet | 3 |
| F-TL-12 | Copyright header management | 3 |

### 3.14 Supported languages & file types
- **Go family (deep support)**: Go, cgo preamble, `go.mod`/`go.work`/`go.sum`, Go templates (`.gotmpl`, `text/template`/`html/template` with type hints and unknown-variable checks), struct tags, `//go:build` / `+build` constraints, `//go:debug` directives, `time.Format` layout strings, fuzz corpus files, Plan 9 x86 assembly (`.s`).
- **Supporting (LSP or TextMate)**: YAML, JSON, TOML, Markdown, Mermaid, SQL, Dockerfile/Containerfile, Makefile, shell (`sh`, `bash`, `zsh`), protobuf, Terraform/`.tf`/`.tofu`, HTML/CSS/JS/TS (basic), XML, `.http`, `.env`, `.editorconfig`, gitignore/gitattributes, INI/properties.
- Full extension list observed in the bundle's document types (to register with the OS): bash, c/cpp/h, cmake, css/scss/less/sass, csv, dart, diff, dockerfile, go, gotmpl, gradle, html, ini, ipynb, js/jsx/ts/tsx/mjs/cjs, json, lua, makefile/mk, markdown variants, php, pl, proto, ps1, py/pyi, r, rst, sh/zsh, sql, tf/tofu, toml, txt/log, vue, xml/xaml, yaml/yml.

### 3.15 UI shell
- F-UI-1 (M) Main window: project tree (left), editor tabs (center, splits), bottom tool windows (Run, Debug, Terminal, Problems, Tests, Git), right (Structure, Database, Services). Draggable, hideable, per-project layout persistence.
- F-UI-2 (M) Command palette / Search Everywhere (double-Shift), Recent files (Cmd+E), Go to file/symbol/action.
- F-UI-3 (M) Keymaps: Goi default, VS Code-compatible (observed `keymap-vscode`), macOS/Windows variants; fully remappable incl. chords.
- F-UI-4 (M) Themes: light, dark, high contrast; editor color schemes incl. Monokai- and Solarized-style **original** palettes; per-language token colors.
- F-UI-5 (2) Settings UI with search; settings stored as layered TOML: defaults → user → project (`.goi/settings.toml`) ; settings sync (export/import, optional cloud).
- F-UI-6 (2) Notifications/balloons; status bar widgets (branch, gopls status, Go version, line sep, encoding, indexing progress).
- F-UI-7 (3) Localization (ship en; ja, ko, zh-Hans observed in bundle).
- F-UI-8 (3) Onboarding / interactive feature trainer ("Learn" lessons in a sample project).
- F-UI-9 (M) Accessibility: screen-reader labels, full keyboard navigation.

### 3.16 AI & automation (v0.3)
- F-AI-1 Inline/full-line code completion via pluggable provider (local model or API).
- F-AI-2 **MCP server**: expose IDE capabilities (open file, get diagnostics, run config, run tests, search symbols, apply edit) to external agents (observed `mcpserver` plugin).
- F-AI-3 Agent Client Protocol host: run external coding agents inside the IDE with diff-review of their edits (observed `platform-acp-plugin`).
- F-AI-4 ML ranking of completion items (recency + acceptance stats, opt-in).

### 3.17 CLI
- `goi <path>[:line[:col]]` — open project/file.
- `goi diff <a> <b>`, `goi merge <l> <r> <base> <out>` (git difftool/mergetool).
- `goi format <paths…>` and `goi inspect <project> --profile <p> --out <sarif>` (headless; GoLand ships `format.sh`/`inspect.sh`).
- `goi --remote <host>` — start split mode.

---

## 4. Non-functional requirements
| ID | Requirement |
|---|---|
| NF-1 | Platforms: macOS 12+ (arm64, x86_64), Linux (x86_64, arm64, glibc & musl), Windows 10+ (x86_64, arm64) |
| NF-2 | Crash safety: unsaved buffers journaled every 2 s; restore on restart |
| NF-3 | No UI freeze > 100 ms; all FS/tool work off the UI thread; freeze detector writes thread dumps to log |
| NF-4 | Telemetry: off by default; opt-in, documented schema, local-only by default |
| NF-5 | Security: tools run with user privileges; untrusted-project mode (no auto-run of `go generate`, tasks, or file watchers until the user trusts the folder); credentials in OS keychain |
| NF-6 | Logs at `<data-dir>/logs/goi.log`, rotating; per-tool log channels (gopls, dlv, lint) |
| NF-7 | Data dirs: config `~/Library/Application Support/Goi/<major.minor>`, caches `~/Library/Caches/Goi/<ver>` (and XDG/AppData equivalents); version-scoped like GoLand's `GoLand2026.2` |
| NF-8 | Offline-capable after first toolchain install |

---

## 5. Data model (core)
```
Workspace { id, roots[], goWork?: path, modules[]: Module, settings: Layered }
Module    { path, dir, goVersion, requires[], replaces[], vendored: bool }
Sdk       { goroot, version, goos, goarch }
RunConfig { id, name, type: app|test|remote|shell|make|docker|http, shared: bool, params: map, beforeLaunch[] }
TestNode  { id, pkg, name, parent?, status: pass|fail|skip|run, durationMs, output[] }
Breakpoint{ file, line, condition?, hitCount?, logMessage?, enabled, kind: line|func|panic }
Inspection{ id, provider: gopls|vet|staticcheck|golangci|goi, severity, enabled, scope }
HistoryEntry { file, ts, contentHash, label? }   // SQLite
```
Project-shared files live in `.goi/` (runConfigurations, settings.toml, inspection profile, dictionaries); user-local state in the data dir keyed by workspace hash.

---

## 6. Milestones & acceptance

**M1 — Editor + Project (4–6 wks)**: F-PRJ-1/2/4/5/7/8, F-ED-1/2/4/10, F-UI-1/2/3/4. *Accept*: open a `go.work` monorepo, edit, gofmt on save, theme switch.

**M2 — Intelligence (4 wks)**: gopls integration F-CI-1..3/5..8, diagnostics, quick-fixes, rename. *Accept*: completion p95 < 150 ms on `kubernetes/kubernetes`; rename across modules.

**M3 — Run/Test/Debug (6 wks)**: §3.7, F-TST-1/2, F-DBG-1..5/10, terminal F-TL-1. *Accept*: debug a test with goroutine filter and conditional breakpoint; rerun failed subtest.

**M4 — Git + Lint + Modules (4 wks)**: F-VCS-1/2, F-INS-1, F-MOD-1/2. → **v0.1 release**.

**M5 — v0.2**: generators/intentions, refactorings, coverage, fuzz, HTTP client, protobuf, inspection batch run, search everywhere polish, local history.

**M6 — v0.3**: profiling & optimization window, Docker/K8s/DB/Terraform, remote mode, MCP/ACP/AI.

---

## 7. Open questions
1. UI stack: Wails/Tauri (web UI) vs. native toolkit — trade-off between dev speed and native feel.
2. Should Goi build any in-process Go analysis (e.g., DFA nil-checks, iota switch exhaustiveness) or contribute those as gopls analyzers upstream?
3. Bundle `dlv`/`gopls` binaries (as GoLand bundles Delve) vs. install on demand — affects signing/notarization and app size.
4. Licensing model and whether an extension API is in v1.

---

## Appendix A — Bundled plugin inventory (as observed)
Go: `go-plugin`, `go-linter` (golangci-lint), `go-template`, `go-sharedIndexes-bundled`, `goland-customization-plugin` (welcome screen, trial). Platform: terminal, vcs-git, vcs-github, vcs-gitlab, vcs-git-commit-modal, platform-{bookmarks, navbar, recentFiles, structureView, structuralSearch, testRunner, todo, ssh, images, execution-serviceView, daemon, ijent-bundledBinaries, acp}, settingsSync, searchEverywhereMl, featuresTrainer, fileWatcher, copyright, editorconfig, configurationScript, packageChecker, performanceTesting. Languages: markdown, mermaid, yaml, json, jsonpath, sh, makefile, protoeditor, terraform, textmate, css, html-tools, javascript (+debugger, eslint, intentions), react, webpack, stylelint, swagger, groovy-scripting, xml-refactoring. Tools: DatabaseTools, grid-core, grid, restClient, microservices, webDeployment, clouds-docker-{impl,gateway}, clouds-kubernetes, remoteRun, remote-dev-server, gateway, cwm (code-with-me), uml, qodana, mcpserver, fullLine, station, jcef (embedded Chromium). Cosmetic: 3 color schemes, keymap-vscode, localization ja/ko/zh.

## Appendix B — Evidence artifacts
Raw REA evidence JSON and extracted descriptors were written to the session scratchpad (not committed): `desktop.json`, `plist.json`, `dlv.json`, `launcher.json`, and `desc/**.xml`.
