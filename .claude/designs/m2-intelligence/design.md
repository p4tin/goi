# M2 — Intelligence: Design

## 1. Architecture

```
frontend/src/lang/*  ──(lib/api.ts: lang.*)──►  internal/rpc/lang.go (LangService, thin)
        ▲                                                │
        │ events: lang.status, lang.diagnostics,         ▼
        │         lang.applyEdit                 internal/gopls (Server: process, sync,
        │                                          restart, diagnostics, progress)
        │                                                │ internal/lsp (JSON-RPC 2.0 over stdio)
        │                                                ▼
        └──────────────────────────────────────────  gopls (one per project window)

frontend/src/lang/findInPath.ts ──► rpc SearchService.FindInPath/ReplaceInPath ──► internal/textsearch
```

- `internal/lsp` has no Goi knowledge. It holds the protocol subset types and `Conn`.
- `internal/gopls` knows gopls and Go tooling but not Wails. It reports through callbacks (`OnStatus`, `OnDiagnostics`, `OnApplyEdit`, `Log`), which `rpc` turns into `events.Bus` topics.
- `internal/textsearch` is pure: a file list plus a query gives matches. It also applies replacements.
- `rpc.session` owns the `*gopls.Server`: started after the workspace opens, closed with the session, fed with watcher batches, restarted on settings changes.

## 2. Positions and DTO rules
- LSP positions are `{line, character}`, with `character` in **UTF-16 code units**, which is the protocol default and what gopls uses. JS strings and CodeMirror offsets are UTF-16 too, so the frontend converts with `doc.line(n+1).from + character` and no transcoding. Go only converts UTF-16 to byte offsets in `gopls.ApplyEdits`, when rewriting files that aren't open.
- Paths cross the bridge as OS-native absolute paths. `file://` URIs exist only inside `internal/gopls` (`uri.go`, which handles Windows drive letters and percent-encoding).
- DTOs are the `lsp` types themselves. Unions are normalized when decoded: `MarkupContent` accepts string, MarkedString and arrays; `CompletionItem.textEdit` asks for `TextEdit` only (`insertReplaceSupport: false`); `Location | LocationLink[]` becomes `[]Location`. Bound methods return non-nil slices and use no `time.Time`.
- Responses carry the document `version` they were computed for. The frontend drops a response if the buffer version moved on (R12.1).

## 3. internal/lsp
```go
type Handler func(ctx context.Context, method string, params json.RawMessage) (result any, err error)
func NewConn(rwc io.ReadWriteCloser, h Handler, log *slog.Logger) *Conn
func (c *Conn) Call(ctx context.Context, method string, params, result any) error // ctx cancel → $/cancelRequest, returns ctx.Err()
func (c *Conn) Notify(ctx context.Context, method string, params any) error
func (c *Conn) Done() <-chan struct{}; func (c *Conn) Close() error
type ResponseError struct{ Code int64; Message string }
```
Framing is `Content-Length` headers. One reader goroutine dispatches responses to waiting callers. Server requests and notifications run the handler on their own goroutine, so a slow handler can't stall responses. Protocol types cover: Position, Range, Location, TextEdit, WorkspaceEdit (`changes` + `documentChanges` text edits), Diagnostic, CompletionList/Item, Hover, SignatureHelp, DocumentSymbol, SymbolInformation/WorkspaceSymbol, DocumentHighlight, CodeAction, Command, PrepareRename result, Initialize params/result, progress, configuration.

## 4. internal/gopls
- `resolve.go`: `Resolve(ResolveOptions{Setting, PATH lookup, GoEnv(GOBIN,GOPATH), ToolsDir}) (string, error)`, with `ErrNotFound`. `Install(ctx, cmd *exec.Cmd builder, toolsDir)` runs `go install golang.org/x/tools/gopls@latest` with `GOBIN=toolsDir`.
- `server.go`: `New(Config) *Server`. `Config{Bin, Root, Env []string, BuildFlags []string, Settings map[string]any, Log, OnStatus, OnDiagnostics, OnApplyEdit}`.
  - `Start()` spawns the process, sends `initialize` (capabilities below) and `initialized`, then answers `workspace/configuration` with the gopls settings: `buildFlags`, `env`, `usePlaceholders`, `completeUnimported`, `staticcheck`, plus the passthrough map.
  - Supervision: when the process exits it reports `error`, restarts after the backoff, then re-sends `didOpen` for every tracked document. Crash accounting follows R1.4.
  - Documents: `Open(path, langID, version, text)`, `Change(path, version, text)` (full sync), `Save`, `Close`. `docs` keeps the last text and version for re-opening after a restart.
  - Requests take `(ctx, path, Position)` and return `lsp` types: Completion, Hover, SignatureHelp, Definition, TypeDefinition, Implementation, References, DocumentHighlight, DocumentSymbol, WorkspaceSymbol, PrepareRename, Rename, CodeActions(range, diagnostics), ExecuteCommand. They wait up to 30 s for the server to be ready, then fail with `ErrNotReady`.
  - `WatchedFilesChanged([]lsp.FileEvent)`.
  - Progress: `window/workDoneProgress/create` is accepted. `$/progress` begin/report/end becomes the status `indexing` with the title and message, and `ready` once all tokens end.
  - `textDocument/publishDiagnostics` triggers `OnDiagnostics(path, []lsp.Diagnostic)`, and gopls's server-side `workspace/applyEdit` triggers `OnApplyEdit`.
