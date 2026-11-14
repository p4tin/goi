# AI Agents — Design

## 1. Overview

```
 Agent process (claude-agent-acp, gemini --acp, …)
   │  ACP v1: JSON-RPC, one message per line, over stdio
   ▼
 internal/agent.Client ── internal/acp.Conn          (protocol + process + agent terminals)
   │  Host callbacks (fs, permission, updates)
   ▼
 internal/rpc  agentManager (per project session)  ── window.ui broker ──► frontend agent/
   │                                                   agent.request ──►  tools.ts (editor-aware fs + IDE tools)
   │                                                   ◄── AgentService.Respond
   ▼
 internal/rpc  mcpHost (process-wide) ── internal/mcp.Server (HTTP, bearer token)
   ▲  tools/call → window broker "tool" → frontend tools.ts
   │
 Agent's MCP client (http, or stdio via `goi mcp-bridge`)   /  external MCP clients
```

Principles:
- `internal/acp` and `internal/mcp` are protocol-only (like `lsp`, `dap`). `internal/agent` knows about processes, npm and the catalogue, not about Goi windows. `internal/rpc` is the bridge.
- **The frontend owns editor-aware operations.** Unsaved buffers, the selection, diagnostics, run consoles and undo history live there. So ACP `fs/*` requests and MCP tool calls are forwarded from the backend to the window's frontend (`agent.request` event), executed by `frontend/src/agent/tools.ts`, and answered with `AgentService.Respond`. The backend checks the paths (inside the project) before forwarding.
- Transcripts and the agent's change list live in the frontend (in memory). The backend forwards `session/update` payloads as raw JSON; it never interprets them beyond routing.

## 1.1 Protocol facts (ACP v1 schema, `@agentclientprotocol/sdk` 2.6.0)

- Client → agent: `initialize`, `authenticate`, `session/new`, `session/prompt`, `session/cancel` (notification), `session/set_mode`, `session/close`.
- Agent → client: `session/update` (notification), `session/request_permission`, `fs/read_text_file`, `fs/write_text_file`, `terminal/create|output|wait_for_exit|kill|release`.
- `protocolVersion` is the integer `1`. Auth required is error `-32000`. `AuthMethod` is `{id, name, description}` with optional `type: "terminal"` (+ `args`, `env`): the client runs the agent program itself with those args.
- `session/new` takes `{cwd, mcpServers}`; MCP servers are `{type:"http", name, url, headers:[{name,value}]}` (if `agentCapabilities.mcpCapabilities.http`) or stdio `{name, command, args, env:[{name,value}]}` (always supported).
- `session/update.update.sessionUpdate` ∈ `user_message_chunk, agent_message_chunk, agent_thought_chunk, tool_call, tool_call_update, plan, available_commands_update, current_mode_update, config_option_update, session_info_update, usage_update, notice, compaction_*`. Unknown kinds are ignored by the UI.
- Content blocks: `text`, `image`, `audio`, `resource_link {uri, name}`, `resource {resource:{uri, text, mimeType}}`. Tool-call content: `{type:"content", content}`, `{type:"diff", path, oldText, newText}`, `{type:"terminal", terminalId}`.

## 2. Backend

### 2.1 `internal/acp` (new, protocol-only)

- `conn.go` — `Conn`: bidirectional JSON-RPC 2.0 over newline-delimited JSON.
  - `NewConn(r io.Reader, w io.Writer, h Handler) *Conn`; `Call(ctx, method, params, result any) error`; `Notify(method, params) error`; `Close() error`; `Done() <-chan struct{}`; `Err() error`.
  - `Handler` interface: `HandleRequest(ctx, method string, params json.RawMessage) (any, error)` and `HandleNotification(method string, params json.RawMessage)`. Requests run in their own goroutine: an agent calls `fs/read_text_file` while our `session/prompt` call is pending. Notifications run in order on the read loop, so `session/update` chunks stay ordered.
  - `*Error{Code, Message, Data}` implements `error`. `ErrMethodNotFound` and friends; a handler returning `*Error` sends it as is, any other error becomes `-32603`.
  - Line reader with no length limit (`bufio.Reader.ReadBytes('\n')`); invalid lines are skipped and logged through an optional `Logf`.
