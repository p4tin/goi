// Navigation (R4.1–4): go to declaration / type / implementation, Mod-click,
// a chooser for several targets, back/forward history, and test ⇄ subject.

import { StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { Decoration, EditorView } from '@codemirror/view';
import type { DecorationSet } from '@codemirror/view';
import { confirmDialog } from '../app/dialog';
import { baseName, dirName } from '../editor/buffer';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { files, lang } from '../lib/api';
import type { Location, Pos, Range } from '../lib/api';
import { h } from '../lib/dom';
import { activeTarget, client, fresh, info, warn } from './context';
import { relPath } from './diagnostics';
import { showListPopup } from './popup';
import { toOffsets, toPos, wordAt } from './positions';

export interface NavEntry {
  path: string;
  /** 1-based. */
  line: number;
  col: number;
}

/** Back/forward history of editor locations, browser style. index points
 * at the stored entry for the current location, or equals entries.length
 * when the current location is new (not stored yet). */
export class NavHistory {
  private entries: NavEntry[] = [];
  private index = 0;

  constructor(private max = 100) {}

  private static same(a: NavEntry | undefined, b: NavEntry): boolean {
    return !!a && a.path === b.path && a.line === b.line;
  }

  /** Records the location being left before a jump; drops forward history. */
  push(from: NavEntry): void {
    this.entries = this.entries.slice(0, Math.min(this.index, this.entries.length));
    if (NavHistory.same(this.entries[this.entries.length - 1], from)) this.entries.pop();
    this.entries.push(from);
    if (this.entries.length > this.max) this.entries.shift();
    this.index = this.entries.length;
  }

  /** The previous location. current is stored first so forward returns. */
  back(current: NavEntry | null): NavEntry | null {
    if (this.index >= this.entries.length) {
      if (current && !NavHistory.same(this.entries[this.entries.length - 1], current)) this.entries.push(current);
      this.index = this.entries.length - 1;
    }
    if (this.index <= 0) return null;
    this.index--;
    return this.entries[this.index];
  }

  forward(): NavEntry | null {
    if (this.index >= this.entries.length - 1) return null;
    this.index++;
    return this.entries[this.index];
  }

  get size(): number {
    return this.entries.length;
  }
}

export const history = new NavHistory();

function currentEntry(): NavEntry | null {
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (!buf || !view) return null;
  const head = view.state.selection.main.head;
  const line = view.state.doc.lineAt(head);
  return { path: buf.path, line: line.number, col: head - line.from + 1 };
}

/** Opens path and selects range (or places the cursor at its start). */
export async function openAt(path: string, range: Range, select = true): Promise<void> {
  const buf = await editors.open(path, { line: range.start.line + 1, col: range.start.character + 1 });
  const view = editors.activeView();
  if (!buf || !view || editors.activeBuffer() !== buf) return;
  const { from, to } = toOffsets(view.state.doc, range);
  view.dispatch({ selection: select ? { anchor: from, head: to } : { anchor: from }, scrollIntoView: true });
  view.focus();
}

/** Jumps to loc, recording the current location in history. */
export async function jumpTo(loc: Location): Promise<void> {
  const cur = currentEntry();
  if (cur) history.push(cur);
  await openAt(loc.path, loc.range, false);
}

async function goEntry(e: NavEntry | null): Promise<void> {
  if (!e) return;
  await editors.open(e.path, { line: e.line, col: e.col });
}

export function back(): Promise<void> {
  return goEntry(history.back(currentEntry()));
}

export function forward(): Promise<void> {
  return goEntry(history.forward());
}

function lineText(loc: Location): string {
  const buf = editors.buffers.get(loc.path);
  if (!buf) return '';
  const doc = buf.doc();
  if (loc.range.start.line >= doc.lines) return '';
  return doc.line(loc.range.start.line + 1).text.trim();
}

/** Shows several targets in a chooser anchored at the cursor. */
export function chooseLocation(locs: Location[], title: string, view: EditorView | null): void {
  const at = view ? view.coordsAtPos(view.state.selection.main.head) : null;
  showListPopup({
    items: locs,
    title,
    at: at ? { x: at.left, y: at.bottom + 4 } : undefined,
    render: (l) => [
      h('span', null, `${baseName(l.path)}:${l.range.start.line + 1}`),
      h('span', { class: 'lp-detail' }, lineText(l) || dirName(relPath(l.path))),
    ],
    onPick: (l) => void jumpTo(l),
  });
}

type Finder = (path: string, p: Pos) => Promise<Location[]>;

async function navigate(find: Finder, what: string, title: string, at?: { buf: Buffer; view: EditorView; pos: Pos }): Promise<void> {
  const t = at ?? (await activeTarget());
  if (!t) return;
  const version = t.buf.version;
  let locs: Location[];
  try {
    locs = await find(t.buf.path, t.pos);
  } catch (err) {
    warn('lang.nav', `Cannot find ${what}`, String(err));
    return;
  }
  if (!fresh(t.buf, version)) return;
  if (locs.length === 0) {
    info('lang.nav', `No ${what} found`);
    return;
  }
  if (locs.length === 1) await jumpTo(locs[0]);
  else chooseLocation(locs, title, t.view);
}

export const gotoDefinition = (at?: { buf: Buffer; view: EditorView; pos: Pos }): Promise<void> =>
  navigate(lang.definition, 'declaration', 'Choose Declaration', at);
export const gotoTypeDefinition = (): Promise<void> => navigate(lang.typeDefinition, 'type declaration', 'Choose Type Declaration');
export const gotoImplementation = (): Promise<void> => navigate(lang.implementation, 'implementations', 'Choose Implementation');

// ---- Mod-click and Mod-hover underline

const setLink = StateEffect.define<{ from: number; to: number } | null>();
const linkField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (e.is(setLink)) return e.value ? Decoration.set([Decoration.mark({ class: 'cm-lang-link' }).range(e.value.from, e.value.to)]) : Decoration.none;
    }
    return tr.docChanged ? Decoration.none : deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

