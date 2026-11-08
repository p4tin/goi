# Tech — Goi

## Stack
| Layer | Choice | Notes |
|---|---|---|
| Backend language | Go ≥ 1.26 | `go.mod` pins `go 1.26.0` (required by Wails v2.17); `GOTOOLCHAIN=auto` fetches it if the local Go is older |
| Desktop shell | **Wails v2.17.1** | WebKit on macOS, WebView2 on Windows, WebKitGTK on Linux |
| Frontend | TypeScript + Vite (Wails `vanilla-ts` template) | No heavy framework in M1; reconsider Solid if UI state grows |
| Editor | **CodeMirror 6** | `@codemirror/*` packages; `@codemirror/lang-*` + `@codemirror/legacy-modes` for languages |
| Settings format | TOML | `github.com/pelletier/go-toml/v2` |
| FS watching | `github.com/fsnotify/fsnotify` | Debounced, recursive walk with excludes |
| Formatting | `go/format` (stdlib gofmt) + `golang.org/x/tools/imports` (goimports) | In-process, no external binary needed |
| Tests (Go) | stdlib `testing` | Table-driven; `t.TempDir()` for FS tests |
| Tests (TS) | Vitest + happy-dom | Pure modules and DOM/CodeMirror units |
| E2E | `tools/e2e/cdp.mjs` | Headless Chrome over CDP against `wails dev` |
| Code intelligence | gopls over LSP (stdio) | Own minimal LSP client (`internal/lsp`); no third-party LSP library |
| Find in Path | `internal/textsearch` | Go `regexp`, line-based; same engine for search and replace |
| Debugger | Delve `dlv dap` over DAP | Own minimal DAP client (`internal/dap`); Delve installed on demand into Goi's tools dir |
| Test runner | `go test -json` | test2json events streamed to a frontend tree reducer |
| Terminal | `github.com/aymanbagabas/go-pty` + `@xterm/xterm` 6 (fit, search, web-links addons) | ConPTY on Windows; zsh/bash integration via ZDOTDIR / --rcfile |

## Prerequisites (dev machine)
- Go 1.26+ (auto-downloaded via GOTOOLCHAIN=auto; local go1.25.6 is fine)
- Node 20+ ✅ (v26.13.0)
- Wails CLI v2.17.1 ✅: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Linux builds: `libgtk-3-dev libwebkit2gtk-4.1-dev` (build tag `webkit2_41`)
- Windows: WebView2 runtime (bundled bootstrapper via Wails)
- gopls (M2+): `go install golang.org/x/tools/gopls@latest`, or "Install gopls" in the app
- dlv (M3+): `go install github.com/go-delve/delve/cmd/dlv@latest`, or "Install Delve" in the app
- golangci-lint: not needed until M4

## Dependencies (as built)
- Go: wails/v2, golang.org/x/{mod,tools,sys}, fsnotify, go-toml/v2, doublestar/v4, gofrs/flock, aymanbagabas/go-pty
- Frontend: @codemirror/{state,view,language,commands,search,autocomplete,lint,merge,lang-*,legacy-modes} (lint and autocomplete carry gopls diagnostics and completion), @lezer/highlight, @tanstack/virtual-core, @xterm/{xterm,addon-fit,addon-search,addon-web-links} (exact versions); dev: vite, typescript, vitest, happy-dom

## Developer commands
```
wails dev                 # run with hot reload
wails build               # production build for host OS
wails build -platform windows/amd64,linux/amd64   # cross targets (CI preferred)
GOI_HOME=/tmp/goi wails dev   # isolated user profile
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
