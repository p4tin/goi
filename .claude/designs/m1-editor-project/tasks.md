# M1 — Editor + Project Model: Tasks

Rules: one task at a time, one commit per task (`feat(m1): T<n> …`), stop for review after each.
Each task lists the requirements it covers (Req), the files it touches, and how it's verified (Verify).
Status: `[ ]` todo · `[~]` in progress · `[x]` done

---

## Phase A — Foundation

- [x] **T1 — Repo bootstrap & Wails scaffold**
  - `git init`; `.gitignore` (build/bin, frontend/node_modules, frontend/dist, `.goi/cache`); install the Wails CLI; `wails init -n goi -t vanilla-ts` merged into the repo root; set `go 1.25` in `go.mod`, module `github.com/p4tin/goi` (confirm path).
  - Create the empty package dirs from `structure.md` with `doc.go` each.
  - Frontend: enable TS `strict`, add vitest + `npm test`.
  - Req: NFR-1 (partial) · Verify: `wails build` produces a launching blank window on macOS; `go vet ./...`, `npm test` pass.

- [x] **T2 — `events` + `applog` + startup wiring**
  - `internal/events` (Bus, Nop); `internal/applog` (slog JSON + 10 MB×5 rotating writer); `internal/rpc/bus.go` (Wails adapter); `AppService.Startup/Info`; flag parsing (`--project`).
  - Req: NFR-5 · Verify: unit test for rotation; log file appears at the cache path on launch.

- [x] **T3 — `settings` package**
  - Embedded `defaults.toml`, layered merge, provenance, `Set` with atomic write, reload callback hook (watcher injected later), invalid-TOML keeps last good.
  - Req: R13.1, R13.2 · Verify: table-driven tests for merge/provenance/invalid/Set round-trip.

- [x] **T4 — Frontend core libs**
  - `lib/store.ts`, `lib/dom.ts`, `lib/commands.ts`, `lib/api.ts` (wrappers for the bindings that exist so far + `on(topic)`).
  - Req: — (infrastructure) · Verify: vitest for store and command registry.

- [x] **T5 — Theme system**
  - `theme/tokens.css` (light/dark/hc), `theme/theme.ts` (auto via `matchMedia`, live switch), `theme/schemes.ts` (4 original schemes → CM highlight styles); contrast test for HC.
  - Req: R11.1–R11.4 · Verify: vitest contrast ≥ 4.5:1; toggling the OS appearance switches live.

## Phase B — Files & project model

- [x] **T6 — `vfs` read/write**
  - `ReadText`/`WriteText` with EOL/BOM/final-newline preservation, binary detection, size modes, atomic write.
  - Req: R6.5–R6.7, NFR-4 · Verify: round-trip tests (LF/CRLF/CR/mixed/BOM/no-final-NL/binary/size thresholds); no temp files left.

- [x] **T7 — `vfs` tree ops, trash, reveal**
  - `List` (excludes via doublestar, dirs first, natural sort), `Create`, `Rename`, `Trash` (darwin/windows/linux files), `Reveal`.
  - Req: R5.2, R5.5, NFR-4 · Verify: unit tests for List/Create/Rename/excludes; Linux trash test against a temp `XDG_DATA_HOME`; manual trash/reveal on macOS.

- [x] **T8 — `vfs` watcher**
  - fsnotify recursive watcher, excludes, auto-add new dirs, 75 ms coalescing, inotify `ENOSPC` → notify; hook settings hot-reload (T3) to it.
  - Req: R5.4, R13.2 · Verify: watcher tests (create/rename/delete batched within 1 s; excluded dirs silent); editing `settings.toml` externally triggers reload.

- [x] **T9 — `workspace` detection & manager**
  - `Detect` (work → module → multi-module → gopath → folder), `ParseError` handling, `Manager` with re-detect on go.mod/go.work changes (300 ms debounce), content roots, excludes persisted to project settings.
  - Req: R2.1–R2.6, R5.3 · Verify: fixture tests in `internal/workspace/testdata`; re-detect test via the watcher.

- [x] **T10 — `gosdk` discovery, validation, runner**
  - `Discover` (all OS locations), `Validate`, version probing cache, `Runner.Command` (env/tags/GOOS/GOARCH/CGO/-mod), `GoEnv`.
  - Req: R3.1–R3.6, R4.1–R4.2 · Verify: tests with fake GOROOT trees and assertions on `cmd.Env`/`cmd.Args`; `GoEnv` integration test skipped if no `go` found.

