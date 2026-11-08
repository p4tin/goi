# M4 — Git + Lint + Modules (v0.1): Retrospective

Delivered: T1–T15 on `feat/m4-git-lint-modules`.
- **Backend:** three packages (`git`, `lint`, `gomod`) and three rpc services (`GitService`, `LintService`, `ModService`).
- **Frontend:** `git/` (status, tree colors, gutter, commit panel, diff, branches, stash, log, blame, merge editor, console), `lint/` and `mod/`.
- **Release:** packaging scripts and a release workflow.
- **Tests:** all Go packages pass with `-race`, with real git, Delve and golangci-lint; 399 frontend tests pass.
- **Acceptance:** 43 E2E steps pass in one run on a fresh sample.

Before starting M4, I found and fixed CI failures I had missed after the M2 merge: a Windows URI bug, a flaky test, and a Windows `.exe` expectation. `main` is green on macOS, Windows and Linux.

## What went right
- **The git CLI instead of a library.** It uses the user's credentials, SSH agent, hooks and configuration for free. Porcelain v2 plus `-z` makes status parsing robust (renames, spaces, conflicts). Every `internal/git` test runs real git in temp repos, including a bare remote.
- **Hunk patches proven against git.** The TypeScript range-patch builder is tested by applying its patches with `git apply --cached` (six edge cases). Partial staging worked first time in E2E.
- **Small hooks instead of edits to core modules:**
  - `treeDecorators` (tree colors);
  - `extraCompletionSources` (go.mod);
  - `actionProviders` (lint quick fixes);
  - multi-source diagnostics (lint plus gopls);
  - close checks.

  Each is a few lines in the owning module, and `editorManager.ts` stayed untouched.
- **Probing golangci-lint's real JSON before writing the parser** showed that v2 sends `SuggestedFixes` with byte offsets and base64 text, not v1's `Replacement`. Both are normalized in Go.
- **Packaging checked locally where possible:** a real universal build, dmg mount and launch, nfpm `.deb`/`.rpm` inspection, and actionlint.

## What went wrong
- **I didn't check CI after the M2 merge.** It had been red for a day on Windows (URI test) and macOS (flaky test). That was only found when I looked at the M3 merge run.
- **UI layering bugs only visible in screenshots:** CodeMirror panels (z 300) drew over dialogs, and the Commit panel became the left dock's default tab through registration order. Neither showed in unit tests.
- **Merge commits** looked empty in the log because `git show` gives merges a combined diff. Found in E2E.
- **E2E friction:** dock tabs switched by the app (the conflict notification opens Commit) broke later tree lookups. Helpers now select the tab they need.

## Action items
1. **Watch CI after every merge to `main`** (`gh run watch`) before starting the next milestone. Add it to the workflow checklist in CLAUDE.md.
2. **Take an E2E screenshot of every new overlay** (dialogs, popups, overlays) with an editor open behind it, and give overlay z-indexes one documented scale: editor panels 300, overlays 320–410.
