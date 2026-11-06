// Mounts the editor area into the workbench and registers editor commands,
// status bar items and the layout part.

import { openSearchPanel } from '@codemirror/search';
import { EditorSelection } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { promptDialog, showDialog } from '../app/dialog';
import { project, resolvedTheme, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { files } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { defaultSchemeFor } from '../theme/schemes';
import type { SchemeId } from '../theme/tokens';
import type { Buffer } from './buffer';
import { EditorManager, leaves, Pane } from './editorManager';
import type { CloseChoice, SplitNode, TabState } from './editorManager';
import { renderPanes } from './panes';
import './editor.css';

/** The single editor manager, available after installEditor runs. */
export let editors: EditorManager;

type SavedTab = TabState & { line?: number; col?: number };

interface SavedPane {
  kind: 'pane';
  tabs: SavedTab[];
  active: string | null;
  focused?: boolean;
}

type SavedNode = SavedPane | { kind: 'split'; dir: 'row' | 'column'; ratio: number; a: SavedNode; b: SavedNode };

interface EditorLayout {
  tree?: SavedNode;
  // Pre-split format (single pane).
  tabs?: SavedTab[];
  active?: string | null;
}

export function installEditor(wb: Workbench): void {
  const m = new EditorManager({
    io: { read: files.read, write: files.write },
    confirmClose: askSave,
    onError: (title, err) => wb.notifications.push({ id: title, level: 'error', title, body: String(err) }),
  });
  editors = m;

  const empty = h('div', { class: 'editor-empty' }, h('div', null, 'No file open'), h('div', { class: 'muted' }, 'Open a file from the Project panel or Search Everywhere'));
  const area = h('div', { class: 'panes' });
  wb.center.append(area, empty);
  renderPanes(m, area, () => wb.layout.scheduleSave());
  effect(() => {
    empty.hidden = !!m.activePath();
  });

  effect(() => {
    const s = settings();
    if (!s) return;
    m.setPrefs({
      fontFamily: s.editor.fontFamily,
      fontSize: s.editor.fontSize,
      tabSize: s.editor.tabSize,
      softWrap: s.editor.softWrap,
    });
    m.setScheme(resolveScheme(s.ui.scheme, resolvedTheme()));
  });

  // Close everything when the project closes or changes.
  let root: string | null = null;
  effect(() => {
    const p = project();
    const next = p?.root ?? null;
    if (root && next !== root) {
      for (const pn of m.panes.peek()) void m.closeWhere(pn, () => true, true);
    }
    root = next;
  });

  wb.layout.register<EditorLayout>('editor', {
    save: () => ({ tree: saveNode(m, m.tree.peek()) }),
    restore: (l) => void restoreLayout(m, l),
  });
  effect(() => {
    m.activePath();
    m.tree();
    for (const p of m.panes()) p.tabs();
    wb.layout.scheduleSave();
  });

  installStatus(wb, m);
  registerCommands(m);
}

function saveNode(m: EditorManager, n: SplitNode): SavedNode {
  if (n.kind === 'split') return { kind: 'split', dir: n.dir, ratio: n.ratio, a: saveNode(m, n.a), b: saveNode(m, n.b) };
  const pn = n.pane;
  return {
    kind: 'pane',
    focused: pn === m.activePane.peek(),
    active: pn.active.peek(),
    tabs: pn.tabs.peek().map((t) => {
      const v = pn.views.get(t.path);
      if (!v) return t;
      const head = v.state.selection.main.head;
      const line = v.state.doc.lineAt(head);
      return { ...t, line: line.number, col: head - line.from + 1 };
    }),
  };
}

/** Rebuilds panes from a saved tree (or the pre-split single-pane format)
 * and reopens their tabs. */
async function restoreLayout(m: EditorManager, l: EditorLayout): Promise<void> {
  const saved: SavedNode | undefined = l?.tree ?? (l?.tabs ? { kind: 'pane', tabs: l.tabs, active: l.active ?? null } : undefined);
  if (!saved) return;
  const first = m.panes.peek()[0];
  const pairs: [Pane, SavedPane][] = [];
  let used = false;
  const build = (n: SavedNode): SplitNode => {
    if (n.kind === 'split') return { kind: 'split', dir: n.dir, ratio: n.ratio, a: build(n.a), b: build(n.b) };
    const pane = used ? new Pane() : first;
    used = true;
    pairs.push([pane, n]);
    return { kind: 'pane', pane };
  };
  const tree = build(saved);
  if (leaves(tree).length > 4) return;
  m.tree.set(tree);
  m.panes.set(leaves(tree));
  let focus: [Pane, SavedTab] | null = null;
  for (const [pane, sp] of pairs) {
    for (const t of sp.tabs) await m.open(t.path, { pane, pinned: t.pinned, background: true });
    const active = sp.tabs.find((t) => t.path === sp.active) ?? sp.tabs[sp.tabs.length - 1];
    if (active && m.buffers.has(active.path)) {
      await m.open(active.path, { pane, line: active.line, col: active.col, focus: false });
      if (sp.focused || !focus) focus = [pane, active];
    }
  }
  // Drop panes whose files all failed to open.
  for (const pane of m.panes.peek()) if (pane.tabs.peek().length === 0 && m.panes.peek().length > 1) await m.closePane(pane);
  if (focus && m.panes.peek().includes(focus[0])) m.activate(focus[0], focus[1].path);
}

async function askSave(buf: Buffer): Promise<CloseChoice> {
  return showDialog<CloseChoice>({
    title: `Save changes to ${buf.name}?`,
    body: 'Your changes will be lost if you don’t save them.',
    cancelValue: 'cancel',
    buttons: [
      { label: 'Don’t Save', value: 'discard', danger: true },
      { label: 'Cancel', value: 'cancel' },
      { label: 'Save', value: 'save', primary: true },
    ],
  });
}

function installStatus(wb: Workbench, m: EditorManager): void {
  const pos = wb.status.add({ id: 'editor-cursor', side: 'right', priority: 50, title: 'Go to Line', onClick: () => void commands.run('editor.gotoLine') });
  const eol = wb.status.add({ id: 'editor-eol', side: 'right', priority: 60, title: 'Line separator' });
  const enc = wb.status.add({ id: 'editor-encoding', side: 'right', priority: 70, title: 'Encoding' });
  const ro = wb.status.add({ id: 'editor-readonly', side: 'right', priority: 80 });
  effect(() => {
    const path = m.activePath();
    const c = m.cursor();
    const buf = path ? m.buffers.get(path) : null;
    for (const el of [pos, eol, enc, ro]) el.hidden = !buf;
    if (!buf) return;
    pos.textContent = `Ln ${c.line}, Col ${c.col}${c.selections > 1 ? ` (${c.selections} carets)` : c.selected ? ` (${c.selected} chars)` : ''}`;
    eol.textContent = buf.meta.eol === '\r\n' ? 'CRLF' : buf.meta.eol === '\r' ? 'CR' : 'LF';
    enc.textContent = buf.meta.bom ? 'UTF-8 BOM' : 'UTF-8';
    ro.textContent = buf.readOnly ? 'Read-only' : '';
    ro.hidden = !buf.readOnly;
  });
}

function registerCommands(m: EditorManager): void {
  const view = (): EditorView | null => m.activeView();
  const reg = (id: string, title: string, run: (...a: unknown[]) => unknown, when?: () => boolean): void => {
    commands.register({ id, title, category: 'Editor', run, when });
  };
  const hasView = (): boolean => !!view();

  reg('editor.open', 'Open File', (path, line, col) =>
    m.open(String(path), { line: typeof line === 'number' ? line : undefined, col: typeof col === 'number' ? col : undefined }),
  );
  commands.register({ id: 'file.save', title: 'Save', category: 'File', run: () => m.save(), when: () => !!m.activeBuffer() });
  commands.register({ id: 'file.saveAll', title: 'Save All', category: 'File', run: () => m.saveAll() });
  reg('editor.close', 'Close Tab', () => {
    const p = m.activePath.peek();
    return p ? m.close(p) : true;
  });
  reg('editor.closeOthers', 'Close Other Tabs', () => {
    const p = m.activePath.peek();
    return m.closeWhere(m.activePane.peek(), (t) => t.path !== p);
  });
  reg('editor.closeAll', 'Close All Tabs', () => m.closeWhere(m.activePane.peek(), () => true));
  reg('editor.nextTab', 'Next Tab', () => m.cycle(1));
  reg('editor.prevTab', 'Previous Tab', () => m.cycle(-1));
  reg('editor.togglePin', 'Pin/Unpin Tab', () => {
    const p = m.activePath.peek();
    if (p) m.togglePin(m.activePane.peek(), p);
  });
  reg('editor.splitRight', 'Split Right', () => m.split(m.activePane.peek(), 'row'), hasView);
  reg('editor.splitDown', 'Split Down', () => m.split(m.activePane.peek(), 'column'), hasView);
  reg('editor.focusNextPane', 'Focus Next Pane', () => m.focusPane(1));
  reg('editor.focusPrevPane', 'Focus Previous Pane', () => m.focusPane(-1));
  reg('editor.moveToOtherPane', 'Move Tab to Other Pane', () => m.moveToOtherPane(), hasView);
  reg('editor.closePane', 'Close Pane', () => m.closePane(m.activePane.peek()));
  reg('editor.toggleSoftWrap', 'Toggle Soft Wrap', async () => {
    const { settingsApi } = await import('../lib/api');
    await settingsApi.set('user', 'editor.softWrap', !m.softWrap);
  });
  reg('editor.find', 'Find in File', () => {
    const v = view();
    if (v) openSearchPanel(v);
  }, hasView);
  reg('editor.replace', 'Replace in File', () => {
    const v = view();
    if (v) openSearchPanel(v);
  }, hasView);
  reg(
    'editor.gotoLine',
    'Go to Line…',
    async () => {
      const v = view();
      if (!v) return;
      const cur = v.state.doc.lineAt(v.state.selection.main.head).number;
      const answer = await promptDialog('Go to Line', String(cur), (s) => (parseLineCol(s) ? null : 'Enter a line or line:column'), `Line[:Column] (1–${v.state.doc.lines})`);
      const lc = answer ? parseLineCol(answer) : null;
      if (!lc) return;
      const line = v.state.doc.line(Math.min(Math.max(1, lc.line), v.state.doc.lines));
      const pos = Math.min(line.from + Math.max(0, lc.col - 1), line.to);
      v.dispatch({ selection: EditorSelection.cursor(pos), effects: EditorView.scrollIntoView(pos, { y: 'center' }) });
      v.focus();
    },
    hasView,
  );
}

/** Parses "12" or "12:5" (also "12,5"). */
export function parseLineCol(s: string): { line: number; col: number } | null {
  const m = /^\s*(\d+)\s*(?:[:,]\s*(\d+))?\s*$/.exec(s);
  if (!m) return null;
  return { line: Number(m[1]), col: m[2] ? Number(m[2]) : 1 };
}

/** Maps the ui.scheme setting to a concrete scheme; "auto" (or unknown)
 * follows the resolved UI theme. */
export function resolveScheme(setting: string, ui: 'light' | 'dark' | 'hc'): SchemeId {
  const known: SchemeId[] = ['goi-light', 'goi-dark', 'goi-ember', 'goi-paper'];
  return known.includes(setting as SchemeId) ? (setting as SchemeId) : defaultSchemeFor(ui);
}
