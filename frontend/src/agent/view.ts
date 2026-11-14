// The AI Agent tool window: session tabs, agent and mode pickers, the
// transcript (streamed, rendered incrementally once per frame), permission
// cards, the plan, the agent's change list (diff / revert) and the prompt box
// with context chips and slash-command suggestions.

import { confirmDialog } from '../app/dialog';
import { project, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { baseName } from '../editor/buffer';
import { editors } from '../editor/install';
import { openDiff } from '../git/diffView';
import { agentApi, openExternal } from '../lib/api';
import type { AgentExit, AgentInfo, AgentInstallEvent, AgentRequestEvent, AgentSessionInfo, AgentStateEvent, AgentTerminalEvent, AgentTurnEvent, AgentUpdateEvent, PromptBlock } from '../lib/api';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { iconButton } from '../run/icons';
import { runInTerminal } from '../terminal/terminal';
import type { ChangeLog } from './changes';
import { compact, diffStats, lineDiff } from './diff';
import { renderMarkdown } from './markdown';
import type { MarkdownOptions } from './markdown';
import { addNotice, addPermission, addUser, answerPermission, applyUpdate, blockText, emptySession, endSession, endTurn, matchCommands, stopLabel } from './model';
import type { Entry, PermissionEntry, PermissionParams, SessionState, ToolContent, ToolEntry } from './model';
import type { AgentTools } from './tools';

interface Sess {
  key: string;
  info: AgentSessionInfo;
  state: SessionState;
  label: string;
}

interface TermOut {
  text: string;
  exit: AgentExit | null;
}

const KIND_GLYPH: Record<string, string> = {
  read: '◱', edit: '✎', delete: '✕', move: '⇄', search: '⌕', execute: '▶', think: '…', fetch: '⇣', switch_mode: '⇆', other: '•',
};

/** file:// URI for an OS path. */
export function fileUri(path: string): string {
  const p = path.replace(/\\/g, '/');
  return 'file://' + (p.startsWith('/') ? '' : '/') + p.split('/').map(encodeURIComponent).join('/').replace(/^\/([A-Za-z])%3A/, '/$1:');
}

/** Builds the prompt blocks for text plus optional file and selection context. */
export function promptBlocks(
  text: string,
  ctx: { file?: { path: string; text: string } | null; selection?: { path: string; startLine: number; endLine: number; text: string } | null; embedded: boolean },
): { blocks: PromptBlock[]; labels: string[] } {
  const blocks: PromptBlock[] = [{ type: 'text', text, uri: '', name: '', mimeType: '' }];
  const labels: string[] = [];
  if (ctx.selection) {
    const s = ctx.selection;
    const range = s.startLine === s.endLine ? `line ${s.startLine}` : `lines ${s.startLine}-${s.endLine}`;
    blocks.push({ type: 'text', text: `Selected code in ${s.path} (${range}):\n\`\`\`\n${s.text}\n\`\`\``, uri: '', name: '', mimeType: '' });
    labels.push(`${baseName(s.path)}:${s.startLine}${s.endLine !== s.startLine ? `-${s.endLine}` : ''}`);
  }
  if (ctx.file) {
    const uri = fileUri(ctx.file.path);
    blocks.push(
      ctx.embedded
        ? { type: 'resource', text: ctx.file.text, uri, name: baseName(ctx.file.path), mimeType: 'text/plain' }
        : { type: 'resource_link', text: '', uri, name: baseName(ctx.file.path), mimeType: '' },
    );
    labels.push(baseName(ctx.file.path));
  }
  return { blocks, labels };
}

export class AgentPanel {
  readonly el: HTMLElement;
  /** Called after every render (status bar updates). */
  onRender: () => void = () => {};
  private sessions: Sess[] = [];
  private active: string | null = null;
  private agents: AgentInfo[] = [];
  private terminals = new Map<string, TermOut>();
  private installing = new Map<string, string>();
  private starting: { agent: string; message: string } | null = null;
  private auth: { agent: string; name: string; methods: AgentSessionInfo['authMethods'] } | null = null;
  private showChanges = false;
  private attachFile = true;
  private suggestion = 0;
  private renderQueued = false;
  private entryEls = new WeakMap<Entry, { el: HTMLElement; sig: string }>();

  private tabs = h('div', { class: 'agent-tabs', role: 'tablist' });
  private picker = h('select', { class: 'agent-select', title: 'Agent for new sessions' });
  private modeSel = h('select', { class: 'agent-select', title: 'Session mode' });
  private changesBtn = h('button', { class: 'btn agent-changes-btn', title: 'Files the agent changed' }, 'Changes');
  private body = h('div', { class: 'agent-body' });
  private transcript = h('div', { class: 'agent-transcript', 'aria-live': 'polite' });
  private planBox = h('div', { class: 'agent-plan' });
  private changesBox = h('div', { class: 'agent-changes' });
  private startView = h('div', { class: 'agent-start' });
  private chips = h('div', { class: 'agent-chips' });
  private input = h('textarea', { class: 'agent-input', rows: 3, placeholder: 'Ask the agent… (Enter sends, Shift+Enter for a new line, / for commands)' });
  private suggest = h('div', { class: 'agent-suggest', hidden: true });
  private sendBtn = h('button', { class: 'btn btn-primary agent-send' }, 'Send');
  private status = h('div', { class: 'agent-status' });

  constructor(
    private wb: Workbench,
    private tools: AgentTools,
    private changes: ChangeLog,
  ) {
    const add = iconButton('plus', 'New agent session', () => void this.startSelected(), 'agent-add');
    this.changesBtn.addEventListener('click', () => {
      this.showChanges = !this.showChanges;
      this.render();
    });
    this.modeSel.addEventListener('change', () => void this.setMode(this.modeSel.value));
    this.sendBtn.addEventListener('click', () => (this.current()?.state.busy ? void this.stop() : void this.send()));
    this.input.addEventListener('keydown', (e) => this.onKey(e));
    this.input.addEventListener('input', () => {
      this.suggestion = 0;
      this.renderSuggestions();
      this.autosize();
    });
    this.el = h(
      'div',
      { class: 'agent-panel' },
      h('div', { class: 'agent-header' }, this.tabs, add),
      h('div', { class: 'agent-toolbar' }, this.picker, this.modeSel, h('span', { class: 'agent-spacer' }), this.changesBtn),
      this.body,
      h('div', { class: 'agent-composer' }, this.chips, h('div', { class: 'agent-input-wrap' }, this.input, this.suggest), h('div', { class: 'agent-composer-foot' }, this.status, this.sendBtn)),
    );
    effect(() => {
      this.changes.version();
      this.schedule();
    });
    effect(() => {
      editors.cursor();
      editors.activePath();
      this.renderChips();
    });
    this.render();
  }

  // ---- state access

  current(): Sess | undefined {
    return this.sessions.find((s) => s.key === this.active);
  }

  private byKey(key: string): Sess | undefined {
    return this.sessions.find((s) => s.key === key);
  }

  hasSessions(): boolean {
    return this.sessions.length > 0;
  }

  busy(): boolean {
    return this.sessions.some((s) => s.state.busy);
  }

  /** The preferred agent: the setting, else the first available one. */
  defaultAgent(): string {
    const pref = settings.peek()?.ai?.defaultAgent ?? '';
    if (pref && this.agents.some((a) => a.id === pref)) return pref;
    return this.agents.find((a) => a.available)?.id ?? this.agents[0]?.id ?? '';
  }

  async refreshAgents(): Promise<void> {
    try {
      this.agents = await agentApi.agents();
    } catch {
      this.agents = [];
    }
    const keep = this.picker.value;
    this.picker.replaceChildren(
      ...this.agents.map((a) => h('option', { value: a.id }, `${a.name}${a.available ? '' : ' (not installed)'}`)),
    );
    this.picker.value = this.agents.some((a) => a.id === keep) ? keep : this.defaultAgent();
    this.render();
  }

  reset(): void {
    this.sessions = [];
    this.active = null;
    this.terminals.clear();
    this.starting = null;
    this.auth = null;
    this.render();
  }

  // ---- actions

  async startSelected(): Promise<void> {
    if (!this.agents.length) await this.refreshAgents();
    const id = this.picker.value || this.defaultAgent();
    if (id) await this.start(id);
  }

  async start(agentId: string): Promise<Sess | null> {
    const a = this.agents.find((x) => x.id === agentId);
    if (a && !a.available) {
      this.active = null;
      this.render();
      this.notify('warn', `${a.name} is not installed`, a.installable ? 'Install it from the AI Agent window.' : a.command);
      return null;
    }
    this.auth = null;
    this.starting = { agent: agentId, message: `Starting ${a?.name ?? agentId}…` };
    this.active = null;
    this.render();
    try {
      const info = await agentApi.newSession(agentId);
      this.starting = null;
      if (info.authRequired) {
        this.auth = { agent: agentId, name: info.agentName, methods: info.authMethods };
        this.render();
        return null;
      }
      const n = this.sessions.filter((s) => s.info.agent === agentId).length + 1;
      const sess: Sess = { key: info.key, info, state: emptySession(info.modes, info.currentMode), label: `${info.agentName}${n > 1 ? ` ${n}` : ''}` };
      if (!info.ideTools) addNotice(sess.state, 'info', 'IDE tools are off for this session (Settings › AI › IDE Tools for Agents).');
      this.sessions.push(sess);
      this.active = info.key;
      void this.refreshAgents();
      this.render();
      this.input.focus();
      return sess;
    } catch (err) {
      this.starting = null;
      this.render();
      this.notify('error', `Cannot start ${a?.name ?? agentId}`, String(err instanceof Error ? err.message : err));
      return null;
    }
  }

  async send(text = this.input.value.trim()): Promise<void> {
    const s = this.current();
    if (!s || !text || s.state.busy || s.state.ended) return;
    const ctx = this.context(s);
    const { blocks, labels } = promptBlocks(text, ctx);
    addUser(s.state, text, labels);
    this.input.value = '';
    this.autosize();
    this.renderSuggestions();
    this.render();
    try {
      await agentApi.prompt(s.key, blocks);
    } catch (err) {
      endTurn(s.state, '', String(err instanceof Error ? err.message : err));
      this.render();
    }
  }

  async stop(): Promise<void> {
    const s = this.current();
    if (!s?.state.busy) return;
    try {
      await agentApi.cancel(s.key);
    } catch (err) {
      this.notify('error', 'Cannot stop the agent', String(err));
    }
  }

  private async setMode(mode: string): Promise<void> {
    const s = this.current();
    if (!s || !mode || mode === s.state.currentMode) return;
    try {
      await agentApi.setMode(s.key, mode);
      s.state.currentMode = mode;
    } catch (err) {
      this.notify('error', 'Cannot switch mode', String(err));
    }
    this.render();
  }

  private async closeSession(key: string): Promise<void> {
    const s = this.byKey(key);
    if (s?.state.busy && !(await confirmDialog('Close Session', 'The agent is still working. Stop it and close the session?', 'Close', true))) return;
    this.sessions = this.sessions.filter((x) => x.key !== key);
    if (this.active === key) this.active = this.sessions[this.sessions.length - 1]?.key ?? null;
    void agentApi.closeSession(key).catch(() => {});
    this.render();
  }

  async install(agentId: string): Promise<void> {
    try {
      this.installing.set(agentId, 'Starting npm…');
      this.render();
      await agentApi.install(agentId);
    } catch (err) {
      this.installing.delete(agentId);
      this.render();
      this.notify('error', 'Cannot install the agent', String(err instanceof Error ? err.message : err));
    }
  }

  /** Puts text (e.g. a question about the selection) in the prompt box. */
  prefill(text: string): void {
    this.input.value = text;
    this.autosize();
    this.input.focus();
    this.input.setSelectionRange(text.length, text.length);
  }

  focusInput(): void {
    this.input.focus();
  }

  // ---- backend events

  onUpdate(ev: AgentUpdateEvent): void {
    const s = this.byKey(ev.session);
    if (s && applyUpdate(s.state, ev.update)) this.schedule();
  }

  onTurn(ev: AgentTurnEvent): void {
    const s = this.byKey(ev.session);
    if (!s) return;
    endTurn(s.state, ev.stopReason, ev.error);
    this.render();
  }

  onState(ev: AgentStateEvent): void {
    if (ev.state === 'starting' || ev.state === 'ready') {
      if (this.starting?.agent === ev.agent) this.starting.message = ev.message;
    } else {
      for (const key of ev.sessions) {
        const s = this.byKey(key);
        if (s) endSession(s.state, ev.state === 'error' ? ev.message : 'The agent stopped.');
      }
      if (ev.state === 'error') this.notify('error', 'Agent error', ev.message);
      void this.refreshAgents();
    }
    this.render();
  }

  onTerminal(ev: AgentTerminalEvent): void {
    const t = this.terminals.get(ev.id) ?? { text: '', exit: null };
    t.text += ev.data;
    if (t.text.length > 200_000) t.text = t.text.slice(-150_000);
    if (ev.exit) t.exit = ev.exit;
    this.terminals.set(ev.id, t);
    this.schedule();
  }

  onInstall(ev: AgentInstallEvent): void {
    if (ev.done) {
      this.installing.delete(ev.agent);
      if (ev.error) this.notify('error', 'Agent install failed', ev.error);
      else this.notify('info', 'Agent installed', `${this.agents.find((a) => a.id === ev.agent)?.name ?? ev.agent} is ready.`);
      void this.refreshAgents();
    } else if (ev.line.trim()) {
      this.installing.set(ev.agent, ev.line.trim());
    }
    this.render();
  }

  /** A permission request: shows a card; resolves via the card's buttons. */
  onPermission(req: AgentRequestEvent): void {
    const s = this.byKey(req.session);
    if (!s) {
      void agentApi.respond(req.id, { outcome: 'cancelled' });
      return;
    }
    addPermission(s.state, req.id, req.params as PermissionParams);
    this.active = s.key;
    const st = this.wb.panels.state.peek().right;
    if (!st.visible || st.active !== 'agent') {
      this.wb.panels.show('agent');
      this.notify('info', `${s.info.agentName} is asking for permission`, (req.params as PermissionParams).toolCall?.title as string | undefined);
    }
    this.render();
  }

  private answer(s: Sess, e: PermissionEntry, optionId: string): void {
    if (e.answer) return;
    answerPermission(s.state, e.requestId, optionId);
    void agentApi.respond(e.requestId, optionId === 'cancelled' ? { outcome: 'cancelled' } : { outcome: 'selected', optionId });
    this.render();
  }

  // ---- context

  private context(s: Sess): Parameters<typeof promptBlocks>[1] {
    const path = editors.activePath.peek();
    const view = editors.activeView();
    let selection = null;
    if (path && view && !view.state.selection.main.empty) {
      const sel = view.state.selection.main;
      selection = { path, startLine: view.state.doc.lineAt(sel.from).number, endLine: view.state.doc.lineAt(sel.to).number, text: view.state.sliceDoc(sel.from, sel.to) };
    }
    const buf = path ? editors.buffers.get(path) : undefined;
    const file = this.attachFile && path && buf && !buf.binary ? { path, text: buf.text() } : null;
    return { file, selection, embedded: s.info.embeddedContext };
  }

  private renderChips(): void {
    const path = editors.activePath.peek();
    const view = editors.activeView();
    const chips: HTMLElement[] = [];
    if (path) {
      if (this.attachFile) {
        chips.push(
          h('span', { class: 'agent-chip', title: `${path} is sent with the prompt` }, baseName(path),
            h('button', { class: 'agent-chip-x', title: 'Don’t send the current file', onClick: () => { this.attachFile = false; this.renderChips(); } }, '×')),
        );
      } else {
        chips.push(h('button', { class: 'agent-chip off', title: 'Send the current file with the prompt', onClick: () => { this.attachFile = true; this.renderChips(); } }, '+ Current file'));
      }
    }
    if (path && view && !view.state.selection.main.empty) {
      const sel = view.state.selection.main;
      const a = view.state.doc.lineAt(sel.from).number;
      const b = view.state.doc.lineAt(sel.to).number;
      chips.push(h('span', { class: 'agent-chip sel', title: 'The selection is sent with the prompt' }, `Selection ${a === b ? `L${a}` : `L${a}–${b}`}`));
    }
    this.chips.replaceChildren(...chips);
  }

  // ---- composer

  private autosize(): void {
    this.input.style.height = 'auto';
    this.input.style.height = `${Math.min(Math.max(this.input.scrollHeight, 54), 220)}px`;
  }

  private suggestions(): { name: string; description: string }[] {
    const s = this.current();
    return s ? matchCommands(s.state.commands, this.input.value) : [];
  }

  private renderSuggestions(): void {
    const list = this.suggestions();
    this.suggest.hidden = list.length === 0;
    this.suggest.replaceChildren(
      ...list.map((c, i) =>
        h('div', { class: `agent-suggest-item${i === this.suggestion ? ' active' : ''}`, onMousedown: (e: Event) => { e.preventDefault(); this.pick(c.name); } },
          h('span', { class: 'agent-suggest-name' }, `/${c.name}`), h('span', { class: 'agent-suggest-desc' }, c.description)),
      ),
    );
  }

  private pick(name: string): void {
    this.input.value = `/${name} `;
    this.renderSuggestions();
    this.input.focus();
  }

  private onKey(e: KeyboardEvent): void {
    const list = this.suggestions();
    if (list.length && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      this.suggestion = (this.suggestion + (e.key === 'ArrowDown' ? 1 : -1) + list.length) % list.length;
      this.renderSuggestions();
      return;
    }
    if (list.length && (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey))) {
      e.preventDefault();
      this.pick(list[Math.min(this.suggestion, list.length - 1)].name);
      return;
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      void this.send();
    } else if (e.key === 'Escape' && !this.suggest.hidden) {
      e.preventDefault();
      this.suggest.hidden = true;
    }
  }

  // ---- rendering

  private schedule(): void {
    if (this.renderQueued) return;
    this.renderQueued = true;
    const run = (): void => {
      this.renderQueued = false;
      this.render();
    };
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(run);
    else setTimeout(run, 16);
  }

  private notify(level: 'info' | 'warn' | 'error', title: string, body?: string): void {
    this.wb.notifications.push({ id: `agent.${title}`, level, title, body });
  }

  render(): void {
    const s = this.current();
    this.renderTabs();
    this.modeSel.hidden = !s || s.state.modes.length === 0;
    if (s && s.state.modes.length) {
      this.modeSel.replaceChildren(...s.state.modes.map((m) => h('option', { value: m.id, title: m.description ?? '' }, m.name)));
      this.modeSel.value = s.state.currentMode;
    }
    const changeKey = s?.key ?? '';
    const nChanges = this.changes.list(changeKey).length;
    this.changesBtn.textContent = nChanges ? `Changes (${nChanges})` : 'Changes';
    this.changesBtn.classList.toggle('on', this.showChanges);
    this.changesBtn.hidden = !nChanges && !this.showChanges;

    if (s) {
      this.renderTranscript(s);
      this.renderPlan(s);
      const parts: HTMLElement[] = [this.transcript];
      if (s.state.plan.length) parts.push(this.planBox);
      if (this.showChanges) parts.push(this.renderChanges(changeKey));
      if (this.body.children.length !== parts.length || parts.some((p, i) => this.body.children[i] !== p)) this.body.replaceChildren(...parts);
    } else {
      this.renderStart();
      const parts: HTMLElement[] = [this.startView];
      if (this.showChanges) parts.push(this.renderChanges(''));
      this.body.replaceChildren(...parts);
    }

    const busy = !!s?.state.busy;
    this.sendBtn.textContent = busy ? 'Stop' : 'Send';
    this.sendBtn.classList.toggle('btn-danger', busy);
    this.sendBtn.classList.toggle('btn-primary', !busy);
    this.sendBtn.disabled = !s || (!!s.state.ended && !busy);
    this.input.disabled = !s || !!s.state.ended;
    this.status.textContent = !s ? '' : s.state.ended ? s.state.ended : busy ? `${s.info.agentName} is working…` : s.info.ideTools ? 'IDE tools on' : '';
    this.status.classList.toggle('busy', busy);
    this.onRender();
  }

  private renderTabs(): void {
    this.tabs.replaceChildren(
      ...this.sessions.map((s) =>
        h('div', { class: `agent-tab${s.key === this.active ? ' active' : ''}${s.state.busy ? ' busy' : ''}${s.state.ended ? ' ended' : ''}`, role: 'tab', title: s.state.title || s.label, onClick: () => { this.active = s.key; this.render(); } },
          h('span', { class: 'agent-tab-label' }, s.state.title || s.label),
          iconButton('close', 'Close session', () => void this.closeSession(s.key), 'agent-tab-close')),
      ),
    );
  }

  private renderStart(): void {
    const parts: HTMLElement[] = [];
    if (this.starting) {
      parts.push(h('div', { class: 'agent-starting' }, h('span', { class: 'agent-spinner' }), this.starting.message));
    }
    if (this.auth) {
      const a = this.auth;
      parts.push(
        h('div', { class: 'agent-card agent-auth' },
          h('div', { class: 'agent-card-title' }, `${a.name} needs you to log in`),
          ...a.methods.map((m) =>
            h('div', { class: 'agent-auth-method' },
              h('button', { class: 'btn btn-primary', onClick: () => void this.authenticate(a.agent, m.id, m.type ?? '') }, m.name),
              m.description ? h('span', { class: 'muted' }, m.description) : null),
          ),
          h('button', { class: 'btn', onClick: () => void this.start(a.agent) }, 'I logged in — try again')),
      );
    }
    if (!project.peek()) {
      parts.push(h('div', { class: 'muted' }, 'Open a project to use agents.'));
    } else {
      parts.push(h('div', { class: 'agent-start-title' }, 'Coding agents'), h('div', { class: 'muted agent-start-sub' }, 'Agents run on your machine and edit this project through Goi: you approve their actions, review their changes and can revert them.'));
      for (const a of this.agents) {
        const line = this.installing.get(a.id);
        parts.push(
          h('div', { class: `agent-card${a.available ? '' : ' unavailable'}` },
            h('div', { class: 'agent-card-head' },
              h('span', { class: 'agent-card-title' }, a.name),
              a.running ? h('span', { class: 'agent-badge' }, 'running') : null,
              h('span', { class: 'agent-spacer' }),
              a.available
                ? h('button', { class: 'btn btn-primary', disabled: !!this.starting, onClick: () => void this.start(a.id) }, 'Start')
                : a.installable
                  ? h('button', { class: 'btn', disabled: !!line, onClick: () => void this.install(a.id) }, line ? 'Installing…' : 'Install')
                  : null),
            h('div', { class: 'muted agent-card-desc' }, a.description),
            line ? h('div', { class: 'agent-install-line' }, line) : h('div', { class: 'agent-card-cmd', title: a.command }, a.available ? `${a.command}` : a.installable ? `npm package ${a.package}` : a.command)),
        );
      }
      parts.push(h('div', { class: 'muted agent-start-foot' }, 'Add other ACP agents in Settings › AI › Agent Commands.'));
    }
    this.startView.replaceChildren(...parts);
  }

  private async authenticate(agent: string, method: string, type: string): Promise<void> {
    try {
      if (type === 'terminal') {
        const cmd = await agentApi.authCommand(agent, method);
        if (!(await runInTerminal(this.wb, cmd))) this.notify('warn', 'Run this in a terminal to log in', cmd);
        return;
      }
      await agentApi.authenticate(agent, method);
      await this.start(agent);
    } catch (err) {
      this.notify('error', 'Login failed', String(err instanceof Error ? err.message : err));
    }
  }

  private renderPlan(s: Sess): void {
    const icon: Record<string, string> = { completed: '☑', in_progress: '◐', pending: '☐' };
    this.planBox.replaceChildren(
      h('div', { class: 'agent-plan-title' }, 'Plan'),
      ...s.state.plan.map((p) => h('div', { class: `agent-plan-item ${p.status}` }, h('span', { class: 'agent-plan-icon' }, icon[p.status] ?? '•'), p.content)),
    );
  }

  private renderChanges(key: string): HTMLElement {
    const list = this.changes.list(key);
    const root = project.peek()?.root ?? '';
    const rel = (p: string): string => (root && p.startsWith(root) ? p.slice(root.length).replace(/^[\\/]/, '') : p);
    this.changesBox.replaceChildren(
      h('div', { class: 'agent-changes-head' },
        h('span', { class: 'agent-changes-title' }, key ? 'Files changed in this session' : 'Files changed by external agents'),
        list.length ? h('button', { class: 'btn', onClick: () => void this.revertAll(key) }, 'Revert All') : null),
      list.length
        ? h('div', null, list.map((c) =>
            h('div', { class: 'agent-change' },
              h('a', { href: '#', class: 'agent-change-path', title: c.path, onClick: (e: Event) => { e.preventDefault(); void editors.open(c.path); } }, rel(c.path)),
              c.before === null ? h('span', { class: 'agent-badge new' }, 'new') : null,
              h('span', { class: 'agent-spacer' }),
              h('button', { class: 'btn', onClick: () => void openDiff({ path: c.path, mode: 'text', left: c.before ?? '', leftLabel: 'Before agent', rightLabel: 'Now' }) }, 'Diff'),
              h('button', { class: 'btn', onClick: () => void this.revert(key, c.path) }, 'Revert')),
          ))
        : h('div', { class: 'muted' }, 'No changes yet.'),
    );
    return this.changesBox;
  }

  private async revert(key: string, path: string): Promise<void> {
    const c = this.changes.get(key, path);
    if (!c) return;
    if (c.before === null && !(await confirmDialog('Revert', `The agent created ${baseName(path)}. Move it to the trash?`, 'Move to Trash', true))) return;
    try {
      await this.tools.revert(key, path);
    } catch (err) {
      this.notify('error', `Cannot revert ${baseName(path)}`, String(err));
    }
  }

  private async revertAll(key: string): Promise<void> {
    const list = this.changes.list(key);
    if (!(await confirmDialog('Revert All', `Restore ${list.length} file${list.length === 1 ? '' : 's'} to their content before the agent's changes? Files the agent created are moved to the trash.`, 'Revert All', true))) return;
    for (const c of list) {
      try {
        await this.tools.revert(key, c.path);
      } catch (err) {
        this.notify('error', `Cannot revert ${baseName(c.path)}`, String(err));
      }
    }
  }

  // ---- transcript

  private md(): MarkdownOptions {
    const root = project.peek()?.root ?? '';
    return {
      openLink: openExternal,
      openFile: (path, line) => {
        const abs = /^([A-Za-z]:)?[\\/]/.test(path) ? path : `${root}/${path}`;
        void editors.open(abs, { line });
      },
      copy: (code) => void navigator.clipboard?.writeText(code),
      insert: (code) => {
        const v = editors.activeView();
        if (v) {
          v.dispatch(v.state.replaceSelection(code));
          v.focus();
        }
      },
    };
  }

  private sig(e: Entry, s: Sess, last: boolean): string {
    switch (e.kind) {
      case 'agent':
      case 'thought':
        return `${e.text.length}:${last && s.state.busy}`;
      case 'tool':
        return `${e.status}|${e.title}|${e.content.length}|${e.locations.length}|${e.content.map((c) => (c.type === 'terminal' ? this.terminals.get(c.terminalId)?.text.length ?? 0 : 0)).join(',')}`;
      case 'permission':
        return e.answer ?? '';
      default:
        return '';
    }
  }

  private renderTranscript(s: Sess): void {
    const t = this.transcript;
    const atBottom = t.scrollHeight - t.scrollTop - t.clientHeight < 60;
    const wanted: HTMLElement[] = [];
    const entries = s.state.entries;
    entries.forEach((e, i) => {
      const sig = this.sig(e, s, i === entries.length - 1);
      let c = this.entryEls.get(e);
      if (!c || c.sig !== sig) {
        c = { el: this.renderEntry(e, s, i === entries.length - 1), sig };
        this.entryEls.set(e, c);
      }
      wanted.push(c.el);
    });
    if (!entries.length) wanted.push(h('div', { class: 'muted agent-empty' }, `Ask ${s.info.agentName} anything about this project.`));
    if (t.children.length !== wanted.length || wanted.some((el, i) => t.children[i] !== el)) t.replaceChildren(...wanted);
    if (atBottom) t.scrollTop = t.scrollHeight;
  }

  private renderEntry(e: Entry, s: Sess, last: boolean): HTMLElement {
    switch (e.kind) {
      case 'user':
        return h('div', { class: 'agent-msg user' },
          e.context.length ? h('div', { class: 'agent-msg-ctx' }, e.context.map((c) => h('span', { class: 'agent-chip' }, c))) : null,
          h('div', { class: 'agent-user-text' }, e.text));
      case 'agent':
        return h('div', { class: 'agent-msg agent' }, renderMarkdown(e.text, this.md()));
      case 'thought': {
        const d = h('details', { class: 'agent-thought' }, h('summary', null, last && s.state.busy ? 'Thinking…' : 'Thought'), renderMarkdown(e.text, this.md()));
        if (last && s.state.busy) d.open = true;
        return d;
      }
      case 'tool':
        return this.renderTool(e);
      case 'permission':
        return this.renderPermission(e, s);
      case 'turn': {
        const label = e.error ? e.error : stopLabel(e.stopReason);
        return h('div', { class: `agent-turn${e.error ? ' error' : ''}`, hidden: !label }, label);
      }
      case 'notice':
        return h('div', { class: `agent-notice ${e.level}` }, e.text);
    }
  }

  private renderContent(c: ToolContent): HTMLElement {
    if (c.type === 'diff') {
      const lines = lineDiff(c.oldText ?? '', c.newText);
      const st = diffStats(lines);
      const rows = compact(lines, 2);
      const shown = rows.slice(0, 80);
      return h('div', { class: 'agent-diff' },
        h('div', { class: 'agent-diff-head' },
          h('span', { class: 'agent-diff-path', title: c.path }, baseName(c.path)),
          h('span', { class: 'agent-diff-stat add' }, `+${st.added}`),
          h('span', { class: 'agent-diff-stat del' }, `−${st.removed}`),
          h('span', { class: 'agent-spacer' }),
          h('button', { class: 'btn', onClick: () => void openDiff({ path: c.path, mode: 'text', left: c.oldText ?? '', leftLabel: 'Before', rightLabel: 'Now' }) }, 'Open Diff')),
        h('pre', { class: 'agent-diff-body' }, shown.map((r) => h('div', { class: `agent-diff-line ${r.op === '+' ? 'add' : r.op === '-' ? 'del' : r.op === '…' ? 'gap' : ''}` }, r.op === '…' ? `⋯ ${r.text}` : `${r.op} ${r.text}`)),
          rows.length > shown.length ? h('div', { class: 'agent-diff-line gap' }, `⋯ ${rows.length - shown.length} more lines`) : null));
    }
    if (c.type === 'terminal') {
      const t = this.terminals.get(c.terminalId);
      const exit = t?.exit ? (t.exit.signal ? `killed by ${t.exit.signal}` : `exit code ${t.exit.exitCode}`) : 'running…';
      const pre = h('pre', { class: 'agent-term' }, t?.text.slice(-20000) || '');
      queueMicrotask(() => (pre.scrollTop = pre.scrollHeight));
      return h('div', { class: 'agent-term-wrap' }, pre, h('div', { class: `agent-term-exit${t?.exit && t.exit.exitCode !== 0 ? ' failed' : ''}` }, exit));
    }
    const text = blockText(c.content);
    return h('div', { class: 'agent-tool-text' }, renderMarkdown(text.length > 8000 ? text.slice(0, 8000) + '\n…' : text, this.md()));
  }

  private renderTool(e: ToolEntry): HTMLElement {
    const root = project.peek()?.root ?? '';
    const rel = (p: string): string => (root && p.startsWith(root) ? p.slice(root.length).replace(/^[\\/]/, '') : p);
    const body = e.content.map((c) => this.renderContent(c));
    const head = h('summary', { class: 'agent-tool-head' },
      h('span', { class: 'agent-tool-glyph' }, KIND_GLYPH[e.toolKind] ?? '•'),
      h('span', { class: 'agent-tool-title' }, e.title),
      h('span', { class: `agent-tool-status ${e.status}` }, e.status.replace('_', ' ')));
    const locs = e.locations.length
      ? h('div', { class: 'agent-tool-locs' }, e.locations.map((l) =>
          h('a', { href: '#', onClick: (ev: Event) => { ev.preventDefault(); void editors.open(l.path, { line: l.line ?? undefined }); } }, `${rel(l.path)}${l.line ? `:${l.line}` : ''}`)))
      : null;
    const d = h('details', { class: `agent-tool ${e.status}` }, head, locs, ...body);
    // Diffs, terminals and running calls are open; finished reads/searches fold away.
    d.open = e.status === 'pending' || e.status === 'in_progress' || e.status === 'failed' || e.content.some((c) => c.type !== 'content');
    return d;
  }

  private renderPermission(e: PermissionEntry, s: Sess): HTMLElement {
    const chosen = e.answer === 'cancelled' ? 'Cancelled' : e.options.find((o) => o.optionId === e.answer)?.name;
    return h('div', { class: `agent-perm${e.answer ? ' answered' : ''}` },
      h('div', { class: 'agent-perm-title' }, h('span', { class: 'agent-tool-glyph' }, KIND_GLYPH[e.toolKind] ?? '•'), e.title),
      ...e.content.map((c) => this.renderContent(c)),
      e.answer
        ? h('div', { class: 'agent-perm-answer' }, chosen ?? e.answer)
        : h('div', { class: 'agent-perm-buttons' },
            e.options.map((o) =>
              h('button', { class: `btn${o.kind === 'allow_once' ? ' btn-primary' : o.kind.startsWith('reject') ? ' btn-danger' : ''}`, onClick: () => this.answer(s, e, o.optionId) }, o.name)),
            e.options.length ? null : h('button', { class: 'btn', onClick: () => this.answer(s, e, 'cancelled') }, 'Dismiss')));
  }
}