- `edits.go`: `ApplyEdits(text string, edits []lsp.TextEdit) (string, error)` uses UTF-16 positions, sorts edits and rejects overlaps. `FileEdits(we WorkspaceEdit) []FileEdit` groups edits per path. Both are pure and table-tested.
- Client capabilities: snippet support, documentation formats `markdown` and `plaintext`, `hierarchicalDocumentSymbolSupport`, `codeAction` literal support with kinds `quickfix`/`refactor*`/`source*`, `prepareSupport` for rename, `workDoneProgress`, `workspace.applyEdit`, `workspace.configuration`, `didChangeWatchedFiles`, and `publishDiagnostics.relatedInformation`.

## 5. internal/textsearch
```go
type Query struct{ Pattern string; Regex, CaseSensitive, WholeWord bool; Include, Exclude []string /*globs on base name*/; Replace string }
type Match struct{ Path string; Line int /*1-based*/; Col, End int /*UTF-16 within line*/; Preview string; PreviewStart int }
func Compile(q Query) (*regexp.Regexp, error)
func Search(ctx, paths []string, q Query, limit int) (matches []Match, truncated bool, err error) // skips binary (NUL in first 8 KB) and files > 5 MB
func ReplaceText(text string, re *regexp.Regexp, repl string) (string, int)
```
Plain patterns are `regexp.QuoteMeta`; case-insensitive uses `(?i)`; whole word wraps the pattern in `\b…\b`. The file list comes from the session's existing file index, which already honours excludes and content roots. Scope is a directory prefix filter applied in `rpc`.

## 6. RPC surface (`internal/rpc/lang.go`, bound as `LangService`)
```
Status() LangStatus                       // {state, message, bin}
Restart() error; InstallGopls() error     // install runs async; status events report progress
DidOpen(path, version int, text) / DidChange(path, version, text) / DidSave(path) / DidClose(path)
Completion(path, line, char) (CompletionResult{items []lsp.CompletionItem, incomplete bool})
Hover(path, line, char) (HoverResult{markdown, range *lsp.Range})
SignatureHelp(path, line, char) (*lsp.SignatureHelp)
Definition / TypeDefinition / Implementation / References(path,line,char) ([]lsp.Location)
DocumentHighlight(path,line,char) ([]lsp.DocumentHighlight)
DocumentSymbols(path) ([]lsp.DocumentSymbol)
WorkspaceSymbols(query) ([]lsp.SymbolInformation)
PrepareRename(path,line,char) (PrepareRenameResult{range, placeholder, ok})
Rename(path,line,char,newName) ([]FileEdit)
CodeActions(path, range lsp.Range, diags []lsp.Diagnostic) ([]lsp.CodeAction)
RunCodeAction(action lsp.CodeAction) ([]FileEdit)   // resolves the edit, or executes the command (edits then arrive via lang.applyEdit)
ApplyFileEdits(edits []FileEdit) error              // writes files that aren't open (vfs.ReadText/WriteText keep EOL/BOM)
```
On `SearchService`: `FindInPath(FindQuery) FindResult{matches, truncated}` and `ReplaceInPath(FindQuery, paths []string) (int, error)`, where `paths` are the files that aren't open. The frontend replaces inside open buffers itself.

Events (`frontend/src/lib/api.ts` Topics): `lang.status` (LangStatus), `lang.diagnostics` ({path, items}), `lang.applyEdit` ({edits: FileEdit[]}).

Settings (defaults.toml):
```toml
[gopls]
enabled = true
path = ""                  # empty: auto-resolve
usePlaceholders = true
staticcheck = false
settings = {}              # passthrough to gopls configuration
```

