# Tech — Goi

## Stack
| Layer | Choice | Notes |
|---|---|---|
| Backend language | Go ≥ 1.26 | `go.mod` pins `go 1.26.0`; `GOTOOLCHAIN=auto` fetches it if the local Go is older |
| Desktop shell | **Wails v3.1.0** | One process, many windows (`SingleInstance`, per-window events). WebKit on macOS, WebView2 on Windows, WebKitGTK 4.1 (`-tags gtk3`) on Linux. Build tooling: `wails3` CLI + Taskfiles, `build/config.yml` |
| Frontend | TypeScript + Vite | No heavy framework; `@wailsio/runtime` + generated `frontend/bindings` (wrapped by `lib/api.ts`) |
| Editor | **CodeMirror 6** | `@codemirror/*` packages; `@codemirror/lang-*` + `@codemirror/legacy-modes` for languages |
| Settings format | TOML | `github.com/pelletier/go-toml/v2` |
| FS watching | `github.com/fsnotify/fsnotify` | Debounced, recursive walk with excludes |
| Formatting | `go/format` (stdlib gofmt) + `golang.org/x/tools/imports` (goimports) | In-process, no external binary needed |
| Tests (Go) | stdlib `testing` | Table-driven; `t.TempDir()` for FS tests |
| Tests (TS) | Vitest + happy-dom | Pure modules and DOM/CodeMirror units |
| E2E | `tools/e2e/cdp.mjs` | Headless Chrome over CDP against the server build (`-tags server`; each tab is a window) |
| Code intelligence | gopls over LSP (stdio) | Own minimal LSP client (`internal/lsp`); no third-party LSP library |
| Find in Path | `internal/textsearch` | Go `regexp`, line-based; same engine for search and replace |
| Debugger | Delve `dlv dap` over DAP | Own minimal DAP client (`internal/dap`); Delve installed on demand into Goi's tools dir |
| Test runner | `go test -json` | test2json events streamed to a frontend tree reducer |
| Version control | `git` CLI | porcelain v2 status, `git apply --cached` for partial staging; no git library |
| Linting | golangci-lint (v1 and v2 JSON) | Installed on demand into Goi's tools dir |
| Packaging | `wails3 task darwin:package:universal` / `windows:package` / `linux:build`, `hdiutil` dmg, nfpm deb/rpm | `release.yml` on `v*` tags |
| Coding agents | Agent Client Protocol v1 over stdio | Own minimal client (`internal/acp`); agents are npm packages (claude-agent-acp, gemini-cli, codex-acp) installed on demand into Goi's tools dir, or any command in `ai.agents` |
| IDE tools for agents | MCP (Streamable HTTP, JSON responses) | Own minimal server (`internal/mcp`) on 127.0.0.1 with a bearer token; `goi mcp-bridge` for stdio-only clients |
| Markdown preview | `@lezer/markdown` (GFM) rendered to DOM nodes; Mermaid 11 (lazy chunk, `securityLevel: 'strict'`) | No HTML injection: raw HTML shows as text; local images via `FileService.ReadImage` data URLs |
| JSON/YAML formatting | Own token-preserving JSON printer; `yaml` 2 (eemeli, lazy chunk) with the failsafe schema | YAML output is re-parsed and compared (values, comments, tags, anchors) before it's applied |
| Terminal | `github.com/aymanbagabas/go-pty` + `@xterm/xterm` 6 (fit, search, web-links addons) | ConPTY on Windows; zsh/bash integration via ZDOTDIR / --rcfile |

## Prerequisites (dev machine)
- Go 1.26+ (auto-downloaded via GOTOOLCHAIN=auto; local go1.25.6 is fine)
- Node 20+ ✅ (v26.13.0)
- Wails CLI v3.1.0 ✅: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`
- Linux builds: `libgtk-3-dev libwebkit2gtk-4.1-dev` (build tag `gtk3`)
- Windows: WebView2 runtime (bundled bootstrapper via Wails)
- gopls (M2+): `go install golang.org/x/tools/gopls@latest`, or "Install gopls" in the app
- dlv (M3+): `go install github.com/go-delve/delve/cmd/dlv@latest`, or "Install Delve" in the app
- golangci-lint (M4+): "Install golangci-lint" in the app, or your own install
- Node.js/npm: only to install agents from the AI Agent window (agents themselves bring their own runtime)
- Releases: nfpm (`go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.43.0`) for Linux packages; NSIS for the Windows installer

## Dependencies (as built)
- Go: wails/v3, golang.org/x/{mod,tools,sys}, fsnotify, go-toml/v2, doublestar/v4, gofrs/flock, aymanbagabas/go-pty
- Frontend: @wailsio/runtime (exact), @codemirror/{state,view,language,commands,search,autocomplete,lint,merge,lang-*,legacy-modes} (lint and autocomplete carry gopls diagnostics and completion), @lezer/highlight, @lezer/markdown, mermaid and yaml (both loaded with dynamic `import()`), @tanstack/virtual-core, @xterm/{xterm,addon-fit,addon-search,addon-web-links} (exact versions); dev: vite, typescript, vitest, happy-dom

## Developer commands
```
wails3 dev                # run with hot reload
wails3 build              # production build for host OS (bin/)
wails3 package            # bundle / installer for host OS
wails3 generate bindings -ts -i   # frontend/bindings after bound-method changes
GOI_HOME=/tmp/goi wails3 dev   # isolated user profile
go test ./internal/...    # backend tests
cd frontend && npm test   # frontend unit tests (vitest)
go vet ./... && gofmt -l .
```

## Coding conventions
- **Go:** `gofmt`/`goimports` clean; errors wrapped with `fmt.Errorf("op: %w", err)`; `context.Context` first arg on anything that does I/O or may block; no global mutable state outside `main`; exported identifiers documented.
- **Concurrency:** long work runs in goroutines and reports through `rpc.Events`; bound methods return quickly or take a context.
- **Paths:** always `filepath` in Go; send paths to the UI as OS-native absolute strings; never assume `/`.
- **Line endings:** preserve a file's existing EOL and encoding on save (UTF-8 assumed; BOM preserved).
- **TypeScript:** `strict: true`; ES modules; no `any` in `src/` except at the Wails boundary wrappers.
- **Styling:** CSS custom properties for all colors (theme tokens); no hard-coded colors in components.
- **Commits:** conventional style (`feat:`, `fix:`, `chore:`), one task per commit.

## CI (later in M1)
GitHub Actions matrix: macos-latest, windows-latest, ubuntu-latest → `go test`, `npm test`, `wails build`.
