# M3 — Run / Test / Debug: Design

## 1. Architecture

```
frontend/src/run/*      ──(api: run.*)────►  rpc/run.go (RunService)      ──► internal/runconfig (types, TOML store, plans)
  configs, widget,                                                        ──► internal/process  (process groups, stdin, signals)
  Run window, console,  ◄── run.output / run.state / run.test / run.configs ─┘
  test tree, benchmarks
frontend/src/debug/*    ──(api: debug.*)──►  rpc/debug.go (DebugService)  ──► internal/debugger (dlv dap session) ──► internal/dap ──► dlv dap
  breakpoints, Debug    ◄── debug.event / debug.state ────────────────────┘
  window, inline values
frontend/src/terminal/* ──(api: term.*)───►  rpc/terminal.go (TerminalService) ──► internal/terminal (PTY via aymanbagabas/go-pty)
  xterm.js tabs         ◄── term.output / term.exit ──────────────────────┘
internal/toolbin: shared resolve/install of Go tools (gopls and dlv)
```

- Domain packages have no Wails or `rpc` knowledge. They report through callbacks; `rpc` turns those into `events.Bus` topics.
- `rpc.session` gains `runs *runManager`, `debug *debugManager` and `terms *terminal.Manager`. All are closed with the session.

## 2. internal/toolbin
`gopls/resolve.go`'s logic moves here as a generic implementation. `gopls.Resolve` and `gopls.Install` become thin wrappers, with the same behavior and tests.
```go
type Options struct{ Name, Setting string; LookPath func(string)(string,error); GOBIN, GOPATH, ToolsDir string }
func Resolve(o Options) (string, error)         // ErrNotFound
func Install(ctx, goBin string, env []string, toolsDir, pkg string) (string, error)  // go install pkg@latest with GOBIN
```

## 3. internal/runconfig
```go
type Config struct {
  Name string; Type Type /*app|test|remote|shell|make*/; Shared bool `toml:"-"`; Temporary bool
  Kind string /*package|file|directory|function*/; Package, Directory string; Files []string; Pattern string
  WorkingDir, Args string; Env map[string]string; EnvFile string
  BuildFlags string; Tags []string; Output string; RunAfterBuild *bool; InputFile string
  Race, Short bool; Count int; Timeout, TestFlags, Bench string; Benchmem bool; Fuzz, FuzzTime string
  Host string; Port int
  Script, ScriptText, Interpreter, Makefile string; Targets []string
  BeforeLaunch []Task
}
type Task struct{ Type string /*config|generate|command*/; Config, Package, Command string }
```
- `Store`: `Load(sharedDir, localDir)` reads `*.toml`. Names map to files via a `slug` (non-alnum → `_`), so names are unique case-insensitively. `Save(cfg)` writes atomically, `Delete(name)`, `List()` sorts with permanent configurations first. Temporary configurations are local only and capped at 5 (`AddTemporary`).
- `Validate(cfg, root) []string`: returns readable errors.
- `Plan(cfg, Context{Root, Runner, TempDir}) (Plan, error)` is pure, apart from reading the `.env` file:
  - `app`: `Build *Cmd` (`go build -o bin [tags] [flags] target`), plus `Exec *Cmd` unless build-only.
  - `test`: `Exec` = `go test -json [flags] target [-run …]`, with `Test: true`.
  - `shell`: interpreter + script + args. `make`: `make -f Makefile targets…`.
  - Working directory defaults: `package` → its directory (import paths resolve with `go list -f {{.Dir}}`, done in rpc); `file`/`directory` → that directory; otherwise the project root.
- `LoadEnvFile(path) (map, error)`: `KEY=VAL` lines, `#` comments, optional `export`, single or double quotes, `\n` escapes inside double quotes.
- `SplitArgs(s) ([]string, error)`: shell-like word splitting with quotes and backslashes. It doesn't expand anything.
- `TestRunPattern(names []string) string`: names are `TestA/sub_1`. Each level becomes `^(a|b)$`, with the levels joined by `/` (R3.3). The frontend has the same function in TS for reruns; both are table-tested on the same cases.
- `DebugLaunch(cfg, ctx) (map[string]any, error)`: builds the `dlv dap` launch or attach arguments (R5.2).

