# M4 — Git + Lint + Modules: Tasks

Status: **all tasks done** (T1–T15 on branch `feat/m4-git-lint-modules`). Results: `acceptance.md`.

Branch `feat/m4-git-lint-modules`, one commit per task. Each task ends green: `gofmt -l`, `go vet ./...`, `go test -race ./internal/...`, and `cd frontend && npx tsc --noEmit && npm test`. After changing bound methods, run `wails generate module`, then `git checkout frontend/wailsjs/runtime`.

| # | Task | Reqs | Verify |
|---|---|---|---|
| T1 | `internal/git`: runner, status, show, diff parse + hunk patches, apply, stage/unstage/discard, commit, branches, merge/rebase/cherry-pick/revert/reset, continue/skip/abort, stash, log, blame, init, gitignore, conflict blocks, network command specs | R1, R3–R10 | real-git temp repo tests |
| T2 | rpc `GitService` + gitSession: status refresh/events, ops, network ops via process with output/stop; settings `[git]`; bindings + api | R1, R6, R14 | rpc tests (bare remote push/pull) |
| T3 | Frontend git core: status store, tree decorations hook, status-bar branch widget, Git console, commands skeleton, theme tokens | R1, R6 | vitest: decoration mapping |
| T4 | Commit panel (staged/changes/conflicts, stage/unstage/rollback, message, amend, commit & push, modal) | R3 | vitest: grouping |
| T5 | Diff viewer (side-by-side/unified, compare modes, per-chunk stage/unstage/revert) + TS patch builder | R4 | vitest: patch builder |
| T6 | Editor change markers + hunk popup (rollback, stage hunk) | R2 | vitest: chunk mapping |
| T7 | Branches popup, stash UI, push rejected / checkout conflicts flows | R5, R6.3, R7 | vitest: branch grouping |
| T8 | Git Log window (commits, files, diff, cherry-pick, revert, reset, branch here), blame annotations | R8, R9 | vitest: relative dates |
| T9 | Conflicts: operation banner (continue/skip/abort), 3-way merge editor, accept yours/theirs for whole files | R10 | vitest: conflict blocks |
| T10 | `internal/lint` + rpc `LintService` + multi-source diagnostics + on-save/on-demand + fixes + install | R11 | parse fixtures, real run if available |
| T11 | `internal/gomod` + rpc `ModService` + go.mod completion/links + module actions | R12, R13 | httptest proxy, vitest line contexts |
| T12 | Keymaps, Git/Tools menus, settings UI, close-check | R14 | menu/keymap tests |
| T13 | Packaging: dmg/zip, NSIS, nfpm deb/rpm, tar.gz, version check, release workflow, README install notes | R15 | workflow_dispatch run + local dmg |
| T14 | Acceptance E2E (`tools/e2e/m4.json`) + `acceptance.md` | all | E2E |
| T15 | Docs: CLAUDE.md, steering sync, retro | — | — |
