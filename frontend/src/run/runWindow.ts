// The Run tool window (R2.2): one tab per launch with a console and a
// toolbar (rerun, stop, soft wrap, scroll to end, clear, search). Test runs
// get the test tree beside the console (testView.ts plugs in through
// setTestViewFactory).

import { project, settings } from '../app/state';
import { h } from '../lib/dom';
import { on, run, Topics } from '../lib/api';
import type { RunInfo, RunOutputEvent, RunTestEvent, TestEvent } from '../lib/api';
import { effect, signal } from '../lib/store';
import { Console } from './console';
import { iconButton, icon } from './icons';
import { openLocation } from './open';
import { exitLine, notifyError, workbench } from './state';

/** The test UI attached to a test run's tab. */
export interface TestView {
  el: HTMLElement;
  /** Extra toolbar buttons (rerun failed, filters). */
  tools: HTMLElement[];
  reset(info: RunInfo): void;
  events(evs: TestEvent[]): void;
  finished(info: RunInfo): void;
  dispose(): void;
}

export type TestViewFactory = (tab: RunTab) => TestView;

let testViewFactory: TestViewFactory | null = null;

export function setTestViewFactory(f: TestViewFactory): void {
  testViewFactory = f;
}

export interface RunTab {
  id: string;
  info: RunInfo;
  console: Console;
  el: HTMLElement;
  tab: HTMLElement;
  tools: HTMLElement;
  tests: TestView | null;
}

export const runTabs = signal<RunTab[]>([]);
export const activeRunId = signal<string>('');

export function activeTab(): RunTab | undefined {
  return runTabs.peek().find((t) => t.id === activeRunId.peek());
}

const stateLabel: Record<string, string> = { starting: 'Starting', building: 'Building', running: 'Running', exited: 'Finished', failed: 'Failed' };

function isLive(info: RunInfo): boolean {
  return info.state === 'starting' || info.state === 'building' || info.state === 'running';
}

/** Base directory for relative locations in a run's output. */
export function runBaseDir(info: RunInfo): string {
  const root = project.peek()?.root ?? '';
  const c = info.config;
  const rel = c.workingDir || c.directory || (c.package.startsWith('.') ? c.package : '') || (c.files[0] ? c.files[0].replace(/[\\/][^\\/]*$/, '') : '');
  if (!rel || rel === '.') return root;
  if (/^([A-Za-z]:)?[\\/]/.test(rel)) return rel;
  return `${root}/${rel.replace(/^\.\//, '')}`;
}

function outputLimit(): number {
  return settings.peek()?.run?.outputLimitLines || 50000;
}

function createTab(info: RunInfo, host: { strip: HTMLElement; body: HTMLElement }): RunTab {
  const tabRef: { t?: RunTab } = {};
  const con = new Console({
    maxLines: outputLimit(),
    onInput: (text) => {
      run.input(info.id, text + '\n').catch((err) => notifyError('run-input', 'Cannot send input', err));
    },
    onLocation: (loc) => void openLocation(loc, runBaseDir(tabRef.t?.info ?? info)),
  });
  const tools = h('div', { class: 'run-tools' });
  const content = h('div', { class: 'run-content' });
  const el = h('div', { class: 'run-tab-body', dataset: { run: info.id } }, tools, content);
  const label = h('span', { class: 'run-tab-label' });
  const status = h('span', { class: 'run-tab-status' });
  const close = iconButton('close', 'Close tab', () => void closeTab(info.id), 'run-tab-close');
  const tab = h('div', { class: 'run-tab', role: 'tab', dataset: { run: info.id }, onClick: () => activeRunId.set(info.id) }, status, label, close);
  tab.addEventListener('auxclick', (e) => {
    if (e.button === 1) void closeTab(info.id);
  });
  const t: RunTab = { id: info.id, info, console: con, el, tab, tools, tests: null };
  tabRef.t = t;

  tools.append(
    iconButton('rerun', 'Rerun', () => void rerunTab(t), 'tb-rerun'),
    iconButton('stop', 'Stop', () => void stopTab(t), 'tb-stop'),
  );
  if (info.test && testViewFactory) {
    t.tests = testViewFactory(t);
    tools.append(...t.tests.tools);
    content.append(h('div', { class: 'run-split' }, t.tests.el, con.el));
  } else {
    content.append(con.el);
  }
  const wrap = iconButton('wrap', 'Soft-wrap output', () => {
    con.setSoftWrap(!con.softWrap);
    wrap.classList.toggle('on', con.softWrap);
  });
  tools.append(
    h('span', { class: 'tb-sep' }),
    wrap,
    iconButton('scrollEnd', 'Scroll to end', () => con.scrollToEnd()),
    iconButton('clear', 'Clear output', () => con.clear()),
    iconButton('search', 'Search output', () => con.toggleSearch()),
  );
  host.strip.append(tab);
  host.body.append(el);
  return t;
}