## 4. internal/process
```go
type Spec struct{ Path string; Args, Env []string; Dir string; Stdin io.Reader /*nil = pipe*/ }
type Process struct{...}
func Start(spec Spec, out func(stream string, data []byte)) (*Process, error)
func (p *Process) Write(b []byte) error; CloseStdin(); Stop() /*SIGINT → 5 s → SIGKILL; second call kills*/; Kill(); Wait() (ExitInfo)
type ExitInfo struct{ Code int; Signal string; Err string }
```
- Unix: `Setpgid`, and signals go to `-pgid`. Windows: `CREATE_NEW_PROCESS_GROUP`. Stop sends `CTRL_BREAK`, falling back to Kill; Kill uses `taskkill /T /F` semantics via `Process.Kill` on the job.
- Output readers deliver raw chunks. `rpc` batches them into events (`batcher`: flush at 30 ms or 64 KB) and carries partial UTF-8 sequences over to the next chunk (`utf8Carry`).

## 5. rpc/run.go: RunService and runManager
- Bound methods:
  - `Configs() []ConfigDTO`, `SaveConfig(c, oldName)`, `DeleteConfig(name)`, `Validate(c) []string`
  - `ContextConfig(req ContextRequest) ConfigDTO`, which builds a temporary configuration from a gutter or context target: `{Path, Kind: main|test|bench|fuzz|example|file|dir, Name}`
  - `Run(name, debug bool) (runId, error)`, `Rerun(runId)`, `Stop(runId)`, `Input(runId, text)`
  - `RunTest(runId?, pkgPatterns…)`, for reruns from the tree: a temporary test configuration with a pattern
  - `BenchHistory() map`, `SaveBench(results)`
  - `Running() []RunInfo`
- A launch runs its before-launch tasks in order: nested configurations run to completion, `generate` runs `go generate pkg`, and `command` runs through `SplitArgs`. Each task's output goes to the same tab. Then the build, then the exec.
- Events:
  - `run.state` `{runId, name, configName, type, state: building|running|exited|failed, exit?, debug, startedMs}`
  - `run.output` `{runId, stream: stdout|stderr|system, text}`
  - `run.test` `{runId, events: []TestEvent}`: test2json lines decoded and batched. Non-JSON lines become `Output` events with no test.
  - `run.configs` `[]ConfigDTO`: on load, change, or a watcher change in either directory.
- Benchmark history lives at `<config>/workspaces/<hash>/bench.json` (name → last result).

## 6. internal/dap
Same structure as `internal/lsp`: `Content-Length` framing over a `net.Conn`, `seq` / `request_seq`, `Call(ctx, command, args, out)`, `OnEvent func(Event)`, and reverse requests (`runInTerminal`) answered with "not supported". The protocol types cover only the subset used: Capabilities, Source, Breakpoint/SourceBreakpoint/FunctionBreakpoint, Thread, StackFrame, Scope, Variable, Evaluate, SetVariable, the Stopped/Output/Exited/Terminated/Breakpoint event bodies, and ExceptionInfo.

## 7. internal/debugger
```go
type Config struct{ Dlv string; Env []string; Dir string; Launch map[string]any; Attach bool /*remote: Addr*/; Addr string; Log *slog.Logger;
                    OnEvent func(Event); OnOutput func(category, text string) }
func Start(ctx, Config) (*Session, error)   // spawn `dlv dap --listen=127.0.0.1:0`, parse "listening at:", dial; or dial Addr for remote
func (s *Session) Initialize/Launch → SetBreakpoints(path, []BP) []Breakpoint; SetFunctionBreakpoints; SetExceptionBreakpoints; ConfigurationDone
Threads, StackTrace(goroutine, start, levels), Scopes(frame), Variables(ref), Evaluate(expr, frame, context), SetVariable(ref, name, value),
Continue/Next/StepIn/StepOut/Pause(goroutine), Disconnect(terminate bool), Restart
```
- Launch arguments always set `output` to a temp binary path, so Delve doesn't write `__debug_bin*` into the project, and `outputMode: "remote"`, so program output arrives as `output` events.
- Run to Cursor: add a temporary line breakpoint, continue, and on the next stop restore the user's breakpoints for that file.
- Events reach rpc as `debug.event` `{sessionId, event, body}`: stopped, continued, output, exited, terminated, breakpoint. rpc adds `debug.state` `{sessionId, state: starting|running|stopped|exited, ...}`.

