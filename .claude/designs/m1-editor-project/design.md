# M1 — Editor + Project Model: Design

Satisfies `requirements.md` (R1–R13, NFR-1..6). Conforms to `.claude/steering/*`.

---

## 1. Architecture overview

```
┌──────────────────────── OS process: one per project window ────────────────────────┐
│                                                                                     │
│  Frontend (WebView, TS + CodeMirror 6)                                              │
│  ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌─────────┐ ┌────────┐ ┌───────────────┐   │
│  │ Shell/    │ │ Editor    │ │ Project  │ │ Palette │ │ Keymap │ │ Theme/Settings│   │
│  │ Layout    │ │ Manager   │ │ Tree     │ │         │ │        │ │ UI            │   │
│  └────┬─────┘ └────┬──────┘ └────┬─────┘ └────┬────┘ └────────┘ └──────┬────────┘   │
│       └────────────┴──── src/lib/api.ts (typed wrappers) ──────────────┘            │
│                              │ Wails bindings (calls)   ▲ Wails events (topics)     │
│  ────────────────────────────┼──────────────────────────┼─────────────────────────  │
│  Go backend                  ▼                          │                           │
│  internal/rpc   (Wails-bound facades + EventBus adapter; ONLY package importing Wails)│
│       │                                                                             │
│       ├─ workspace ─ detection, roots, excludes, project model, go.mod watcher      │
│       ├─ vfs ─────── read/write w/ EOL+BOM, tree listing, fs watcher, trash, reveal │
│       ├─ gosdk ───── SDK discovery, `go env -json`, version probing, command runner │
│       ├─ settings ── layered TOML (defaults→user→project), provenance, hot reload   │
│       ├─ format ──── gofmt / goimports                                              │
│       ├─ gosyntax ── go/parser-based run markers                                    │
│       ├─ index ───── file index + fuzzy matcher                                     │
│       ├─ journal ─── unsaved-buffer journal + crash detection                       │
│       ├─ recent ──── recent projects store (cross-process, file-locked)            │
│       └─ applog ──── rotating log (slog → lumberjack-style writer)                  │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### 1.1 Multi-window strategy (R12.3)
Wails v2 supports **one window per process**, so each project window is its own Goi process:
- `goi` with no args → Welcome window (or reopens the last project, R1.5).
- "Open in New Window" / opening a 2nd project → backend runs `os.Executable()` with `--project <abs path>` (macOS: `open -n -a <bundle> --args --project …`, so the new window joins the Dock icon).
- If a process already has that project open, focus it instead of opening a duplicate. Each process writes `<UserCacheDir>/Goi/<ver>/instances/<wsHash>.lock` (pid + a loopback port). A second launch for the same project pings that port with `focus` and exits.
- Shared state (recent projects, user settings) lives in files guarded by a lock file and watched by every process, so changes propagate between windows.
- Wails `SingleInstanceLock` is **not** used (it would force one window app-wide).

Trade-off: higher memory per window, but process isolation and a clean path to the remote core later. Wails v3 (multi-window) can replace this later without touching `internal/`.

---

## 2. Backend packages

All packages are UI-agnostic, context-aware and tested. Key types and signatures follow.

### 2.1 `internal/events` (shared, tiny)
```go
type Bus interface { Emit(topic string, payload any) }
type Nop struct{}                     // tests
```
`rpc` provides the Wails implementation (`runtime.EventsEmit`). Domain packages receive a `Bus` and never import Wails.

**Topics (backend → UI):**
| Topic | Payload |
|---|---|
| `fs.changed` | `{events: [{path, op: create\|remove\|rename\|write, isDir}]}` (debounced 75 ms batches) |
| `workspace.changed` | `Project` (full model after re-detect) |
| `settings.changed` | `{layer, effective: Settings}` |
| `recent.changed` | `[]RecentEntry` |
| `index.ready` / `index.updated` | `{count}` |
| `notify` | `{id, level: info\|warn\|error, title, body, file?, line?, actions?[]}` |
| `markers.updated` | `{path, version, markers: []RunMarker}` |

### 2.2 `internal/workspace` (R2, R5.2–5.3)
```go
type Kind string // "workspace" | "module" | "multi-module" | "gopath" | "folder"

