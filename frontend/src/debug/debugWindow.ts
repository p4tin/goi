// The Debug tool window (R5.3–R9): one tab per debug session with a toolbar,
// goroutines (grouping, filter, hide, thread ids), frames, variables,
// watches, a console, and the editor's execution line and inline values.

import { showMenu } from '../app/menu';
import { project, settings } from '../app/state';
import { editors } from '../editor/install';
import { debug, on, run, settingsApi, Topics } from '../lib/api';
import type { DapStackFrame, DebugStateEvent, RunInfo, RunOutputEvent } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { Console } from '../run/console';
import { icon, iconButton } from '../run/icons';
import { openLocation } from '../run/open';
import { runBaseDir } from '../run/runWindow';
import { exitLine, notifyError, workbench } from '../run/state';
import { showDebugPosition, currentDebugPosition } from './inline';
import { filterGoroutines, formatValue, goroutineLabel, groupByFunction, inlineValues, inProject, parseGoroutine } from './model';
import type { Goroutine } from './model';
import { VarTree } from './variables';
import type { VarNode } from './variables';

export interface DebugTab {
  id: string;
  info: RunInfo;
  state: DebugStateEvent | null;
  goroutines: Goroutine[];
  threadId: number;
  frames: DapStackFrame[];
  frameIndex: number;
  filter: string;
  hidden: Set<number>;
  groupBy: boolean;
  collapsedGroups: Set<string>;
  /** Bumped on every stop/resume to drop stale async results. */
  seq: number;
  console: Console;
  el: HTMLElement;
  tab: HTMLElement;
  tools: Record<string, HTMLButtonElement>;
  status: HTMLElement;
  exception: HTMLElement;
  goroutineList: HTMLElement;
  frameList: HTMLElement;
  vars: VarTree;
  watchTree: VarTree;
}

export const debugTabs = signal<DebugTab[]>([]);
export const activeDebugId = signal('');
/** Watch expressions (persisted per project). */
export const watches = signal<string[]>([]);
export const showThreads = signal(false);

export function activeDebugTab(): DebugTab | undefined {
  return debugTabs.peek().find((t) => t.id === activeDebugId.peek());
}

const opt = {
  showTypes: (): boolean => settings.peek()?.debug?.showTypes ?? true,
  showAddresses: (): boolean => settings.peek()?.debug?.showAddresses ?? false,
  showUnreadable: (): boolean => settings.peek()?.debug?.showUnreadable ?? true,
  inlineValues: (): boolean => settings.peek()?.debug?.inlineValues ?? true,
};

function isLive(t: DebugTab): boolean {
  return t.info.state === 'starting' || t.info.state === 'building' || t.info.state === 'running';
}

function isStopped(t: DebugTab): boolean {
  return isLive(t) && t.state?.state === 'stopped';
}

/** The session commands act on: the active tab, else the newest live one. */
export function targetTab(): DebugTab | undefined {
  const a = activeDebugTab();
  if (a && isLive(a)) return a;
  return [...debugTabs.peek()].reverse().find(isLive);
}

function currentFrame(t: DebugTab): DapStackFrame | undefined {
  return t.frames[t.frameIndex];
}

// ---- Actions

async function act(name: string, fn: (t: DebugTab) => Promise<unknown>, needStopped = true): Promise<void> {
  const t = targetTab();
  if (!t) return;
  if (needStopped && !isStopped(t)) return;
  try {
    await fn(t);
  } catch (err) {
    notifyError('debug', `${name} failed`, err);
  }
}

