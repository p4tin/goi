# CLAUDE.md — Goi

Goi is a cross-platform Go IDE (Wails v2 + CodeMirror 6). Product spec: `SPEC.md`. Steering: `.claude/steering/{product,structure,tech}.md`. Feature specs: `.claude/designs/<feature>/` (requirements, design, tasks, acceptance, retro).

## Commands

```
wails dev                        # hot reload (uses GOI_HOME if set)
wails build                      # macOS: build/bin/Goi.app
wails generate module            # regenerate frontend/wailsjs bindings after changing bound Go methods
go test ./internal/...           # backend tests (add -race for rpc/workspace/vfs)
cd frontend && npm test          # vitest
cd frontend && npx tsc --noEmit  # type-check
node tools/e2e/cdp.mjs tools/e2e/smoke.json   # headless E2E against a running `wails dev`
```

Wails CLI on this machine: `/Users/p4tin/.gvm/pkgsets/go1.25.6/global/bin/wails` (v2.17.1). `go.mod` needs Go 1.26; `GOTOOLCHAIN=auto` fetches it.

`GOI_HOME=<dir>` redirects user config/cache to `<dir>/config` and `<dir>/cache`. Use it for dev runs and tests so the real profile is untouched.

## Architecture rules

- `internal/*` never imports Wails. `internal/rpc` is the only bridge: bound services are thin, and the domain logic lives in other packages.
- Backend → UI updates go through `events.Bus` topics (`internal/events`). Frontend topic names are in `frontend/src/lib/api.ts` (`Topics`).
- The frontend never touches the filesystem; it goes through `lib/api.ts` wrappers and never imports `wailsjs/` directly.
- Frontend features are installers listed in `frontend/src/features.ts`. Features extend the editor through `editors.hooks` (`opened`, `closed`, `beforeSave`, `saved`, `viewCreated`, `activeChanged`) and `editors.addExtension(buf => ext)`; don't edit `editorManager.ts` for feature work.
- Every user action is a command (`lib/commands.ts`). Shortcuts live only in `keymap/keymaps.ts`.
- Colors come only from theme tokens (`var(--goi-*)`, source of truth `theme/tokens.ts`).

## Gotchas

- Don't use `time.Time` (or other non-JSON-native types) in bound method parameters: Wails then generates a `Time` class that retypes every timestamp in `models.ts`. Pass RFC 3339 strings.
- Return empty slices, not nil, from bound methods. Nil arrives as `null` in JS.
- `frontend/dist` is generated (Vite empties it). Run `npm run build` before `go vet ./...` or `wails generate module` in a fresh checkout or worktree.
- `wails generate module` may flip file modes under `frontend/wailsjs/runtime`; revert with `git checkout frontend/wailsjs/runtime`.
- `wails dev` regenerates bindings on Go changes. If its Go rebuild fails, the old backend keeps running; restart it.
- happy-dom tests: open editor views with `{ focus: false }`. happy-dom fires `selectionchange` synchronously, which re-enters CodeMirror's update.
- Each project window is its own process (Wails v2 is single-window). Instance locks live in `<cache>/instances/<hash>.lock` (`internal/instance`).
