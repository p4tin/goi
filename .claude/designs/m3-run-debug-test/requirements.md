# M3 — Run / Test / Debug: Requirements

Scope (SPEC.md §6 M3): run configurations (§3.7: Go Build/Application, Go Test, Go Remote, plus Shell script and Makefile target), the Run tool window (F-RUN-1), configuration storage (F-RUN-2), the test runner (F-TST-1, F-TST-2), the debugger (F-DBG-1..5, F-DBG-10) and the terminal (F-TL-1).
*Accept*: debug a test with a goroutine filter and a conditional breakpoint; rerun a failed subtest.

Out of scope, by SPEC priority:
- The Coverage, Profile and Record & Replay runners (F-TST-4, F-PRF-1, F-DBG-8).
- Docker and HTTP request configurations (v0.2/v0.3) and the Services view (F-RUN-3).
- Fuzz corpus UI (F-TST-3). Fuzz mode is still a test-config option.
- Auto-test (F-TST-5), attach by PID (F-DBG-6), core dumps (F-DBG-7) and disassembly (F-DBG-9).
- "Run with sudo": Goi never escalates privileges itself (NF-5).

Behaviour follows the documented behaviour and default shortcuts of mainstream Go IDEs, with Goi's own wording, look and command ids.

## R1. Run configurations (§3.7, F-RUN-2)
1. A run configuration has a name, a type (`app`, `test`, `remote`, `shell`, `make`), a storage location, and type-specific options:
   - **Go Build (`app`)**
     - Kind: `package`, `file` or `directory`.
     - Package import path or directory, or files.
     - Working directory, program arguments, environment variables, and a `.env` file.
     - `go build` flags, build tags and output path.
     - "Run after build" (off = build only) and "Redirect input from" a file.
   - **Go Test (`test`)**
     - Kind: `directory`, `package`, `file` or `function` (`-run` pattern).
     - Working directory, environment and `.env`.
     - Flags: `-race`, `-count`, `-short`, `-timeout`, tags and extra flags.
     - Benchmarks (`-bench` pattern, `-benchmem`) and fuzzing (`-fuzz`, `-fuzztime`).
   - **Go Remote (`remote`)**: host and port of a `dlv --headless --listen` server.
   - **Shell script (`shell`)**: a script file or inline text, interpreter, arguments, working directory and environment.
   - **Makefile (`make`)**: Makefile path, target(s), arguments and working directory.
   - **Before launch (all types)**: an ordered list of tasks. Each is another configuration, `go generate` (package), or an external command. Any task failing SHALL abort the launch.
2. Shared configurations SHALL be stored as `.goi/runConfigurations/<name>.toml` in the project (shareable through VCS). Local ones SHALL be stored in the user config dir, keyed by workspace hash. Both SHALL load when the project opens and reload when their files change on disk.
3. WHEN a run or debug starts from a gutter marker or context (R4) THEN Goi SHALL create a **temporary** configuration and select it. At most 5 temporary configurations SHALL be kept (oldest dropped), and **Save Configuration** SHALL make one permanent.
4. A **Run/Debug Configurations** dialog SHALL let the user add, copy, delete, rename and edit configurations, move them between shared and local, and validate them. Invalid configurations SHALL show their errors and SHALL NOT launch.
5. A run widget SHALL show the selected configuration with **Run**, **Debug** and **Stop**, and a dropdown to choose a configuration or edit configurations.

## R2. Running programs (§3.7 runners, F-RUN-1)
1. **Run** on an `app` configuration SHALL build with the project SDK, environment, tags and flags into a temp binary (or the output path), then start that binary in the working directory (default: package dir, or project root for files), with the arguments, environment (`.env` first, then explicit variables) and stdin.
2. The **Run** tool window SHALL show one tab per launch, named after the configuration. Each tab SHALL have:
   - output with ANSI colors (SGR 16/256/true-color, bold, italic, underline) and stderr distinguished;
   - a line typed into its input SHALL go to the process's stdin;
   - clickable `file.go:line[:col]` locations, panic and goroutine stack traces included, opening the file at that line;
   - Rerun, Stop, Clear, soft-wrap toggle, scroll-to-end and search in output;
   - an exit line: `Process finished with exit code N` (or the signal), with the elapsed time.
3. **Stop** SHALL send SIGINT (Windows: terminate) to the process group, then SIGKILL after 5 s or on a second Stop.
4. **Rerun** SHALL stop the tab's process if it is running and launch the same configuration in the same tab.
5. Output SHALL be capped per tab (default 50,000 lines, oldest dropped) so long-running programs don't exhaust memory.

