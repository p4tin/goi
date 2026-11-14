# AI Agents — Tasks

One commit per task (`feat(agent): … (Tn)`). Each task leaves `go test ./internal/...`, `npx tsc --noEmit` and `npm test` green.

| # | Task | Files | Verify |
|---|---|---|---|
| T1 | ACP protocol client: ndjson JSON-RPC `Conn`, protocol types | `internal/acp/{conn,protocol}.go` + tests | framing, concurrent requests, ordered notifications, errors |
| T2 | Fake ACP agent for tests | `internal/acp/acptest/fake.go`, `tools/fakeagent/main.go` | used by T4/T6 tests |
| T3 | MCP server + stdio bridge | `internal/mcp/{server,bridge}.go` + tests | auth, origin, initialize/list/call, notifications 202, bridge round trip |
| T4 | Agent package: catalogue, command lines, resolve, install, terminals, client | `internal/agent/*` + tests | client session against the fake agent (fs, permission, terminal, cancel) |
| T5 | Settings `[ai]` | `internal/settings/{settings.go,defaults.toml}`, settings schema (frontend) | settings tests |
| T6 | rpc: ui broker, agent manager, AgentService, MCP host, tool definitions, events, `goi mcp-bridge` | `internal/rpc/{agent,agentui,agenttools,mcp}.go`, `session.go`, `window.go`, `events.go`, `main.go` + tests | end-to-end session with a fake frontend responder; MCP routing; cleanup |
| T7 | Bindings + `lib/api.ts` | `frontend/bindings`, `lib/api.ts` | tsc |
| T8 | Transcript reducer, Markdown, line diff, change log | `agent/{model,markdown,diff,changes}.ts` + tests | vitest |
| T9 | IDE tools (fs + MCP tools) | `agent/tools.ts` + tests; Go contract test | vitest, go test |
| T10 | AI Agent panel, commands, menu, text diff mode, CSS | `agent/{view,install}.ts`, `agent/agent.css`, `features.ts`, `app/menuBar.ts`, `git/diffView.ts` | tsc, vitest, manual |
| T11 | E2E against the fake agent | `tools/e2e/agent.json`, `tools/e2e/agent-sample.sh` | E2E passes |
| T12 | Real-agent smoke test (Gemini CLI `--acp`, installed here) | — | session + one prompt (manual/scripted) |
| T13 | Docs: CLAUDE.md, steering, acceptance, retro | `.claude/…` | — |

Sequential: one implementer. T1–T4 are independent of the UI; T6 needs T1–T5; T7 needs T6; T8–T10 need T7.
