# AI Agents — Requirements

Status: draft written without a review round. The user asked for spec → implementation in one pass and will review everything at once. Decisions taken without the user are marked **(decision)**.

## Context

GoLand runs external coding agents (Claude Code, Gemini CLI, Codex…) inside the IDE through the **Agent Client Protocol** (ACP, `platform-acp` plugin). The agent edits files, runs commands and asks for permission through the IDE. The IDE can also expose its own capabilities to agents as an **MCP server** (`mcpserver` plugin). `SPEC.md` §3.16 lists both for Goi: F-AI-2 (MCP server) and F-AI-3 (ACP host with diff review of agent edits).

This feature delivers F-AI-2 and F-AI-3. F-AI-1 (inline completion) and F-AI-4 (ML ranking) are out of scope.

## Terms

- **Agent**: an external program that speaks ACP v1 over stdio (JSON-RPC 2.0, one message per line).
- **Agent session**: one conversation with an agent, rooted at the project folder.
- **IDE tools**: Goi operations an agent can call over MCP (open file, diagnostics, run configuration, …).

## R1 Agent catalogue and installation

- R1.1 Goi SHALL ship a catalogue of known agents: **Claude Code** (`@agentclientprotocol/claude-agent-acp`), **Gemini CLI** (`gemini --acp`), **Codex** (`@agentclientprotocol/codex-acp`). **(decision)** These three are the ones GoLand's registry features and that publish ACP adapters on npm.
- R1.2 WHEN the user lists agents THEN Goi SHALL show each agent's name, whether it is available, and where its command comes from (setting, Goi tools dir, PATH).
- R1.3 Goi SHALL resolve an agent's command in this order: the `ai.agents.<id>` setting, Goi's tools dir (`<config>/tools/agents/<id>`), then the agent's binary on `PATH` (and the usual `$GOBIN`-style places are not needed; npm global bins are on `PATH`).
- R1.4 WHEN the user installs a catalogue agent and npm is available THEN Goi SHALL install its npm package into the Goi tools dir and report progress and the result.
- R1.5 IF npm is not available THEN Goi SHALL explain that Node.js is required and SHALL NOT attempt the install.
- R1.6 The user SHALL be able to add a custom agent as `ai.agents.<id> = "<command line>"` (user settings). A custom entry with a catalogue id overrides that agent's command.

## R2 Agent sessions (ACP host)