- `protocol.go` — the types Goi sends or must read: `InitializeParams/Result`, `ClientCapabilities`, `AgentCapabilities` (`PromptCapabilities`, `MCPCapabilities`), `AuthMethod`, `NewSessionParams/Result`, `SessionModeState`, `MCPServer` (custom `MarshalJSON` for the http/stdio variants), `ContentBlock`, `PromptParams/Result`, `SessionNotification{SessionID, Update json.RawMessage}`, `RequestPermissionParams` (raw `toolCall`, typed `options`), `PermissionOutcome`, `ReadTextFileParams/Result`, `WriteTextFileParams`, terminal request/response types, method name constants, `ProtocolVersion = 1`, `CodeAuthRequired = -32000`.

### 2.2 `internal/mcp` (new, protocol-only)

- `server.go` — `Server` implements `http.Handler` for the MCP Streamable HTTP transport, stateless, JSON responses only:
  - `POST` with one JSON-RPC message. Requests get `200 application/json`; notifications and responses get `202`. `GET`/`DELETE` → `405`. Missing or wrong `Authorization: Bearer <token>` → `401` (constant-time compare). `Origin` other than none/localhost → `403` (DNS-rebinding guard).
  - Methods: `initialize` (echo the client's `protocolVersion` if it's one we know — `2025-06-18`, `2025-03-26`, `2024-11-05` — else the newest), `ping`, `tools/list`, `tools/call`. Others → `-32601`.
  - `Tool{Name, Description, InputSchema json.RawMessage}`; `Options{Token, Tools []Tool, Call CallFunc, Target func(*http.Request) string, Info Implementation}`. `CallFunc(ctx, target, name string, args json.RawMessage) (ToolResult, error)`. A returned error becomes a result with `isError: true` (MCP's convention for tool failures); unknown tool → `-32602`.
  - `ToolResult{Content []Content{Type:"text", Text}, IsError bool}` with `TextResult(s)` / `JSONResult(v)` helpers.
- `bridge.go` — `Bridge(ctx, in io.Reader, out io.Writer, url, token string, client *http.Client) error`: reads newline-delimited messages from `in`, POSTs each, writes the JSON response (if any) as one line to `out`. `RunBridge(args []string) int` parses `--url`/`--token` for `main.go`.

### 2.3 `internal/agent` (new)

- `catalogue.go` — `Definition{ID, Name, Description, Package, Bin string; Args []string; Homepage string}` and `Catalogue()`:
  | id | name | npm package | bin | args |
  |---|---|---|---|---|
  | `claude` | Claude Code | `@agentclientprotocol/claude-agent-acp` | `claude-agent-acp` | |
  | `gemini` | Gemini CLI | `@google/gemini-cli` | `gemini` | `--acp` |
  | `codex` | Codex | `@agentclientprotocol/codex-acp` | `codex-acp` | |
- `cmdline.go` — `SplitCommandLine(string) ([]string, error)`: whitespace split with single/double quotes and backslash escapes (POSIX-like; on Windows backslashes are kept literally).
- `resolve.go` — `Resolve(def Definition, override, toolsDir string, lookPath func(string) (string, error)) (Command, error)`; `Command{Path string; Args []string; Source string /* setting|tools|path */}`. The tools-dir binary is `<toolsDir>/agents/<id>/node_modules/.bin/<bin>` (`.cmd` on Windows). Custom agents (no catalogue entry) are `Definition{ID, Name: id}` with only an override.
- `install.go` — `Install(ctx, def, toolsDir, npm string, out func(line string)) error`: `npm install --prefix <toolsDir>/agents/<id> --no-audit --no-fund --loglevel=error <package>`, output streamed line by line.
- `client.go` — `Client`: one running agent.
  - `Start(cmd Command, dir string, env []string, host Host) (*Client, error)`: `process.Start` (process group, interrupt-then-kill stop); stdout feeds an `io.Pipe` read by the `acp.Conn`; stderr goes to a 16 KiB ring kept for error messages (`StderrTail()`).
  - `Initialize(ctx) (acp.InitializeResult, error)` (capabilities: fs read/write, terminal; `clientInfo{name:"goi", version}`), `NewSession(ctx, cwd, servers)`, `Authenticate(ctx, method)`, `Prompt(ctx, sid, blocks) (stopReason, error)`, `Cancel(sid)`, `SetMode(ctx, sid, mode)`, `Close()` (closes stdin, stops the process, releases terminals), `Done()`, `ExitErr()`.
  - Incoming: `session/update` → `host.SessionUpdate(sid, raw)`; `session/request_permission` → `host.RequestPermission(ctx, params) (acp.PermissionOutcome, error)`; `fs/read_text_file` → `host.ReadTextFile(ctx, params)`; `fs/write_text_file` → `host.WriteTextFile(ctx, params)`; `terminal/*` → the client's `Terminals`.
  - `Host` interface carries those four methods plus `TerminalSpec(cwd string) (dir string, env []string, err error)` (the host validates the cwd and supplies the project environment) and `TerminalOutput(id string, data []byte, exit *ExitStatus)` for live UI updates.
- `terminal.go` — `Terminals`: agent-created command runs. `Create(spec) (id)` runs `process.Start` with stdin closed, appends combined output to a buffer capped at `outputByteLimit` (default 1 MiB; truncation drops whole UTF-8 characters from the start), `Output(id)`, `Wait(ctx, id)`, `Kill(id)`, `Release(id)` (kills if running, forgets), `CloseAll()`. Ids are `t<n>` prefixed by the client's id prefix so they are unique per window.

### 2.4 Settings (`internal/settings`)

```toml
[ai]
mcpServer = true      # give agents started by Goi the IDE tools (MCP)
externalMcp = false   # also serve the IDE tools to other MCP clients on mcpPort
mcpPort = 64342
defaultAgent = ""     # agent preselected in the AI Agent window
agents = {}           # id = "command line": custom agents, or overrides for claude/gemini/codex
```
`Settings.AI` struct; `decode` defaults `Agents` to an empty map.

### 2.5 `internal/rpc`

- `agent.go`
  - `agentManager` (per project `session`, field `agents`): `procs map[agentID]*agentProc`, `sessions map[key]*agentSess` (keys `s<n>`), mutex. `close()` stops every process (called from `session.close`).
  - `agentProc`: `*agent.Client`, init result, state. Implements `agent.Host`:
    - `ReadTextFile`/`WriteTextFile`: map the ACP session id to the Goi key, check the path is absolute and inside the project (`session.contains`), then `win.ui.request(ctx, "fs.read"|"fs.write", key, params)`.
    - `RequestPermission`: `win.ui.request(ctx, "permission", key, params)`; the frontend answers with an ACP outcome. When a turn is cancelled, pending permission requests for that key are answered `cancelled` by the backend (`ui.cancelSession(key)`).
    - `SessionUpdate`: emit `agent.update {session: key, update: raw}`.
    - `TerminalSpec`: cwd inside the project (default root); env = project Go environment (`w.runner`) + request env.
    - `TerminalOutput`: emit `agent.terminal {id, data, exit}` (batched like run output).
  - On process exit: emit `agent.state {agent, state:"stopped"|"error", message: stderr tail}` and `agent.turn {session, error}` for sessions with a turn in flight.
- `agentui.go` — `uiBroker` on each `window` (`w.ui`): `request(ctx, kind, session string, params any) (json.RawMessage, error)` emits `agent.request {id, kind, session, params}` and waits for `respond(id, result, errMsg)` or ctx/timeout (2 min for fs/tools; permissions wait until answered or cancelled). Pending requests fail with "window closed" when the window goes away.
- `AgentService` (bound; ctx first; long work detached from ctx):
  - `Agents() []AgentInfo` — catalogue + custom agents with availability.
  - `Install(agentID) error` — validates and starts the npm install in the background; progress on `appBus` topic `agent.install {agent, line, done, error}`.
  - `NewSession(agentID) (SessionInfo, error)` — starts or reuses the process, initializes, creates the session with the MCP server; `SessionInfo{Key, Agent, AgentName, Modes, CurrentMode, EmbeddedContext, AuthRequired, AuthMethods}`.
  - `Authenticate(agentID, methodID) error`; `AuthCommand(agentID, methodID) (string, error)` — the command line for a terminal auth method.
  - `Prompt(key, blocks []PromptBlock) error` — returns once sent; `agent.turn {session, stopReason, error}` reports the end.
  - `Cancel(key)`, `SetMode(key, mode)`, `CloseSession(key)`, `StopAgent(agentID)`.
  - `Respond(id, result, errMsg string)` — answers an `agent.request`.
  - `TerminalOutput(id) (AgentTerminal, error)`.
  - `MCPConfig() (MCPConfig, error)` — external URL, token, JSON snippet, `claude mcp add` command.
- `mcp.go` — `mcpHost` (process-wide, on `App`): two `http.Server`s sharing one `mcp.Server` per token:
  - internal: `127.0.0.1:0`, random token per process, started on first use. URL per agent session: `/mcp/<windowID>/<sessionKey>`.
  - external (if `ai.externalMcp` at startup): `127.0.0.1:<ai.mcpPort>`, token in `<config>/mcp-token` (0600), path `/mcp` (focused window, or `project` argument).
  - `Call` → resolve the window → `w.ui.request(ctx, "tool", key, {name, args})`.
  - Tool definitions (`name`, description, JSON schema) live in `agenttools.go`; implementations are in `frontend/src/agent/tools.ts`. A Go test checks every tool name appears there.
  - Stdio fallback for agents without MCP-over-HTTP: `{command: os.Executable(), args: ["mcp-bridge", "--url", u, "--token", t]}`.
- `main.go`: `if len(os.Args) > 1 && os.Args[1] == "mcp-bridge" { os.Exit(mcp.RunBridge(os.Args[2:])) }` before anything else (no single-instance handoff, no GUI).
- Events (`internal/events`): `agent.state`, `agent.update`, `agent.request`, `agent.turn`, `agent.terminal`, `agent.install`.

## 3. Frontend (`frontend/src/agent/`)

- `lib/api.ts`: `agentApi` wrappers, types (`AgentInfo`, `AgentSessionInfo`, `AgentRequest`, `PromptBlock`, …), topics.
- `model.ts` (pure) — `Transcript` reducer: `apply(state, update)` for each `sessionUpdate`; entries: `user`, `agent` (Markdown text; chunks merge into the last agent entry or by `messageId`), `thought`, `tool` (by `toolCallId`; updates patch fields, `content`/`locations` replace when present), `permission` (request id, tool call, options, answer), `turn` (stop reason / error), `notice`. Session-level state: `plan`, `modes`, `currentMode`, `commands`, `usage`, `title`, `busy`.
- `markdown.ts` (pure DOM) — safe Markdown subset → DOM nodes (never `innerHTML`): paragraphs, headings, lists, fenced code (language label, Copy, Insert at cursor), inline code, bold/italic, links (`openExternal`), file links `path:line` opening in the editor.
- `diff.ts` (pure) — line diff (Myers on lines, capped) for inline tool-call diffs.
- `changes.ts` — `ChangeLog` per session key: first-change snapshot `{path, before, created}`; `revert(path)`, `revertAll()`.
- `tools.ts` — `createAgentTools(deps)`: handlers for `fs.read`, `fs.write` and the MCP tools (R6.4). `deps` = editor manager, file/lang/run/search/git APIs, problems store, changes, notifier — injected for tests. Edits to open buffers are one CodeMirror transaction (`userEvent: 'input.agent'`) followed by a save; closed files go through `files.write`/`files.create`.
- `view.ts` — the AI Agent panel: session tabs, header (agent picker, mode picker, Stop, New session, Changes), transcript (virtualized by appending; batched per animation frame), plan box, permission cards, composer (textarea, context chips for current file and selection, `/` command suggestions).
- `install.ts` — `installAgent(wb)`: panel registration (right dock), commands, `agent.request` dispatch (tools vs permission), status item showing a running turn, menu entries (`app/menuBar.ts`), settings descriptors (`views/settings/schema.ts`, section "AI").
- `git/diffView.ts` gains a `text` mode (`DiffRequest.left/right/leftLabel/rightLabel`) for "before agent → now" diffs.
- `agent.css` — theme tokens only.

## 4. Testing

- Fake agent: `internal/acp/acptest` exports `RunFakeAgent(in, out)` — a scripted ACP agent driven by the prompt text (`echo`, `read <path>`, `write <path> <text>` (asks permission first), `run <cmd…>` (terminal), `mcp <tool> <json>` (calls the session's MCP server over HTTP), `plan`, `wait` (until cancelled), `auth` mode via env). Go tests re-exec the test binary as the agent (`GOI_FAKE_AGENT=1`). `tools/fakeagent/main.go` wraps it for E2E.
- Go: `acp` (framing, concurrency, errors), `mcp` (auth, origin, methods, bridge), `agent` (cmdline, resolve, terminals, client against the fake agent), `rpc` (session lifecycle, fs routing through a fake frontend responder, path guard, permission and cancel, MCP tool routing per window, process cleanup on close).
- Frontend: reducer, Markdown, line diff, change log, tools against a fake editor manager.
- E2E `tools/e2e/agent.json`: server build with `ai.agents.fake = "<fakeagent>"`; start a session, echo, read an open (dirty) buffer, write with permission → buffer changed and saved, diff/revert, run a command, call an MCP tool, cancel a `wait` turn.

## 5. Risks

- Agent adapters change quickly; the catalogue only names packages and binaries, and users can override any command in settings.
- Real agents need their own login; Goi only relays ACP auth methods.
- Windows: npm bins are `.cmd` shims, which `exec` can run; `SplitCommandLine` keeps backslashes.
