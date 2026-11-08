# M4 — Git + Lint + Modules → v0.1: Requirements

Scope (SPEC.md §6 M4): Git (F-VCS-1), diff and merge (F-VCS-2), golangci-lint (F-INS-1), the `go.mod`/`go.work` editor (F-MOD-1) and module actions (F-MOD-2). This milestone completes **v0.1**, so it also ships **installers and a release pipeline** for macOS, Windows and Linux.

Out of scope, by SPEC priority:
- The log graph with filters (F-VCS-3). A plain recent-commits list is in scope, as the place to cherry-pick, revert and view commits from.
- GitHub/GitLab integration (F-VCS-4) and the batch "Inspect Code" (F-INS-2).
- `//nolint` quick-fixes (F-INS-3), dependency-update inspections (F-MOD-3) and govulncheck (F-MOD-5).
- Code signing and notarization. These need the publisher's certificates; the pipeline has hooks for them (R15.5).

Behaviour follows mainstream Go IDEs, with Goi's own wording, look and command ids. Git is driven through the `git` CLI, so it uses the user's configuration, credential helpers, SSH agent and hooks.

## R1. Repository and status
1. WHEN a project opens THEN the core SHALL find the git repository containing the project root (`git rev-parse --show-toplevel`). IF there is none THEN Git features SHALL be hidden, apart from **Initialize Repository**.
2. File status (modified, added, deleted, renamed, untracked, ignored, conflicted) SHALL be computed with `git status --porcelain=v2 -z --branch`. It SHALL refresh after file changes (debounced 500 ms), after every Git action, and when the window gains focus.
3. The project tree SHALL color file names by status and mark directories that contain changes.
4. The status bar SHALL show the current branch (or the detached commit) with ahead/behind counts and any in-progress operation (merging, rebasing, cherry-picking, reverting). Clicking it SHALL open the branches popup.

## R2. Editor change markers
1. Go and text editors in the repository SHALL show gutter markers for lines added, modified or deleted compared with the index (staged content), updated as the user types.
2. Clicking a marker SHALL show the hunk's previous text with **Rollback**, **Stage Hunk** and **Show Diff**. Rollback SHALL restore the hunk in the editor as an undoable edit.

## R3. Commit (non-modal panel and modal dialog)
1. A **Commit** tool window (left dock) SHALL list **Staged** and **Changes** (unstaged plus untracked), and **Merge Conflicts** while a merge is in progress. Each file shows its status letter and folder, with stage/unstage per file and per group.
2. Double-clicking a file SHALL open its diff (R4). The context menu SHALL offer Stage, Unstage, Rollback (with confirmation; untracked files go to the trash), Open, Show Diff, and Add to .gitignore.
3. A message box SHALL commit the staged changes on **Commit** (Mod-Enter) or **Commit and Push**. An **Amend** toggle SHALL prefill the last message and amend the last commit. Committing with nothing staged SHALL offer to stage all changes first.
   - An empty message SHALL block the commit.
   - A failing `git commit` (hooks, identity not configured) SHALL show git's output.
4. **Commit…** (Mod-K) SHALL focus the commit panel. **Commit (Modal)…** SHALL open the same form in a dialog.

## R4. Diff viewer (F-VCS-2)
1. A diff tab SHALL show two versions side by side (CodeMirror merge view) or unified, with a toggle. Supported comparisons: working tree vs index, index vs HEAD, working tree vs HEAD, and a commit vs its parent.
2. In working-tree-vs-index diffs, each change SHALL offer **Stage** and **Revert**. In index-vs-HEAD diffs, each change SHALL offer **Unstage**. This is partial (hunk) staging through `git apply --cached`.
3. The diff SHALL use the file's language highlighting and support next/previous change navigation (F7 / Shift-F7 inside the diff).