const isMac = (): boolean => /Mac|iPhone|iPad/.test(navigator.platform);
const modDown = (e: MouseEvent | KeyboardEvent): boolean => (isMac() ? e.metaKey : e.ctrlKey);

export function navigationExtension(buf: Buffer): Extension {
  const clear = (view: EditorView): void => {
    if (view.state.field(linkField).size) view.dispatch({ effects: setLink.of(null) });
  };
  return [
    linkField,
    EditorView.domEventHandlers({
      mousemove(e, view) {
        if (!modDown(e) || !client.live()) {
          clear(view);
          return false;
        }
        const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
        const w = pos === null ? null : wordAt(view.state.doc, pos);
        const cur = view.state.field(linkField);
        let same = false;
        cur.between(0, view.state.doc.length, (from, to) => {
          if (w && from === w.from && to === w.to) same = true;
        });
        if (!same) view.dispatch({ effects: setLink.of(w ? { from: w.from, to: w.to } : null) });
        return false;
      },
      mouseleave(_e, view) {
        clear(view);
        return false;
      },
      keyup(e, view) {
        if (!modDown(e)) clear(view);
        return false;
      },
      mousedown(e, view) {
        if (!modDown(e) || e.button !== 0 || !client.live()) return false;
        const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
        if (pos === null || !wordAt(view.state.doc, pos)) return false;
        e.preventDefault();
        view.dispatch({ selection: { anchor: pos } });
        clear(view);
        void client.flush(buf).then(() => gotoDefinition({ buf, view, pos: toPos(view.state.doc, pos) }));
        return true;
      },
    }),
  ];
}

// ---- Go to Test ⇄ Subject

/** The test file for a source file, or the source file for a test. */
export function counterpartPath(path: string): string | null {
  if (!path.endsWith('.go')) return null;
  return path.endsWith('_test.go') ? path.slice(0, -'_test.go'.length) + '.go' : path.slice(0, -'.go'.length) + '_test.go';
}

