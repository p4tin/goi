# M2 — Intelligence: Requirements

Scope (SPEC.md §6 M2): gopls integration for F-CI-1, F-CI-2, F-CI-3, F-CI-5, F-CI-6, F-CI-7, F-CI-8, plus diagnostics, quick-fixes and rename.
Out of scope: call/type hierarchy (F-CI-4, v0.2), structural search (F-CI-9), inlay hints and code vision (F-ED-3), refactorings other than rename (F-RF-2..7), golangci-lint (F-INS-1, M4), live templates and postfix *templates* (F-ED-5/6; gopls postfix completions still appear because gopls sends them).

Behaviour mirrors the documented behaviour and default shortcuts of mainstream Go IDEs, with Goi's own wording, look and command ids.

## R1. gopls lifecycle (SPEC §2.2)
1. WHEN a project opens and `gopls.enabled` is true THEN the core SHALL resolve gopls in this order: the `gopls.path` setting → `$PATH` → `$GOBIN` / `$(go env GOPATH)/bin` → the Goi-managed tools dir `<config>/tools/bin`. It SHALL start one gopls per project window, with the project root as the workspace folder (a `go.work` root covers all its modules).
2. IF gopls is not found THEN the UI SHALL show status "gopls not installed" and offer **Install gopls**, which SHALL run `go install golang.org/x/tools/gopls@latest` with the project SDK and `GOBIN=<config>/tools/bin`. On success, gopls SHALL start without a restart.
3. The gopls environment SHALL include the project's `go.env`, GOOS/GOARCH/CGO_ENABLED and the selected SDK's `bin` first on `PATH`. Build tags SHALL be passed as `buildFlags: ["-tags=…"]` (F-PRJ-5).
4. IF gopls exits unexpectedly THEN the core SHALL restart it with exponential backoff (1 s, 2 s, 4 s … max 30 s) and re-open the open documents. After 5 crashes within 3 minutes it SHALL stop and show an error status with **Restart**.
5. WHEN `go.*` or `gopls.*` settings change THEN the core SHALL restart gopls.
6. gopls stderr and protocol errors SHALL go to the Goi log with `src=gopls` (NF-6).
7. The status bar SHALL show the gopls state (starting, indexing with progress text, ready, error, not installed, disabled). Clicking it SHALL offer Restart / Install.
8. WHEN files change on disk (watcher batches for `*.go`, `go.mod`, `go.sum`, `go.work`) THEN the core SHALL send `workspace/didChangeWatchedFiles`.
9. IF a buffer is in large-file or read-only mode (> `editor.maxHighlightSizeKB`) THEN it SHALL NOT be sent to gopls (SPEC §2.4).

## R2. Document sync
1. WHEN a Go-family buffer (`.go`, `go.mod`, `go.work`) opens THEN the UI SHALL send `didOpen`. Edits SHALL send the full text with an increasing version, debounced ≤ 150 ms. Every intelligence request SHALL first flush pending changes, so it always sees the current text.
2. WHEN a buffer saves or closes THEN `didSave` / `didClose` SHALL be sent.

## R3. Completion (F-CI-1)
1. WHEN the user types an identifier character or `.`, or presses Ctrl+Space, THEN the editor SHALL show gopls completions (fields, methods incl. promoted through embedding, packages, keywords, snippets, unimported packages). Order SHALL follow gopls ranking, and typing more SHALL filter locally.
2. WHEN an item with a text edit is accepted THEN its edit SHALL replace the requested range. Snippet placeholders (`${1:x}`) SHALL become tab stops, and `additionalTextEdits` (auto-added imports) SHALL be applied in the same undo step.
3. Each item SHALL show its kind icon, label, detail (type/signature) and documentation in an info pane.

## R4. Navigation (F-CI-2)
1. WHEN **Go to Declaration** runs (Mod-B or Mod-click on an identifier) THEN the editor SHALL open the definition. Several targets SHALL show a chooser. The previous location SHALL be pushed on the navigation history.
2. **Go to Type Declaration** (Mod-Shift-B) and **Go to Implementation(s)** (Mod-Alt-B: implementations of an interface, or interfaces a type implements) SHALL work the same way.
3. **Go to Test / Subject** (Mod-Shift-T) SHALL switch between `x.go` and `x_test.go`, landing on `TestFoo` for `Foo` (and back) when the cursor is in a function. If the target file doesn't exist, it SHALL offer to create it.
4. **Back / Forward** (Mod-[ / Mod-]) SHALL move through navigation history, across files.
5. **File Structure** (Mod-F12) SHALL show the file's symbols (from `documentSymbol`) in a filterable popup. Enter jumps to the symbol.
6. Go buffers SHALL show breadcrumbs (enclosing type › func) under the editor, updated as the cursor moves. Clicking a crumb jumps to it.