export const actions = {
  resume: () => act('Resume', (t) => debug.continue(t.id, t.threadId)),
  pause: () => act('Pause', (t) => debug.pause(t.id, t.threadId || 1), false),
  stepOver: () => act('Step Over', (t) => debug.next(t.id, t.threadId)),
  stepInto: () => act('Step Into', (t) => debug.stepIn(t.id, t.threadId)),
  // Delve has no "skip library code" stepping, so Force Step Into is Step Into.
  forceStepInto: () => act('Force Step Into', (t) => debug.stepIn(t.id, t.threadId)),
  stepOut: () => act('Step Out', (t) => debug.stepOut(t.id, t.threadId)),
  runToCursor: () =>
    act('Run to Cursor', async (t) => {
      const buf = editors.activeBuffer();
      const view = editors.activeView();
      if (!buf || !view) return;
      await debug.runToCursor(t.id, t.threadId, buf.path, view.state.doc.lineAt(view.state.selection.main.head).number);
    }),
  stop: () => act('Stop', (t) => debug.stop(t.id), false),
  restart: () => act('Restart', (t) => run.rerun(t.id), false),
  setNextStatement: () =>
    act('Set Next Statement', async (t) => {
      if (!t.state?.caps.supportsGotoTargets) {
        workbench().notifications.push({ id: 'debug-goto', level: 'info', title: 'Set Next Statement is not available', body: 'Delve does not support moving the execution point.' });
      }
    }),
};

// ---- Rendering helpers

function renderStatus(t: DebugTab): void {
  const st = t.state;
  let text = '';
  if (t.info.state === 'exited' || t.info.state === 'failed' || st?.state === 'exited') {
    text = t.info.exit ? exitLine(t.info.exit.code, t.info.exit.signal, t.info.elapsedMs) : t.info.message || 'Session ended';
  } else if (t.info.state === 'building' || st?.state === 'starting' || !st) {
    text = t.info.state === 'building' ? 'Building and starting the debugger…' : 'Starting…';
  } else if (st.state === 'running') {
    text = 'Running';
  } else {
    const f = t.frames[0];
    const where = f ? ` in ${f.name}${f.source?.name ? ` (${f.source.name}:${f.line})` : ''}` : '';
    const reason = st.reason === 'breakpoint' || st.reason === 'function breakpoint' ? 'Breakpoint hit' : st.reason === 'exception' || st.reason === 'panic' ? 'Panic' : st.reason === 'step' ? 'Stepped' : st.reason === 'pause' ? 'Paused' : `Stopped (${st.reason})`;
    text = `${reason}${where}, goroutine ${t.threadId}`;
  }
  t.status.textContent = text;
  const stopped = isStopped(t);
  const live = isLive(t);
  for (const k of ['resume', 'stepOver', 'stepInto', 'forceStepInto', 'stepOut', 'runToCursor', 'evaluate']) t.tools[k].disabled = !stopped;
  t.tools.pause.disabled = !(live && st?.state === 'running');
  t.tools.stop.disabled = !live;
  const tabStatus = t.tab.querySelector('.run-tab-status') as HTMLElement;
  tabStatus.replaceChildren(icon(live ? (stopped ? 'pause' : 'running') : 'debug', live ? 'st-running' : ''));
}

function renderGoroutines(t: DebugTab): void {
  const list = filterGoroutines(t.goroutines, t.filter, t.hidden);
  const row = (g: Goroutine, indent: number): HTMLElement =>
    h(
      'div',
      {
        class: `dbg-row${g.id === t.threadId ? ' selected' : ''}${g.current ? ' current' : ''}`,
        dataset: { goroutine: String(g.id) },
        style: `padding-left:${6 + indent * 14}px`,
        title: g.name,
      },
      h('span', { class: 'dbg-mark' }, g.current ? '▶' : ''),
      h('span', null, goroutineLabel(g, showThreads.peek())),
    );
  const rows: HTMLElement[] = [];
  if (t.groupBy) {
    for (const grp of groupByFunction(list)) {
      const open = !t.collapsedGroups.has(grp.func);
      rows.push(
        h(
          'div',
          { class: 'dbg-row dbg-group', dataset: { group: grp.func } },
          h('span', { class: 'vt-twisty' }, open ? '▾' : '▸'),
          h('span', null, grp.func),
          h('span', { class: 'dbg-count' }, String(grp.items.length)),
        ),
      );
      if (open) for (const g of grp.items) rows.push(row(g, 1));
    }
  } else {
    for (const g of [...list].sort((a, b) => a.id - b.id)) rows.push(row(g, 0));
  }
  const hiddenCount = t.goroutines.filter((g) => t.hidden.has(g.id) && !g.current).length;
  if (!rows.length) rows.push(h('div', { class: 'results-empty' }, t.goroutines.length ? 'No goroutines match the filter' : isStopped(t) ? 'No goroutines' : 'Goroutines appear when the program is suspended'));
  if (hiddenCount) rows.push(h('div', { class: 'dbg-hidden-note' }, `${hiddenCount} hidden`));
  t.goroutineList.replaceChildren(...rows);
}

