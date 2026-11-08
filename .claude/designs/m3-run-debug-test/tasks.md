# M3 — Run / Test / Debug: Tasks

Status: **all tasks done** (T1–T16 on branch `feat/m3-run-debug-test`). Results: `acceptance.md`.

Branch `feat/m3-run-debug-test`, one commit per task. Each task ends green: `gofmt -l`, `go vet ./internal/...`, `go test ./internal/...` (with `-race` for process/dap/debugger/terminal/rpc), and `cd frontend && npx tsc --noEmit && npm test`. After changing bound methods, run `wails generate module`, then `git checkout frontend/wailsjs/runtime`.

| # | Task | Reqs | Verify |
|---|---|---|---|
| T1 | `internal/toolbin` (generic resolve/install); `gopls` delegates to it | R5.1 | existing gopls tests + toolbin tests |
| T2 | `internal/runconfig`: types, store (shared/local/temporary), validate, env file, split args, test pattern, plans, debug launch args | R1, R2.1, R3.3, R5.2 | table tests |
| T3 | `internal/process`: process groups, stdin, stop escalation, exit info | R2.3 | tests (children killed, SIGINT handled, stdin echo) |
| T4 | rpc `RunService` + runManager: config CRUD/watch, launches with before-launch tasks, output batching, test2json events, bench history; settings `[run]`/`[debug]`/`[terminal]`; bindings + `api.ts` | R1, R2, R3.1, R3.5, R11.2–3, R12 | rpc tests on a temp module |
| T5 | Frontend run core: ansi, links, console, Run window, run widget, configurations dialog, commands | R1.4–5, R2 | vitest: ansi, links, console cap |
| T6 | Test runner UI: test tree reducer, view, rerun failed / single, testify diff, benchmarks table | R3 | vitest: tree, pattern, bench, diff |
| T7 | Run markers and context: gutter Run/Debug, context configuration, project tree "Run Tests" | R4 | vitest: context mapping |
| T8 | `internal/dap`: protocol types and Conn | R12.2 | fake adapter tests |
| T9 | `internal/debugger`: dlv session, launch/attach, breakpoints, requests, run to cursor | R5, R6–R9 backend | real dlv integration test (skipped if missing) |
| T10 | rpc `DebugService` + install Delve; events | R5.1–2 | rpc tests |
| T11 | Frontend breakpoints: gutter, editor dialog, View Breakpoints, function/panic, persistence | R6 | vitest: mapping, hit-condition labels |
| T12 | Debug window: toolbar/stepping, goroutines (group/filter/hide/thread ids), frames, variables (options, view as string, set value), watches, evaluate, console, stopped line, inline values | R5.3–5, R7–R9 | vitest: goroutine parse/group/filter, value formatting, inline value matching |
| T13 | `internal/terminal` + rpc `TerminalService` + shell integration; frontend xterm terminal tabs and links | R10 | PTY round-trip test; vitest: link parsing |
| T14 | Keymaps, native Run menu, settings UI entries, close confirmation, theme tokens | R11 | tsc, menu test |
| T15 | Acceptance: E2E (`tools/e2e/m3.json`) against `wails dev` with real dlv; `acceptance.md` | all | E2E |
| T16 | Docs: CLAUDE.md, steering sync, retro | — | — |
