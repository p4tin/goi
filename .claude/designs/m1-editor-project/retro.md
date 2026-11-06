# M1 — Editor + Project Model: Retrospective

Delivered: T1–T29 complete; T30 complete for macOS, with Windows/Linux runs pending (see `acceptance.md`). 13 Go packages, ~25 frontend modules. Tests: all Go packages pass (race-checked for rpc, workspace, vfs, settings), 233 frontend tests pass. Headless E2E against `wails dev` covers the main flows.

## What went right
- **Parallel waves worked.** The dependency matrix and task splits (T21a/b, T24a/b, T25a/b, T29a/b) let up to 4 worktree forks run at once. Wave 2 (9 tasks) and Wave 8 (8 tasks) finished in a fraction of sequential time. Merges were clean apart from predictable `go.mod`/`go.sum` conflicts, resolved by `go mod tidy`.
- **Contracts first.** T12 (RPC), T13 (workbench, panels, layout, dialogs, menus) and T18 (editor manager hooks and `addExtension`) were done inline before the wide waves. That gave forks stable seams: the "installer in `features.ts`, never edit `editorManager.ts`" rule meant 8 parallel editor features never collided.
- **UI-agnostic core.** Every `internal/` package is unit-tested without Wails. The headless CDP harness against `wails dev` gave real end-to-end checks without native screen access.
- **Pure helpers everywhere** (tree model, chord parsing, ranking, diffing, external-change decision): most UI behaviour is covered by fast unit tests.
- **Performance came for free** from the chosen stack: cold start 0.15 s, 10k-file open 0.33 s, file search under 2 ms over 50k paths, goimports 18 ms over 5k lines, one-frame typing.

## What went wrong
- **Generated-binding friction.** `time.Time` in a bound method made Wails emit a `Time` class that broke unrelated types. Nil slices arrive as `null` (crashed the restore prompt). `wails dev` silently kept a stale backend after a failed rebuild. Each cost a debug loop; all three are now in CLAUDE.md.
- **Environment gaps.** No Chrome extension and no screen-recording permission meant inventing the CDP harness mid-flight. `frontend/dist` not existing in fresh worktrees broke `go build` in forks and CI, which was caught by a fork rather than planned.
- **Rate limit mid-wave** stopped one fork (T27); it resumed from its saved state, but the wave stalled.
- **Leftover sharp edges:** macOS fsnotify uses kqueue (descriptor per file), so very large repos will hit limits; Linux has no `.desktop` file association; Windows/Linux haven't been run.
- **Scope added on the fly:** `internal/paths`, `internal/instance`, `workspace.Manager.Subscribe`, the `auto` scheme default, settings-file writes outside the project. Each was small and justified, but none was in the design.

## Action items
1. **Make CI the gate before starting M2.** Push to GitHub, get the 3-OS matrix and arm64 job green, and do one manual Windows and Linux pass. Add the CDP smoke run (`tools/e2e`) to CI on Linux with `wails dev` under Xvfb.
2. **Plan seams and environment up front.** For M2 (gopls/LSP), write the binding/DTO rules (no `time.Time`, non-nil slices, string enums) into design.md, and add a "fresh worktree builds" check (`npm run build` before Go builds) to every fork brief from the start.