- R2.1 WHEN the user starts a session with an agent THEN Goi SHALL start the agent process in the project root (with the project's Go environment), perform `initialize` (protocol version 1; capabilities: `fs.readTextFile`, `fs.writeTextFile`, `terminal`), and create a session (`session/new`) with the project root as `cwd`.
- R2.2 Goi SHALL run at most one process per agent per window and reuse it for further sessions with that agent.
- R2.3 WHEN the user sends a prompt THEN Goi SHALL send it with `session/prompt`, stream the agent's updates into the transcript as they arrive, and show the stop reason when the turn ends.
- R2.4 A prompt MAY carry context: the active file (embedded as a resource when the agent supports embedded context, otherwise as a resource link) and the editor selection (as text with its path and line range).
- R2.5 The transcript SHALL show: user messages, agent messages (Markdown), agent thoughts (collapsed), tool calls with kind, title, status, locations, text output, diffs and terminal output, and the agent's plan.
- R2.6 WHEN the user presses Stop during a turn THEN Goi SHALL send `session/cancel`, answer pending permission requests as cancelled, and show the turn as cancelled.
- R2.7 WHEN the agent advertises session modes THEN Goi SHALL show the current mode and let the user switch it (`session/set_mode`).
- R2.8 WHEN the agent sends `available_commands_update` THEN typing `/` in the prompt box SHALL suggest those commands.
- R2.9 IF `session/new` fails with "authentication required" (-32000) THEN Goi SHALL list the agent's auth methods; choosing an agent method SHALL call `authenticate` and retry; choosing a terminal method SHALL open an integrated terminal running the agent's command with the method's arguments.
- R2.10 IF the agent process exits or fails to start THEN Goi SHALL show the error (with the tail of its stderr) and mark its sessions as ended.
- R2.11 WHEN the project closes or its window closes THEN Goi SHALL stop that window's agent processes.
- R2.12 Several sessions (tabs) MAY be open at once in a window, with the same or different agents.

## R3 IDE as the agent's file system and terminal

- R3.1 WHEN an agent reads a file (`fs/read_text_file`) THEN Goi SHALL return the editor's content if the file is open (including unsaved changes), otherwise the file on disk, honoring `line` and `limit`.
- R3.2 WHEN an agent writes a file (`fs/write_text_file`) THEN Goi SHALL apply the content to the open buffer as one undoable change and save it, or write the file on disk (creating it and its folders) when it is not open.
- R3.3 Goi SHALL refuse agent file reads and writes outside the project's content roots.
- R3.4 Every agent write SHALL be recorded for review (R5).
- R3.5 WHEN an agent creates a terminal (`terminal/create`) THEN Goi SHALL run the command in the project root (or the given `cwd` inside the project) with the project environment plus the given variables, capture combined output up to `outputByteLimit` (truncating from the start), and support `terminal/output`, `wait_for_exit`, `kill` and `release`.
- R3.6 Terminal output embedded in a tool call SHALL be visible in the transcript while the command runs.

## R4 Permissions

- R4.1 WHEN an agent requests permission (`session/request_permission`) THEN Goi SHALL show the tool call and the agent's options (allow once / always, reject once / always) in the transcript and send the user's choice.
- R4.2 Goi SHALL NOT answer permission requests on the user's behalf. **(decision)** "Always" choices are remembered by the agent, not by Goi.
- R4.3 WHEN a window has a pending permission request and the agent panel is hidden THEN Goi SHALL show the panel (or a notification) so the request is not missed.

## R5 Reviewing agent edits

- R5.1 Goi SHALL keep, per session, the list of files the agent changed, with each file's content before the agent's first change in that session.
- R5.2 The user SHALL be able to open a side-by-side diff (before agent → now) for each changed file, from the session's change list and from a tool call's diff.
- R5.3 The user SHALL be able to revert one file or all files to their content before the agent's changes (files the agent created are deleted after a confirmation).
- R5.4 Tool-call diffs sent by the agent (`content.type = diff`) SHALL be shown inline as a compact line diff.

## R6 IDE tools over MCP (Goi MCP server)

- R6.1 Goi SHALL run an MCP server (Streamable HTTP transport, JSON responses) on `127.0.0.1` that requires a bearer token.
- R6.2 WHEN Goi creates an agent session and `ai.mcpServer` is on THEN it SHALL pass the server to the agent (`mcpServers`): as `http` when the agent supports MCP over HTTP, otherwise as `stdio` through `goi mcp-bridge`.
- R6.3 Tool calls from an agent launched in a window SHALL act on that window. Calls from external clients SHALL act on the focused window, or on the window whose project contains the `project` argument when given.
- R6.4 Goi SHALL provide at least these tools: `get_open_editors`, `get_active_editor` (path, cursor, selection), `open_file`, `read_file`, `replace_text`, `write_file`, `get_diagnostics`, `find_symbol`, `find_references`, `go_to_definition`, `search_in_project`, `list_run_configurations`, `run_configuration` (waits and returns exit code and output tail), `run_tests`, `format_file`, `git_status`.
- R6.5 Edit tools (`replace_text`, `write_file`) SHALL follow R3.2–R3.4.
- R6.6 WHEN `ai.externalMcp` is on THEN the server SHALL listen on `ai.mcpPort` with a token stored in the user config dir, and a command SHALL copy ready-to-use client configuration (a JSON `mcpServers` entry and a `claude mcp add` command line).
- R6.7 `goi mcp-bridge --url <url> --token <token>` SHALL relay MCP over stdio to the HTTP server without starting the GUI or touching the running instance.

## R7 UI and settings

- R7.1 An **AI Agent** tool window (right dock) SHALL hold session tabs, the agent picker, mode picker, transcript, change list and prompt box. Enter sends; Shift+Enter inserts a newline.
- R7.2 Commands: *AI Agent: Show*, *New Agent Session…*, *Ask Agent About Selection*, *Stop Agent Turn*, *Install Agent…*, *Copy MCP Server Config*. The native menu gets an entry under Tools.
- R7.3 Settings (section "AI"): `ai.mcpServer` (bool, default on), `ai.externalMcp` (bool, default off), `ai.mcpPort` (int, default 64342), `ai.agents` (map id → command line), `ai.defaultAgent` (string).
- R7.4 Colors come from theme tokens only; the panel works in every theme.

## R8 Quality

- R8.1 `internal/acp` and `internal/mcp` are protocol-only (no Goi knowledge), like `internal/lsp` and `internal/dap`. `internal/*` stays free of Wails.
- R8.2 Backend tests cover the protocol layers and an end-to-end agent session against a scripted fake agent (no network, no real agent).
- R8.3 Frontend tests cover the transcript reducer, Markdown rendering, and the IDE tools against a fake editor.
- R8.4 An E2E spec drives a session against the fake agent in the server build.
- R8.5 No UI freeze: agent traffic is handled off the UI thread; transcript updates are batched per animation frame.

## Out of scope

- Inline completion (F-AI-1), completion ranking (F-AI-4), Goi-hosted models or API keys.
- Restoring past sessions after restart (`session/load`); transcripts live in memory.
- Images/audio in prompts, elicitation, session config options other than modes.
- An online agent registry; the catalogue is built in.