- [x] **T11 — `recent` store + `journal`**
  - Recent: JSON + flock, max 20, `Exists` flag. Journal: `session.lock`, crash detection by stale pid, Put/Drop/Pending/CloseClean.
  - Req: R1.3, R1.4, R6.10 · Verify: unit tests incl. two-goroutine flock contention and stale-pid detection.

- [x] **T12 — RPC services for project & files**
  - `AppService` (OpenFolderDialog, OpenProject, PendingRestore, SaveLayout/LoadLayout), `WorkspaceService`, `FileService`, `SDKService`, `SettingsService`, `RecentService`; full startup order; regenerate bindings; extend `lib/api.ts`.
  - Req: wiring for R1–R5, R13 · Verify: `wails dev` boots; calling each binding from devtools returns expected DTOs; `go test ./internal/rpc/...` for DTO mapping.

## Phase C — Shell & views

- [x] **T13 — Workbench layout & panels**
  - CSS-grid shell, resizable/collapsible left/right/bottom panels, toggle commands, status bar skeleton, notifications (toasts + panel consuming `notify`), layout save/restore per project.
  - Req: R12.1, R12.2 · Verify: resize/collapse persists across restart; `notify` events render.

- [x] **T14 — Welcome screen**
  - Open Folder, New Project placeholder (creates folder + `go mod init` via Runner after asking for the module path), recent list with missing-entry greying/remove, reopen-last-project on start.
  - Req: R1.1–R1.5 · Verify: manual flow; deleted folder shows greyed with Remove.

- [x] **T15 — Multi-window instances**
  - `--project` launch, instance lock + loopback `focus` ping, "Open in New Window", macOS `open -n` path.
  - Req: R12.3 · Verify: opening project A twice focuses the existing window; A and B in two windows simultaneously; recent list updates in both.

- [x] **T16 — Project tree**
  - Virtualized lazy tree over content roots, `fs.changed` reconcile keeping expansion/selection, context menu (new file/dir/Go file, rename, trash, copy path, reveal), additional-root and exclude actions.
  - Req: R5.1–R5.6 · Verify: 5,000-entry dir stays smooth; external `touch`/`rm` reflected < 1 s; new Go file gets `package <dir>`.

- [x] **T17 — SDK picker, Go env & status bar**
  - SDK picker dialog (discover, select, add manually with validation), "No Go SDK" warning, `go env -json` on open, GOOS/GOARCH/GOVERSION + cross badge in status bar.
  - Req: R3.3–R3.6, R4.3, R4.4 · Verify: switch SDK → status bar updates; invalid GOROOT rejected with message.

## Phase D — Editor

- [x] **T18 — Editor manager, tabs & base extensions**
  - Buffers, open/close/dirty, tabs (drag reorder, pin, close variants), base CM extensions (multi-cursor, rectangular select, folding, brackets, auto-indent, soft-wrap, line numbers, search/replace, go to line, undo).
  - Req: R6.1, R6.3, R6.4 (tabs) · Verify: manual checklist; vitest for buffer dirty-state logic.

- [ ] **T19 — Splits**
  - Split right/down up to 2×2, same file in two panes sharing a buffer, focus/move-between-pane commands; persisted in the layout.
  - Req: R6.4 (splits), R12.2 · Verify: edits in one pane appear in the other; layout restores.

- [ ] **T20 — Languages**
  - `languages.ts` mapping + lazy imports; custom `gomod.ts` and `gotmpl.ts` stream languages; plain-text fallback; large-file banners/modes; binary placeholder.
  - Req: R6.2, R6.5, R6.6 · Verify: open a sample file for each listed type (fixtures in `frontend/test/samples/`); 3 MB file → no-highlight banner; 25 MB → read-only.

- [~] **T21 — Save flow & format on save**
  - `internal/format` (gofmt/goimports, SyntaxError, benchmark), `EditorService.Format`, `saveFlow.ts` (single-transaction minimal diff, lint diagnostic on syntax error), "Reformat File" command, save-mode setting.
  - Req: R8.1–R8.5, R6.7 · Verify: golden tests; benchmark < 200 ms at 5k lines; one undo reverts formatting; a broken file still saves.