function renderFrames(t: DebugTab): void {
  const root = project.peek()?.root ?? '';
  const rows = t.frames.map((f, i) =>
    h(
      'div',
      {
        class: `dbg-row${i === t.frameIndex ? ' selected' : ''}${inProject(f.source?.path, root) ? '' : ' muted'}`,
        dataset: { frame: String(i) },
        title: f.source?.path ? `${f.source.path}:${f.line}` : f.name,
      },
      h('span', { class: 'dbg-frame-name' }, f.name),
      f.source?.name ? h('span', { class: 'dbg-frame-loc' }, `${f.source.name}:${f.line}`) : null,
    ),
  );
  if (!rows.length) rows.push(h('div', { class: 'results-empty' }, isStopped(t) ? 'No frames' : 'Frames appear when the program is suspended'));
  t.frameList.replaceChildren(...rows);
}

// ---- Stop handling

async function loadFrames(t: DebugTab, seq: number): Promise<void> {
  try {
    const st = await debug.stackTrace(t.id, t.threadId, 0, 100);
    if (seq !== t.seq) return;
    t.frames = st.stackFrames;
  } catch (err) {
    if (seq !== t.seq) return;
    t.frames = [];
    t.status.textContent = `Cannot load frames: ${err instanceof Error ? err.message : String(err)}`;
  }
  // Start at the first frame with source (skip runtime frames without files).
  t.frameIndex = Math.max(0, t.frames.findIndex((f) => !!f.source?.path));
  renderFrames(t);
}

async function selectFrame(t: DebugTab, index: number, reveal: boolean): Promise<void> {
  const seq = t.seq;
  t.frameIndex = index;
  renderFrames(t);
  const f = currentFrame(t);
  if (!f) {
    t.vars.clear('No frame');
    return;
  }
  const path = f.source?.path;
  if (path && reveal) {
    try {
      await editors.open(path, { line: f.line, focus: false });
    } catch {
      // the file may not be readable (e.g. outside the project)
    }
  }
  let roots: VarNode[] = [];
  const values = new Map<string, string>();
  try {
    const scopes = await debug.scopes(t.id, f.id);
    if (seq !== t.seq) return;
    roots = scopes.map((s) => ({
      key: `scope:${s.name}`,
      name: s.name,
      value: '',
      type: '',
      evaluateName: '',
      ref: s.variablesReference,
      parentRef: 0,
      children: null,
      expanded: false,
      loading: false,
      error: '',
      kind: 'scope' as const,
      watchIndex: -1,
    }));
    for (const s of scopes.filter((x) => !x.expensive).slice(0, 2)) {
      for (const v of await debug.variables(t.id, s.variablesReference)) {
        if (!values.has(v.name)) values.set(v.name, formatValue(v.value, { showAddresses: opt.showAddresses() }));
      }
    }
  } catch (err) {
    t.vars.clear(`Cannot load variables: ${err instanceof Error ? err.message : String(err)}`);
  }
  if (seq !== t.seq) return;
  t.vars.setRoots(roots);
  if (path) {
    const buf = editors.buffers.get(path);
    const lines = buf ? buf.text().split('\n') : [];
    showDebugPosition({ path, line: f.line, kind: index === 0 || index === t.frames.findIndex((x) => !!x.source?.path) ? 'top' : 'frame', values: opt.inlineValues() && lines.length ? inlineValues(lines, f.line, values) : new Map() });
  }
  await refreshWatches(t);
}

