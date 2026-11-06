# M1 — Editor + Project Model: Requirements

Source: `SPEC.md` §3.1, §3.2, §3.15, §4. Spec IDs are in brackets.
Scope: a usable cross-platform editor for Go projects, with no language server, debugger, terminal or git.

## Out of scope for M1
gopls / code intelligence, run/test/debug, terminal, git, live templates, inlay hints, local history, settings sync, localization, the SDK download from go.dev [F-PRJ-2 download → M2], GOPATH-only project mode beyond detection [F-PRJ-3].

---

## R1 — Welcome screen & recent projects [F-PRJ-8]
**Story:** As a developer, I want a start screen listing my recent projects so I can reopen work quickly.

1. WHEN Goi starts with no project to restore THEN the system SHALL show the Welcome screen with "Open Folder…", "New Project…" and a list of recent projects (most recent first, max 20).
2. WHEN the user selects a recent project THEN the system SHALL open it in the main window.
3. IF a recent project's folder no longer exists THEN the system SHALL show it greyed out with a "Remove" action and SHALL NOT try to open it.
4. WHEN a project is opened THEN the system SHALL move it to the top of the recent list and persist the list in the user config dir.
5. WHEN Goi starts and the last session had a project open THEN the system SHALL reopen that project (user setting `startup.reopenLastProject`, default true).

## R2 — Open folder & project detection [F-PRJ-1]
**Story:** As a developer, I want Goi to understand my project layout when I open a folder.

1. WHEN the user opens a folder THEN the system SHALL detect the project kind in this order: `go.work` at the root → workspace; `go.mod` at the root → module; any `go.mod` in subdirectories (depth ≤ 4, excluding excluded dirs) → multi-module; else, if the folder is under `$GOPATH/src` → GOPATH; else → plain folder.
2. WHEN the project is a workspace THEN the system SHALL list every module named in `go.work` `use` directives, with module path and Go version.
3. WHEN the project is a module THEN the system SHALL read the module path and `go` version from `go.mod`.
4. IF `go.mod` or `go.work` fails to parse THEN the system SHALL still open the folder as plain, and SHALL show a non-blocking notification with the parse error and its file/line.
5. WHEN `go.mod` or `go.work` changes on disk THEN the system SHALL re-run detection within 1 s and update the project model.
6. Opening a project SHALL NOT execute any project-defined code or commands (`go generate`, scripts, tools). Read-only `go env` / `go list` calls are allowed.

## R3 — Go SDK selection [F-PRJ-2]
**Story:** As a developer, I want to pick which Go SDK the project uses.

1. WHEN Goi first needs an SDK THEN the system SHALL discover candidates from `go` on `$PATH` (via `go env GOROOT`), `/usr/local/go`, `/opt/homebrew/opt/go/libexec`, `~/sdk/go*`, `~/go/pkg/mod/golang.org/toolchain@*`, `C:\Program Files\Go`, and `/usr/lib/go`.
2. WHEN candidates are listed THEN each SHALL show its version (from `<GOROOT>/VERSION` or `go version`) and path, deduplicated by real path.
3. WHEN the user selects an SDK for a project THEN the system SHALL store it in `.goi/settings.toml` (`go.goroot`) and use that `go` binary for all project commands.
4. IF no SDK is configured and exactly one candidate exists THEN the system SHALL use it automatically.
5. IF no SDK can be found THEN the system SHALL show a persistent status-bar warning "No Go SDK" with a link to SDK settings, while editing keeps working.
6. WHEN the user adds a GOROOT path manually THEN the system SHALL validate it contains `bin/go[.exe]` and reject it with a clear message if not.

## R4 — Go modules & build environment [F-PRJ-4, F-PRJ-5]
**Story:** As a developer, I want to set the Go environment and build constraints the IDE uses.

1. The system SHALL provide project settings for: environment variables (key/value list, e.g. `GOPROXY`, `GOPRIVATE`, `GOFLAGS`), vendoring mode (auto / on / off), build tags, GOOS, GOARCH and CGO_ENABLED.
2. WHEN any of these settings change THEN the system SHALL persist them to `.goi/settings.toml` and apply them to every subsequent `go` invocation made for that project.
3. WHEN the project is opened THEN the system SHALL run `go env -json` with the project environment and show the effective GOOS/GOARCH/GOVERSION in the status bar.
4. IF the user sets GOOS/GOARCH different from the host THEN the status bar SHALL indicate cross-target mode.