- [ ] **T22 — Disk sync & close guard**
  - External-change reload / conflict banner / compare (merge view); ignore self-writes; Save All / Discard / Cancel on window close.
  - Req: R6.8, R6.9 · Verify: modify an open file externally in both clean and dirty states; close with dirty buffers prompts.

- [ ] **T23 — Crash journal & restore**
  - 2 s `journalTicker`, Drop on save/discard, restore dialog on startup from `PendingRestore`.
  - Req: R6.10 · Verify: edit, `kill -9` the process, relaunch → restore offered and content intact.

- [~] **T24 — Run markers gutter**
  - `internal/gosyntax.RunMarkers` + tests, `EditorService.RunMarkers`, `runGutter.ts` with 300 ms debounce and version check, placeholder popup.
  - Req: R7.1–R7.3 · Verify: rule tests (`Testfoo` excluded, dot-import, generics); markers update while typing.

## Phase E — Navigation & input

- [ ] **T25 — File index & search**
  - `internal/index` Build/Apply/Search with scorer, recent boost, 50k benchmark; `SearchService.Files`; incremental updates from the watcher.
  - Req: R9.2, R9.5 · Verify: scoring goldens (`wsmgr` → `workspace_manager.go` top); benchmark < 100 ms.

- [ ] **T26 — Search Everywhere palette**
  - Palette UI with All/Files/Actions/Settings tabs, virtualized results, double-Shift detector, Recent Files and Go to Line commands.
  - Req: R9.1, R9.3, R9.4 · Verify: vitest for the double-Shift detector and `rank.ts`; manual palette checklist.

- [ ] **T27 — Keymaps & rebinding**
  - `keymaps.ts` (goi-default, vscode × mac/other), chord dispatcher, CM keymap from the same table, user overrides in `settings.keys`, conflict prompt in the settings UI.
  - Req: R10.1–R10.3 · Verify: vitest for chord parsing and conflict detection; switching keymap applies immediately.

- [ ] **T28 — Settings UI**
  - Schema-driven searchable form over all M1 settings, origin badges (default/user/project), write to the chosen layer, theme/scheme/keymap pickers.
  - Req: R13.3 (+ UI for R4, R8.1, R10, R11) · Verify: change a project setting → `.goi/settings.toml` updated and badge shows "project".

## Phase F — Release readiness

- [~] **T29 — Platform packaging & CI**
  - `build/darwin/Info.plist` document types, Windows WebView2 embed, Linux `webkit2_41` tag; GitHub Actions matrix (macOS/Windows/Ubuntu: vet, test, npm test, wails build) + arm64 compile-only job.
  - Req: NFR-1 · Verify: CI green on all jobs.

- [ ] **T30 — Performance & acceptance pass**
  - Measure NFR-2/3 (cold start, 10k-file open, typing latency) and fix regressions; run the manual acceptance checklist on macOS, Windows and Linux; record results in `retro.md` notes.
  - Req: NFR-2, NFR-3, all R* · Verify: checklist complete, numbers recorded.

---

---

## Parallelization matrix

### Task splits (to unlock earlier parallel work)
Several tasks combine a pure-Go package, which has no UI dependency, with its UI wiring. The package half is pulled forward:

| Split | Part a: backend package only (early) | Part b: wiring + UI (late) |
|---|---|---|
| T21 | ✅ **T21a** `internal/format` + goldens + benchmark | **T21b** `EditorService.Format`, `saveFlow.ts`, Reformat command |
| T24 | ✅ **T24a** `internal/gosyntax` + rule tests | **T24b** `EditorService.RunMarkers`, `runGutter.ts` |
| T25 | ✅ **T25a** `internal/index` Build/Apply/Search + benchmark | **T25b** `SearchService.Files`, watcher hookup |
| T29 | ✅ **T29a** CI workflow (vet, test, npm test, wails build on 3 OSes) | **T29b** Info.plist doc types, WebView2 embed, arm64 job |

### Dependency matrix
`Depends on` = must be merged first. `Hotspots` = shared files whose edits must be merged one at a time (see below).