export async function refreshWatches(t: DebugTab | undefined = targetTab()): Promise<void> {
  if (!t) return;
  const f = currentFrame(t);
  const list = watches.peek();
  const nodes: VarNode[] = await Promise.all(
    list.map(async (expr, i): Promise<VarNode> => {
      const base: VarNode = { key: `watch:${i}:${expr}`, name: expr, value: '', type: '', evaluateName: expr, ref: 0, parentRef: 0, children: null, expanded: false, loading: false, error: '', kind: 'watch', watchIndex: i };
      if (!f || !isStopped(t)) return { ...base, value: isLive(t) ? '' : 'not available' };
      try {
        const r = await debug.evaluate(t.id, expr, f.id, 'watch');
        return { ...base, value: r.result, type: r.type, ref: r.variablesReference };
      } catch (err) {
        return { ...base, error: (err instanceof Error ? err.message : String(err)).replace(/^dap: evaluate: /, '') };
      }
    }),
  );
  t.watchTree.setRoots(nodes);
}

async function onStopped(t: DebugTab, ev: DebugStateEvent): Promise<void> {
  const seq = ++t.seq;
  t.state = ev;
  t.exception.hidden = true;
  try {
    t.goroutines = (await debug.threads(t.id)).map(parseGoroutine);
  } catch {
    t.goroutines = [];
  }
  if (seq !== t.seq) return;
  t.threadId = ev.threadId || t.goroutines.find((g) => g.current)?.id || t.goroutines[0]?.id || 0;
  renderGoroutines(t);
  await loadFrames(t, seq);
  if (seq !== t.seq) return;
  renderStatus(t);
  if (ev.reason === 'exception' || ev.reason === 'panic') {
    try {
      const info = await debug.exceptionInfo(t.id, t.threadId);
      t.exception.textContent = `${info.exceptionId}: ${info.description}`;
      t.exception.hidden = false;
    } catch {
      if (ev.description || ev.text) {
        t.exception.textContent = ev.text || ev.description;
        t.exception.hidden = false;
      }
    }
  }
  await selectFrame(t, t.frameIndex, true);
  activeDebugId.set(t.id);
  ensureHeight(360);
  workbench().panels.show('debug');
}

/** Grows the bottom dock so the debugger's lists are usable. */
export function ensureHeight(px: number): void {
  const p = workbench().panels;
  const avail = window.innerHeight * 0.7;
  if (p.state.peek().bottom.size < Math.min(px, avail)) p.patch('bottom', { size: Math.round(Math.min(px, avail)) });
}

function onRunning(t: DebugTab, ev: DebugStateEvent): void {
  t.seq++;
  t.state = ev;
  t.frames = [];
  t.goroutines = [];
  t.exception.hidden = true;
  renderFrames(t);
  renderGoroutines(t);
  t.vars.clear('The program is running');
  void refreshWatches(t);
  const pos = currentDebugPosition();
  if (pos) showDebugPosition(null);
  renderStatus(t);
}

// ---- Tab creation

function splitPane(cls: string, a: HTMLElement, b: HTMLElement): HTMLElement {
  return h('div', { class: `dbg-split ${cls}` }, a, b);
}

function section(title: string, tools: Node[], body: HTMLElement): HTMLElement {
  return h('div', { class: 'dbg-section' }, h('div', { class: 'dbg-section-head' }, h('span', null, title), h('span', { class: 'dbg-section-tools' }, tools)), body);
}

async function toggleSetting(key: 'showTypes' | 'showAddresses' | 'showUnreadable' | 'inlineValues', current: boolean): Promise<void> {
  try {
    await settingsApi.set('user', `debug.${key}`, !current);
  } catch (err) {
    notifyError('debug-setting', 'Cannot change setting', err);
  }
}