## R5 — Project tree, content roots & excludes [F-PRJ-7, F-UI-1]
**Story:** As a developer, I want to browse and manage the project's files.

1. WHEN a project opens THEN the system SHALL show a project tree of all content roots, lazily loading directories on expand.
2. The tree SHALL hide excluded directories, which by default are: `.git`, `node_modules`, `.idea`, `.goi/cache`. Users can add or remove excludes (persisted per project).
3. WHEN the user adds an additional content root (folder outside the project) THEN the system SHALL show it as a separate top-level node and persist it.
4. WHEN files are created, deleted or renamed on disk (inside or outside Goi) THEN the tree SHALL reflect the change within 1 s without losing expansion or selection state.
5. The tree SHALL support: new file, new directory, new Go file (with `package <dirname>` pre-filled), rename, delete (to OS trash where available), copy path, reveal in OS file manager.
6. WHEN a directory contains > 5,000 entries THEN the tree SHALL stay responsive (virtualized rendering).

## R6 — Editor core [F-ED-1, F-ED-2]
**Story:** As a developer, I want a fast, capable text editor.

1. WHEN a file is opened THEN the system SHALL show it in a tab, with syntax highlighting chosen by file name or extension.
2. Highlighting SHALL be provided for at least: Go, `go.mod`/`go.work`/`go.sum`, Go templates (`.gotmpl`, `.tmpl`), YAML, JSON, TOML, Markdown, SQL, Dockerfile, Makefile, shell (sh/bash/zsh), protobuf, HTML, CSS, JavaScript/TypeScript, XML, `.env`, plain text. Unknown types open as plain text.
3. The editor SHALL support: multiple cursors (add by click+modifier and by "add next occurrence"), rectangular selection, code folding (indentation/syntax-based), bracket matching and auto-close, auto-indent, soft-wrap toggle, line numbers, find/replace in file (regex, case, whole word), go to line, undo/redo with per-file history.
4. WHEN the user opens multiple files THEN tabs SHALL support reorder by drag, close/close others/close all, pin, and split right/down (at least 2×2 splits) showing the same or different files.
5. IF a file is larger than 2.5 MB THEN the system SHALL open it with highlighting disabled and show a banner; IF larger than 20 MB THEN it SHALL open read-only.
6. IF a file is binary (NUL bytes in the first 8 KB) THEN the system SHALL NOT open it as text and SHALL show a "binary file" placeholder.
7. WHEN saving THEN the system SHALL preserve the file's original line endings, trailing-newline state and UTF-8 BOM.
8. WHEN a file open in an editor changes on disk AND the buffer is unmodified THEN the system SHALL reload it silently; IF the buffer is modified THEN it SHALL show a conflict banner offering "Reload from disk" / "Keep mine" / "Compare".
9. WHEN the app is closed with unsaved buffers THEN the system SHALL prompt to Save All / Discard / Cancel.
10. The system SHALL journal unsaved buffers to the user cache dir at least every 2 s, and WHEN Goi restarts after a crash THEN it SHALL offer to restore them [NF-2].

## R7 — Gutter run markers (static) [F-ED-4]
**Story:** As a developer, I want to see what's runnable in a Go file.

1. WHEN a Go file is open THEN the gutter SHALL show a marker on `func main()` in `package main`, and on `func TestX(*testing.T)`, `BenchmarkX(*testing.B)`, `FuzzX(*testing.F)` and `ExampleX()` in `_test.go` files.
2. Markers SHALL be computed by parsing the file with `go/parser` in the backend (no gopls) and refreshed within 300 ms of edits stopping.
3. WHEN a marker is clicked in M1 THEN the system SHALL show a popup with the item name and a disabled "Run (coming in M3)" entry. (The hook point for M3.)

## R8 — Actions on save [F-ED-10]
**Story:** As a developer, I want my Go code formatted automatically.

1. The system SHALL provide project settings: on save → `gofmt` | `goimports` | none (default `goimports`); "optimize imports" (implied by goimports).
2. WHEN a `.go` file is saved AND formatting is enabled THEN the system SHALL format the buffer in the backend, apply the result as one undoable edit preserving cursor and scroll positions, and then write to disk.
3. IF formatting fails (syntax error) THEN the system SHALL save the unformatted content and show the error position inline (squiggle + status bar message), without a modal.
4. Formatting SHALL complete in < 200 ms for a 5,000-line file.
5. The user SHALL be able to run "Reformat File" manually from the command palette.

