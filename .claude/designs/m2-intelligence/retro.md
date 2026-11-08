# M2 — Intelligence: Retrospective

Delivered: T1–T16, one commit each, on `feat/m2-intelligence`.
- **Backend:** three new packages (`lsp`, `gopls`, `textsearch`) and `rpc/lang.go`.
- **Frontend:** a 17-module `frontend/src/lang/` feature.
- **Tests:** all 15 Go packages pass (race-checked for lsp, gopls, rpc and textsearch), as do 318 frontend tests.
- **Acceptance:** every M2 requirement was checked on macOS against real gopls v0.24.0 through `wails dev` (`acceptance.md`). Completion p95 is about 101 ms on this repository; the target is 150 ms.

## What went right
- **Own small LSP client.** About 600 lines covering exactly the protocol subset Goi uses, with LSP unions normalized at decode time (markup, locations, completion lists, code actions vs commands, prepare-rename). Every consumer, Go and TypeScript, sees one shape. No third-party LSP dependency.
- **UTF-16 positions end to end.** LSP's default encoding matches JS and CodeMirror offsets, so the frontend converts positions with plain arithmetic. Go only transcodes when rewriting files that aren't open (`gopls.ApplyEdits`, emoji-tested).
- **A fake LSP server by re-executing the test binary** (`GOI_FAKE_LSP=1`). It made crash/restart, give-up, progress, configuration and `applyEdit` deterministic to test. A separate real-gopls test caught integration issues the fake couldn't.
- **The M1 seams held.** Every editor feature went in through `editors.addExtension` and hooks; `editorManager.ts` was never touched. The palette gained symbols through a provider hook, so it doesn't know about gopls.
- **Acceptance against the real app found real bugs** that unit tests missed (below). That's the payoff of running the CDP harness against `wails dev`.

## What went wrong
- **Two production bugs surfaced only in E2E:**
  1. The tooltip `mount()` hook assumed the `TooltipView.dom` is wrapped by CodeMirror; it isn't. That styled the whole editor as a tooltip, with a 360 px max-height.
  2. Re-opening a document gopls already tracked sent a lower-version `didChange`, which gopls rejects, leaving it with stale text after a frontend reload.

  Both are fixed and now covered: a unit test for the second, an E2E assertion for the first. Both were API-semantics mistakes I could have avoided by reading the library contract first.
- **Self-inflicted test noise:** several first-run failures were wrong expectations in my own tests (UTF-16 columns, snippet offsets), plus a racy "status is still ready" check in the give-up test. They cost cycles, not quality.
- **Harness friction:** synthetic Cmd-A doesn't select all in inputs over CDP; one check read the first `.results-head` (the Problems panel) instead of the Find panel; and a multi-file rename's preview dialog swallowed a later step's Enter. All three are fixed in `m2.json` and noted in CLAUDE.md.
- **Scope adjustments:** the design's "DTOs are lsp types" became path-carrying `gopls.Location` / `Symbol` / `FileEdit` (recorded in design.md). Breadcrumbs went into an editor bottom panel rather than a separate navbar. Go to Implementation uses gopls's two-way `implementation`; there's no separate "super-interface" action.
- **Not verified:** Windows and Linux runs, "Install gopls" end to end, and native-window (WKWebView) interaction beyond browser-mode E2E.

## Action items
1. **Read the contract before using a library hook.** For each CodeMirror or LSP API a module depends on (tooltip DOM ownership, document versioning), add a one-line note or test asserting the assumption in the same task, not at acceptance.
2. **Make the E2E harness part of every milestone's task list from T1.** Run `tools/e2e/m2.json`, and its M3 successor, on Linux in CI under Xvfb with gopls installed. That also covers the Windows/Linux gap that M1's retro already flagged.
