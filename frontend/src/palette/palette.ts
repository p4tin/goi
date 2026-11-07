// Search Everywhere: one modal for files, actions and settings.

import './palette.css';
import type { Workbench } from '../app/workbench';
import { editors } from '../editor/install';
import { search } from '../lib/api';
import type { FileMatch } from '../lib/api';
import { commands } from '../lib/commands';
import type { Command } from '../lib/commands';
import { h } from '../lib/dom';
import { createDoubleShift } from './doubleShift';
import { rankItems } from './rank';
import { installRecentFiles, recentFiles } from './recentFiles';

export type PaletteTab = 'all' | 'files' | 'symbols' | 'actions' | 'settings';

const TABS: { id: PaletteTab; title: string }[] = [
  { id: 'all', title: 'All' },
  { id: 'files', title: 'Files' },
  { id: 'symbols', title: 'Symbols' },
  { id: 'actions', title: 'Actions' },
  { id: 'settings', title: 'Settings' },
];

const SECTION_LIMIT = 10;
const TAB_LIMIT = 50;
const FILE_DEBOUNCE_MS = 30;
const SYMBOL_DEBOUNCE_MS = 120;

/** A symbol search result supplied by the language feature. */
export interface SymbolEntry {
  label: string;
  detail: string;
  kind?: string;
  run: () => void;
}

/** Returns symbols for a query; throws (with a user-facing message) when
 * symbol search is unavailable. */
export type SymbolProvider = (query: string) => Promise<SymbolEntry[]>;
let symbolProvider: SymbolProvider | null = null;

/** Lets the language feature supply workspace symbols (R10). */
export function setSymbolProvider(fn: SymbolProvider): void {
  symbolProvider = fn;
}

export interface SettingEntry {
  key: string;
  title: string;
}

/** M1 settings reachable from the palette. */
export const SETTINGS: SettingEntry[] = [
  { key: 'editor.fontFamily', title: 'Editor: Font Family' },
  { key: 'editor.fontSize', title: 'Editor: Font Size' },
  { key: 'editor.tabSize', title: 'Editor: Tab Size' },
  { key: 'editor.softWrap', title: 'Editor: Soft Wrap' },
  { key: 'editor.maxHighlightSizeKB', title: 'Editor: Max Highlighted File Size' },
  { key: 'editor.maxOpenSizeKB', title: 'Editor: Max Editable File Size' },
  { key: 'save.format', title: 'Save: Format on Save' },
  { key: 'ui.theme', title: 'Appearance: Theme' },
  { key: 'ui.scheme', title: 'Appearance: Editor Color Scheme' },
  { key: 'ui.keymap', title: 'Keymap' },
  { key: 'go.goroot', title: 'Go: SDK (GOROOT)' },
  { key: 'go.env', title: 'Go: Environment Variables' },
  { key: 'go.tags', title: 'Go: Build Tags' },
  { key: 'go.goos', title: 'Go: Target OS (GOOS)' },
  { key: 'go.goarch', title: 'Go: Target Architecture (GOARCH)' },
  { key: 'go.cgo', title: 'Go: CGO_ENABLED' },
  { key: 'go.vendor', title: 'Go: Vendoring Mode' },
  { key: 'project.excludes', title: 'Project: Excluded Folders' },
  { key: 'project.contentRoots', title: 'Project: Content Roots' },
  { key: 'startup.reopenLastProject', title: 'Startup: Reopen Last Project' },
];

type KeyHintProvider = (commandId: string) => string | undefined;
let keyHint: KeyHintProvider = () => undefined;

/** Lets the keymap feature supply shortcut labels for actions. */
export function setKeyHintProvider(fn: KeyHintProvider): void {
  keyHint = fn;
}

export function actionTitle(c: Command): string {
  return c.category ? `${c.category}: ${c.title}` : c.title;
}

/** Splits a slash path into basename and directory (no trailing slash). */
export function splitRel(rel: string): { dir: string; base: string; baseStart: number } {
  const i = rel.lastIndexOf('/');
  return { dir: i >= 0 ? rel.slice(0, i) : '', base: rel.slice(i + 1), baseStart: i + 1 };
}

