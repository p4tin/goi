// Integrated terminal (R10): xterm.js tabs backed by backend PTYs, themed
// from the UI tokens, with clickable file locations (resolved against the
// directory the shell reports through OSC 7) and URLs, search, copy/paste,
// and "press a key to close" after the shell exits.

import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { Terminal } from '@xterm/xterm';
import type { ILink, ILinkProvider, ITheme } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { promptDialog } from '../app/dialog';
import { project, resolvedTheme, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { on, openExternal, terminalApi, Topics } from '../lib/api';
import type { TermExitEvent, TermOutputEvent } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { iconButton } from '../run/icons';
import { findLocations } from '../run/links';
import { openLocation } from '../run/open';
import './terminal.css';

interface TermTab {
  id: string;
  title: string;
  term: Terminal;
  fit: FitAddon;
  search: SearchAddon;
  el: HTMLElement;
  tab: HTMLElement;
  cwd: string;
  exited: boolean;
  /** Output received before the xterm was attached. */
  queued: string[];
}

const tabs = signal<TermTab[]>([]);
const activeId = signal('');

/** Parses an OSC 7 payload ("file://host/path") into a path. */
export function parseOsc7(data: string): string | null {
  const m = /^file:\/\/[^/]*(\/.*)$/.exec(data.trim());
  if (!m) return null;
  try {
    let p = decodeURIComponent(m[1]);
    // Windows: "/C:/Users/x" → "C:/Users/x".
    if (/^\/[A-Za-z]:\//.test(p)) p = p.slice(1);
    return p;
  } catch {
    return null;
  }
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** xterm theme from the --goi-* tokens. */
export function terminalTheme(): ITheme {
  const v = (k: string): string => cssVar(`--goi-${k}`) || '';
  return {
    background: v('bg'),
    foreground: v('fg'),
    cursor: v('fg'),
    cursorAccent: v('bg'),
    selectionBackground: v('selection'),
    black: v('ansi-black'),
    red: v('ansi-red'),
    green: v('ansi-green'),
    yellow: v('ansi-yellow'),
    blue: v('ansi-blue'),
    magenta: v('ansi-magenta'),
    cyan: v('ansi-cyan'),
    white: v('ansi-white'),
    brightBlack: v('ansi-bright-black'),
    brightRed: v('ansi-bright-red'),
    brightGreen: v('ansi-bright-green'),
    brightYellow: v('ansi-bright-yellow'),
    brightBlue: v('ansi-bright-blue'),
    brightMagenta: v('ansi-bright-magenta'),
    brightCyan: v('ansi-bright-cyan'),
    brightWhite: v('ansi-bright-white'),
  };
}

function fontOptions(): { fontFamily: string; fontSize: number; scrollback: number } {
  const s = settings.peek();
  return {
    fontFamily: s?.editor?.fontFamily || 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: s?.terminal?.fontSize || s?.editor?.fontSize || 13,
    scrollback: s?.terminal?.scrollback || 10000,
  };
}

/** Link provider for "file.go:12:3" locations in terminal lines. */
function locationLinks(term: Terminal, t: () => TermTab | undefined): ILinkProvider {
  return {
    provideLinks(y, callback) {
      const line = term.buffer.active.getLine(y - 1)?.translateToString(true) ?? '';
      const links: ILink[] = findLocations(line).map((loc) => ({
        range: { start: { x: loc.start + 1, y }, end: { x: loc.end, y } },
        text: line.slice(loc.start, loc.end),
        decorations: { underline: true, pointerCursor: true },
        activate: () => void openLocation(loc, t()?.cwd ?? project.peek()?.root ?? ''),
      }));
      callback(links.length ? links : undefined);
    },
  };
}

function createTab(info: { id: string; shell: string; cwd: string }, host: { strip: HTMLElement; body: HTMLElement }): TermTab {
  const fo = fontOptions();
  const term = new Terminal({
    ...fo,
    theme: terminalTheme(),
    cursorBlink: true,
    allowProposedApi: true,
    macOptionIsMeta: false,
    convertEol: false,
  });
  const fit = new FitAddon();
  const search = new SearchAddon();
  term.loadAddon(fit);
  term.loadAddon(search);
  term.loadAddon(new WebLinksAddon((_e, uri) => openExternal(uri)));
  const el = h('div', { class: 'term-body', dataset: { term: info.id } });
  const shellName = info.shell.split(/[\\/]/).pop()?.replace(/\.exe$/i, '') || 'shell';
  const label = h('span', { class: 'run-tab-label' }, shellName);
  const close = iconButton('close', 'Close terminal', () => void closeTab(info.id), 'run-tab-close');
  const tab = h('div', { class: 'run-tab', role: 'tab', onClick: () => activate(info.id) }, label, close);
  const t: TermTab = { id: info.id, title: shellName, term, fit, search, el, tab, cwd: info.cwd, exited: false, queued: [] };
  tab.addEventListener('dblclick', async () => {
    const name = await promptDialog('Rename Terminal', t.title);
    if (name) {
      t.title = name;
      label.textContent = name;
    }
  });
  term.registerLinkProvider(locationLinks(term, () => t));
  term.parser.registerOscHandler(7, (data) => {
    const p = parseOsc7(data);
    if (p) t.cwd = p;
    return true;
  });
  term.onTitleChange((title) => {
    if (title && t.title === shellName) label.textContent = title;
  });
  term.onData((d) => {
    if (t.exited) {
      void closeTab(t.id);
      return;
    }
    void terminalApi.write(t.id, d).catch(() => {});
  });
  term.attachCustomKeyEventHandler((e) => {
    if (e.type !== 'keydown') return true;
    const mod = navigator.platform.toLowerCase().includes('mac') ? e.metaKey : e.ctrlKey && e.shiftKey;
    if (mod && e.code === 'KeyC' && term.hasSelection()) {
      void navigator.clipboard?.writeText(term.getSelection());
      return false;
    }
    if (mod && e.code === 'KeyV') {
      void navigator.clipboard?.readText().then((text) => text && term.paste(text));
      return false;
    }
    if (mod && e.code === 'KeyF') {
      toggleSearch(t, true);
      return false;
    }
    if (mod && e.code === 'KeyK' && !e.shiftKey) {
      term.clear();
      return false;
    }
    return true;
  });
  host.strip.insertBefore(tab, host.strip.lastElementChild);
  host.body.append(el);
  return t;
}

let searchBar: HTMLElement | null = null;
let searchInput: HTMLInputElement | null = null;

function toggleSearch(t: TermTab | undefined, show: boolean): void {
  if (!searchBar || !searchInput) return;
  searchBar.hidden = !show;
  if (show) {
    searchInput.focus();
    searchInput.select();
  } else {
    t?.search.clearDecorations();
    t?.term.focus();
  }
}

function activeTab(): TermTab | undefined {
  return tabs.peek().find((t) => t.id === activeId.peek());
}

function activate(id: string): void {
  activeId.set(id);
  const t = activeTab();
  if (t) requestAnimationFrame(() => {
    fitTab(t);
    t.term.focus();
  });
}

function fitTab(t: TermTab): void {
  if (!t.el.isConnected || t.el.hidden || !t.el.clientWidth) return;
  try {
    t.fit.fit();
  } catch {
    return;
  }
  if (!t.exited) void terminalApi.resize(t.id, t.term.cols, t.term.rows).catch(() => {});
}

async function closeTab(id: string): Promise<void> {
  const t = tabs.peek().find((x) => x.id === id);
  if (!t) return;
  if (!t.exited) await terminalApi.close(id).catch(() => {});
  t.term.dispose();
  t.tab.remove();
  t.el.remove();
  const rest = tabs.peek().filter((x) => x.id !== id);
  tabs.set(rest);
  if (activeId.peek() === id) {
    const next = rest[rest.length - 1];
    if (next) activate(next.id);
    else activeId.set('');
  }
}

let hostRef: { strip: HTMLElement; body: HTMLElement } | null = null;
const pending = new Map<string, string[]>();

export async function newTerminal(wb: Workbench): Promise<void> {
  if (!project.peek()) return;
  wb.panels.show('terminal');
  // The panel renders on first show; wait a frame for the host.
  for (let i = 0; i < 10 && !hostRef; i++) await new Promise((r) => requestAnimationFrame(r));
  if (!hostRef) return;
  try {
    const info = await terminalApi.open(80, 24);
    const t = createTab(info, hostRef);
    tabs.set([...tabs.peek(), t]);
    t.term.open(t.el);
    for (const d of pending.get(info.id) ?? []) t.term.write(d);
    pending.delete(info.id);
    activate(info.id);
  } catch (err) {
    wb.notifications.push({ id: 'terminal', level: 'error', title: 'Cannot start terminal', body: err instanceof Error ? err.message : String(err) });
  }
}

function renderTerminalPanel(wb: Workbench, container: HTMLElement): () => void {
  const add = iconButton('plus', 'New terminal tab', () => void newTerminal(wb), 'term-add');
  const strip = h('div', { class: 'run-tabs term-tabs', role: 'tablist' }, add);
  const body = h('div', { class: 'term-bodies' });
  searchInput = h('input', { class: 'input', type: 'search', placeholder: 'Search terminal', spellcheck: false, 'aria-label': 'Search terminal' });
  searchBar = h('div', { class: 'con-search term-search', hidden: true }, searchInput);
  searchInput.addEventListener('keydown', (e) => {
    const t = activeTab();
    if (!t || !searchInput) return;
    if (e.key === 'Enter') {
      e.preventDefault();
      if (e.shiftKey) t.search.findPrevious(searchInput.value);
      else t.search.findNext(searchInput.value);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      toggleSearch(t, false);
    }
  });
  const empty = h('div', { class: 'results-empty' }, 'No terminal. Click + to start one.');
  container.append(h('div', { class: 'run-window term-window' }, strip, searchBar, body, empty));
  hostRef = { strip, body };

  const ro = new ResizeObserver(() => {
    const t = activeTab();
    if (t) fitTab(t);
  });
  ro.observe(body);

  const stopFx = effect(() => {
    const active = activeId();
    const list = tabs();
    empty.hidden = list.length > 0;
    for (const t of list) {
      t.tab.classList.toggle('active', t.id === active);
      t.el.hidden = t.id !== active;
    }
  });
  const themeFx = effect(() => {
    resolvedTheme();
    settings();
    // Tokens are applied to :root in the same tick; read them next frame.
    requestAnimationFrame(() => {
      const theme = terminalTheme();
      const fo = fontOptions();
      for (const t of tabs.peek()) {
        t.term.options.theme = theme;
        t.term.options.fontFamily = fo.fontFamily;
        t.term.options.fontSize = fo.fontSize;
        fitTab(t);
      }
    });
  });
  if (!tabs.peek().length) void newTerminal(wb);
  return () => {
    ro.disconnect();
    stopFx();
    themeFx();
    hostRef = null;
  };
}

export function installTerminal(wb: Workbench): void {

  wb.panels.register({ id: 'terminal', title: 'Terminal', dock: 'bottom', order: 8, render: (c) => renderTerminalPanel(wb, c) });
  on<TermOutputEvent>(Topics.termOutput, (ev) => {
    const t = tabs.peek().find((x) => x.id === ev.id);
    if (t) t.term.write(ev.data);
    else pending.set(ev.id, [...(pending.get(ev.id) ?? []), ev.data]);
  });
  on<TermExitEvent>(Topics.termExit, (ev) => {
    const t = tabs.peek().find((x) => x.id === ev.id);
    if (!t) return;
    t.exited = true;
    t.term.write(`\r\n\x1b[2m[Process exited with code ${ev.code}. Press any key to close.]\x1b[0m\r\n`);
    t.tab.classList.add('exited');
  });
  // Terminals belong to the project: drop tabs when it closes.
  effect(() => {
    if (!project()) for (const t of tabs.peek()) void closeTab(t.id);
  });
  commands.register({
    id: 'terminal.toggle',
    title: 'Terminal',
    category: 'View',
    when: () => !!project.peek(),
    run: () => {
      const st = wb.panels.state.peek().bottom;
      const focused = !!document.activeElement?.closest('.xterm');
      if (st.visible && st.active === 'terminal' && focused) {
        wb.panels.toggle('bottom');
        return;
      }
      wb.panels.show('terminal');
      const t = activeTab();
      if (t) activate(t.id);
    },
  });
  commands.register({ id: 'terminal.new', title: 'New Terminal Tab', category: 'View', when: () => !!project.peek(), run: () => newTerminal(wb) });
  commands.register({
    id: 'terminal.close',
    title: 'Close Terminal Tab',
    category: 'View',
    when: () => !!activeTab(),
    run: async () => {
      const t = activeTab();
      if (t) await closeTab(t.id);
    },
  });
}

/** Number of live terminals (for the close confirmation). */
export function liveTerminals(): number {
  return tabs.peek().filter((t) => !t.exited).length;
}
