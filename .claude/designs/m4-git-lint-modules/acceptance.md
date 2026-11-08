# M4 — Acceptance Results

Date: 2026-11-08 · Host: macOS (arm64), Go 1.26.10, Wails 2.17.1, git 2.54, golangci-lint 2.15.0

Method:
- **Unit and integration tests:**
  - `go test -race ./internal/...`, with `GOI_DLV` and `GOI_GOLANGCI` set so the real-tool tests run;
  - `npm test` (399 tests);
  - real git in temporary repos (`internal/git`, rpc git tests, and a vitest that applies Goi's hunk patches with `git apply`).
- **Cross-builds and workflow lint:** `GOOS=windows go build ./...`, `GOOS=linux go vet ./internal/...`, and `actionlint` on both workflows.
- **End to end:** headless Chrome over CDP against `wails dev`, using `tools/e2e/m4.json` (43 steps) on `tools/e2e/m4-sample.sh`. The sample is a repository with a bare remote, a conflicting `feature` branch, a lint issue, a two-hunk change and an untracked file. It passed in one run on a fresh sample.
- **Packaging:** a local universal macOS build with the version stamped in, `make-dmg.sh`, then the dmg mounted and the app smoke-launched. Locally built `.deb` and `.rpm` were inspected. The full release workflow can only run on GitHub (see Not verified).

Reproduce:
```
tools/e2e/m4-sample.sh /tmp/goi-m4
GOI_HOME=/tmp/goi-m4-home GIT_CONFIG_GLOBAL=/tmp/goi-m4/gitconfig wails dev -appargs "--project /tmp/goi-m4/work"   # keep stdin open
node tools/e2e/cdp.mjs tools/e2e/m4.json
```
(Set `[lint] golangciPath` in `/tmp/goi-m4-home/config/settings.toml` if golangci-lint isn't on PATH.)

## Requirements

| Req | Check | Observed | Result |
|---|---|---|---|
| R1.1–2 | Repository, status | Porcelain v2 parser covers modified, added, renamed, untracked, conflicted and operation detection (`TestStatusCodes`, `TestBranchesMergeConflictAndResolve`). An edit on disk reaches `git.status` through the watcher (`TestGitStatusStageCommitPush`). | ✅ |
| R1.3 | Tree colors | `main.go` modified, `notes.txt` untracked | ✅ |
| R1.4 | Branch widget | `⎇ main ↑1`, then `⎇ main (MERGING)` during the merge | ✅ |
| R2.1 | Gutter markers | Two modified-line markers for the two changes | ✅ |
| R2.2 | Hunk popup, rollback, stage hunk | The popup shows the old line. **Rollback** restores `"ten"` and the markers clear; Mod-Z brings `"TEN"` back. A vitest stages range patches with real `git apply` for six cases (modify, add start/middle/end, delete middle/end). | ✅ |
| R3.1–3 | Commit panel | Groups `Changes 2`, then `Staged 1 / Changes 2` after a partial stage. Commit `Shout ONE` succeeded and staged emptied. Amend and the modal dialog share the same form (unit plus rpc commit/amend tests). | ✅ |
| R4.1–2 | Diff viewer, partial staging | `Change 1 of 2`. **Stage Change** put only `ONE` in the index (`index: ONE … ten`). Unstage and revert are covered by `TestPartialStagingRoundTrip`. | ✅ |
| R5 | Branches popup | Actions plus `★ main → origin/main`, `⎇ feature`, `☁ origin/main`. **Merge 'feature' into Current** worked. Name validation, tracking checkout and unmerged delete are tested. | ✅ |
| R6 | Push / pull / fetch | Mod-Shift-K: `git push --progress -u origin main` set the upstream, the Git console showed progress, then "Push finished". Rejected-push detection is unit-tested. | ✅ |
| R7 | Stash | `TestStashBlameGitignoreRemoteCheckout` covers stash with untracked files, list, diff and pop. The UI uses the same API. | ✅ unit |
| R8 | Git Log | The list includes the merge. A merge commit's files are listed against its first parent (`Mutil.go`). The menu offers Cherry-Pick, Revert, Checkout Revision, New Branch Here, Reset and Copy Hash. | ✅ |
| R9 | Blame | `9f2c64e Goi Test 2026-11-08 …` per line. Unsaved edits show as not committed (`TestStashBlame…`). | ✅ |
| R10 | Conflicts, 3-way merge | Banner `Merge in progress [Continue] [Abort]`, conflict row `C util.go`. The merge editor showed `Conflict 1 of 1`, with Yours `hey` / Result / Theirs `hi`. **Accept Theirs** then **Apply** staged the file, and **Continue** committed: HEAD has `return "hi " + name`. | ✅ |
| R11 | golangci-lint | Saving `util.go` gave `✖ 0 ⚠ 2`. Problems shows `func cleanup is unused` from `golangci-lint (unused)`; errcheck also reported. v1/v2 JSON, suggested fixes (v2 `SuggestedFixes`, v1 `Replacement`), a bad config surfacing as an error, and package-scoped replacement are all tested. | ✅ |
| R12 | go.mod editor | Path completion `golang.org/x/mod` (module cache). Versions from the proxy `v0.29.0 \| v0.28.0 …`. Link decoration on `golang.org/x/mod`. | ✅ |
| R13 | Module actions | Actions bar `go mod tidy \| Sync (go mod download) \| go mod vendor`. **go mod tidy** ran as a Run tab: `exit code 0`. | ✅ |
| R14 | Menus, keymaps, settings | Git and Tools menus and a Help menu. Version Control and Linting settings. Keymap, menu and settings-schema tests pass. | ✅ |
| R15 | Installers and release | A local universal `Goi.app` (x86_64 + arm64), `Goi-0.1.0-macos-universal.dmg` (15.8 MB, with an Applications link) mounted and launched, `0.1.0` embedded. The `.deb` control file has `libgtk-3-0, libwebkit2gtk-4.1-0`; `.rpm` builds. Release workflow lints clean; the version check passes for `v0.1.0` and rejects other tags. | ✅ local / ⏳ CI |
| R15.6 | Version in About | `Version 0.1 (dev)` in dev; `0.1.0` in release builds (ldflags) | ✅ |

## Bugs found by acceptance and fixed

1. **The Commit panel took over the left dock.** It registered before the Project tree, so it became the dock's default tab. The git feature now installs after the project tree.
2. **Merge commits showed no changed files.** `git show` gives merges an empty combined diff. Both the file list and the diffs now use `-m --first-parent`.
3. **Editor panels (breadcrumbs, the go.mod bar) were drawn over dialogs and the diff view.** CodeMirror panels use `z-index: 300`. Overlay layers are now above 300; this bug predated M4.
4. **Diff toolbar buttons wrapped** at narrow widths.

## Not verified

- **The GitHub release workflow itself.** Windows NSIS, the Linux build on Ubuntu, the arm64 Windows zip, checksums and the draft release need a push of the workflow and a manual run or tag. They were checked locally where possible (macOS dmg, nfpm packages, actionlint).
- **Code signing and notarization:** no certificates. The steps are gated on secrets and skipped otherwise.
- **Windows and Linux runs of the git, lint and module features.** CI runs their tests on all three operating systems.
- **Native window (WKWebView) interaction:** everything was driven through browser-mode E2E.
- **Install golangci-lint end to end:** it shares the toolbin path that Delve's install exercised in M3.
