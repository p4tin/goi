# M2 — Intelligence: Tasks

Work on branch `feat/m2-intelligence`, one commit per task. Each task ends green: `go vet ./internal/...`, `go test ./internal/...` (with `-race` for lsp/gopls/rpc), `cd frontend && npx tsc --noEmit && npm test`. Run `npm run build` before Go builds in fresh trees. Regenerate bindings with `wails generate module` after changing bound methods, then `git checkout frontend/wailsjs/runtime`.

| # | Task | Reqs | Verify |
|---|---|---|---|
| T1 | `internal/lsp`: protocol types (with union normalization) + `Conn` (framing, calls, notifications, server requests, cancellation) | R1, R12 | fake-server unit tests, -race |
| T2 | `internal/gopls` pure parts: `uri.go`, `edits.go` (UTF-16 ApplyEdits, FileEdits), `resolve.go` (+ `Install`) | R1.1–2, R9.3 | table tests |
| T3 | `internal/gopls/server.go`: process, initialize, configuration, document tracking, requests, diagnostics, progress/status, applyEdit, supervision/backoff | R1.3–8, R2 | fake LSP server tests (re-exec) + real gopls integration test (skipped if missing) |
| T4 | `internal/textsearch`: Compile/Search/ReplaceText | R11 | table tests |
| T5 | Settings `[gopls]`; rpc: session starts and stops the gopls server; watcher → didChangeWatchedFiles; settings → restart; `LangService` + events; `SearchService.FindInPath/ReplaceInPath`; `gosdk.Runner.Environ()`; regenerate bindings; `lib/api.ts` wrappers + Topics | R1, R2, R7–R11 | rpc tests (missing binary status; find scope; edit application) |
| T6 | Frontend core: `positions.ts`, `markdown.ts`, `client.ts` (sync), `edits.ts`, `status.ts`, `install.ts` + feature registration | R1.7, R2, R9.3 | vitest: positions, markdown, edits |
| T7 | Completion (`completion.ts`) | R3 | vitest: mapping, snippets |
| T8 | Hover / quick doc / keyword docs / parameter info (`hover.ts`, `keywords.ts`, `signature.ts`) | R6 | vitest: keyword lookup, active-param split |
| T9 | Diagnostics + Problems panel + status counts (`diagnostics.ts`) | R7 | vitest: grouping/sorting, severity mapping |
| T10 | Navigation: definition/type/impl, Mod-click, chooser, history, go to test (`navigation.ts`, `popup.ts`) | R4.1–4 | vitest: history, test-name mapping |
| T11 | Find usages + highlight usages (`usages.ts`) | R5 | vitest: grouping, read/write heuristic |
| T12 | Rename + context actions (`rename.ts`, `codeActions.ts`) | R8, R9 | vitest: preview summary |
| T13 | File structure + breadcrumbs (`structure.ts`); palette Symbols tab (`symbols.ts`, `palette.ts`) | R4.5–6, R10 | vitest: symbol flatten/filter, crumb path |
| T14 | Find / Replace in Path UI (`findInPath.ts`) | R11 | vitest: mask parsing, result grouping |
| T15 | Keymaps, native menus, palette settings entries, CLAUDE.md/steering sync | all | tsc, tests, menu test |
| T16 | Acceptance: real gopls run in `wails dev` (CDP harness) + `acceptance.md`; retro | all | E2E smoke + manual notes |