## R3. Test runner (F-TST-1, F-TST-2)
1. A `test` configuration SHALL run `go test -json` with its flags and stream the events into a **test tree** (package → test → subtests). Each node SHALL show its status (running, passed, failed, skipped), its duration and its own output, and a summary (`Tests passed: 12, failed: 1, skipped: 2`) SHALL head the tree. Build failures SHALL show as a failed package with the compiler output, its locations clickable.
2. Selecting a node SHALL show that node's output, and double-clicking SHALL open the test function. A **Hide passed** toggle SHALL filter the tree.
3. **Rerun Failed Tests** SHALL rerun only the failed leaf tests in their packages, building a `-run` pattern with per-level anchored, regex-quoted names. **Run** / **Debug** from a node's context menu SHALL rerun that test or subtest alone.
4. WHEN a failed test's output contains a testify-style `expected:` / `actual:` pair THEN the node SHALL offer **Show Difference**, opening a side-by-side diff.
5. Benchmark results (`BenchmarkX-N  iterations  ns/op  B/op  allocs/op` plus custom metrics) SHALL appear in a **Benchmarks** table. Each row SHALL show the change against the previous run of the same benchmark in this project (kept across restarts).

## R4. Run markers and context (F-DBG-10)
1. Clicking a gutter run marker SHALL offer **Run** and **Debug**, plus **Modify Run Configuration…**, for `main`, `Test…`, `Benchmark…`, `Fuzz…` and `Example…` functions. Tests and benchmarks run with `-run` / `-bench` anchored to that function.
2. **Run Context Configuration** (Ctrl-Shift-R on macOS, Ctrl-Shift-F10 elsewhere) and **Debug Context Configuration** (Ctrl-Shift-D on macOS, Ctrl-Alt-Shift-F9 elsewhere) SHALL run or debug the function at the cursor:
   - the test or benchmark at the cursor;
   - else the file's tests (in a `_test.go` file);
   - else the package (for `package main`).
3. The project tree context menu SHALL offer **Run Tests in '<dir>'** for directories containing Go test files.

## R5. Debugger session (DAP → `dlv dap`)
1. Delve SHALL be resolved like gopls: the `debug.dlvPath` setting → `$PATH` → `$GOBIN` / `$(go env GOPATH)/bin` → `<config>/tools/bin`. IF it is missing THEN **Debug** SHALL offer **Install Delve** (`go install github.com/go-delve/delve/cmd/dlv@latest` with `GOBIN=<config>/tools/bin`) and continue after installation.
2. **Debug** on an `app` configuration SHALL launch `dlv dap` with mode `debug` (the package, tags and build flags), the working dir, args and env. A `test` configuration SHALL use mode `test`, with test flags passed as `-test.*` arguments. A `remote` configuration SHALL attach to the host:port.
3. The **Debug** tool window SHALL show one tab per session with:
   - a toolbar (Resume, Pause, Step Over, Step Into, Force Step Into, Step Out, Run to Cursor, Restart, Stop);
   - a goroutines and frames view, variables, watches, and a console with program output and the debugger log.
4. WHEN the program stops (breakpoint, step, pause, panic) THEN the UI SHALL:
   - select the stopped goroutine and its top frame;
   - open the source line and highlight it in the editor;
   - load the variables;
   - refresh watches and inline values.
5. WHEN the program exits or the session ends THEN the tab SHALL show the exit code and the editor highlight SHALL clear.

## R6. Breakpoints (F-DBG-1)
1. Clicking the line-number gutter of a Go file SHALL toggle a line breakpoint, as SHALL **Toggle Breakpoint** (Mod-F8). Breakpoints SHALL follow edits, persist per project, and be sent to every running session when changed.
2. A breakpoint SHALL support a condition, a hit count (`hitCondition`, e.g. `>= 3`, `% 2`, or `n`, meaning `== n`), a log message (logpoint, `{expr}` interpolated, no suspension) and enabled/disabled.
   - Right-click or Shift-click on a breakpoint SHALL open its editor.
   - Conditional, logpoint, disabled and unverified breakpoints SHALL look distinct.
3. **View Breakpoints** (Mod-Shift-F8) SHALL list all breakpoints with enable toggles and editing. It SHALL also manage **function breakpoints** (by name, with an optional condition) and **Break on panic**, which stops at every `runtime.gopanic`, including recovered panics. Unrecovered panics and fatal errors always stop.
4. Breakpoints that the debugger reports as unverified (e.g. on a line without code) SHALL show an error tooltip with the reason.

## R7. Stepping (F-DBG-2)
1. Step Over (F8), Step Into (F7), Force Step Into (Alt-Shift-F7), Step Out (Shift-F8), Resume (Alt-Mod-R on macOS, F9 elsewhere), Pause and **Run to Cursor** (Alt-F9) SHALL act on the selected goroutine. Run to Cursor runs to the line with the cursor, ignoring other breakpoints on the way only in that they still stop.
2. **Set Next Statement** SHALL be offered only when the debugger supports it (DAP `supportsGotoTargetsRequest`). Delve does not, so the command shall explain that.
3. Force Step Into is the same as Step Into with Delve: Delve has no "skip library code" stepping to bypass.