function createTab(info: RunInfo, host: { strip: HTMLElement; body: HTMLElement }, evaluateFn: (t: DebugTab) => void): DebugTab {
  const ref: { t?: DebugTab } = {};
  const con = new Console({
    maxLines: settings.peek()?.run?.outputLimitLines || 50000,
    onLocation: (loc) => void openLocation(loc, runBaseDir(ref.t?.info ?? info)),
  });
  const status = h('div', { class: 'dbg-status' });
  const exception = h('div', { class: 'dbg-exception', hidden: true });
  const goroutineList = h('div', { class: 'dbg-list', role: 'listbox', tabindex: '0' });
  const frameList = h('div', { class: 'dbg-list', role: 'listbox', tabindex: '0' });
  const sid = (): string => info.id;
  const vars = new VarTree({
    sessionId: sid,
    showTypes: opt.showTypes,
    showAddresses: opt.showAddresses,
    showUnreadable: opt.showUnreadable,
    canSetValue: () => !!ref.t?.state?.caps.supportsSetVariable && !!ref.t && isStopped(ref.t),
    frameId: () => (ref.t ? currentFrame(ref.t)?.id ?? 0 : 0),
    onAddWatch: (expr) => addWatch(expr),
    onChanged: () => {
      if (ref.t) void selectFrame(ref.t, ref.t.frameIndex, false);
    },
    emptyText: 'Variables appear when the program is suspended',
  });
  const watchTree = new VarTree({
    sessionId: sid,
    showTypes: opt.showTypes,
    showAddresses: opt.showAddresses,
    showUnreadable: () => true,
    canSetValue: () => false,
    frameId: () => (ref.t ? currentFrame(ref.t)?.id ?? 0 : 0),
    onRemoveWatch: (i) => watches.set(watches.peek().filter((_, j) => j !== i)),
    onEditWatch: (i) => void editWatch(i),
    emptyText: 'No watches. Add one below or with Add to Watches.',
  });
  const watchInput = h('input', { class: 'input dbg-watch-input', type: 'text', placeholder: 'Add watch expression (Enter)', spellcheck: false });
  watchInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && watchInput.value.trim()) {
      addWatch(watchInput.value.trim());
      watchInput.value = '';
    }
  });

  const filter = h('input', { class: 'input dbg-filter', type: 'search', placeholder: 'Filter goroutines', spellcheck: false, 'aria-label': 'Filter goroutines' });
  const groupBtn = iconButton('collapseAll', 'Group by function', () => {
    if (!ref.t) return;
    ref.t.groupBy = !ref.t.groupBy;
    groupBtn.classList.toggle('on', ref.t.groupBy);
    renderGoroutines(ref.t);
  });
  groupBtn.classList.add('on');
  const threadsBtn = iconButton('settings', 'Show thread IDs', () => showThreads.set(!showThreads.peek()));
  const resetBtn = iconButton('eye', 'Reset hidden goroutines', () => {
    if (!ref.t) return;
    ref.t.hidden.clear();
    renderGoroutines(ref.t);
  });

  const typesBtn = iconButton('filter', 'Show types', () => void toggleSetting('showTypes', opt.showTypes()));
  const addrBtn = iconButton('search', 'Show pointer addresses', () => void toggleSetting('showAddresses', opt.showAddresses()));
  const unreadBtn = iconButton('mute', 'Show unreadable variables', () => void toggleSetting('showUnreadable', opt.showUnreadable()));
  const inlineBtn = iconButton('eye', 'Show inline values in the editor', () => void toggleSetting('inlineValues', opt.inlineValues()));
  const syncToggles = (): void => {
    typesBtn.classList.toggle('on', opt.showTypes());
    addrBtn.classList.toggle('on', opt.showAddresses());
    unreadBtn.classList.toggle('on', opt.showUnreadable());
    inlineBtn.classList.toggle('on', opt.inlineValues());
    threadsBtn.classList.toggle('on', showThreads.peek());
  };

  const debuggerView = h(
    'div',
    { class: 'dbg-main' },
    h('div', { class: 'dbg-top' }, status, exception),
    splitPane(
      'dbg-columns',
      splitPane(
        'dbg-rows',
        section('Goroutines', [groupBtn, threadsBtn, resetBtn], h('div', { class: 'dbg-col' }, filter, goroutineList)),
        section('Frames', [], frameList),
      ),
      splitPane('dbg-rows', section('Variables', [typesBtn, addrBtn, unreadBtn, inlineBtn], vars.el), section('Watches', [], h('div', { class: 'dbg-col' }, watchTree.el, watchInput))),
    ),
  );
  const viewTabs = h('div', { class: 'dbg-view-tabs', role: 'tablist' });
  const views = h('div', { class: 'dbg-views' }, debuggerView, con.el);
  const showView = (which: 'debugger' | 'console'): void => {
    debuggerView.hidden = which !== 'debugger';
    con.el.hidden = which !== 'console';
    for (const b of viewTabs.children) b.classList.toggle('active', (b as HTMLElement).dataset.view === which);
  };
  viewTabs.append(
    h('button', { class: 'dbg-view-tab active', type: 'button', dataset: { view: 'debugger' }, onClick: () => showView('debugger') }, 'Debugger'),
    h('button', { class: 'dbg-view-tab', type: 'button', dataset: { view: 'console' }, onClick: () => showView('console') }, 'Console'),
  );
  showView('debugger');

  const tools: Record<string, HTMLButtonElement> = {
    restart: iconButton('rerun', 'Restart (Rerun)', () => void run.rerun(info.id), 'tb-rerun'),
    resume: iconButton('resume', 'Resume Program', () => void actions.resume(), 'tb-run'),
    pause: iconButton('pause', 'Pause Program', () => void actions.pause()),
    stop: iconButton('stop', 'Stop', () => void debug.stop(info.id), 'tb-stop'),
    stepOver: iconButton('stepOver', 'Step Over', () => void actions.stepOver()),
    stepInto: iconButton('stepInto', 'Step Into', () => void actions.stepInto()),
    forceStepInto: iconButton('forceStepInto', 'Force Step Into', () => void actions.forceStepInto()),
    stepOut: iconButton('stepOut', 'Step Out', () => void actions.stepOut()),
    runToCursor: iconButton('runToCursor', 'Run to Cursor', () => void actions.runToCursor()),
    evaluate: iconButton('search', 'Evaluate Expression', () => ref.t && evaluateFn(ref.t)),
    breakpoints: iconButton('breakpoints', 'View Breakpoints', () => void import('./breakpoints').then((m) => m.viewBreakpoints())),
  };
  const toolCol = h(
    'div',
    { class: 'run-tools' },
    tools.restart,
    tools.resume,
    tools.pause,
    tools.stop,
    h('span', { class: 'tb-sep' }),
    tools.stepOver,
    tools.stepInto,
    tools.forceStepInto,
    tools.stepOut,
    tools.runToCursor,
    h('span', { class: 'tb-sep' }),
    tools.evaluate,
    tools.breakpoints,
  );
  const el = h('div', { class: 'run-tab-body dbg-tab-body', dataset: { debug: info.id } }, toolCol, h('div', { class: 'dbg-content' }, viewTabs, views));
  const label = h('span', { class: 'run-tab-label' }, info.config.name);
  const tabStatus = h('span', { class: 'run-tab-status' });
  const close = iconButton('close', 'Close tab', () => void closeTab(info.id), 'run-tab-close');
  const tab = h('div', { class: 'run-tab', role: 'tab', onClick: () => activeDebugId.set(info.id) }, tabStatus, label, close);
  const t: DebugTab = {
    id: info.id,
    info,
    state: null,
    goroutines: [],
    threadId: 0,
    frames: [],
    frameIndex: 0,
    filter: '',
    hidden: new Set(),
    groupBy: true,
    collapsedGroups: new Set(),
    seq: 0,
    console: con,
    el,
    tab,
    tools,
    status,
    exception,
    goroutineList,
    frameList,
    vars,
    watchTree,
  };
  ref.t = t;

  filter.addEventListener('input', () => {
    t.filter = filter.value;
    renderGoroutines(t);
  });
  goroutineList.addEventListener('click', (e) => {
    const r = (e.target as HTMLElement).closest('.dbg-row') as HTMLElement | null;
    if (!r) return;
    if (r.dataset.group !== undefined) {
      const g = r.dataset.group;
      if (t.collapsedGroups.has(g)) t.collapsedGroups.delete(g);
      else t.collapsedGroups.add(g);
      renderGoroutines(t);
      return;
    }
    const id = Number(r.dataset.goroutine);
    if (!id || id === t.threadId) return;
    t.threadId = id;
    renderGoroutines(t);
    const seq = ++t.seq;
    void loadFrames(t, seq).then(() => {
      if (seq === t.seq) {
        renderStatus(t);
        void selectFrame(t, t.frameIndex, true);
      }
    });
  });
  goroutineList.addEventListener('contextmenu', (e) => {
    const r = (e.target as HTMLElement).closest('.dbg-row') as HTMLElement | null;
    if (!r) return;
    e.preventDefault();
    const group = r.dataset.group;
    const id = Number(r.dataset.goroutine);
    const g = t.goroutines.find((x) => x.id === id);
    showMenu(e.clientX, e.clientY, [
      ...(group !== undefined
        ? [{ label: `Hide Goroutines in ${group}`, run: () => (t.goroutines.filter((x) => x.func === group).forEach((x) => t.hidden.add(x.id)), renderGoroutines(t)) }]
        : [
            { label: `Hide Goroutine ${id}`, disabled: !!g?.current, run: () => (t.hidden.add(id), renderGoroutines(t)) },
            { label: `Hide Goroutines in ${g?.func ?? ''}`, run: () => (t.goroutines.filter((x) => x.func === g?.func && !x.current).forEach((x) => t.hidden.add(x.id)), renderGoroutines(t)) },
            { label: 'Copy', run: () => void navigator.clipboard?.writeText(g?.name ?? '') },
          ]),
      { separator: true },
      { label: 'Reset Hidden', disabled: t.hidden.size === 0, run: () => (t.hidden.clear(), renderGoroutines(t)) },
    ]);
  });
  frameList.addEventListener('click', (e) => {
    const r = (e.target as HTMLElement).closest('.dbg-row') as HTMLElement | null;
    if (!r || r.dataset.frame === undefined) return;
    void selectFrame(t, Number(r.dataset.frame), true);
  });

  effect(() => {
    settings();
    showThreads();
    syncToggles();
    vars.render();
    watchTree.render();
    renderGoroutines(t);
  });

  host.strip.append(tab);
  host.body.append(el);
  return t;
}

