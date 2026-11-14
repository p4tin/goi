// EditorManager owns buffers, panes, tabs and editor views. Features extend
// it through hooks and extension factories instead of editing it.

import { Annotation, Compartment, EditorSelection, EditorState, Transaction } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import type { FileMeta, OpenedFile } from '../lib/api';
import { h } from '../lib/dom';
import { signal } from '../lib/store';
import type { SchemeId } from '../theme/tokens';
import { schemeExtension } from '../theme/schemes';
import { Buffer } from './buffer';
import { baseExtensions, fontExtension, tabExtension, wrapExtension } from './extensions';
import type { EditorPrefs } from './extensions';
import { AsyncHook, Hook } from './hooks';

/** Marks transactions that mirror another view's edit. */
const mirrored = Annotation.define<boolean>();

export interface EditorIO {
  read(path: string): Promise<OpenedFile>;
  write(path: string, content: string, meta: FileMeta): Promise<FileMeta>;
}

export type CloseChoice = 'save' | 'discard' | 'cancel';

export interface ManagerOptions {
  io: EditorIO;
  /** Asks what to do with a dirty buffer being closed. */
  confirmClose(buf: Buffer): Promise<CloseChoice>;
  onError(title: string, err: unknown): void;
}

export interface TabState {
  path: string;
  pinned: boolean;
}

export interface OpenOptions {
  /** 1-based line and column to reveal. */
  line?: number;
  col?: number;
  pane?: Pane;
  focus?: boolean;
  pinned?: boolean;
  /** Open without activating (used by layout restore). */
  background?: boolean;
}

let paneSeq = 0;

/** Maximum number of editor panes (a 2×2 grid). */
export const MAX_PANES = 4;

export type SplitDir = 'row' | 'column';

export type SplitNode =
  | { kind: 'pane'; pane: Pane }
  | { kind: 'split'; dir: SplitDir; ratio: number; a: SplitNode; b: SplitNode };

/** Replaces the leaf holding pane with node. */
export function replaceLeaf(tree: SplitNode, pane: Pane, node: SplitNode): SplitNode {
  if (tree.kind === 'pane') return tree.pane === pane ? node : tree;
  return { ...tree, a: replaceLeaf(tree.a, pane, node), b: replaceLeaf(tree.b, pane, node) };
}

/** Removes pane's leaf, promoting its sibling. Returns null if tree is that leaf. */
export function removeLeaf(tree: SplitNode, pane: Pane): SplitNode | null {
  if (tree.kind === 'pane') return tree.pane === pane ? null : tree;
  const a = removeLeaf(tree.a, pane);
  const b = removeLeaf(tree.b, pane);
  if (!a) return b;
  if (!b) return a;
  return { ...tree, a, b };
}

/** Panes in reading order (left-to-right, top-to-bottom). */
export function leaves(tree: SplitNode): Pane[] {
  return tree.kind === 'pane' ? [tree.pane] : [...leaves(tree.a), ...leaves(tree.b)];
}

export class Pane {
  readonly id = `pane-${++paneSeq}`;
  readonly tabs = signal<TabState[]>([]);
  readonly active = signal<string | null>(null);
  readonly views = new Map<string, EditorView>();
  /** Per text tab: the element holding its editor view. Features may add
   * siblings beside the editor (viewCreated's content). */
  readonly contents = new Map<string, HTMLElement>();
  readonly placeholders = new Map<string, HTMLElement>();
  readonly el: HTMLElement;
  readonly tabBar: HTMLElement;
  readonly host: HTMLElement;

  constructor() {
    this.tabBar = h('div', { class: 'tabbar', role: 'tablist' });
    this.host = h('div', { class: 'pane-host' });
    this.el = h('div', { class: 'pane', dataset: { pane: this.id } }, this.tabBar, this.host);
  }

  has(path: string): boolean {
    return this.tabs.peek().some((t) => t.path === path);
  }
}

export interface Hooks {
  /** A buffer was loaded (first open). */
  opened: Hook<Buffer>;
  /** A buffer's last view closed. */
  closed: Hook<Buffer>;
  /** Before writing; handlers may edit the document (e.g. formatting). */
  beforeSave: AsyncHook<Buffer>;
  saved: Hook<Buffer>;
  /** A view was created for a buffer. content is the tab's element: a flex
   * row holding view.dom, where features may add elements beside it. */
  viewCreated: Hook<{ buffer: Buffer; view: EditorView; pane: Pane; content: HTMLElement }>;
  /** The active pane or tab changed. */
  activeChanged: Hook<Buffer | null>;
}