type Module struct {
    Path      string // module path from go.mod
    Dir       string // absolute
    GoVersion string
}
type Project struct {
    Root         string
    Kind         Kind
    Modules      []Module
    ContentRoots []string // Root + additional roots
    Excludes     []string // root-relative globs
    Hash         string   // sha256(real root path)[:16], keys user-side state
    ParseErrors  []ParseError
}
type ParseError struct{ File string; Line int; Msg string }

func Detect(ctx context.Context, root string, gopath []string, excludes []string) (Project, error)
type Manager struct{ /* holds Project, watcher subscription, bus */ }
func Open(ctx context.Context, root string, s settings.Provider, w vfs.Watcher, bus events.Bus) (*Manager, error)
func (m *Manager) Project() Project
func (m *Manager) AddContentRoot(path string) error
func (m *Manager) SetExcludes(globs []string) error
```
- Parse `go.mod`/`go.work` with `golang.org/x/mod/modfile`. On error, record a `ParseError`, use Kind `folder` and emit `notify` (R2.4).
- Nested-module scan: `filepath.WalkDir`, depth ≤ 4, skipping excludes and `vendor/`, `testdata/`.
- Re-detect when `fs.changed` touches any `go.mod`/`go.work` (debounced 300 ms, so well under the 1 s target).
- Never runs project code (R2.6).

### 2.3 `internal/vfs` (R5, R6.5–6.8, NFR-4)
```go
type FileMeta struct {
    Path     string
    Size     int64
    ModTime  time.Time
    Binary   bool
    EOL      string // "\n" | "\r\n" | "\r" (majority vote; default "\n")
    BOM      bool
    FinalNL  bool
    Mode     OpenMode // Normal | NoHighlight (>2.5MB) | ReadOnly (>20MB)
}
func ReadText(path string) (FileMeta, string, error) // content normalized to "\n"
func WriteText(path string, content string, meta FileMeta) (FileMeta, error)
//  -> restores EOL/BOM/final-newline; atomic write (temp file in same dir + rename;
//     on Windows uses os.Rename replace semantics with retry on sharing violation);
//     preserves file mode.
type Entry struct{ Name, Path string; IsDir bool; Size int64 }
func List(dir string, excludes Matcher) ([]Entry, error)   // sorted: dirs first, natural order
func Create(path string, isDir bool, content string) error
func Rename(from, to string) error
func Trash(path string) error   // trash_darwin.go | trash_windows.go | trash_linux.go
func Reveal(path string) error  // open -R | explorer /select, | xdg-open <dir>

