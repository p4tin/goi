// AI agents feature: the AI Agent tool window, its commands and status item,
// and the frontend side of agent.request (editor-aware file access, IDE
// tools for MCP, permission prompts).

import { showDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { editors } from '../editor/install';
import { agentApi, on, Topics } from '../lib/api';
import type { AgentInstallEvent, AgentRequestEvent, AgentStateEvent, AgentTerminalEvent, AgentTurnEvent, AgentUpdateEvent } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { changeLog } from './changes';
import { diskPort, editorPort, idePort } from './ports';
import { AgentTools } from './tools';
import { AgentPanel } from './view';
import './agent.css';

/** Answers one agent.request. Exported for tests. */
export async function handleRequest(req: AgentRequestEvent, tools: AgentTools, panel: Pick<AgentPanel, 'onPermission'>): Promise<void> {
  if (req.kind === 'permission') {
    panel.onPermission(req);
    return;
  }
  try {
    const p = (req.params ?? {}) as Record<string, unknown>;
    let result: unknown;
    if (req.kind === 'fs.read') {
      result = await tools.read(p as { path: string; line?: number; limit?: number });
    } else if (req.kind === 'fs.write') {
      await tools.write(req.session, String(p.path), String(p.content ?? ''));
      result = {};
    } else if (req.kind === 'tool') {
      result = await tools.call(req.session, String(p.name), (p.args ?? {}) as Record<string, unknown>);
    } else {
      throw new Error(`unknown request ${String(req.kind)}`);
    }
    await agentApi.respond(req.id, result);
  } catch (err) {
    await agentApi.respond(req.id, null, err instanceof Error ? err.message : String(err)).catch(() => {});
  }
}

async function showMcpConfig(wb: Workbench): Promise<void> {
  let cfg;
  try {
    cfg = await agentApi.mcpConfig();
  } catch (err) {
    wb.notifications.push({ id: 'agent.mcp', level: 'warn', title: 'MCP server for external clients is off', body: err instanceof Error ? err.message : String(err) });
    return;
  }
  const copy = (text: string, what: string): void => {
    void navigator.clipboard?.writeText(text);
    wb.notifications.push({ id: 'agent.mcp.copied', level: 'info', title: `${what} copied` });
  };
  const body = h('div', { class: 'agent-mcp-config' },
    h('p', null, 'Other MCP clients can use this window’s IDE tools at ', h('code', null, cfg.url), '. The token stays the same across restarts.'),
    h('div', { class: 'agent-mcp-label' }, 'Claude Code (terminal)'),
    h('pre', { class: 'agent-mcp-pre' }, cfg.claudeCommand),
    h('button', { class: 'btn', onClick: () => copy(cfg.claudeCommand, 'Command') }, 'Copy Command'),
    h('div', { class: 'agent-mcp-label' }, 'JSON (mcpServers)'),
    h('pre', { class: 'agent-mcp-pre' }, cfg.json),
    h('button', { class: 'btn', onClick: () => copy(cfg.json, 'JSON') }, 'Copy JSON'));
  await showDialog({ title: 'Goi MCP Server', body, buttons: [{ label: 'Close', value: undefined, primary: true }], cancelValue: undefined });
}

export function installAgent(wb: Workbench): void {
  const tools = new AgentTools({ editor: editorPort(), disk: diskPort(), ide: idePort(), changes: changeLog });
  const panel = new AgentPanel(wb, tools, changeLog);

  // Bottom dock, next to Terminal, like GoLand's agent tool windows.
  wb.panels.register({ id: 'agent', title: 'AI Agent', dock: 'bottom', order: 9, render: (c) => void c.append(panel.el) });

  on<AgentRequestEvent>(Topics.agentRequest, (req) => void handleRequest(req, tools, panel));
  on<AgentUpdateEvent>(Topics.agentUpdate, (e) => panel.onUpdate(e));
  on<AgentTurnEvent>(Topics.agentTurn, (e) => panel.onTurn(e));
  on<AgentStateEvent>(Topics.agentState, (e) => panel.onState(e));
  on<AgentTerminalEvent>(Topics.agentTerminal, (e) => panel.onTerminal(e));
  on<AgentInstallEvent>(Topics.agentInstall, (e) => panel.onInstall(e));

  // Agents belong to the project: the backend stops them when it closes.
  let root: string | null = null;
  effect(() => {
    const p = project();
    if ((p?.root ?? null) === root) return;
    root = p?.root ?? null;
    panel.reset();
    for (const c of changeLog.list('')) changeLog.forget('', c.path);
    if (p) void panel.refreshAgents();
  });

  const status = wb.status.add({ id: 'agent', side: 'right', priority: 6, title: 'AI Agent', onClick: () => show() });
  panel.onRender = () => {
    status.hidden = !panel.hasSessions();
    status.textContent = panel.busy() ? '✦ Agent working…' : '✦ Agent';
    status.classList.toggle('agent-status-busy', panel.busy());
  };
  panel.onRender();

  // A chat needs more room than the bottom dock's default height.
  const showPanel = (): void => {
    wb.panels.show('agent');
    if (wb.panels.state.peek().bottom.size < 320) wb.panels.patch('bottom', { size: 320 });
  };
  function show(): void {
    showPanel();
    queueMicrotask(() => panel.focusInput());
  }

  const hasProject = (): boolean => !!project.peek();
  commands.register({ id: 'agent.show', title: 'AI Agent', category: 'View', when: hasProject, run: show });
  commands.register({
    id: 'agent.newSession',
    title: 'New Agent Session',
    category: 'Tools',
    when: hasProject,
    run: async () => {
      showPanel();
      await panel.startSelected();
    },
  });
  commands.register({
    id: 'agent.askSelection',
    title: 'Ask Agent About Selection',
    category: 'Code',
    when: () => hasProject() && !!editors.activeView(),
    run: async () => {
      showPanel();
      if (!panel.current() || panel.current()?.state.ended) await panel.startSelected();
      const v = editors.activeView();
      panel.prefill(v && !v.state.selection.main.empty ? 'Explain the selected code.' : 'Explain this file.');
    },
  });
  commands.register({
    id: 'agent.stop',
    title: 'Stop Agent Turn',
    category: 'Tools',
    when: () => !!panel.current()?.state.busy,
    run: () => panel.stop(),
  });
  commands.register({
    id: 'agent.install',
    title: 'Install Agent…',
    category: 'Tools',
    when: hasProject,
    run: async () => {
      showPanel();
      await panel.refreshAgents();
    },
  });
  commands.register({ id: 'agent.copyMcpConfig', title: 'Copy MCP Server Config…', category: 'Tools', run: () => showMcpConfig(wb) });
}