function updateTab(t: RunTab): void {
  const info = t.info;
  const label = t.tab.querySelector('.run-tab-label') as HTMLElement;
  label.textContent = info.config.name;
  t.tab.title = `${info.config.name}: ${stateLabel[info.state] ?? info.state}${info.message ? ` (${info.message})` : ''}`;
  const status = t.tab.querySelector('.run-tab-status') as HTMLElement;
  const live = isLive(info);
  const failed = info.state === 'failed' || (info.state === 'exited' && !!info.exit && (info.exit.code !== 0 || !!info.exit.signal));
  status.replaceChildren(icon(live ? 'running' : failed ? 'failed' : 'passed', live ? 'st-running' : failed ? 'st-failed' : 'st-passed'));
  (t.tools.querySelector('.tb-stop') as HTMLButtonElement).disabled = !live;
  t.console.setInputEnabled(info.state === 'running' && !info.config.inputFile && info.config.type !== 'test');
}

async function closeTab(id: string): Promise<void> {
  const t = runTabs.peek().find((x) => x.id === id);
  if (!t) return;
  try {
    await run.close(id);
  } catch {
    // the backend may already have forgotten the run
  }
  t.tests?.dispose();
  t.tab.remove();
  t.el.remove();
  const rest = runTabs.peek().filter((x) => x.id !== id);
  runTabs.set(rest);
  if (activeRunId.peek() === id) activeRunId.set(rest[rest.length - 1]?.id ?? '');
}

export async function rerunTab(t: RunTab | undefined = activeTab()): Promise<void> {
  if (!t) return;
  try {
    await run.rerun(t.id);
  } catch (err) {
    notifyError('rerun', 'Cannot rerun', err);
  }
}

export async function stopTab(t: RunTab | undefined = activeTab()): Promise<void> {
  if (!t) return;
  try {
    await run.stop(t.id);
  } catch (err) {
    notifyError('stop', 'Cannot stop', err);
  }
}

/** The most recent live run, preferring the active tab. */
export function liveTab(): RunTab | undefined {
  const a = activeTab();
  if (a && isLive(a.info)) return a;
  return [...runTabs.peek()].reverse().find((t) => isLive(t.info));
}

export function renderRunWindow(container: HTMLElement): () => void {
  const strip = h('div', { class: 'run-tabs', role: 'tablist' });
  const body = h('div', { class: 'run-bodies' });
  const empty = h('div', { class: 'results-empty' }, 'Run a configuration (Ctrl+R) or click ▶ in the gutter');
  container.append(h('div', { class: 'run-window' }, strip, body, empty));
  const host = { strip, body };

  const byId = (id: string): RunTab | undefined => runTabs.peek().find((t) => t.id === id);

  const onState = (info: RunInfo): void => {
    if (info.debug) return;
    let t = byId(info.id);
    if (!t) {
      t = createTab(info, host);
      runTabs.set([...runTabs.peek(), t]);
      activeRunId.set(info.id);
      workbench().panels.show('run');
    } else if (info.state === 'starting') {
      t.console.clear();
      activeRunId.set(info.id);
    }
    const prev = t.info;
    t.info = info;
    if (info.state === 'starting') t.tests?.reset(info);
    if ((info.state === 'exited' || info.state === 'failed') && prev.state !== info.state) {
      if (info.exit) t.console.append('system', `\n${exitLine(info.exit.code, info.exit.signal, info.elapsedMs)}\n`);
      else if (info.message) t.console.append('system', `\n${info.message}\n`);
      t.tests?.finished(info);
    }
    updateTab(t);
  };

  const offs = [
    on<RunInfo>(Topics.runState, onState),
    on<RunOutputEvent>(Topics.runOutput, (ev) => {
      const t = byId(ev.id);
      if (!t) return;
      for (const c of ev.chunks) t.console.append(c.stream, c.text);
    }),
    on<RunTestEvent>(Topics.runTest, (ev) => {
      const t = byId(ev.id);
      if (!t) return;
      if (t.tests) t.tests.events(ev.events);
      else for (const e of ev.events) if (e.output) t.console.append('stdout', e.output);
    }),
  ];

  // Restore runs that are still known to the backend (frontend reload).
  void run.runs().then((list) => {
    for (const info of list.sort((a, b) => a.id.localeCompare(b.id, undefined, { numeric: true }))) {
      if (!byId(info.id) && !info.debug) onState(info);
    }
  });

  const stop = effect(() => {
    const active = activeRunId();
    const tabs = runTabs();
    empty.hidden = tabs.length > 0;
    for (const t of tabs) {
      t.tab.classList.toggle('active', t.id === active);
      t.tab.setAttribute('aria-selected', String(t.id === active));
      t.el.hidden = t.id !== active;
    }
  });
  return () => {
    for (const off of offs) off();
    stop();
  };
}
