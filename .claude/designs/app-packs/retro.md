# Application Packs — Retro

## What went right
- Splitting protocol from policy early (`internal/lsp`, `internal/dap`, the supervised `gopls` server) meant the language server and debugger moved behind interfaces with small changes. `internal/gopls` became `internal/langserver` after removing about 40 lines of Go knowledge.
- Comparing every E2E suite's output against a baseline server built from `main` replaced judging specs that print values without asserting. Nine suites matched apart from timings and hashes.
- The Wails generator handled several main packages, so one bindings tree serves both apps. The capability flags in `AppInfo` let core hide pack UI without branching on the app.
- Pyi worked end to end (basedpyright, interpreter, run, icon, bundle) once the Python pack implemented the interfaces. Very little core code was written for it.

## What went wrong
- Moving the product name to `pack.ts` created an import cycle the type checker can't see: the bundle threw a TDZ error at load, and only the smoke E2E caught it. Fixed with a separate `product.ts`, plus a rule to read `pack.ts` exports only inside functions.
- A custom `MarshalJSON` on `settings.Settings` made the bindings generator type `Settings` as `any`. Replaced by a plain `Extra` field.
- E2E harness mistakes cost time: the m4 project is `<dir>/work`, and copying a binary over an existing one on macOS gets it killed at launch (exit 137).
- Some Go remains in core: the Go settings fields in `settings.Settings`, the Go run configuration forms in `run/configDialog.ts`, and a few Go-flavoured hints (`*_test.go`, Go to Test). They are hidden or harmless in Pyi but are still debt.

## Action items
1. Before moving a module that other modules read at load time, check for import cycles through the moved code. Run the server-build smoke after every frontend refactor, not just vitest.
2. In the `python-app` spec, move the Go run forms and the typed Go settings into `packs/go` and `packs/golang` (`registerRunForm`, `settings.Section`) while adding pytest/debugpy. The second real consumer will shape those hooks better than guessing now.