## R5. Branches
1. A **Branches** popup (Ctrl-Shift-` on all platforms, plus the status-bar widget) SHALL list local and remote branches, with the current one marked and a filter. Actions: **New Branch…** (from the current HEAD, checked out), and per branch: **Checkout**, **New Branch from…**, **Rename…**, **Delete** (force after confirmation if unmerged), **Merge into Current**, **Rebase Current onto**, **Compare with Current**.
2. Checking out a remote branch SHALL create a local tracking branch. A checkout that local changes would overwrite SHALL show git's message and offer **Stash and Checkout**.

## R6. Remote operations
1. **Fetch** (all remotes, prune), **Pull** (merge, or rebase per setting `git.pullRebase`) (Mod-T) and **Push** (Mod-Shift-K) SHALL run in the background with progress in a **Git** console tab. A push with no upstream SHALL set one (`-u origin <branch>`).
2. Git SHALL run with `GIT_TERMINAL_PROMPT=0`, so it never waits for terminal input. Authentication uses the user's credential helper and SSH agent, and failures SHALL show git's message.
3. A rejected push (non-fast-forward) SHALL offer **Pull and Push**.

## R7. Stash
**Stash Changes…** (message, include untracked) and a **Stashes** list with Apply, Pop, Drop and Show Diff.

## R8. History, cherry-pick, revert
1. A **Git Log** tool window SHALL list recent commits (default 200, more on scroll) for the repository or a file (**Show History**). Each row shows hash, subject, author and relative date, with a text filter.
2. Selecting a commit SHALL show its changed files, and opening one SHALL show its diff against the parent. The context menu SHALL offer **Cherry-Pick**, **Revert Commit**, **Checkout Revision**, **New Branch Here…**, **Reset Current Branch to Here…** (soft/mixed/hard, hard needing confirmation) and **Copy Hash**.

## R9. Blame / annotate
**Annotate with Git Blame** SHALL show, beside each line, the short hash, author and date of the commit that last changed it, using the buffer's current contents (`git blame --porcelain --contents -`). Hovering SHALL show the full message. Clicking SHALL show that commit in the log. Running the command again SHALL hide the annotations.

## R10. Conflicts and 3-way merge
1. WHEN a merge, rebase, cherry-pick, revert or stash pop stops on conflicts THEN Goi SHALL say so, list the conflicted files, and show **Continue**, **Skip** (rebase) and **Abort** for the operation.
2. **Resolve** on a conflicted file SHALL open a 3-way merge editor:
   - **Yours** (`:2`, read-only) on the left, **Theirs** (`:3`, read-only) on the right, and the editable **Result** in the middle.
   - The Result starts from the working file, with conflict blocks detected from the markers.
   - Each block offers **Accept Yours**, **Accept Theirs** and **Accept Both**. **Accept All Yours/Theirs** applies to every block.
   - **Apply** saves the result and stages it, but only once no markers remain, or after the user confirms.
3. Conflicted files SHALL also offer **Accept Yours** / **Accept Theirs** for the whole file from the commit panel.

## R11. golangci-lint (F-INS-1)
1. golangci-lint SHALL be resolved like gopls and dlv: the `lint.golangciPath` setting, then PATH, GOBIN, GOPATH/bin, then the tools dir. IF it is missing THEN Goi SHALL offer **Install golangci-lint** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`).
2. Saving a Go file (`lint.onSave`, default on when `lint.enabled`) SHALL lint that file's package. **Run golangci-lint** (Project or current package) runs on demand. golangci-lint reads the project's own config files, such as `.golangci.yml` and `.golangci.toml`; an optional `lint.args` setting adds arguments.
3. Issues SHALL become diagnostics in the editor and in Problems, with source `golangci-lint (<linter>)`, merged with gopls diagnostics. Each run replaces the previous lint diagnostics of the packages it covered. Issues with a suggested replacement SHALL offer **Apply golangci-lint fix**.
4. Both v1 (`--out-format json`) and v2 (`--output.json.path stdout`) output SHALL be supported, detected from `golangci-lint version`. A configuration or run error SHALL show as a notification with the tool's message.

## R12. go.mod / go.work editor (F-MOD-1)
1. `go.mod`, `go.work` and `go.sum` SHALL be highlighted, and gopls diagnostics and hover SHALL keep working.
2. In `require`, `replace` and `exclude` lines, completion SHALL offer:
   - module paths: modules already in the module cache and `go.sum`, plus the current requires;
   - versions for the module on the line: `@v/list` from the GOPROXY in `go env` (with proxy.golang.org as fallback, and none when `GOPROXY=off`), newest first, with pseudo-versions excluded unless typed.
   - Requests SHALL be cached for 10 minutes and time out after 5 s.
3. Module paths SHALL be links (Mod-click or hover): they open `https://pkg.go.dev/<path>@<version>`. In `replace` lines with a local directory, the link opens that directory in the project tree.

## R13. Module actions (F-MOD-2)
**go mod tidy**, **go mod vendor**, **go mod download** (sync), **go work use** (choose directory), **go work sync**, **New go.mod** (`go mod init <path>`, prompting for the module path) and **New go.work** (`go work init` with the project's modules). They SHALL be available from:
- the Tools → Go Modules menu;
- the editor context of go.mod/go.work files, through a toolbar strip above the editor;
- the project tree context menu.

They run in the module's directory, with output in the Run window (a tab per action).

## R14. Integration
1. Commands with keymap bindings (Goi default and VS Code keymaps); a **Git** menu and **Tools** entries in the native menu bar; settings: `[git]` (`path`, `pullRebase`, `autoFetchMinutes` default 0) and `[lint]` (`enabled`, `onSave`, `golangciPath`, `args`).
2. Theme tokens SHALL cover VCS colors (added, modified, deleted, untracked, ignored, conflict) and diff backgrounds.

## R15. Installers and release (v0.1)
1. **macOS**: a universal (arm64 + amd64) `Goi.app` packaged as `Goi-<version>-macos-universal.dmg` (drag to Applications) and a `.zip`.
2. **Windows**: an NSIS installer `Goi-<version>-windows-amd64-installer.exe` (Start menu, uninstaller, file associations from `wails.json`) and a portable `.zip`. arm64 builds a portable `.zip`.
3. **Linux** (amd64): `.deb` and `.rpm` packages (desktop entry, icon, dependency on WebKitGTK 4.1) and a `.tar.gz`.
4. A **release workflow** SHALL run on tags `v*`. It builds all packages, generates SHA-256 checksums and publishes a GitHub Release with them. It can also be started manually and then produces artifacts only. CI on `main` SHALL keep uploading the raw builds.
5. Signing and notarization SHALL run when their secrets are present (`MACOS_CERTIFICATE`…, `WINDOWS_CERTIFICATE`…) and be skipped otherwise. The README SHALL explain how to open unsigned builds.
6. The version SHALL come from one place (`wails.json` productVersion, checked against the tag) and appear in About and in the package names.

## R16. Non-functional
1. Git calls SHALL never block the UI. Status of a 10k-file repository SHALL stay under 500 ms (porcelain v2 plus untracked cache).
2. All network operations SHALL be cancellable (Stop in the Git console).
3. Domain logic SHALL live in UI-agnostic packages (`internal/git`, `internal/lint`, `internal/gomod`), with `internal/rpc` as a thin bridge.
