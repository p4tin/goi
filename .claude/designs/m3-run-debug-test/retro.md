# M3 — Run / Test / Debug: Retrospective

Delivered: T1–T16 on `feat/m3-run-debug-test`.
- **Backend:** six new packages (`toolbin`, `runconfig`, `process`, `dap`, `debugger`, `terminal`) and three rpc services (`RunService`, `DebugService`, `TerminalService`).
- **Frontend:** `run/`, `debug/` and `terminal/` features.
- **Tests:** all Go packages pass with `-race`, with real Delve, and 374 frontend tests pass.
- **Acceptance:** both milestone criteria and every requirement were checked end to end against real `go`, Delve and a PTY (`acceptance.md`, 66 E2E steps).

## What went right
- **Probing the real tool before designing.** A 60-line Node DAP probe against `dlv dap` settled several things up front:
  - the listen-line format and capabilities;
  - goroutine naming and output routing (`outputMode: remote`);
  - that Delve drops `__debug_bin` into the project unless `output` is set.

  The design was right first time on those points.
- **Small own protocol client again.** `internal/dap` has the same shape as `internal/lsp`, plus an ordered event queue. Handlers can call back into the connection without deadlocking the reader. Fake-adapter tests use `net.Pipe`.
- **Real-tool integration tests.** `debugger` and rpc tests drive real Delve, with `GOI_DLV` pointing at a binary, through conditional and hit-count breakpoints, goroutines, set variable, run to cursor and remote attach. They found four real problems before any UI existed: symlinked paths, missing build errors, sessions not ending, and test-mode subtest naming.
- **The run id doubles as the debug session id.** Before-launch tasks, the output pipeline, stop and rerun are shared between Run and Debug, with no second lifecycle.
- **Installing Delve through the UI** was exercised for real in E2E. Delve landed in the isolated `GOI_HOME` tools folder, not on the developer's machine.

## What went wrong
- **Two UI bugs only E2E caught:**
  - Tool windows subscribed to events on first render, so the first run ever had no tab.
  - Line-number clicks never reached a gutter handler: CodeMirror's `domEventHandlers` listen on the content DOM only.

  Unit tests couldn't see either, because they constructed the pieces directly. Both are fixed, and the second has a regression test.
- **Harness friction:**
  - CDP `insertText` with `\r` doesn't press Enter in xterm.
  - The shell needs a moment after its prompt before input.
  - A killed run left a headless Chrome holding the debugging port.
  - Breakpoints and watches persist per project, so steps must be idempotent.
- **Library contracts:** go-pty's `Close` isn't concurrency-safe (caught by `-race`). Delve keeps the DAP connection open after `terminated` until the client disconnects.
- **Shell noise:** this machine's zsh `chpwd` hook prints errors on every `cd`. It made command output noisy throughout. It's harmless to Goi, but it's a reminder that user rc files run inside the integrated terminal.

## Action items
1. **Tool windows must subscribe at install, not on render.** Add a lint-style check to the feature checklist in CLAUDE.md, and keep a unit test that starts a run before the panel renders.
2. **Make E2E scripts idempotent from the start.** Clear or tolerate persisted state such as breakpoints, watches and temporary configurations at the top of each script. Kill stray `--remote-debugging-port` Chrome processes before a run (a harness pre-step for M4).