/** Renders text with the characters at positions wrapped in <mark>. */
export function highlight(text: string, positions: readonly number[], offset = 0): Node[] {
  const set = new Set(positions.map((p) => p - offset).filter((p) => p >= 0 && p < text.length));
  if (set.size === 0) return [document.createTextNode(text)];
  const out: Node[] = [];
  let run = '';
  let marked = false;
  const flush = (): void => {
    if (!run) return;
    out.push(marked ? h('mark', null, run) : document.createTextNode(run));
    run = '';
  };
  for (let i = 0; i < text.length; i++) {
    const m = set.has(i);
    if (m !== marked) {
      flush();
      marked = m;
    }
    run += text[i];
  }
  flush();
  return out;
}

interface Row {
  kind: 'item' | 'header' | 'info';
  render: () => Node[];
  hint?: string;
  run?: () => void;
}

interface Palette {
  open(tab: PaletteTab, opts?: { title?: string }): void;
  close(): void;
  isOpen(): boolean;
}

export function installPalette(wb: Workbench): void {
  installRecentFiles(wb, editors.activePath);
  const palette = createPalette(wb);

  const reg = (id: string, title: string, tab: PaletteTab, extra?: { title?: string }): void => {
    commands.register({ id, title, category: 'Navigate', run: () => palette.open(tab, extra) });
  };
  reg('nav.searchEverywhere', 'Search Everywhere', 'all');
  reg('nav.gotoFile', 'Go to File…', 'files');
  reg('nav.gotoAction', 'Find Action…', 'actions');
  reg('nav.recentFiles', 'Recent Files', 'files', { title: 'Recent Files' });
  reg('nav.gotoSymbol', 'Go to Symbol…', 'symbols');

  const ds = createDoubleShift(() => palette.open('all'));
  const onKey = (e: KeyboardEvent): void => ds.handle({ type: e.type as 'keydown' | 'keyup', key: e.key, repeat: e.repeat });
  document.addEventListener('keydown', onKey, true);
  document.addEventListener('keyup', onKey, true);
}