type Watcher interface { Subscribe(fn func([]Event)) (cancel func()); Close() error }
func NewWatcher(roots []string, excludes Matcher, bus events.Bus) (Watcher, error)
```
- Binary detection: NUL byte in the first 8 KB (R6.6).
- Size thresholds come from settings (`editor.maxHighlightSizeKB=2500`, `editor.maxOpenSizeKB=20000`).
- Watcher: fsnotify, recursive add on dirs (fsnotify isn't recursive), excludes skipped, new dirs added on create. Events coalesced over 75 ms then emitted once. On macOS, a later swap to FSEvents is an option behind the same interface.
- Trash:
  - **darwin:** `osascript -e 'tell application "Finder" to delete POSIX file "<p>"'`
  - **windows:** `SHFileOperationW` with `FOF_ALLOWUNDO` via `golang.org/x/sys/windows`
  - **linux:** FreeDesktop Trash spec (`$XDG_DATA_HOME/Trash/{files,info}`), falling back to an error that asks the user to confirm a permanent delete
- Matcher: gitignore-style globs (`github.com/bmatcuk/doublestar/v4`).

### 2.4 `internal/gosdk` (R3, R4)
```go
type SDK struct{ GOROOT, Version, GoBin string; Source string /* path|PATH|toolchain|manual */ }
func Discover(ctx context.Context) []SDK          // R3.1 locations; dedupe by EvalSymlinks
func Validate(goroot string) (SDK, error)         // R3.6: bin/go[.exe] exists; version from VERSION file or `go version`
type Env struct{ Vars map[string]string; Tags []string; GOOS, GOARCH, CGO string; Vendor string }
type Runner struct{ SDK SDK; Env Env; Dir string }
func (r Runner) Command(ctx context.Context, args ...string) *exec.Cmd // merges os.Environ + Env; -tags; -mod=vendor|mod
func (r Runner) GoEnv(ctx context.Context) (map[string]string, error)  // `go env -json`
```
- Every `go` invocation in Goi goes through `Runner`, so R4.2 holds automatically.
- Version probing is cached by (GOROOT, mtime of `bin/go`).
- Cross-target flag (R4.4) = `GOOS/GOARCH != runtime.GOOS/GOARCH`.

### 2.5 `internal/settings` (R13, persistence for R3–R11)
```go
type Layer string // "default" | "user" | "project"
type Settings struct {
    Startup  struct{ ReopenLastProject bool }                         `toml:"startup"`
    Editor   struct{ FontFamily string; FontSize int; TabSize int; SoftWrap bool
                     MaxHighlightSizeKB, MaxOpenSizeKB int }           `toml:"editor"`
    Go       struct{ GOROOT string; Env map[string]string; Tags []string
                     GOOS, GOARCH, CGO, Vendor string }               `toml:"go"`
    Project  struct{ ContentRoots, Excludes []string }                `toml:"project"`
    Save     struct{ Format string /* goimports|gofmt|none */ }       `toml:"save"`
    UI       struct{ Theme string /* auto|light|dark|hc */; Scheme string; Keymap string } `toml:"ui"`
    Keys     map[string][]string                                       `toml:"keys"` // user-only overrides
}
type Provider interface {
    Effective() Settings
    Origin(key string) Layer            // dotted key, e.g. "go.goroot"
    Set(layer Layer, key string, value any) error
    Subscribe(fn func(Settings)) (cancel func())
}
func Load(userPath, projectPath string, w vfs.Watcher, bus events.Bus) (Provider, error)
```
- Merge: decode each layer into `map[string]any`, deep-merge (project over user over defaults; maps merged per key, slices replaced), then decode into `Settings`. Provenance is recorded per leaf key during the merge.
- `Set` edits the layer's TOML document with `go-toml/v2` and writes atomically. Comments in user files are lost on rewrite; accepted for M1 and noted.
- Invalid TOML on reload: keep the last good layer, emit `notify` with the line from the decode error (R13.2).
- Defaults are embedded (`defaults.toml` via `embed`) and also serve as documentation.

### 2.6 `internal/format` (R8)
```go
type Mode string // "goimports" | "gofmt" | "none"
type Result struct{ Content string; Changed bool }
type SyntaxError struct{ Line, Col int; Msg string }
func Format(path, src string, mode Mode, localPrefix string) (Result, error) // returns *SyntaxError on parse failure
```
- `gofmt` → `go/format.Source`; `goimports` → `golang.org/x/tools/imports.Process` (`Comments:true, TabIndent:true, TabWidth:8, FormatOnly:false`).
- Runs in-process, so no SDK is required. Performance target is checked by a benchmark (`BenchmarkFormat5kLines`).

### 2.7 `internal/gosyntax` (R7)
```go
type MarkerKind string // main|test|bench|fuzz|example
type RunMarker struct{ Kind MarkerKind; Name string; Line int }
func RunMarkers(filename, src string) []RunMarker // go/parser with parser.SkipObjectResolution; tolerant of errors
```
Rules: `main` only if `package main` and `func main()` without params or results. Test, Bench and Fuzz require a `_test.go` file, the right prefix with the next rune not lowercase, and one param of type `*testing.T|B|F` (selector `testing.X` or a dot-import alias). Example requires no params.

### 2.8 `internal/index` (R9)
```go
type Index struct{ /* []string relPaths, lowercased forms, segment boundaries */ }
func Build(ctx context.Context, roots []string, ex vfs.Matcher) (*Index, error)
func (ix *Index) Apply(events []vfs.Event)
type Match struct{ Path string; Score int; Positions []int }
func (ix *Index) Search(q string, limit int, recent []string) []Match
```
- Scorer (fzf-style): subsequence match; bonuses for segment start (`/`, `_`, `-`, `.`, camel hump), consecutive runs, basename match, and recent-file boost. Penalty for path depth.
- Parallel scoring across `GOMAXPROCS` shards. Target < 100 ms at 50k paths, checked by a benchmark with a synthetic 50k corpus.
- The action and settings tabs of the palette are searched in the frontend (small sets).

### 2.9 `internal/journal` (R6.10, NF-2)
```go
type Entry struct{ Path string; Content string; BaseModTime time.Time; SavedAt time.Time }
type Journal struct{ dir string }
func Open(cacheDir, wsHash string) (*Journal, crashed bool, err error) // crashed = session.lock existed & pid not alive
func (j *Journal) Put(e Entry) error     // file name sha1(path).json, atomic write
func (j *Journal) Drop(path string) error
func (j *Journal) Pending() ([]Entry, error)
func (j *Journal) CloseClean() error     // removes session.lock and entries
```
The frontend calls `Put` for dirty buffers on a 2 s ticker (only buffers changed since the last put) and `Drop` after save or discard.

### 2.10 `internal/recent` (R1)
```go
type Entry struct{ Path, Name string; Kind workspace.Kind; OpenedAt time.Time; Exists bool }
func Load(path string) (*Store, error) // JSON, max 20, flock-protected (gofrs/flock)
func (s *Store) Touch(e Entry) error; Remove(path string) error; List() []Entry
```

### 2.11 `internal/applog` (NFR-5)
`slog.New(slog.NewJSONHandler(rotating))`. The rotating writer is ~80 LOC (10 MB × 5), with no dependency. Wails' own logger is routed into it.

### 2.12 `internal/rpc` — Wails-bound services
Each service is a struct bound in `main.go`. Methods return DTOs (JSON-tagged) and errors; Wails generates the TS types.

| Service | Methods |
|---|---|
| `AppService` | `Startup(ctx)`, `Info() AppInfo`, `OpenFolderDialog() string`, `OpenProject(path string, newWindow bool) error`, `PendingRestore() []journal.Entry`, `Quit()` |
| `WorkspaceService` | `Project() Project`, `AddContentRoot(p)`, `SetExcludes([]string)`, `SDKStatus() SDKStatus`, `GoEnv() map[string]string` |
| `FileService` | `List(dir) []Entry`, `Read(path) OpenedFile{Meta, Content}`, `Write(path, content, meta) FileMeta`, `Create(path, isDir, content)`, `CreateGoFile(dir, name)`, `Rename`, `Trash`, `Reveal`, `CopyPath` |
| `SDKService` | `Discover() []SDK`, `Validate(goroot) SDK`, `Select(goroot)` |
| `SettingsService` | `Effective() Settings`, `Origins() map[string]Layer`, `Set(layer, key, value)`, `Schema() []SettingDescriptor` (for the settings UI) |
| `EditorService` | `Format(path, content) FormatResult`, `RunMarkers(path, content, version) []RunMarker`, `JournalPut(entry)`, `JournalDrop(path)` |
| `SearchService` | `Files(query, limit) []Match` |
| `RecentService` | `List()`, `Remove(path)` |

`AppService.Startup` order: parse flags → applog → settings(user) → recent → if `--project`, then workspace.Open → watcher → settings(project) → index.Build (async, emits `index.ready`) → journal.Open. A crash flag is exposed via `PendingRestore`.

Shutdown (`OnBeforeClose`): the frontend is asked whether dirty buffers exist (`app.beforeClose` event + reply binding). If so, Wails blocks the close until the user decides (R6.9). After that, `journal.CloseClean`, watcher close, instance lock release.

---

## 3. Frontend design

### 3.1 Tech
TypeScript strict, Vite, no UI framework. Small reactive store (`src/lib/store.ts`, ~60 LOC signal/subscribe) plus plain DOM components. Dependencies:
- `codemirror` and `@codemirror/{state,view,commands,language,search,autocomplete,lint,fold}`
- `@codemirror/lang-{go,javascript,json,yaml,markdown,sql,html,css,xml}`
- `@codemirror/legacy-modes` (toml, shell, dockerfile, protobuf, properties)
- `@tanstack/virtual-core` for the virtualized tree and palette
- dev: `vitest`, `@types/*`

### 3.2 Module map
```
frontend/src/
├── main.ts                 # boot: load settings, theme, keymap, route Welcome | Workbench
├── lib/
│   ├── api.ts              # typed wrappers over wailsjs/go/rpc/* and EventsOn topics
│   ├── store.ts            # signal(), computed(), effect()
│   ├── commands.ts         # Command registry: {id, title, category, run, when?}
│   └── dom.ts              # h() helper, disposables
├── app/
│   ├── workbench.ts        # CSS-grid shell: left | center | right / bottom / status
│   ├── panels.ts           # Panel registry, resize splitters, collapse, layout persistence
│   ├── statusbar.ts        # SDK, GOOS/GOARCH (+ cross badge), cursor, EOL, encoding, index progress
│   └── notifications.ts    # toasts + Notifications panel (consumes `notify`)
├── views/
│   ├── welcome.ts          # R1
│   ├── projectTree.ts      # R5: virtualized tree, context menu, fs.changed reconcile
│   ├── settings.ts         # R13.3: searchable form generated from Schema() + origin badges
│   └── sdkPicker.ts        # R3 dialog
├── editor/
│   ├── editorManager.ts    # buffers (path→EditorState+meta+dirty), tabs, splits (2×2 grid)
│   ├── tabs.ts             # drag reorder, pin, close variants
│   ├── languages.ts        # filename/extension → LanguageSupport (lazy dynamic import)
│   ├── lang/gomod.ts       # StreamLanguage for go.mod/go.work/go.sum
│   ├── lang/gotmpl.ts      # StreamLanguage for {{ }} actions over HTML/text
│   ├── extensions.ts       # base extension set (multi-cursor, fold, brackets, search, …)
│   ├── runGutter.ts        # R7 gutter from markers.updated / RunMarkers()
│   ├── largeFile.ts        # banners + read-only/no-highlight modes (R6.5)
│   ├── saveFlow.ts         # R8: format → single transaction → Write; syntax-error lint diagnostic
│   ├── diskSync.ts         # R6.8 reload / conflict banner / compare (CM merge view)
│   └── journalTicker.ts    # R6.10 2 s dirty-buffer journaling
├── palette/
│   ├── palette.ts          # R9 Search Everywhere UI (tabs All/Files/Actions/Settings)
│   └── rank.ts             # client-side ranking for actions/settings (shares scorer logic)
├── keymap/
│   ├── keymaps.ts          # "goi-default", "vscode" × mac/other tables: commandId → keys[]
│   ├── dispatcher.ts       # global keydown → chord state machine → commands.run
│   └── doubleShift.ts      # double-Shift detector (≤ 300 ms, no other key between)
└── theme/
    ├── tokens.css          # CSS custom properties per theme (light/dark/hc)
    ├── schemes.ts          # 4 editor schemes → CM HighlightStyle + EditorView.theme
    └── theme.ts            # auto mode via matchMedia('(prefers-color-scheme: dark)')
```

### 3.3 Key flows
- **Open file:** tree click → `editorManager.open(path)` → `FileService.Read` → language by name → `EditorState` with the `largeFile` mode → tab. `meta` (EOL/BOM) is kept with the buffer.
- **Save (Cmd/Ctrl+S):** `saveFlow`: if `.go` and format ≠ none → `EditorService.Format` → if changed, dispatch one transaction with a minimal diff (computed with `@codemirror/merge`'s `diff` helper, so cursors map correctly) → `FileService.Write(content, meta)` → clear dirty → `JournalDrop`. On `SyntaxError` → set a lint diagnostic, save unformatted, status message (R8.3).
- **External change:** `fs.changed` for an open path → if not dirty, re-read and replace the doc preserving selection (R6.8). If dirty, show the conflict banner. Self-writes are ignored by comparing the returned `FileMeta.ModTime` and size.
- **Run markers:** on doc change debounce 300 ms → `EditorService.RunMarkers(path, content, version)` → update `runGutter` when the version matches.
- **Layout persistence:** `panels.ts` serializes `{sizes, visibility, tabs:[{path, cursor, scroll, pinned}], splits}` to `<UserConfigDir>/Goi/<ver>/workspaces/<hash>/layout.json` via `SettingsService`-adjacent `AppService.SaveLayout/LoadLayout` (2 extra methods), debounced 1 s.

### 3.4 Commands & keymaps
All user actions are registered as commands (`file.save`, `file.saveAll`, `editor.reformat`, `nav.searchEverywhere`, `nav.recentFiles`, `nav.gotoLine`, `view.toggleProject`, `editor.splitRight`, …). Keymaps map command IDs to key sequences such as `"Mod-k Mod-s"`. User overrides live in `settings.keys`. The dispatcher detects conflicts and the settings UI shows the replace prompt (R10.3). CodeMirror's own keymap is built from the same table, so the editor and global shortcuts agree.

### 3.5 Theming
Tokens are things like `--bg`, `--fg`, `--panel-bg`, `--border`, `--accent`, `--selection`, and `--syn-keyword`, `--syn-string`, etc. Schemes set only `--syn-*` and editor tokens; UI themes set the rest. High Contrast is validated by a vitest test that computes WCAG contrast for every fg/bg token pair (R11.4).

---

## 4. Data & file locations

| Data | Location | Format |
|---|---|---|
| User settings | `<UserConfigDir>/Goi/0.1/settings.toml` | TOML |
| Project settings | `<root>/.goi/settings.toml` | TOML |
| Recent projects | `<UserConfigDir>/Goi/0.1/recent.json` (+ `.lock`) | JSON |
| Window layout | `<UserConfigDir>/Goi/0.1/workspaces/<hash>/layout.json` | JSON |
| Journal | `<UserCacheDir>/Goi/0.1/journal/<hash>/` | JSON per buffer + `session.lock` |
| Instance locks | `<UserCacheDir>/Goi/0.1/instances/<hash>.lock` | text (pid, port) |
| Logs | `<UserCacheDir>/Goi/0.1/logs/goi.log` | JSON lines |

---

## 5. Go dependencies
`github.com/wailsapp/wails/v2`, `golang.org/x/mod`, `golang.org/x/tools` (imports), `golang.org/x/sys`, `github.com/fsnotify/fsnotify`, `github.com/pelletier/go-toml/v2`, `github.com/bmatcuk/doublestar/v4`, `github.com/gofrs/flock`. Nothing else without updating `tech.md`.

## 6. Build & platform notes
- `wails.json`: `frontend:install` = `npm ci`, `frontend:build` = `npm run build`, `outputfilename` = `goi`.
- Linux: build with `-tags webkit2_41`.
- macOS: `build/darwin/Info.plist` registers document types for `.go`, `go.mod`, `.gotmpl`, etc. (opening a file from Finder → `--project <parent root>` plus open file). Code signing and notarization are out of M1.
- Windows: WebView2 bootstrapper embedded (`wails build -webview2 embed`).
- CI (`.github/workflows/ci.yml`): a matrix of `{macos-latest, windows-latest, ubuntu-24.04}` runs `go vet`, `go test ./internal/...`, `npm test` and `wails build`. A separate arm64 compile-only job covers NFR-1.

## 7. Testing strategy
| Area | Tests |
|---|---|
| workspace.Detect | table-driven fixtures in `testdata/` (work, module, nested, gopath, broken go.mod, Windows-style paths) |
| vfs Read/Write | round-trip for LF/CRLF/CR/mixed, BOM, no-final-newline, binary detection, size modes; atomic write leaves no temp files |
| watcher | create/rename/delete batching, new-dir auto-watch, excludes (with `Eventually` helper, 2 s timeout) |
| gosdk | Discover with fake GOROOT trees; Validate errors; Runner env/tags/mod flags (asserts `cmd.Args`/`cmd.Env`) |
| settings | layer merge, provenance, invalid TOML keeps last good, Set round-trip |
| format | gofmt/goimports goldens, syntax error position, benchmark 5k lines |
| gosyntax | marker rules incl. `Testfoo` (not a test), dot-imported testing, generics |
| index | scoring order goldens, benchmark 50k |
| journal/recent | crash detection via stale pid, flock contention between two goroutines |
| frontend | vitest: keymap conflicts and chord parsing, double-Shift detector, rank.ts, theme contrast |
| manual | checklist per OS in `tasks.md` final task (open/edit/save/format/watch/restore) |

## 8. Risks & mitigations
| Risk | Mitigation |
|---|---|
| fsnotify on huge trees (Linux inotify watch limit) | respect excludes; on `ENOSPC` emit a `notify` with the sysctl fix and fall back to polling open files only |
| WebKitGTK/WebView2 rendering differences | CI builds on all 3; CSS limited to features in Safari 15 / Chromium 100 |
| One process per window uses more memory | acceptable for M1; revisit with Wails v3 multi-window |
| goimports needs module context to resolve imports | pass `imports.Options` with the file path inside the module; it uses the module cache read-only (no network) |
| Comment loss when Goi rewrites TOML | documented; only keys set via the UI rewrite the file; consider `toml-edit`-style preserving writer later |