| Task | Depends on | Layer | Hotspots touched |
|---|---|---|---|
| T1 | — | both | everything (scaffold) |
| T2 | T1 | Go | main.go, go.mod |
| T3 | T2 | Go | go.mod |
| T4 | T1 | TS | package.json |
| T5 | T4 | TS | — |
| T6 | T2 | Go | — |
| T7 | T2 | Go | go.mod |
| T8 | T3, T7 | Go | go.mod |
| T9 | T3, T8 | Go | go.mod |
| T10 | T2 | Go | — |
| T11 | T2 | Go | go.mod |
| T12 | T4, T6–T11 | Go+TS | main.go, rpc/*, api.ts, wailsjs |
| T13 | T5, T12 | TS | commands.ts |
| T14 | T13 | TS | — |
| T15 | T13 | Go+TS | rpc/app.go, api.ts |
| T16 | T13 | TS | commands.ts |
| T17 | T13 | TS | — |
| T18 | T13 | TS | commands.ts, package.json |
| T19 | T18 | TS | editorManager.ts |
| T20 | T18 | TS | package.json |
| T21a | T2 | Go | go.mod |
| T21b | T18, T21a | Go+TS | rpc/editor.go, api.ts, commands.ts |
| T22 | T18 | TS | editorManager.ts |
| T23 | T11, T18 | TS | editorManager.ts |
| T24a | T2 | Go | — |
| T24b | T18, T24a | Go+TS | rpc/editor.go, api.ts |
| T25a | T7 | Go | — |
| T25b | T8, T12, T25a | Go+TS | rpc/search.go, api.ts |
| T26 | T18, T25b | TS | commands.ts |
| T27 | T18 | TS | commands.ts |
| T28 | T5, T27 | TS | — |
| T29a | T1 | CI | .github/ |
| T29b | T29a, T20 | build | build/ |
| T30 | all | — | — |

### Execution waves
Tasks in a wave are independent and can run concurrently. A wave starts once every dependency from earlier waves is merged.

| Wave | Tasks (parallel) | Width | Gate to next wave |
|---|---|---|---|
| W0 | T1 | 1 | scaffold builds |
| W1 | T2, T4 | 2 | — |
| W2 | T3, T5, T6, T7, T10, T11, T21a, T24a, T29a | **9** | all Go pkgs green in CI |
| W3 | T8, T25a | 2 | — |
| W4 | T9 | 1 | backend packages complete |
| W5 | T12 | 1 | bindings regenerated |
| W6 | T13 | 1 | workbench shell |
| W7 | T14, T15, T16, T17, T18, T25b | 6 | editor core exists |
| W8 | T19, T20, T21b, T22, T23, T24b, T26, T27 | **8** | — |
| W9 | T28, T29b | 2 | — |
| W10 | T30 | 1 | M1 done |

Critical path: T1 → T2 → T3 → T8 → T9 → T12 → T13 → T18 → T27 → T28 → T30 (11 steps, versus 34 sequential tasks once the splits are counted).

### Hotspot rules (merge conflict control)
- **`main.go`, `internal/rpc/*`, `frontend/src/lib/api.ts`, `frontend/wailsjs/`:** only one task per wave may add bindings. In W8, T21b and T24b both add to `EditorService`: merge T21b first, then rebase T24b and regenerate bindings.
- **`go.mod`/`go.sum`, `package.json`/lockfile:** parallel tasks add dependencies on their branches. On merge, run `go mod tidy` / `npm install` and commit the result in the merge commit.
- **`commands.ts`:** each task registers its commands in its own module (`editor/commands.ts`, `palette/commands.ts`, …) imported from one index, so parallel edits stay additive.
- **`editorManager.ts` (T19, T22, T23 in W8):** these extend it through hooks (`onOpen`, `onExternalChange`, `onDirty`) defined in T18, not by editing the same functions.

### How parallel waves are executed
- Each task in a wave runs in its own git worktree on branch `m1/T<n>`, with at most 4 at once.
- Each task still ends with its own report. The user reviews the whole wave at once (one combined summary, one diff per task), then the branches merge to `main` in task-number order.
- A wave of width 1–2 runs inline with no worktrees, because the overhead isn't worth it.

---

M1 milestone acceptance (from SPEC §6): *open a `go.work` monorepo, edit, gofmt on save, switch theme*. Reachable after W8 (T21b).