function createPalette(wb: Workbench): Palette {
  let root: HTMLElement | null = null;
  let tab: PaletteTab = 'all';
  let rows: Row[] = [];
  let selected = -1;
  let fileTimer: ReturnType<typeof setTimeout> | null = null;
  let fileReq = 0;
  let fileRows: Row[] = [];
  let fileInfo = '';
  let symTimer: ReturnType<typeof setTimeout> | null = null;
  let symReq = 0;
  let symRows: Row[] = [];
  let symInfo = '';
  let prevFocus: HTMLElement | null = null;
  let titleOverride: string | undefined;

  const input = h('input', {
    class: 'palette-input',
    type: 'text',
    spellcheck: false,
    placeholder: 'Search files, symbols, actions and settings',
    'aria-label': 'Search',
  });
  const tabBar = h('div', { class: 'palette-tabs', role: 'tablist' });
  const title = h('div', { class: 'palette-title' });
  const list = h('ul', { class: 'palette-list', role: 'listbox' });

  const runSelected = (): void => {
    const r = rows[selected];
    if (r?.run) {
      close();
      r.run();
    }
  };

  input.addEventListener('input', () => refresh());
  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      move(e.key === 'ArrowDown' ? 1 : -1);
    } else if (e.key === 'Enter') {
      runSelected();
    } else if (e.key === 'Escape') {
      close();
    } else if (e.key === 'Tab') {
      const i = TABS.findIndex((t) => t.id === tab);
      setTab(TABS[(i + (e.shiftKey ? -1 : 1) + TABS.length) % TABS.length].id);
    } else {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
  });

  function move(delta: number): void {
    if (rows.length === 0) return;
    let i = selected;
    for (let n = 0; n < rows.length; n++) {
      i = (i + delta + rows.length) % rows.length;
      if (rows[i].run) break;
    }
    select(i);
  }

  function select(i: number): void {
    selected = i;
    list.querySelectorAll('.palette-row').forEach((el, idx) => {
      el.classList.toggle('selected', idx === i);
      el.setAttribute('aria-selected', String(idx === i));
      if (idx === i) (el as HTMLElement).scrollIntoView?.({ block: 'nearest' });
    });
  }

  function setTab(t: PaletteTab): void {
    tab = t;
    titleOverride = undefined;
    refresh();
  }

  function renderTabs(): void {
    tabBar.replaceChildren(
      ...TABS.map((t) =>
        h(
          'button',
          {
            class: `palette-tab${t.id === tab ? ' active' : ''}`,
            role: 'tab',
            tabIndex: -1,
            'aria-selected': String(t.id === tab),
            onMousedown: (e: MouseEvent) => {
              e.preventDefault();
              setTab(t.id);
              input.focus();
            },
          },
          t.title,
        ),
      ),
    );
    title.textContent = titleOverride ?? '';
    title.hidden = !titleOverride;
  }

  function actionRows(q: string, limit: number): Row[] {
    const cmds = commands.all().filter((c) => c.id !== 'nav.searchEverywhere');
    const enabled = cmds.filter((c) => commands.isEnabled(c.id));
    return rankItems(q, enabled, actionTitle)
      .slice(0, limit)
      .map((r) => ({
        kind: 'item' as const,
        render: () => [h('span', { class: 'palette-label' }, highlight(actionTitle(r.item), r.positions))],
        hint: keyHint(r.item.id),
        run: () => void commands.run(r.item.id),
      }));
  }

  function settingRows(q: string, limit: number): Row[] {
    return rankItems(q, SETTINGS, (s) => s.title)
      .slice(0, limit)
      .map((r) => ({
        kind: 'item' as const,
        render: () => [
          h('span', { class: 'palette-label' }, highlight(r.item.title, r.positions)),
          h('span', { class: 'palette-detail' }, r.item.key),
        ],
        run: () => {
          if (commands.get('settings.open')) void commands.run('settings.open', r.item.key);
          else wb.notifications.push({ id: 'palette.settings', level: 'info', title: 'Settings UI coming soon', body: r.item.key });
        },
      }));
  }

  function fileRow(m: FileMatch): Row {
    const { dir, base, baseStart } = splitRel(m.rel);
    return {
      kind: 'item',
      render: () => [
        h('span', { class: 'palette-label' }, highlight(base, m.positions ?? [], baseStart)),
        h('span', { class: 'palette-detail' }, highlight(dir, m.positions ?? [])),
      ],
      run: () => void editors.open(m.path),
    };
  }

  function scheduleFiles(q: string): void {
    if (fileTimer) clearTimeout(fileTimer);
    const req = ++fileReq;
    fileTimer = setTimeout(async () => {
      fileTimer = null;
      try {
        const res = await search.files(q, TAB_LIMIT, recentFiles.peek());
        if (req !== fileReq || !root) return;
        fileRows = (res.matches ?? []).map(fileRow);
        fileInfo = !res.index?.ready ? 'Indexing…' : fileRows.length === 0 ? (q ? 'No matching files' : 'No recent files') : '';
      } catch (err) {
        if (req !== fileReq || !root) return;
        fileRows = [];
        fileInfo = String(err);
      }
      build();
    }, FILE_DEBOUNCE_MS);
  }

  function symbolRow(e: SymbolEntry): Row {
    return {
      kind: 'item',
      render: () => [
        e.kind ? h('span', { class: 'palette-kind' }, e.kind) : null,
        h('span', { class: 'palette-label' }, e.label),
        h('span', { class: 'palette-detail' }, e.detail),
      ].filter((n): n is HTMLElement => n !== null),
      run: e.run,
    };
  }

  function scheduleSymbols(q: string): void {
    if (symTimer) clearTimeout(symTimer);
    const req = ++symReq;
    if (!q.trim()) {
      symRows = [];
      symInfo = 'Type to search symbols';
      return;
    }
    if (!symbolProvider) {
      symRows = [];
      symInfo = 'Symbol search is not available';
      return;
    }
    const provider = symbolProvider;
    symTimer = setTimeout(async () => {
      symTimer = null;
      try {
        const res = await provider(q);
        if (req !== symReq || !root) return;
        symRows = res.slice(0, TAB_LIMIT).map(symbolRow);
        symInfo = symRows.length ? '' : 'No matching symbols';
      } catch (err) {
        if (req !== symReq || !root) return;
        symRows = [];
        symInfo = err instanceof Error ? err.message : String(err);
      }
      build();
    }, SYMBOL_DEBOUNCE_MS);
  }

  function refresh(): void {
    renderTabs();
    if (tab === 'all' || tab === 'files') scheduleFiles(input.value);
    if (tab === 'all' || tab === 'symbols') scheduleSymbols(input.value);
    build();
  }

  function build(): void {
    const q = input.value;
    const out: Row[] = [];
    const header = (text: string): Row => ({ kind: 'header', render: () => [document.createTextNode(text)] });
    const info = (text: string): Row => ({ kind: 'info', render: () => [document.createTextNode(text)] });
    if (tab === 'files') {
      out.push(...fileRows.slice(0, TAB_LIMIT));
      if (fileInfo) out.push(info(fileInfo));
    } else if (tab === 'symbols') {
      out.push(...symRows);
      if (symInfo) out.push(info(symInfo));
    } else if (tab === 'actions') {
      const a = actionRows(q, TAB_LIMIT);
      out.push(...(a.length ? a : [info('No matching actions')]));
    } else if (tab === 'settings') {
      const s = settingRows(q, TAB_LIMIT);
      out.push(...(s.length ? s : [info('No matching settings')]));
    } else {
      const f = fileRows.slice(0, SECTION_LIMIT);
      if (f.length || fileInfo === 'Indexing…') out.push(header('Files'), ...f);
      if (fileInfo === 'Indexing…') out.push(info(fileInfo));
      if (q.trim()) {
        if (symRows.length) out.push(header('Symbols'), ...symRows.slice(0, SECTION_LIMIT));
        const a = actionRows(q, SECTION_LIMIT);
        if (a.length) out.push(header('Actions'), ...a);
        const s = settingRows(q, SECTION_LIMIT);
        if (s.length) out.push(header('Settings'), ...s);
      }
      if (out.length === 0) out.push(info(q.trim() ? 'Nothing found' : 'Type to search files, actions and settings'));
    }
    rows = out;
    list.replaceChildren(
      ...rows.map((r, i) =>
        r.kind === 'item'
          ? h(
              'li',
              {
                class: 'palette-row',
                role: 'option',
                onMousemove: () => {
                  if (selected !== i) select(i);
                },
                onMousedown: (e: MouseEvent) => {
                  e.preventDefault();
                  selected = i;
                  runSelected();
                },
              },
              r.render(),
              r.hint ? h('span', { class: 'palette-hint' }, r.hint) : null,
            )
          : h('li', { class: r.kind === 'header' ? 'palette-header' : 'palette-info', role: 'presentation' }, r.render()),
      ),
    );
    const first = rows.findIndex((r) => r.run);
    select(first);
  }

  function open(t: PaletteTab, opts: { title?: string } = {}): void {
    tab = t;
    titleOverride = opts.title;
    if (!root) {
      prevFocus = document.activeElement as HTMLElement | null;
      fileRows = [];
      fileInfo = '';
      symRows = [];
      symInfo = '';
      input.value = '';
      const panel = h('div', { class: 'palette', role: 'dialog', 'aria-label': 'Search Everywhere' }, title, tabBar, input, list);
      root = h('div', { class: 'palette-backdrop' }, panel);
      root.addEventListener('mousedown', (e) => {
        if (e.target === root) close();
      });
      document.body.appendChild(root);
    } else {
      input.select();
    }
    refresh();
    input.focus();
  }

  function close(): void {
    if (!root) return;
    if (fileTimer) clearTimeout(fileTimer);
    if (symTimer) clearTimeout(symTimer);
    fileReq++;
    symReq++;
    root.remove();
    root = null;
    prevFocus?.focus?.();
  }

  return { open, close, isOpen: () => !!root };
}