export interface FuncRef {
  name: string;
  recv?: string;
}

/** The function or method declared most recently before offset. */
export function enclosingFunc(text: string, offset: number): FuncRef | null {
  const re = /^func\s+(?:\(\s*(?:\w+\s+)?\*?\s*(\w+)(?:\[[^\]]*\])?\s*\)\s*)?(\w+)/gm;
  let found: FuncRef | null = null;
  for (let m = re.exec(text); m && m.index <= offset; m = re.exec(text)) {
    found = { name: m[2], recv: m[1] || undefined };
  }
  return found;
}

/** Name of the counterpart function: Foo → TestFoo, (T).M → TestT_M, and
 * back (TestFoo_case → Foo, TestT_M → M on T). */
export function counterpartFunc(f: FuncRef, inTest: boolean): FuncRef {
  if (!inTest) return { name: `Test${f.recv ? `${f.recv}_` : ''}${f.name}` };
  const m = /^(?:Test|Benchmark|Fuzz|Example)_?(.+)$/.exec(f.name);
  if (!m) return { name: f.name };
  const base = m[1];
  const parts = base.split('_');
  // TestT_M names a method M on T (both exported); TestFoo_case is a case of Foo.
  if (parts.length >= 2 && /^[A-Z]/.test(parts[0]) && /^[A-Z]/.test(parts[1])) return { name: parts[1], recv: parts[0] };
  return { name: parts[0] };
}

/** 1-based line declaring f in text, or 0. */
export function findFuncLine(text: string, f: FuncRef): number {
  const lines = text.split('\n');
  const name = f.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const method = f.recv ? new RegExp(`^func\\s+\\([^)]*\\b${f.recv}\\b[^)]*\\)\\s*${name}\\b`) : null;
  const fn = new RegExp(`^func\\s+(?:\\([^)]*\\)\\s*)?${name}\\b`);
  for (let i = 0; i < lines.length; i++) {
    if (method?.test(lines[i])) return i + 1;
  }
  for (let i = 0; i < lines.length; i++) {
    if (fn.test(lines[i])) return i + 1;
  }
  return 0;
}

function packageName(text: string): string {
  return /^package\s+(\w+)/m.exec(text)?.[1] ?? 'main';
}

export async function gotoTest(): Promise<void> {
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (!buf || !view) return;
  const target = counterpartPath(buf.path);
  if (!target) {
    info('lang.gotoTest', 'Go to Test works in Go files');
    return;
  }
  const inTest = buf.path.endsWith('_test.go');
  const text = view.state.doc.toString();
  const fn = enclosingFunc(text, view.state.selection.main.head);
  const want = fn ? counterpartFunc(fn, inTest) : null;

  let opened;
  try {
    opened = await files.read(target);
  } catch {
    opened = null;
  }
  if (!opened) {
    if (inTest) {
      info('lang.gotoTest', `${baseName(target)} does not exist`);
      return;
    }
    const ok = await confirmDialog('Create Test', `${baseName(target)} does not exist. Create it?`, 'Create');
    if (!ok) return;
    const pkg = packageName(text);
    const body = `package ${pkg}\n\nimport "testing"\n${want ? `\nfunc ${want.name}(t *testing.T) {\n\t\n}\n` : ''}`;
    try {
      await files.create(target, false, body);
    } catch (err) {
      warn('lang.gotoTest', 'Cannot create test file', String(err));
      return;
    }
    const cur = currentEntry();
    if (cur) history.push(cur);
    const lines = body.split('\n');
    await editors.open(target, { line: want ? lines.length - 2 : 1, col: 2 });
    return;
  }
  const cur = currentEntry();
  if (cur) history.push(cur);
  const line = want ? findFuncLine(opened.content, want) : 0;
  await editors.open(target, { line: line || 1, col: 1 });
}