## R8. Goroutines and frames (F-DBG-3)
1. The goroutines view SHALL list all goroutines. Each row shows the id, the current function and location, and the label/state as reported. Selecting a goroutine SHALL load its frames, and selecting a frame SHALL load its variables and open its source.
2. Goroutines SHALL be groupable by their top user function (**Group by function**), with counts, or shown as a flat list.
3. A **filter** box SHALL show only goroutines whose id, function, location or label matches the text. **Hide** a goroutine (or a whole group) SHALL remove it from the view until **Reset hidden**.
4. A **Show thread IDs** toggle SHALL show the OS thread a goroutine is running on, when reported.

## R9. Variables, watches, evaluation (F-DBG-4, F-DBG-5)
1. The variables tree SHALL load children lazily (`variablesReference`) and SHALL show the locals and arguments of the selected frame, with name, type and value. Options:
   - **Show types**;
   - **Show pointer addresses** (off strips `0x…` addresses from values);
   - **Show unreadable variables** (off hides values Delve marks unreadable).
2. **View as String** on a `[]byte`, `[]uint8` or `[N]byte` variable SHALL evaluate `string(x)` and show the result. **Copy Value** SHALL copy the value.
3. **Set Value** (F2 or double-click on a value) SHALL change a variable through `setVariable` and refresh the tree.
4. **Evaluate Expression** (Alt-F8) SHALL evaluate an expression in the selected frame, prefilled with the selection, and show the result as an expandable tree. **Add to Watches** SHALL keep it in the watch list.
5. **Watches** SHALL be re-evaluated on every stop, persist per project, and be editable and removable.
6. WHEN stopped THEN the editor SHALL show **inline values** (`name = value`, truncated to 60 chars) after the lines of the stopped function that mention a local or argument, up to the current line. They SHALL clear on resume.

## R10. Terminal (F-TL-1)
1. A **Terminal** tool window (Alt-F12) SHALL run the user's shell in a PTY in the project root:
   - shell: the `terminal.shell` setting, else `$SHELL` (Unix), else `pwsh` / `powershell.exe` / `cmd.exe` (Windows);
   - environment: the project environment, with the SDK's `bin` first on PATH, and `TERM=xterm-256color`.
2. The terminal SHALL support multiple tabs (new, close, rename), resize with the panel, copy/paste and search, and SHALL use theme colors and the editor font.
3. **Shell integration** (`terminal.shellIntegration`, default on) for zsh and bash SHALL load the user's rc files, then:
   - report the current directory (OSC 7), so relative paths resolve;
   - add `go` subcommand and flag completion.
4. `file.go:line[:col]` paths, absolute or relative to the current directory, SHALL be clickable and open in the editor. URLs SHALL open in the browser.
5. WHEN the shell exits THEN its tab SHALL show `[Process exited with code N]` and SHALL close on the next key press.

## R11. Integration
1. Every action SHALL be a command with keymap bindings (Goi default and VS Code keymaps) and entries in the native **Run** menu:
   - Run, Debug, Stop, Run/Debug Context Configuration, Edit Configurations, Save Configuration, Rerun, Rerun Failed Tests;
   - stepping commands, Toggle Breakpoint, View Breakpoints, Evaluate Expression;
   - and in **View**: Run, Debug, Terminal tool windows.
2. Settings SHALL add a `[debug]` section (`dlvPath`, `showTypes`, `showAddresses`, `showUnreadable`, `inlineValues`) and a `[terminal]` section (`shell`, `shellIntegration`, `fontSize`, `scrollback`), plus `run.outputLimitLines`, all editable in the settings UI.
3. Closing the project or window SHALL stop all runs, debug sessions and terminals. IF processes are running THEN closing SHALL ask for confirmation.

## R12. Non-functional
1. Output and terminal data SHALL be batched (≤ 30 ms or 64 KB per event) and SHALL NOT freeze the UI at 10 MB/s of output (NF-3).
2. Bound DTOs SHALL be JSON-native, with no `time.Time`, and SHALL return non-nil slices. Paths SHALL cross the bridge as OS paths. DAP line and column numbers SHALL be 1-based on both sides (`linesStartAt1: true`).
3. Domain logic SHALL live in UI-agnostic packages (`internal/dap`, `internal/debugger`, `internal/runconfig`, `internal/process`, `internal/terminal`). `internal/rpc` SHALL stay a thin bridge.
4. All processes SHALL run with the user's privileges. Configurations from a project's `.goi/` SHALL NOT run until the user starts them (NF-5).