## R5. Find usages (F-CI-3)
1. WHEN **Find Usages** (Alt-F7) runs on an identifier THEN a **Usages** tool window SHALL list all references. They SHALL be grouped by production vs test code, then by package directory, then by file, each with its line preview. Each usage SHALL be marked read or write.
2. Clicking a usage SHALL open it with the reference selected.
3. WHEN the cursor rests on an identifier (≥ 250 ms) THEN all occurrences in the file SHALL be highlighted (`documentHighlight`), with write occurrences styled distinctly.

## R6. Documentation (F-CI-5)
1. WHEN the mouse rests over an identifier THEN a hover popup SHALL show the rendered godoc (signature in a code block, Markdown body, links).
2. **Quick Documentation** (F1 on macOS, Ctrl-Q elsewhere) SHALL show the same popup at the cursor. For Go keywords (`defer`, `select`, `range`, …), where gopls has nothing, Goi SHALL show its own keyword documentation.
3. **Parameter Info** (Mod-P), and typing `(` or `,` inside a call, SHALL show the signature with the active parameter highlighted.

## R7. Diagnostics (F-CI-6)
1. WHEN gopls publishes diagnostics THEN the editor SHALL underline them (error/warning/info/hint) with gutter markers and hover messages, including the source analyzer name.
2. A **Problems** tool window SHALL list diagnostics for all files, grouped by file with counts, errors first. Clicking one SHALL navigate to it. The status bar SHALL show error and warning totals.

## R8. Quick-fixes (Alt-Enter)
1. WHEN **Show Context Actions** (Alt-Enter) runs THEN a popup SHALL list gopls code actions for the selection, with quick-fixes for overlapping diagnostics first.
2. Choosing an action SHALL apply its `WorkspaceEdit`, or execute its command. Server-initiated `workspace/applyEdit` SHALL be applied the same way.

## R9. Rename (Shift-F6)
1. WHEN **Rename** runs on an identifier THEN Goi SHALL validate it with `prepareRename`, prompt for the new name, request `rename`, and apply the edit across all files.
2. IF the edit touches more than one file THEN a preview SHALL list the files and change counts before applying.
3. Edits to open buffers SHALL be one undo step per buffer. Files that aren't open SHALL be rewritten on disk, preserving EOL and BOM.

## R10. Search Everywhere symbols (F-CI-7)
1. The palette SHALL gain a **Symbols** tab, and a Symbols section in **All**, fed by `workspace/symbol` (fuzzy). **Go to Symbol** (Mod-Alt-O) SHALL open the tab. Choosing a symbol SHALL open its location.

## R11. Find / Replace in Path (F-CI-8)
1. **Find in Path** (Mod-Shift-F) SHALL search all project files, honouring excludes. It SHALL support case sensitivity, whole word and regex options, a file mask (e.g. `*.go, !*_test.go`) and a scope (whole project or a directory).
2. Results SHALL appear in a **Find** tool window grouped by file, with line previews and the match highlighted. Clicking SHALL open the match selected. Results SHALL be capped (2,000) with a notice when truncated.
3. **Replace in Path** (Mod-Shift-R) SHALL add a replacement (regex groups `$1` allowed), show the preview, and on **Replace All** rewrite matches. Open buffers SHALL be edited in place; other files on disk.

## R12. Non-functional
1. Intelligence calls SHALL NOT block the UI. Stale responses (document changed since the request) SHALL be dropped.
2. Completion SHALL appear within 150 ms p95 on this repository once gopls is ready (SPEC §2.4).
3. Bound DTOs SHALL use only JSON-native types (no `time.Time`), and slices SHALL never be nil (CLAUDE.md).
4. All new behaviour SHALL be covered by unit tests: protocol client, gopls manager against a fake server, edit application, text search, and the frontend pure helpers.
