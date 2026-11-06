# M1 — Acceptance & Performance Results

Date: 2026-11-06 · Host: macOS (Apple M5 Pro, arm64), Go 1.26.0, Wails 2.17.1
Method: unit/integration tests (`go test ./internal/...`, `npm test`), plus headless Chrome driven over CDP against `wails dev` (real backend, real bindings).

## Performance (NFR-2, NFR-3, R8.4, R9.2)

| Target | Requirement | Measured | Result |
|---|---|---|---|
| Cold start to backend ready | < 1.5 s | 0.14–0.18 s (3 runs, launch → `startup` log) | ✅ |
| Open 10k-file project → tree listing | < 1 s | 0.33 s (`TestPerfOpenLargeProject`) | ✅ |
| Search index ready, 10k files | — | 0.35 s | ✅ |
| File search, 50k paths | < 100 ms | 0.4–1.7 ms/query (`BenchmarkSearch50k`) | ✅ |
| gofmt / goimports, 5k lines | < 200 ms | 6 ms / 18 ms (`BenchmarkFormat5kLines`) | ✅ |
| Typing latency, 10k-line Go file | < 16 ms | p50 16.7, p95 17.5, max 17.7 ms (input → next rAF, 60 Hz; frame-bound) | ✅ (one frame) |

## Functional acceptance (macOS, headless E2E unless noted)

| Req | Check | Result |
|---|---|---|
| R1 | Welcome, recent list, reopen last project | ✅ unit + reopen observed on every dev start |
| R2 | Detection (work/module/multi/gopath/folder), re-detect on go.mod change | ✅ unit + integration |
| R3 | SDK discovery, status bar "Go 1.26.0" | ✅ E2E |
| R4 | Env/build settings, status bar `darwin/arm64` | ✅ E2E |
| R5 | Project tree, excludes, external changes | ✅ E2E (tree) + unit (fs reconcile) |
| R6 | Tabs, splits, highlighting (Go 196+ spans), large/binary modes, EOL/BOM, disk sync, close guard, crash restore | ✅ E2E for tabs/splits/highlight; crash restore observed (dev sessions killed → "Restore unsaved changes?" dialog); others unit |
| R7 | Run markers gutter (`▶ Run program main`) | ✅ E2E |
| R8 | Format on save, single undo, syntax error still saves | ✅ unit/integration (backend + diff helper) |
| R9 | Search Everywhere via double-Shift / ⌘⇧O / ⌘⇧A with key hints | ✅ E2E (⌘⇧A, ⌘⇧O opened `perf10k.go`) |
| R10 | Keymaps + chords + rebinding with conflict check | ✅ unit; ⌘\ split and ⌘, settings via E2E |
| R11 | Themes, auto scheme follows UI, HC contrast ≥ 4.5 | ✅ unit (contrast) + E2E (light UI → goi-light) |
| R12 | Layout persistence, split tree restore, multi-window locks | ✅ E2E (layout restored across runs) + unit (instance locks) |
| R13 | Layered settings, hot reload, settings UI | ✅ E2E (⌘, search) + unit |

## Not yet verified

- **Windows and Linux**: the code cross-compiles (`GOOS=windows` amd64/arm64 app, `GOOS=linux` arm64 core), but nothing has been run there. CI (`.github/workflows/ci.yml`) has not run because the repo has no GitHub remote yet.
- **Native window behaviours** on macOS that headless Chrome can't exercise: native folder dialog, Finder "Open With Goi", a second window launched with `open -n`, the close-window prompt (OnBeforeClose), trash/reveal.
- **macOS file watching at scale**: fsnotify uses kqueue (one descriptor per watched file). Fine at 10k files; very large repos may hit the descriptor limit (a warning notification is shown). FSEvents is the follow-up.