type Factory = (buf: Buffer) => Extension;

export class EditorManager {
  readonly buffers = new Map<string, Buffer>();
  readonly panes = signal<Pane[]>([]);
  readonly activePane = signal<Pane>(new Pane());
  /** Split layout of the panes. */
  readonly tree = signal<SplitNode>({ kind: 'pane', pane: this.activePane.peek() });
  /** Path of the active buffer in the active pane. */
  readonly activePath = signal<string | null>(null);
  /** Cursor of the active view: 1-based line/col, selection count. */
  readonly cursor = signal({ line: 1, col: 1, selections: 1, selected: 0 });
  readonly hooks: Hooks = {
    opened: new Hook(),
    closed: new Hook(),
    beforeSave: new AsyncHook(),
    saved: new Hook(),
    viewCreated: new Hook(),
    activeChanged: new Hook(),
  };

  private factories: Factory[] = [];
  private extra = new Compartment();
  private fontC = new Compartment();
  private tabC = new Compartment();
  private wrapC = new Compartment();
  private schemeC = new Compartment();
  private prefs: EditorPrefs = { fontFamily: 'monospace', fontSize: 13, tabSize: 4, softWrap: false };
  private scheme: SchemeId = 'goi-dark';
  private loading = new Map<string, Promise<Buffer | null>>();

  constructor(private opts: ManagerOptions) {
    this.panes.set([this.activePane.peek()]);
  }

  /** Registers an extension added to every view; existing views update. */
  addExtension(f: Factory): () => void {
    this.factories.push(f);
    this.refreshExtensions();
    return () => {
      this.factories = this.factories.filter((x) => x !== f);
      this.refreshExtensions();
    };
  }

  /** Recomputes factory extensions for all views (e.g. after a language or
   * marker change for a buffer). */
  refreshExtensions(buf?: Buffer): void {
    for (const b of buf ? [buf] : this.buffers.values()) {
      for (const v of b.views) v.dispatch({ effects: this.extra.reconfigure(this.extrasFor(b)) });
    }
  }

  setPrefs(p: EditorPrefs): void {
    this.prefs = p;
    this.eachView((v) =>
      v.dispatch({
        effects: [
          this.fontC.reconfigure(fontExtension(p)),
          this.tabC.reconfigure(tabExtension(p)),
          this.wrapC.reconfigure(wrapExtension(p)),
        ],
      }),
    );
  }

  get softWrap(): boolean {
    return this.prefs.softWrap;
  }

  setScheme(id: SchemeId): void {
    this.scheme = id;
    this.eachView((v) => v.dispatch({ effects: this.schemeC.reconfigure(schemeExtension(id)) }));
  }

  activeBuffer(): Buffer | null {
    const p = this.activePath.peek();
    return p ? (this.buffers.get(p) ?? null) : null;
  }

  activeView(): EditorView | null {
    const p = this.activePath.peek();
    return p ? (this.activePane.peek().views.get(p) ?? null) : null;
  }

  /** Opens path (loading it if needed) in a pane and activates it. */
  async open(path: string, o: OpenOptions = {}): Promise<Buffer | null> {
    const buf = await this.load(path);
    if (!buf) return null;
    const pane = o.pane ?? this.activePane.peek();
    if (!pane.has(path)) {
      pane.tabs.update((ts) => insertTab(ts, { path, pinned: !!o.pinned }, pane.active.peek()));
    }
    if (o.background) return buf;
    this.activate(pane, path);
    const view = pane.views.get(path);
    if (view && o.line) {
      const line = view.state.doc.line(Math.min(Math.max(1, o.line), view.state.doc.lines));
      const pos = Math.min(line.from + Math.max(0, (o.col ?? 1) - 1), line.to);
      view.dispatch({ selection: EditorSelection.cursor(pos), effects: EditorView.scrollIntoView(pos, { y: 'center' }) });
    }
    if (view && o.focus !== false) view.focus();
    return buf;
  }