## R9 — Command palette & navigation [F-UI-2]
**Story:** As a developer, I want keyboard-driven access to everything.

1. WHEN the user double-taps Shift (or presses the configured shortcut) THEN the system SHALL open Search Everywhere with tabs: All, Files, Actions, Settings.
2. File search SHALL fuzzy-match on path segments and camel/snake humps (e.g. `wsmgr` → `workspace_manager.go`), with recent files ranked first, and return results in < 100 ms for 50,000 files.
3. Action search SHALL list every registered command with its shortcut.
4. The system SHALL provide "Recent Files" (Cmd/Ctrl+E) and "Go to Line" (Cmd/Ctrl+L / Ctrl+G).
5. The file index SHALL respect excludes and update incrementally from fs events.

## R10 — Keymaps [F-UI-3]
**Story:** As a developer, I want familiar shortcuts.

1. The system SHALL ship two keymaps: "Goi Default" and "VS Code", each with macOS and Windows/Linux variants (Cmd ↔ Ctrl), selected automatically by OS.
2. WHEN the user rebinds a command (including two-stroke chords) THEN the system SHALL persist the override in the user config and apply it immediately.
3. IF a new binding conflicts with an existing one THEN the system SHALL show the conflict and ask whether to replace it.

## R11 — Themes & color schemes [F-UI-4]
**Story:** As a developer, I want a comfortable visual theme.

1. The system SHALL ship UI themes: Light, Dark, High Contrast; and editor color schemes: Goi Light, Goi Dark, plus two additional original palettes (warm dark, low-contrast light).
2. WHEN the OS appearance is set to auto THEN the system SHALL follow the OS light/dark setting and switch live.
3. WHEN the user changes theme or scheme THEN the change SHALL apply immediately to all windows without reload.
4. All UI and syntax colors SHALL be defined as theme tokens; High Contrast SHALL meet WCAG AA (4.5:1) for text.

## R12 — Main window layout [F-UI-1]
**Story:** As a developer, I want a familiar IDE layout I can rearrange.

1. The main window SHALL have: left panel (Project), center editor area, bottom panel (Problems placeholder, Notifications), right panel (Structure placeholder), and status bar.
2. Panels SHALL be resizable, collapsible and toggleable by shortcut; WHEN the project is reopened THEN the layout (sizes, visibility, open tabs, cursor positions) SHALL be restored.
3. The system SHALL support multiple project windows, each with its own workspace (one project per window).

## R13 — Settings [F-UI-5 subset]
**Story:** As a developer, I want to configure Goi in one place.

1. The system SHALL resolve settings in layers: built-in defaults → user (`<UserConfigDir>/Goi/<ver>/settings.toml`) → project (`.goi/settings.toml`); project overrides user.
2. WHEN a settings file is edited externally THEN the system SHALL reload it within 1 s; IF it is invalid TOML THEN it SHALL keep the last valid values and notify with file and line.
3. The system SHALL provide a Settings UI with search, covering every M1 setting, and indicating which layer each value comes from.

## Non-functional (M1)
- **NFR-1 Platforms:** builds and runs on macOS (arm64, amd64), Windows 10+ (amd64), Linux (amd64, WebKitGTK 4.1). arm64 Windows/Linux builds compile in CI; manual testing is best-effort.
- **NFR-2 Startup:** cold start to Welcome screen < 1.5 s; opening a 10k-file project to an interactive tree < 1 s.
- **NFR-3 Responsiveness:** no UI-thread task > 100 ms; typing latency < 16 ms in files ≤ 10k lines.
- **NFR-4 Safety:** Goi never writes outside the project, `.goi/`, user config/cache dirs or explicitly chosen paths; deletes go to the OS trash when available.
- **NFR-5 Logs:** backend logs to `<UserCacheDir>/Goi/<ver>/logs/goi.log`, rotating at 10 MB × 5.
- **NFR-6 Tests:** every `internal/` package has unit tests; the detection, settings layering, formatting and EOL preservation logic has table-driven tests covering Windows paths and CRLF.
