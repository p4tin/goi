# Wails v3: one process, many windows — Retro

## What went right

- **Spikes first.** A 150-line self-checking v3 app (it logged its own assertions and quit) confirmed per-window calls and events, `SingleInstance`, close/quit cancellation and the Dock-menu hook in one run, before any refactor. It also turned up the three facts the design had wrong: `SecondInstanceData.Args` includes argv[0]; server mode broadcasts all events; a browser tab has no window until its WebSocket connects.
- **Refactoring on v2 before switching.** Moving per-window state behind a `window` record (T2) while still on Wails v2 kept the v3 switch (T5) to the bridge itself, and the existing rpc test suite guarded both steps.
- **`internal/rpc` as the only bridge paid off.** Wails was imported in 7 files, so the port touched the shell, bus, menu, dialogs and clipboard, and the domain packages didn't change at all (apart from the deleted `internal/instance`).
- **Waves with file ownership.** The four Wave 3 routing tasks ran in parallel worktrees and merged without conflicts, because each owned a disjoint file set and the shared output (bindings) was regenerated once after the merge.
- **The multi-window E2E found a real bug** that unit tests couldn't: the "only window" fallback in `windowFor` answered a second tab's first call for the first window.

## What went wrong

- **Worktree agents started from `main`, not the feature branch.** Each agent had to `git reset --hard feat/wails-v3` first, and changes were cherry-picked back. It worked, but every prompt had to carry that instruction.
- **The `_windows_test.go` trap.** I told three agents to create `*_windows_test.go` files. Go treats that as a GOOS constraint, so those tests silently don't run on macOS or Linux. Two agents caught it and one file had to be renamed after the merge. The rule is now in CLAUDE.md.
- **Generated-type drift.** The v3 generator types every slice and map as nullable and turns Go string constants into TS enums. Fixed in `lib/api.ts` with narrowing helpers and aliases rather than in feature code, but it cost a round of type errors.
- **Scope folding.** T8 (open/focus/handoff) and most of T9 (close/quit/reopen) landed inside T5, because the v3 shell needed them to be coherent. The task list's wave boundaries were cleaner on paper than in the code.
- **No v2 performance baseline.** R8.5 compares against nothing measured; only v3 numbers are recorded.
- **E2E specs depended on v2 internals** (`window.go.rpc…` globals) and on the sample's directory name and location (`/tmp` vs `/private/tmp`). They pass again with a compatibility global and a real path, but they're more brittle than they look.
- **Not done / follow-ups:**
  - `recoverSession` (R3.7) wasn't added; isolation relies on Wails' per-call panic recovery and gopls' restart backoff.
  - A hung, as opposed to dead, primary process isn't detected by `SingleInstance`.
  - Windows, Linux and the release workflow dry run still need real runs.

## Action items

1. **Spec template:** worktree agents get "reset to the feature branch" in a shared preamble, and test files for multi-window or multi-anything use a `_multi*` suffix, never a GOOS name.
2. **Before a migration, capture baselines:** the E2E suite output and startup timings on the old stack, saved in the design folder, so regressions and the performance requirement can be checked against something real.