  private load(path: string): Promise<Buffer | null> {
    const existing = this.buffers.get(path);
    if (existing) return Promise.resolve(existing);
    let p = this.loading.get(path);
    if (!p) {
      p = this.opts.io
        .read(path)
        .then((f) => {
          const buf = new Buffer(path, f.meta, f.content);
          this.buffers.set(path, buf);
          this.hooks.opened.emit(buf);
          return buf;
        })
        .catch((err: unknown) => {
          this.opts.onError(`Cannot open ${path}`, err);
          return null;
        })
        .finally(() => this.loading.delete(path));
      this.loading.set(path, p);
    }
    return p;
  }

  /** Shows path in pane, creating its view lazily. */
  activate(pane: Pane, path: string): void {
    const buf = this.buffers.get(path);
    if (!buf) return;
    let el: HTMLElement;
    if (buf.binary) {
      let ph = pane.placeholders.get(path);
      if (!ph) {
        ph = h('div', { class: 'binary-placeholder' }, `${buf.name} is a binary file and cannot be shown as text.`);
        pane.placeholders.set(path, ph);
        pane.host.appendChild(ph);
      }
      el = ph;
    } else {
      if (!pane.views.has(path)) this.createView(buf, pane);
      el = pane.contents.get(path)!;
    }
    for (const child of pane.host.children) (child as HTMLElement).hidden = child !== el;
    pane.active.set(path);
    this.activePane.set(pane);
    this.activePath.set(path);
    const v = pane.views.get(path);
    if (v) this.updateCursor(v.state);
    this.hooks.activeChanged.emit(buf);
  }

  private createView(buf: Buffer, pane: Pane): EditorView {
    const peer = buf.views.values().next().value as EditorView | undefined;
    const state = EditorState.create({
      doc: peer ? peer.state.doc : buf.saved,
      selection: peer ? peer.state.selection : undefined,
      extensions: [
        baseExtensions(),
        this.fontC.of(fontExtension(this.prefs)),
        this.tabC.of(tabExtension(this.prefs)),
        this.wrapC.of(wrapExtension(this.prefs)),
        this.schemeC.of(schemeExtension(this.scheme)),
        EditorState.readOnly.of(buf.readOnly),
        EditorView.editable.of(!buf.readOnly),
        this.extra.of(this.extrasFor(buf)),
      ],
    });
    const content = h('div', { class: 'tab-content' });
    pane.host.appendChild(content);
    const view = new EditorView({
      state,
      parent: content,
      dispatchTransactions: (trs, v) => this.dispatch(buf, trs, v),
    });
    content.addEventListener('focusin', () => {
      if (this.activePane.peek() !== pane || this.activePath.peek() !== buf.path) this.activate(pane, buf.path);
    });
    buf.views.add(view);
    pane.views.set(buf.path, view);
    pane.contents.set(buf.path, content);
    this.hooks.viewCreated.emit({ buffer: buf, view, pane, content });
    return view;
  }

  private extrasFor(buf: Buffer): Extension[] {
    return this.factories.map((f) => {
      try {
        return f(buf);
      } catch (err) {
        console.error('editor extension', err);
        return [];
      }
    });
  }

  private dispatch(buf: Buffer, trs: readonly Transaction[], view: EditorView): void {
    view.update(trs);
    let changed = false;
    for (const tr of trs) {
      if (!tr.docChanged) continue;
      changed = true;
      if (tr.annotation(mirrored)) continue;
      for (const other of buf.views) {
        if (other !== view) {
          other.dispatch({ changes: tr.changes, annotations: [mirrored.of(true), Transaction.addToHistory.of(false)] });
        }
      }
    }
    if (changed && !trs.some((t) => t.annotation(mirrored))) buf.changed(view.state.doc);
    if (view === this.activeView() && trs.some((t) => t.selection || t.docChanged)) this.updateCursor(view.state);
  }

  private updateCursor(state: EditorState): void {
    const main = state.selection.main;
    const line = state.doc.lineAt(main.head);
    let selected = 0;
    for (const r of state.selection.ranges) selected += r.to - r.from;
    this.cursor.set({ line: line.number, col: main.head - line.from + 1, selections: state.selection.ranges.length, selected });
  }

