# M4 — Git + Lint + Modules: Design

## 1. Architecture

```
frontend/src/git/*   ──(api: git.*)──►  rpc/git.go (GitService, gitSession)  ──► internal/git (CLI wrapper, parsers, patches)
  status store, tree    ◄── git.status / git.output / git.op ──┘                   └─► internal/process (network ops)
  decorations, gutter,
  commit panel, diff, branches, log, blame, merge editor, stash
frontend/src/lint/*  ──(api: lint.*)──► rpc/lint.go (LintService) ──► internal/lint (resolve via toolbin, run, parse v1/v2)
  diagnostics source   ◄── lint.diagnostics / lint.status
frontend/src/mod/*   ──(api: mod.*)───► rpc/mod.go (ModService)   ──► internal/gomod (proxy client, module-cache index, go.sum)
  go.mod completion, links, actions (actions run as Run-window tabs through RunService.RunConfig, type "shell")
.github/workflows/release.yml + build/package/* (dmg, nsis, nfpm)
```

## 2. internal/git
- `Repo{Root, Bin string, Env []string}`, built with `Open(dir, bin)` (`rev-parse --show-toplevel`; returns `ErrNotRepo` outside a repository).
- **Running git:**
  - `run(ctx, stdin, args...) ([]byte, error)` always adds `-c core.quotepath=off -c color.ui=never` and runs with `GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0` for status, and `LC_ALL=C` so messages can be parsed.
  - Errors are `*Error{Args, Stderr, Code}`, carrying git's message.