## 8. Breakpoints
- Breakpoints are owned by the frontend (`debug/breakpoints.ts`) and keyed by path. A CodeMirror `StateField` holds a `RangeSet` per open buffer, so lines map through edits. They persist in the layout store (`debug.breakpoints`), and so do function breakpoints, break-on-panic and watches.
- On change, the frontend calls `DebugService.SetBreakpoints(sessionId, path, bps)` for every active session. A new session receives all of them before `configurationDone` through `DebugService.Start(name, Breakpoints{lines, functions, panic})`. Verification results come back and update the gutter marks.

## 9. Frontend
- `run/`:
  - `install.ts`: commands, panels, the status-bar run widget, and gutter integration via `editor/runGutter.ts` calling the `run.marker` command.
  - `configs.ts`: a configuration store signal, and the editor dialog with type-specific forms.
  - `ansi.ts`: a pure SGR parser producing segments.
  - `links.ts`: pure location detection (`path.go:12:3`, `/abs/x.go:12 +0x1d`).
  - `console.ts`: renders segments into line rows, with a cap, search, soft wrap and input.
  - `runWindow.ts`: tabs and toolbar.
  - `testTree.ts`: a pure test2json → tree reducer, summary and failed leaves.
  - `testView.ts`, `bench.ts` (pure benchmark parsing, plus a table), `diff.ts` (pure testify parse, plus the merge view), `pattern.ts`.
- `debug/`:
  - `install.ts`, `breakpoints.ts` (gutter, dialog, store), `session.ts` (client state per session).
  - `debugWindow.ts`: toolbar, goroutines and frames, variables, watches, console.
  - `goroutines.ts`: pure parse of thread names, grouping and filtering.
  - `variables.ts`: lazy tree and the value formatting options.
  - `inline.ts`: inline value decorations and the stopped-line highlight.
  - `evaluate.ts`.
- `terminal/terminal.ts`: `@xterm/xterm` with fit, search and web-links addons. It has a link provider for file locations, resolved against the cwd from OSC 7. Its theme is built from `--goi-*` tokens, and themes update when the UI theme changes.
- New theme tokens: `--goi-ansi-{black,red,green,yellow,blue,magenta,cyan,white}` plus bright variants, `--goi-debug-line`, `--goi-breakpoint` and `--goi-inline-value`.

## 10. Terminal and shell integration
- `internal/terminal`: `Manager.Open(Spec{Shell, Args, Dir, Env, Cols, Rows}) (id)`, `Write`, `Resize`, `Close`, with output and exit callbacks.
- zsh: `ZDOTDIR=<cache>/shell/zsh`, with `.zshenv`, `.zprofile` and `.zlogin` sourcing the user's files from `$GOI_USER_ZDOTDIR` (or `$HOME`). Its `.zshrc` sources the user's `.zshrc`, then adds an OSC 7 `precmd` hook and `_goi_go` completion (`compdef`).
- bash: `--rcfile <cache>/shell/bash/goi.bashrc`, which sources `~/.bashrc` (or the login profile files) and adds `PROMPT_COMMAND` OSC 7 and `complete -F _goi_go go`.
- The completion lists come from one Go table: subcommands, common flags per subcommand, and the `go help` topics.

## 11. DTO and protocol rules
DAP lines and columns are 1-based and are passed through unchanged; frontend editor lines are already 1-based. Every ID crossing the bridge is a string (`runId`, `sessionId`, `termId`) or a number (DAP references). There is no `time.Time`: times are elapsed milliseconds or RFC 3339 strings.

## 12. Testing
- **Go**:
  - `runconfig` (store round trip, validate, plan, env file, split args, pattern) and `process` (exit codes, stdin, stop escalation, process-group kill of children; Unix only where needed) are table-tested.
  - `dap`: Conn against a fake adapter over `net.Pipe`.
  - `debugger`: integration against real `dlv` with conditional breakpoints, goroutines, evaluate, setVariable and test mode. Skipped when dlv is missing; a `GOI_DLV` env var points tests at a binary.
  - `terminal`: a PTY echo round trip on Unix.
  - rpc: config CRUD and events, plus run and test launches on a temp module.
- **Frontend**: vitest for ansi, links, testTree, bench, diff, pattern, goroutines, the variable formatting options, and the console cap.
- **E2E**: `tools/e2e/m3.json` against `wails dev` on `tools/e2e/m3-sample.sh`. It covers run, stop, the test tree, rerunning a failed subtest, debugging a test with a conditional breakpoint, the goroutine filter, inline values, evaluate, watches, set value, and terminal echo.
