# Wails v3: one process, many windows — Acceptance

Status as of 2026-11-12, branch `feat/wails-v3`, macOS 26.6 (arm64). "Manual" rows are for the reviewer: they need a person at the machine (Dock, Cmd-Tab) or another OS.

## Automated checks

| Check | Result |
|---|---|
| `go test -race ./internal/...` | pass |
| `go vet ./...`, `go vet -tags server ./internal/rpc`, gofmt | clean |
| `cd frontend && npx tsc --noEmit && npm test` | clean; 399 tests pass |
| `go build -tags production .` / `go build -tags server .` | ok |
| E2E `multiwindow.json` (server build, two tabs = two windows) | pass: tab 2 starts on Welcome, opens `b` from Recent; A keeps only A's tree, B shows only B's |
| E2E `smoke.json` (server build) | pass |
| E2E `m2.json` (gopls) | pass (completion, navigation, docs, usages, structure, palette symbols, quick-fix, multi-file rename, find in path) |
| E2E `m3.json` | pass except the debugger block (see below) |
| E2E `m4.json` | pass except golangci-lint steps (not installed here) |

Two-window unit tests (`*_multiwindow_test.go`, `windows_test.go`, `window_test.go`) cover: routing by call context for every service group; project/run/terminal/git/lint/diagnostic events reaching only their window; user settings syncing to every window; project settings staying per window; the file guard per window; open-or-focus, Welcome reuse, symlinked paths; the second-instance handoff (relative paths, files, no args); quit closing windows one by one and stopping on cancel; reopening the open set; `WindowID` waiting for ready.

## Requirements

| Req | Status | Evidence |
|---|---|---|
| R1.1–R1.2 single process, handoff | pass | Real run: second `goi --project B` exited 0, one `goi` process, B opened in a new window (log `second instance` → `project opened`); unit tests |
| R1.3 take over from a dead primary | pass (by design) | Wails `SingleInstance` uses an OS lock released on process death. A *hung* primary isn't detected (noted in retro) |
| R1.4 separate `GOI_HOME`s | pass | `UniqueID` derives from the config dir (`instanceID`) |
| R2.1–R2.3 project windows, dedupe | pass | unit tests; multiwindow E2E |
| R2.4 titles | pass | `title_test.go`; per-window `SetTitle` |
| R2.5 cascade | implemented | `newNativeWindow` offsets 28 px from the focused window. Manual check |
| R2.6 files from the OS | pass | `TestSecondInstanceOpensFileInItsWindow`, `TestOpenFileQueuedThenOpensProject`; `ApplicationOpenedWithFile` wired. Manual: Finder "Open With" |
| R3.1–R3.5 per-window sessions/events/settings | pass | multi-window unit tests; E2E |
| R3.6 closing a window releases its project | pass | `TestRemoveWindowClosesItsSession`, close tests |
| R3.7 isolation of failures | partial | Bound-call panics are recovered by Wails; gopls restarts per server. No extra `recoverSession` wrapper was added (retro) |
| R4.1–R4.3 Dock icon and menu | implemented | `internal/dockmenu` (spike-verified `applicationDockMenu:` hook), `rpc/dock.go`. **Manual:** two projects open → one Dock icon; right-click lists both + New Window + Open Recent; choosing switches |
| R4.4 Window menu | implemented | `WindowMenu` role (spike: sets `NSApp.windowsMenu`). Manual |
| R4.5 Dock click with no windows | implemented | `ApplicationShouldHandleReopen` → `reopen()`. Manual |
| R4.6 Cmd-Tab / App Exposé | expected by construction (one process). Manual |
| R5.1–R5.3 menus and commands | pass | `menu_test.go`; mac: focused window; Windows/Linux: menu per window. Manual on Windows/Linux |
| R6.1 last window | pass | Closing the last project window opens Welcome (`TestLastProjectWindowOpensWelcome`); closing a last Welcome window quits off macOS (`TestLastWindowQuitsOffMac`) |
| R6.2 quit with cancel | pass | `TestQuitClosesWindowsAndStopsOnCancel`; frontend reports cancels (`CancelClose`). Manual: Cmd-Q with unsaved files in two windows |
| R6.3 reopen open projects | pass | `TestStartupReopensOpenSet` (`open-projects.json`) |
| R6.4 crash journal per project | pass | `TestCrashJournalRestore` |
| R7.1 Windows | pending | CI builds (`wails3 build`); installer/taskbar grouping need a Windows run |
| R7.2 Linux | pending | Builds with `-tags gtk3`, nfpm deps switched to GTK3/WebKitGTK 4.1; needs a Linux run |
| R7.3 macOS file associations, sheets, appearance, universal | partial | Associations in `build/darwin/Info.plist` + `FileAssociations`; `OpenFolderDialog` attaches to the calling window. Manual: open/save sheets, `wails3 task darwin:package:universal` |
| R8.1 no M1–M4 regressions | see E2E | |
| R8.2 frontend API shape | pass | Generated TS signatures unchanged by the ctx params (only doc comments differ); `lib/api.ts` keeps its exports |
| R8.3 architecture rules | pass | Wails imported only in `internal/rpc` and nowhere in `main.go`; `internal/dockmenu` is plain cgo |
| R8.4 test suites | pass (see above) | |
| R8.5 performance | pass (v3 numbers; no v2 baseline taken) | Cold start to project open: 0.54 s. Second project via a second launch: 0.55 s to its window's project open (≈0.38 s of it is the short-lived second process starting). Opening from inside Goi (Recent/Dock) skips that process: the project opens in ~0.17 s after the request (log `second instance` → `project opened`) |
| R9 tooling | pass | CLAUDE.md, steering, `.run/`, `ci.yml`, `release.yml` updated. Release dry run (workflow_dispatch) still to run on GitHub |

## E2E M3/M4

Run against `bin/goi-server` (server build), fresh `GOI_HOME`, samples under `/private/tmp/goi-e2e/<suite>sample`. Notes:
- The samples must live at a real path. Under the `/tmp` symlink, git's paths and the project's paths differ and the git steps time out. That isn't caused by this change.
- The specs call `window.go.rpc.*`, which `lib/api.ts` now provides as in v2.

- **M3:**
  - **Pass:** run configurations and the widget; the test tree, rerun failed, whole-file runs and the expected/actual diff; the console (ANSI, stderr, links, stdin, exit codes, stop/rerun); benchmarks with comparison; breakpoints and conditions; terminals (shell integration, project-root cwd, Ctrl-R kept by the shell, tabs, exit codes).
  - **Not run:** the Delve session steps, because Delve isn't installed on this machine.
  - **Unexplained, needs a check with Delve installed:** "gutter Debug 'TestWorkers'" didn't find the gutter run marker for TestWorkers in `worker_test.go`. That step comes before Delve is involved, so it could be spec timing or a real issue.
- **M4:**
  - **Pass:** git status colours, gutter markers and the hunk popup (rollback and undo), the commit panel, diff with partial staging, commit, push with the Git console, branches, merge conflict with the 3-way merge and continue, the log and commit details, annotate, go.mod completion with proxy versions, module actions, About.
  - **Not run:** the golangci-lint steps, because golangci-lint isn't installed here.
