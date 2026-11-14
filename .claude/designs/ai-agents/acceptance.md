# AI Agents — Acceptance

Status as of 2026-11-14, branch `feat/ai-agents`, macOS (arm64). Written by the implementer; the user's review is pending.

## Automated checks

| Check | Result |
|---|---|
| `go test -race ./internal/...` | pass |
| `go vet ./...`, `go vet -tags server ./internal/rpc`, gofmt | clean |
| `npx tsc --noEmit`, `npm test` | clean; 415 tests (48 files) pass |
| E2E `tools/e2e/agent.json` (server build + scripted fake agent) | every step passes (log below) |
| Real agent (`GOI_REAL_AGENT`) | **not verified**, see "Real agents" |

New tests:
- **`internal/acp`:** framing, nested requests during a call, ordered notifications, errors, auth-required, close.
- **`internal/mcp`:** auth, origin guard, initialize, version negotiation, list/call, notifications 202, tool errors, bridge.
- **`internal/agent`:**
  - The command-line parser, resolution order, npm install (fake npm) and terminals (truncation, kill, release).
  - A client session against the fake agent: echo, fs read/write, permission reject, terminal, mode, cancel, prompt error, auth, process exit, start failure.
- **`internal/rpc`:**
  - An end-to-end session through `AgentService` with a scripted frontend: fs routing, the project path guard, permission, MCP over HTTP and over the stdio bridge, terminal events, mode, process cleanup when the project closes.
  - Cancel answering a pending permission; turn already in progress; context blocks; auth; errors; MCP routing (internal path, `project` argument, focused window); the external server and its persistent token; tool names matching the frontend.
- **Frontend:** the transcript reducer, Markdown (incl. no HTML injection and streaming code fences), line diff, change log, and every IDE tool against fakes.

## Requirements