export function addWatch(expr: string): void {
  if (!expr.trim()) return;
  watches.set([...watches.peek(), expr.trim()]);
  workbench().panels.show('debug');
}

async function editWatch(i: number): Promise<void> {
  const { promptDialog } = await import('../app/dialog');
  const v = await promptDialog('Edit Watch', watches.peek()[i] ?? '');
  if (v === null) return;
  watches.set(watches.peek().map((w, j) => (j === i ? v : w)));
}

async function closeTab(id: string): Promise<void> {
  const t = debugTabs.peek().find((x) => x.id === id);
  if (!t) return;
  try {
    await run.close(id);
  } catch {
    // already gone
  }
  if (currentDebugPosition() && isLive(t)) showDebugPosition(null);
  t.tab.remove();
  t.el.remove();
  const rest = debugTabs.peek().filter((x) => x.id !== id);
  debugTabs.set(rest);
  if (activeDebugId.peek() === id) activeDebugId.set(rest[rest.length - 1]?.id ?? '');
}

let root: HTMLElement | null = null;

/** Builds the Debug window and subscribes to events at install time (see
 * installRunWindow); the panel attaches the element when it renders. */
export function installDebugWindow(evaluateFn: (t: DebugTab) => void): void {
  const strip = h('div', { class: 'run-tabs', role: 'tablist' });
  const body = h('div', { class: 'run-bodies' });
  const empty = h('div', { class: 'results-empty' }, 'Debug a configuration (Ctrl+D) or click ▶ in the gutter and choose Debug');
  root = h('div', { class: 'run-window' }, strip, body, empty);
  const host = { strip, body };
  const byId = (id: string): DebugTab | undefined => debugTabs.peek().find((t) => t.id === id);

  const onRunState = (info: RunInfo): void => {
    if (!info.debug) return;
    let t = byId(info.id);
    if (!t) {
      t = createTab(info, host, evaluateFn);
      debugTabs.set([...debugTabs.peek(), t]);
      activeDebugId.set(info.id);
      workbench().panels.show('debug');
    } else if (info.state === 'starting') {
      t.console.clear();
      t.state = null;
      t.frames = [];
      t.goroutines = [];
      t.seq++;
      renderFrames(t);
      renderGoroutines(t);
      t.vars.clear('Variables appear when the program is suspended');
      activeDebugId.set(info.id);
    }
    const prev = t.info.state;
    t.info = info;
    if ((info.state === 'exited' || info.state === 'failed') && prev !== info.state) {
      t.seq++;
      if (info.exit) t.console.append('system', `\n${exitLine(info.exit.code, info.exit.signal, info.elapsedMs)}\n`);
      else if (info.message) t.console.append('system', `\n${info.message}\n`);
      if (info.state === 'failed') {
        // Show the console with the build/launch error.
        (t.el.querySelector('.dbg-view-tab[data-view="console"]') as HTMLElement | null)?.click();
      }
      t.frames = [];
      t.goroutines = [];
      renderFrames(t);
      renderGoroutines(t);
      t.vars.clear('The session has ended');
      void refreshWatches(t);
      if (activeDebugId.peek() === t.id || !debugTabs.peek().some(isLive)) showDebugPosition(null);
    }
    renderStatus(t);
  };

  on<RunInfo>(Topics.runState, onRunState);
  on<RunOutputEvent>(Topics.runOutput, (ev) => {
    const t = byId(ev.id);
    if (t) for (const c of ev.chunks) t.console.append(c.stream, c.text);
  });
  on<DebugStateEvent>(Topics.debugState, (ev) => {
    const t = byId(ev.id);
    if (!t) return;
    if (ev.state === 'stopped') void onStopped(t, ev);
    else if (ev.state === 'running') onRunning(t, ev);
    else {
      t.state = ev;
      renderStatus(t);
    }
  });
  effect(() => {
    if (!project()) {
      for (const t of debugTabs.peek()) {
        t.tab.remove();
        t.el.remove();
      }
      debugTabs.set([]);
      activeDebugId.set('');
      showDebugPosition(null);
      return;
    }
    void run.runs().then((list) => {
      for (const info of list.sort((a, b) => a.id.localeCompare(b.id, undefined, { numeric: true }))) if (info.debug && !byId(info.id)) onRunState(info);
    });
  });
  effect(() => {
    const active = activeDebugId();
    const tabs = debugTabs();
    empty.hidden = tabs.length > 0;
    for (const t of tabs) {
      t.tab.classList.toggle('active', t.id === active);
      t.el.hidden = t.id !== active;
    }
  });
  effect(() => {
    watches();
    for (const t of debugTabs.peek()) void refreshWatches(t);
  });
}

export function renderDebugWindow(container: HTMLElement): void {
  if (root) container.append(root);
}
