# AI Agents — Retro

## What went right

- **Protocol facts from the source.** Reading the ACP v1 JSON schema in `@agentclientprotocol/sdk` before designing gave the exact method names, the `type: "terminal"` auth methods, the http/stdio MCP server variants and the -32000 auth code. The Go types matched on the first try, and the Claude adapter's current npm name turned out to be `@agentclientprotocol/claude-agent-acp`, not the older Zed package.
- **One scripted fake agent for everything.** `internal/acp/acptest` drives the agent package tests, the rpc tests (the test binary re-runs itself as the agent) and the E2E (`tools/fakeagent`). Every path is tested without network or credentials: permissions, cancel, terminals, MCP over HTTP and stdio, auth.
- **"The frontend owns editor-aware work" held up.** Routing agent `fs/*` requests and MCP tool calls through one `agent.request`/`Respond` broker kept rpc thin, and made unsaved-buffer reads, undoable edits, review and revert fall out naturally. The backend's only job there is the project path check.
- **Ports in `tools.ts`.** All 16 IDE tools are unit-tested against fakes; only `ports.ts` touches the editor and APIs. A Go test keeps tool names in sync across the two languages.
- **The E2E found real UI bugs** that unit tests didn't. Moving the transcript in the DOM reset its scroll, and the toolbar overflowed the default 280 px dock.

## What went wrong

- **Two infinite recursions in the Markdown renderer.** One came from a shared global regex in a recursive function; the other from an autolink label rendered as Markdown again. Both crashed the vitest worker with out-of-memory errors, and the stack traces didn't point at the code. Recursion with a shared `lastIndex` regex is a known trap.
- **Test-binary re-exec ordering.** The `mcp-bridge` child inherited `GOI_FAKE_AGENT=1` and became a fake agent. Then the fake agent didn't pass the MCP server's env, so the token never arrived. Both bugs were in test scaffolding, but they took two rounds to find.
- **E2E sequencing.** A loose check returned on an older message, so the next prompt clicked Send while the agent was still busy, and Send means Stop then. `__ask` now waits for the agent to be idle.
- **No real-agent run.** Gemini CLI 0.44.1 on this machine never answered `initialize`. Running the Claude adapter via npx was blocked by the session's permission policy. Compatibility with real agents rests on the schema and the opt-in `TestRealAgent`, still to be run.
- **Scope kept tight on purpose.** The following were not built and are in the requirements' out-of-scope list:
  - session restore (`session/load`)
  - images in prompts
  - session config options other than modes
  - an online agent registry
  - auto-restart of a crashed agent

## Action items

1. **Real-agent check in the loop.** Next time, run `GOI_REAL_AGENT=…` with at least one real agent before UI work, with the user's go-ahead for installs and logins. Then add its transcript (initialize/new-session payloads) as a golden test fixture.
2. **Lint rule for regexes.** Ban module-level `/g` regexes used by recursive functions (a `lastIndex` trap). A small vitest or eslint check would have caught the Markdown loops before the worker crashed.