  /** Replaces a buffer's whole document in all its views (e.g. reload from
   * disk). keepHistory=false marks it as not undoable. */
  replaceDoc(buf: Buffer, content: string, keepHistory = true): void {
    const v = buf.views.values().next().value as EditorView | undefined;
    if (!v) return;
    v.dispatch({
      changes: { from: 0, to: v.state.doc.length, insert: content },
      annotations: keepHistory ? [] : [Transaction.addToHistory.of(false)],
    });
  }

  /** Saves a buffer (default: active). Returns false if not saved. */
  async save(path?: string): Promise<boolean> {
    const buf = path ? this.buffers.get(path) : this.activeBuffer();
    if (!buf || buf.readOnly) return false;
    await this.hooks.beforeSave.run(buf);
    const doc = buf.doc();
    try {
      const meta = await this.opts.io.write(buf.path, doc.toString(), buf.meta);
      buf.markSaved(doc, meta);
      this.hooks.saved.emit(buf);
      return true;
    } catch (err) {
      this.opts.onError(`Cannot save ${buf.name}`, err);
      return false;
    }
  }

  async saveAll(): Promise<boolean> {
    let ok = true;
    for (const b of this.buffers.values()) if (b.dirty.peek()) ok = (await this.save(b.path)) && ok;
    return ok;
  }

  dirtyBuffers(): Buffer[] {
    return [...this.buffers.values()].filter((b) => b.dirty.peek());
  }

  /** Closes path's tab in pane. Asks about unsaved changes when this is the
   * buffer's last tab. Returns false if cancelled. */
  async close(path: string, pane: Pane = this.activePane.peek(), force = false): Promise<boolean> {
    const buf = this.buffers.get(path);
    if (!pane.has(path)) return true;
    const lastTab = this.panes.peek().filter((p) => p.has(path)).length === 1;
    if (buf && lastTab && buf.dirty.peek() && !force) {
      const choice = await this.opts.confirmClose(buf);
      if (choice === 'cancel') return false;
      if (choice === 'save' && !(await this.save(path))) return false;
    }
    const tabs = pane.tabs.peek();
    const idx = tabs.findIndex((t) => t.path === path);
    pane.tabs.set(tabs.filter((t) => t.path !== path));
    const view = pane.views.get(path);
    if (view) {
      buf?.views.delete(view);
      view.destroy();
      pane.views.delete(path);
    }
    pane.contents.get(path)?.remove();
    pane.contents.delete(path);
    pane.placeholders.get(path)?.remove();
    pane.placeholders.delete(path);
    if (buf && lastTab) {
      this.buffers.delete(path);
      this.hooks.closed.emit(buf);
    }
    if (pane.tabs.peek().length === 0 && this.panes.peek().length > 1) {
      this.removePane(pane);
      return true;
    }
    if (pane.active.peek() === path) {
      const rest = pane.tabs.peek();
      const next = rest[Math.min(idx, rest.length - 1)];
      if (next) this.activate(pane, next.path);
      else {
        pane.active.set(null);
        if (this.activePane.peek() === pane) {
          this.activePath.set(null);
          this.hooks.activeChanged.emit(null);
        }
      }
    }
    return true;
  }

  /** Closes tabs matching pred (pinned tabs are kept unless includePinned). */
  async closeWhere(pane: Pane, pred: (t: TabState, i: number) => boolean, includePinned = false): Promise<boolean> {
    for (const [i, t] of pane.tabs.peek().map((t, i) => [i, t] as const)) {
      if ((includePinned || !t.pinned) && pred(t, i) && !(await this.close(t.path, pane))) return false;
    }
    return true;
  }

  togglePin(pane: Pane, path: string): void {
    pane.tabs.update((ts) => {
      const t = ts.find((x) => x.path === path);
      if (!t) return ts;
      return sortPinned(ts.map((x) => (x === t ? { ...x, pinned: !x.pinned } : x)));
    });
  }

  /** Moves the tab at index from to index to (pinned tabs stay first). */
  moveTab(pane: Pane, from: number, to: number): void {
    pane.tabs.update((ts) => {
      if (from < 0 || from >= ts.length) return ts;
      const next = [...ts];
      const [t] = next.splice(from, 1);
      next.splice(Math.max(0, Math.min(to, next.length)), 0, t);
      return sortPinned(next);
    });
  }