## 7. Frontend (`frontend/src/lang/`, installed through `features.ts`)
| Module | Responsibility |
|---|---|
| `positions.ts` | pure `toPos(doc, offset)`, `toOffset(doc, pos)`, `rangeToSel`, and `applyEditsToDoc` (TextEdits → CM ChangeSpec[]) |
| `client.ts` | `LangClient`: tracks Go buffers through `editors.hooks` (opened/closed/saved, plus a change listener added with `addExtension`); debounced full sync; `flush(buf)` before every request; `isGoBuffer(buf)` (`.go`, `go.mod`, `go.work`, normal mode only) |
| `edits.ts` | `applyFileEdits(edits)`: open buffers get one transaction each (undoable); the rest go to `lang.applyFileEdits`. Handles `lang.applyEdit` events |
| `markdown.ts` | small safe Markdown → DOM (code fences, inline code, emphasis, links, paragraphs, lists); no innerHTML from server data |
| `completion.ts` | CM `autocompletion({override:[source]})` for Go buffers; kind → CM type; boost from gopls order; `validFor /^\w*$/`; snippet conversion (`$1`/`${1:x}` → CM `${x}` fields); additional edits applied together with the main edit |
| `hover.ts` | `hoverTooltip` with rendered docs; `lang.quickDoc`; keyword docs from `keywords.ts` (Goi-authored text) |
| `signature.ts` | signature tooltip StateField; triggered by `(`/`,` input and `lang.parameterInfo`; closed by `)` or Escape |
| `diagnostics.ts` | `lang.diagnostics` → `setDiagnostics` on the buffer's views + `problems` signal; Problems panel (bottom); status counts |
| `navigation.ts` | definition/type/implementation, Mod-click, chooser popup (`popup.ts`), back/forward history, go to test |
| `usages.ts` | find usages → Usages panel (prod/test › package › file), read/write heuristic, documentHighlight decorations |
| `rename.ts` | prepare → prompt → rename → preview (if more than one file) → apply |
| `codeActions.ts` | Alt-Enter popup; apply edit / run command |
| `structure.ts` | file structure popup + breadcrumbs (CM bottom panel) from cached document symbols |
| `symbols.ts` | workspace symbols provider used by the palette (Symbols tab) |
| `findInPath.ts` | find/replace dialog + Find panel |
| `status.ts` | gopls status bar item + restart/install menu |
| `install.ts` | `installLang(wb)`: wires everything and registers commands |

Palette: `PaletteTab` gains `'symbols'`. `palette.ts` takes an optional symbol provider through `setSymbolProvider(fn)`, so `lang` stays the only module that knows about gopls.

Commands and default keys (goi-default; vscode variant in brackets):
`lang.gotoDefinition` Mod-b [F12], `lang.gotoTypeDefinition` Mod-Shift-b, `lang.gotoImplementation` Mod-Alt-b [Mod-F12], `lang.gotoTest` Mod-Shift-t, `nav.back` Mod-[ [Ctrl--], `nav.forward` Mod-] [Ctrl-Shift--], `lang.fileStructure` Mod-F12 [Mod-F12], `lang.findUsages` Alt-F7 [Shift-F12], `lang.quickDoc` mac F1 / other Ctrl-q [Mod-k Mod-i], `lang.parameterInfo` Mod-p (goi only) [Mod-Shift-Space], `lang.rename` Shift-F6 [F2], `lang.contextActions` Alt-Enter [Mod-Period], `nav.gotoSymbol` Mod-Alt-o [Mod-t], `search.findInPath` Mod-Shift-f, `search.replaceInPath` Mod-Shift-r [Mod-Shift-h], `lang.restart`, `lang.installGopls`. Completion uses CM's Ctrl-Space.

Menus: Navigate gains definition/type/implementation/test/structure/symbol/back/forward; Code gains context actions, rename, find usages, quick doc, parameter info; Edit gains Find in Path / Replace in Path.

## 8. Error handling and UX rules
- A missing or not-ready gopls never shows an error popup for passive features (hover, highlight). Explicit actions show a short notification ("gopls is starting…" / "gopls not installed").
- Every request is tied to the buffer version at send time. Results are ignored if the version or active buffer changed.
- Edits to unsaved open buffers go through CodeMirror, so they're undoable. Disk writes go only to files that aren't open.

## 9. Testing
- `lsp`: an in-memory pipe pair with a scripted fake server covers call/response, notifications, server→client requests, cancellation and EOF.
- `gopls`: `Resolve` order (fake dirs); `ApplyEdits` / `FileEdits` (UTF-16 incl. emoji, multiple edits, overlaps); a `Server` against a fake LSP server binary (`TestMain` re-exec, `GOI_FAKE_LSP=1`) covering initialize, configuration, diagnostics callback, restart after crash with documents re-opened, and progress → status; plus an integration test against real gopls (skipped when it isn't installed): completion and definition on a temp module.
- `textsearch`: plain, regex, case, whole word, include/exclude, binary skip, limit, replace with groups.
- `rpc`: LangService with a missing binary reports `missing`; FindInPath scope filtering.
- Frontend (vitest): positions, snippet conversion, markdown rendering, completion mapping, usage grouping and read/write heuristic, go-to-test name mapping, Problems grouping, edit application to CM docs.