| Req | Status | Evidence |
|---|---|---|
| R1.1 catalogue | pass | Claude Code (`@agentclientprotocol/claude-agent-acp`), Gemini CLI (`gemini --acp`), Codex (`@agentclientprotocol/codex-acp`); package names checked on npm 2026-11-13 |
| R1.2 availability and source | pass | E2E: "Claude Code:Install, Gemini CLI:Start, Codex:Install, fake:Start" |
| R1.3 resolution order | pass | `TestResolve` |
| R1.4 / R1.5 npm install | pass (unit) | `TestInstall` with a fake npm; `FindNpm` error. A real `npm install` of an agent was **not** run |
| R1.6 custom agents | pass | `ai.agents.fake` in E2E and rpc tests |
| R2.1 start, initialize, session/new | pass | rpc and E2E |
| R2.2 one process per agent per window | pass | `agentManager.proc` reuses; `Agents().Running` |
| R2.3 streaming and stop reason | pass | E2E echo; turn events |
| R2.4 file and selection context | pass | E2E: the prompt carries `resource:file://…/main.go` with the unsaved text; selection is a text block (unit `promptBlocks` via view code; manual check suggested) |
| R2.5 transcript content | pass | E2E: thought collapsed, tool calls, diff, terminal, plan; reducer tests |
| R2.6 Stop | pass | E2E "stopped, Send is back"; rpc cancel tests |
| R2.7 modes | pass | E2E mode picker Ask/Code |
| R2.8 slash commands | pass | E2E "/hello" suggestion |
| R2.9 auth methods | pass (fake) | `TestAgentAuth`; terminal method opens a terminal with `AuthCommand` (manual check suggested) |
| R2.10 agent exit / start failure | pass | `TestStartFailure`, `watch()` emits `agent.state` with the stderr tail |
| R2.11 stop with the project/window | pass | `TestAgentSessionEndToEnd` (close project → stopped) |
| R2.12 several sessions | implemented | session tabs; keys `s<n>` per window. Manual check |
| R3.1 reads see unsaved text | pass | E2E "unsaved-marker" |
| R3.2 writes through the buffer, saved | pass | E2E (editor shows the agent's text, tab not dirty) |
| R3.3 project-only access | pass | E2E `/etc/hosts` refused; rpc test checks the frontend never sees it |
| R3.4 writes recorded | pass | change list in E2E |
| R3.5 / R3.6 agent terminals | pass | E2E "agent-terminal / exit code 0"; `TestTerminals` |
| R4.1 permission cards | pass | E2E Allow/Reject with inline diff |
| R4.2 no auto-answers | pass | only the user's click (or a cancelled turn) answers |
| R4.3 surfacing a request | implemented | shows the AI Agent window + notification when hidden. Manual check |
| R5.1–R5.3 review, diff, revert | pass | E2E: Changes → Diff (Before agent / Now) → Revert restores the unsaved text. Created files go to the trash after a confirmation (unit) |
| R5.4 inline tool diffs | pass | E2E "1 added, 8 removed" |
| R6.1 MCP server with token | pass | `internal/mcp` tests |
| R6.2 passed to agents (http / stdio) | pass | `TestAgentSessionEndToEnd` (http), `TestAgentStdioMCPBridge` (stdio via `mcp-bridge`) |
| R6.3 routing | pass | `TestMCPRouting` |
| R6.4 tool set | pass | 16 tools; frontend tests; E2E get_open_editors, read_file (unsaved), get_diagnostics, list_run_configurations |
| R6.5 edit tools follow R3 | pass | tools tests (buffer then save, change log) |
| R6.6 external clients | pass (unit) | `TestMCPRouting` starts it and checks the config; the dialog is UI-only. Manual: turn on `ai.externalMcp`, restart, *Copy MCP Server Config…*, `claude mcp add …` |
| R6.7 `goi mcp-bridge` | pass | stdio bridge test (the test binary stands in for goi) |
| R7.1 window | pass | E2E screenshot (`/tmp/goi-e2e-agent/agent.png`) |
| R7.2 commands, menu | pass | Tools menu: New Agent Session, Ask Agent About Selection, Stop Agent Turn, Install Agent…, Copy MCP Server Config…; View › AI Agent |
| R7.3 settings | pass | `[ai]` defaults and the Settings › AI section |
| R7.4 theme tokens | pass | `agent.css` uses only `--goi-*` |
| R8.1 boundaries | pass | `internal/acp`, `internal/mcp` know nothing about Goi; no Wails outside `internal/rpc` |
| R8.2–R8.4 tests | pass | above |
| R8.5 no UI freeze | implemented | traffic handled in Go goroutines; transcript rendered once per animation frame, incrementally |

## Real agents

- **Gemini CLI 0.44.1 (installed here):** `gemini --acp` (and `--experimental-acp`, `--acp --debug`) started but wrote nothing, neither an `initialize` answer nor stderr, within 60–120 s when driven by hand. This looks like the local install (for example a login or update prompt it is waiting on) rather than the protocol. Retry with `GOI_REAL_AGENT="gemini --acp" go test -run RealAgent -v ./internal/agent` after checking `gemini` works interactively.
- **Claude Code adapter:** not run. Fetching and running `npx @agentclientprotocol/claude-agent-acp` was blocked by this session's permission policy. To check it, run `npx -y @agentclientprotocol/claude-agent-acp` once (or use *Install* in the AI Agent window), then `GOI_REAL_AGENT=…/claude-agent-acp go test -run RealAgent -v ./internal/agent`.
- **Codex:** not installed here.

## E2E log (agent.json)

```
R7.1 AI Agent window opens from the palette "Claude Code, Gemini CLI, Codex, fake"
R1.2 catalogue + custom agent listed (fake available) "Claude Code:Install | Gemini CLI:Start | Codex:Install | fake:Start"
R2.1 start a session with the fake agent "fake"
R2.4 current file chip shown "main.go×"
R2.3 echo streams an agent message "hello from goi"
R2.5 thought shown collapsed "Thoughtthinking about hello from goi"
R3.1 fs read sees the unsaved buffer "content: package main … // unsaved-marker"
R3.3 reads outside the project are refused "read failed: acp: Goi only lets agents access files inside the project: /etc/hosts (code -32602)"
R2.4 prompt carries the file as embedded context "text | resource:file:///private/tmp/goi-e2e/agentsample/proj/main.go=package main …"
R4.1 write asks permission (card with options) "Allow/Reject"
R4.1 inline diff in the permission card "1 added, 8 removed"
R3.2 agent wrote through the editor "package main // written by agent"
R3.2 buffer saved (tab not dirty) "tab active"
R5.1 change list shows main.go "main.go"
R5.2 diff opens (before agent → now) "Before agentNow"
R5.3 revert restores the unsaved text "restored"
R3.5/R3.6 agent terminal command with live output "agent-terminal / exit code 0"
R6.2/R6.4 MCP tool get_open_editors "mcp: { \"editors\": [ { \"active\": true, \"dirty\": false, \"path\": \"…/main.go\" } ] }"
R6.4 MCP tool read_file sees unsaved text "mcp: package main"
R6.4 MCP tool get_diagnostics "mcp: { \"count\": 0, … }"
R6.4 MCP tool list_run_configurations "mcp: { \"configurations\": [] }"
R2.7 mode picker switches mode "Ask/Code"
R2.8 slash commands suggested "/helloSay hello"
plan shown "☑Read the code | ◐Fix the bug"
R2.6 Stop cancels a running turn "stopped, Send is back"
status bar item "✦ Agent"
```

## Manual checks for the reviewer

1. `wails3 dev`, open a project, View › AI Agent: the agent cards show what's installed.
2. Start Gemini or Claude Code (after *Install* if needed) and ask it to change a function. Approve the edit; check the editor updates and *Changes* → *Diff* / *Revert*.
3. Select code → Tools › Ask Agent About Selection.
4. Settings › AI › External MCP Clients on, restart, Tools › Copy MCP Server Config…, then `claude mcp add …` in a terminal and ask that Claude Code to call `get_open_editors`.
5. Close the project while an agent runs: its process exits (Activity Monitor), and the tab shows it ended.