  /** Activates the tab offset steps from the active one (wrapping). */
  cycle(offset: number): void {
    const pane = this.activePane.peek();
    const ts = pane.tabs.peek();
    if (ts.length < 2) return;
    const i = ts.findIndex((t) => t.path === pane.active.peek());
    const next = ts[(i + offset + ts.length) % ts.length];
    this.activate(pane, next.path);
    pane.views.get(next.path)?.focus();
  }

  /** Splits pane in dir, showing its active file in the new pane. Returns
   * the new pane, or null at the pane limit. */
  async split(pane: Pane, dir: SplitDir): Promise<Pane | null> {
    if (this.panes.peek().length >= MAX_PANES) return null;
    const np = new Pane();
    this.tree.set(replaceLeaf(this.tree.peek(), pane, { kind: 'split', dir, ratio: 0.5, a: { kind: 'pane', pane }, b: { kind: 'pane', pane: np } }));
    this.panes.set(leaves(this.tree.peek()));
    const active = pane.active.peek();
    if (active) await this.open(active, { pane: np });
    else this.activePane.set(np);
    return np;
  }

  /** Closes all of pane's tabs (asking about unsaved changes) and removes it. */
  async closePane(pane: Pane): Promise<boolean> {
    if (this.panes.peek().length === 1) return this.closeWhere(pane, () => true, true);
    if (!(await this.closeWhere(pane, () => true, true))) return false;
    if (this.panes.peek().includes(pane)) this.removePane(pane);
    return true;
  }

  private removePane(pane: Pane): void {
    const next = removeLeaf(this.tree.peek(), pane);
    if (!next) return;
    for (const v of pane.views.values()) {
      for (const b of this.buffers.values()) b.views.delete(v);
      v.destroy();
    }
    pane.views.clear();
    pane.contents.clear();
    pane.el.remove();
    this.tree.set(next);
    this.panes.set(leaves(next));
    if (this.activePane.peek() === pane) {
      const target = this.panes.peek()[0];
      const path = target.active.peek();
      if (path) this.activate(target, path);
      else {
        this.activePane.set(target);
        this.activePath.set(null);
        this.hooks.activeChanged.emit(null);
      }
    }
  }

  /** Activates the pane offset steps away in reading order. */
  focusPane(offset: number): void {
    const ps = this.panes.peek();
    if (ps.length < 2) return;
    const i = ps.indexOf(this.activePane.peek());
    const target = ps[(i + offset + ps.length) % ps.length];
    const path = target.active.peek();
    if (path) {
      this.activate(target, path);
      target.views.get(path)?.focus();
    } else this.activePane.set(target);
  }

  /** Moves the active tab to the next pane (splitting right if alone). */
  async moveToOtherPane(): Promise<void> {
    const from = this.activePane.peek();
    const path = from.active.peek();
    if (!path) return;
    const ps = this.panes.peek();
    if (ps.length === 1) {
      await this.split(from, 'row');
      await this.close(path, from, true);
      return;
    }
    const to = ps[(ps.indexOf(from) + 1) % ps.length];
    await this.open(path, { pane: to });
    await this.close(path, from, true);
  }

  eachView(fn: (v: EditorView, buf: Buffer) => void): void {
    for (const b of this.buffers.values()) for (const v of b.views) fn(v, b);
  }
}

/** Inserts a tab after the active one, keeping pinned tabs first. */
export function insertTab(ts: TabState[], tab: TabState, active: string | null): TabState[] {
  const i = active ? ts.findIndex((t) => t.path === active) : -1;
  const next = [...ts];
  next.splice(i >= 0 ? i + 1 : next.length, 0, tab);
  return sortPinned(next);
}

/** Stable sort: pinned tabs before unpinned ones. */
export function sortPinned(ts: TabState[]): TabState[] {
  return [...ts.filter((t) => t.pinned), ...ts.filter((t) => !t.pinned)];
}

/** Disambiguating labels: basenames, plus the parent dir for duplicates. */
export function tabLabels(paths: string[]): Map<string, string> {
  const by = new Map<string, string[]>();
  for (const p of paths) {
    const name = p.split(/[\\/]/).pop()!;
    by.set(name, [...(by.get(name) ?? []), p]);
  }
  const out = new Map<string, string>();
  for (const [name, ps] of by) {
    for (const p of ps) {
      const parts = p.split(/[\\/]/);
      out.set(p, ps.length > 1 ? `${name} — ${parts[parts.length - 2] ?? ''}` : name);
    }
  }
  return out;
}