- **Status:** `Status(ctx) (Status, error)` parses porcelain v2 `-z --branch --untracked-files=all`.
  - `Status{Branch, Upstream, Head, Ahead, Behind, Detached, Files []FileStatus, Op string}`.
  - `FileStatus{Path (abs), Orig, Index, Work byte, Conflict bool, Untracked, Ignored bool}`.
  - `Op` (merge, rebase, cherry-pick, revert) is detected from the git dir (`MERGE_HEAD`, `rebase-merge/`, `rebase-apply/`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`); `GitDir()` comes from `rev-parse --absolute-git-dir`.
- **Reading contents:** `Show(ctx, rev, path)` gives the content at a revision, where `:0`, `:1`, `:2`, `:3` are index stages.
- **Diffs:**
  - `Diff(ctx, DiffSpec{Path, Staged, Rev})` returns unified text.
  - `ParseDiff(text) []FileDiff` gives `FileDiff{OldPath, NewPath, Hunks []Hunk}`, with `Hunk{OldStart, OldLines, NewStart, NewLines, Lines []string}`.
  - `HunkPatch(fd, hunks)` rebuilds a patch for chosen hunks.
- **Partial staging:** `ApplyPatch(ctx, patch, cached, reverse)` runs `git apply --cached [--reverse] --recount -`.
- **Files and commits:**
  - `Stage(paths)` (`add -A --`), `Unstage(paths)` (`restore --staged --`, or `rm --cached` before the first commit), `Discard(paths)` (`restore --worktree --source=HEAD` for tracked files; untracked files are trashed by rpc through `vfs.Trash`).
  - `Commit(msg, amend)` (message via `-F -`); `LastMessage()`.
- **Branches:** `Branches() []Branch{Name, Remote bool, Current, Upstream, Hash, Subject}` (for-each-ref), `Checkout(ref, newBranch, track)`, `CreateBranch`, `RenameBranch`, `DeleteBranch(force)`, `Merge(ref)`, `Rebase(onto)`.
- **Other operations:** `CherryPick(hash)`, `Revert(hash)`, `Reset(hash, mode)`, `Continue/Skip/Abort(op)`.
- **Stash:** `Stash(msg, untracked)`, `Stashes()`, `StashApply/Pop/Drop(i)`.
- **History:** `Log(LogQuery{Path, Skip, Max, Grep}) []Commit{Hash, Short, Parents, Author, Email, Date (RFC 3339), Subject, Body}` with a record separator format, and `CommitFiles(hash) []ChangedFile{Path, Status}`.
- **Blame:** `Blame(ctx, path, contents) []BlameLine{Line, Hash, Author, Date, Summary}` (porcelain; contents via `--contents -`).
- **Network:** `Fetch/Pull/Push` return a `*exec.Cmd` spec, which rpc runs through `internal/process`, streaming output.
  - `Push` adds `-u origin <branch>` when there is no upstream.
  - A rejected push is detected from stderr (`[rejected]` / `non-fast-forward`) into `ErrRejected`.
- **Setup and ignores:** `Init(dir)`, `AddToGitignore(paths)` (appends repo-relative lines).
- **Conflict blocks:** `Conflicts(text) []Block{Start, Mid, End, Base?, OursLabel, TheirsLabel}` parses conflict markers (diff3 style supported). It is pure, so the merge editor's logic can be tested in Go; the frontend has a TS mirror for live editing.

## 3. internal/lint
- `Resolve` via toolbin (`Name: "golangci-lint"`). `Version(ctx, bin)` parses `has version X.Y.Z`.
- `Args(v, extra, targets)` builds the run arguments:
  - v2: `run --output.json.path stdout --output.text.path stderr --show-stats=false --issues-exit-code=0 <targets>`;
  - v1: `run --out-format json --issues-exit-code=0 --print-issued-lines=false`.
- `Parse(out, baseDir) ([]Issue, error)` reads `{Issues:[{FromLinter, Text, Severity, Pos{Filename, Line, Column}, Replacement{NeedOnlyDelete, NewLines, Inline{StartCol, Length, NewString}}}]}` into `Issue{Path abs, Line, Col, Linter, Message, Severity, Fix *Fix}`.
- `Run(ctx, Spec{Bin, Dir, Env, Targets, Extra})` returns issues plus the packages (directories) covered, so the frontend can clear stale diagnostics. A config error (non-zero exit with no JSON) becomes an error carrying stderr.

## 4. internal/gomod
- **Versions:** `Proxy{URLs []string, Client}` built from GOPROXY (comma or pipe list; `direct` and `off` skipped). `Versions(ctx, module)` fetches `<proxy>/<escaped>/@v/list`, sorted with `golang.org/x/mod/semver` (newest first), and falls back to `@latest` when the list is empty. Results are cached for 10 minutes.
- **Module paths:** `KnownModules(modcache, gosum string) []string` walks `$GOMODCACHE/cache/download` (unescaping `!x` → `X`, stopping at `@v` dirs) and adds the paths in `go.sum`. Cached, with a 5 s budget.
- `ModuleAt(line string) (path, version string)` parses one go.mod line, for links and version completion.

## 5. rpc
- **`gitSession`** (per session): repo plus status cache.
  - It refreshes on watcher batches (500 ms debounce; `.git/index`, `HEAD`, `refs` and work-tree changes) and after every mutating call.
  - Events: `git.status` (Status DTO with repo-relative and absolute paths), `git.output {opId, chunks}` and `git.op {opId, title, state, error}`.
  - **`GitService`** is thin and maps one-to-one onto the `internal/git` operations. Network operations return an `opId` immediately and can be stopped with `StopOp`. `Init()` creates a repository.
- **`LintService`**:
  - `Status()` reports `{bin, version, found}`. `Install()` uses toolbin and reports progress as `lint.status`.
  - `Run(scope "package"|"project", path)` lints and emits `lint.diagnostics {dirs []string, files map[path][]Diagnostic}`; LSP-shaped diagnostics are reused, so the frontend renders them like gopls's.
  - On save, the frontend calls `Run("package", path)`, debounced per package and cancelling a previous run of the same package.
- **`ModService`**:
  - `Versions(module)` and `Modules(prefix)` back completion.
  - `Init(dir, path)` runs `go mod init`.
  - The other actions run as Run-window tabs: the frontend builds a `shell` RunConfig whose script is the go command, using `${GO}` (the SDK binary that runconfig substitutes) in `dir`.

## 6. Frontend
- **`git/state.ts`:** `gitStatus` signal and helpers (`statusOf(path)`, `dirHasChanges`).
- **`git/decorations.ts`:** a project-tree decorator hook. `views/projectTree.ts` gets `treeDecorators`, mirroring `treeMenuContributors`: each returns a class for a row, and the tree re-renders when `decorationsVersion` changes.
- **`git/gutter.ts`:** change markers.
  - Base content is `git.show(':0', path)`, re-fetched on status changes; the diff uses `@codemirror/merge`'s `diff()`.
  - It is a gutter with added/modified/deleted markers; clicking one opens a popup with the old text plus Rollback, Stage Hunk and Show Diff.
  - Stage Hunk applies a patch built in TS from the chunk.
- **`git/commitPanel.ts`:** the left-dock panel and the modal variant. `git/diffView.ts` is the diff editor tab:
  - It is a pseudo-buffer in the editor area. Its "virtual tabs" render a MergeView or a unified merge view, and opening a diff calls `editors.openVirtual`. I'll check whether the editor manager supports virtual tabs; if it doesn't, diffs open in a large dialog.
  - Per-chunk Stage / Unstage / Revert controls: the chunk becomes a patch on the TS side (`git/patch.ts`, pure, tested), sent to `ApplyPatch`.
- **Other git modules:**
  - `git/branches.ts` (popup), `git/log.ts` (tool window, bottom), `git/blame.ts` (gutter annotations), `git/merge.ts` (3-way merge editor in a large dialog, with blocks via `git/conflicts.ts`, pure), `git/stash.ts`;
  - `git/console.ts`: a "Git" tab in the Run window area that reuses `Console`;
  - `git/install.ts`: commands, menus and the status widget.
- **`lint/lint.ts`:**
  - `lang/diagnostics.ts` becomes multi-source: `setDiagnosticsSource(source, path, items)`, with `problems` holding the merged view.
  - Lint diagnostics carry `source: 'golangci-lint (errcheck)'`. A save hook triggers package runs, and quick fixes come from `Fix` data through the existing code-action UI (`lang/codeActions.ts`), which gets a provider hook for extra actions.
- **`mod/gomod.ts`:**
  - completion source for go.mod/go.work (path and version contexts) added through `editors.addExtension`;
  - Mod-click and hover links;
  - a module-actions toolbar panel above go.mod/go.work editors, plus commands.

## 7. Packaging
- **`build/package/macos/`:** `make-dmg.sh` (hdiutil UDZO with an `/Applications` symlink) and a zip made with `ditto`.
- **`build/package/linux/nfpm.yaml`:** deb and rpm, `/usr/bin/goi`, desktop file `goi.desktop`, icon `/usr/share/icons/hicolor/512x512/apps/goi.png`, and `depends: libgtk-3-0, libwebkit2gtk-4.1-0` (rpm: `gtk3, webkit2gtk4.1`).
- **`build/windows/installer`:** Wails' NSIS template; `wails build -nsis` (CI installs NSIS with choco).
- **`scripts/version.sh`:** reads productVersion from `wails.json`. The release job fails if the tag doesn't match `v<version>`.
- **`.github/workflows/release.yml`:**
  - triggers: tags `v*` and `workflow_dispatch`;
  - jobs: macos (universal, dmg+zip), windows (amd64 installer+zip, arm64 zip), linux (deb, rpm, tar.gz);
  - then `release`: `sha256sum` and `softprops/action-gh-release`, on tags only;
  - optional signing steps are gated on secrets.
- **README:** a section on installing unsigned builds.

## 8. Testing
- **Go:**
  - `git` against real git in temp repos: status codes, rename, conflicts and op detection, hunk patch apply and partial staging round trip, branches, stash, log parsing, blame, push to a local bare remote, rejected-push detection;
  - `lint` parse fixtures for v1 and v2, plus a real run when `GOI_GOLANGCI` is set;
  - `gomod` with a proxy served by `httptest`, escaping, and the known-modules walk;
  - rpc events.
- **Frontend:** vitest for the patch builder, conflict blocks, gutter chunk mapping, status decoration mapping, go.mod line contexts, and multi-source diagnostics.
- **E2E:** `tools/e2e/m4.json` on `tools/e2e/m4-sample.sh`, a repo with a bare remote, a branch with a conflict, a lint issue and go.mod. It covers the status tree, gutter, partial staging, commit, push, the branch popup, the merge editor resolution, blame, lint diagnostics, go.mod completion and tidy.
- **Packaging:** run the release workflow with `workflow_dispatch` on the branch and download its artifacts. On macOS, mount the dmg locally and launch the app.
