// Editor change markers (R2): lines added, modified or deleted compared with
// the index, updated as you type. Clicking a marker shows the previous text
// with Rollback (undoable), Stage Hunk and Show Diff.

import { Chunk } from '@codemirror/merge';
import { RangeSet, StateEffect, StateField, Text } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { gutter, GutterMarker, ViewPlugin } from '@codemirror/view';
import type { EditorView } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { gitApi } from '../lib/api';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { openDiff } from './diffView';
import { gitError, gitState, normalize } from './state';

export type ChangeKind = 'added' | 'modified' | 'deleted';

export interface ChangeRange {
  kind: ChangeKind;
  /** 1-based lines in the current document (deleted: the line after the
   * removed block, clamped to the last line). */
  from: number;
  to: number;
  /** 1-based lines in the base (index) document; empty for additions. */
  baseFrom: number;
  baseTo: number;
  /** Character offsets of the chunk (line-aligned) in base and doc. */
  aFrom: number;
  aTo: number;
  bFrom: number;
  bTo: number;
}

/** Line-level changes between base and doc. */
export function changeRanges(base: Text, doc: Text): ChangeRange[] {
  const out: ChangeRange[] = [];
  for (const c of Chunk.build(base, doc)) {
    const hasA = c.toA > c.fromA;
    const hasB = c.toB > c.fromB;
    const bFrom = doc.lineAt(Math.min(c.fromB, doc.length)).number;
    const bTo = hasB ? doc.lineAt(Math.min(c.endB, doc.length)).number : bFrom;
    const aFrom = base.lineAt(Math.min(c.fromA, base.length)).number;
    const aTo = hasA ? base.lineAt(Math.min(c.endA, base.length)).number : aFrom - 1;
    const kind: ChangeKind = !hasA ? 'added' : !hasB ? 'deleted' : 'modified';
    out.push({
      kind,
      from: bFrom,
      to: kind === 'deleted' ? bFrom : bTo,
      baseFrom: aFrom,
      baseTo: hasA ? aTo : aFrom - 1,
      aFrom: c.fromA,
      aTo: Math.min(c.toA, base.length),
      bFrom: c.fromB,
      bTo: Math.min(c.toB, doc.length),
    });
  }
  return out;
}

/** A patch (3 lines of index context) that changes the base's lines
 * [baseFrom, baseTo] into the document's lines [from, to]. */
export function rangePatch(rel: string, base: Text, doc: Text, r: ChangeRange): string {
  const baseLines = base.toString().split('\n');
  const docLines = doc.toString().split('\n');
  const oldLines = r.kind === 'added' ? [] : baseLines.slice(r.baseFrom - 1, r.baseTo);
  const newLines = r.kind === 'deleted' ? [] : docLines.slice(r.from - 1, r.to);
  // Position in the base where the change starts (for additions: after the
  // base line preceding the insertion).
  const start = r.kind === 'added' ? r.baseFrom - 1 + 1 : r.baseFrom; // 1-based first changed base line
  const ctxBefore = baseLines.slice(Math.max(0, start - 1 - 3), start - 1);
  const afterIdx = r.kind === 'added' ? start - 1 : r.baseTo;
  const ctxAfter = baseLines.slice(afterIdx, afterIdx + 3).filter((_, i, a) => !(i === a.length - 1 && a[i] === '' && afterIdx + i === baseLines.length - 1));
  const oldStart = start - ctxBefore.length;
  const body = [...ctxBefore.map((l) => ' ' + l), ...oldLines.map((l) => '-' + l), ...newLines.map((l) => '+' + l), ...ctxAfter.map((l) => ' ' + l)];
  const oldCount = ctxBefore.length + oldLines.length + ctxAfter.length;
  const newCount = ctxBefore.length + newLines.length + ctxAfter.length;
  return [`diff --git a/${rel} b/${rel}`, `--- a/${rel}`, `+++ b/${rel}`, `@@ -${oldStart},${oldCount} +${oldStart},${newCount} @@`, ...body].join('\n') + '\n';
}

class ChangeMarker extends GutterMarker {
  constructor(readonly kind: ChangeKind, readonly index: number) {
    super();
  }

  eq(o: ChangeMarker): boolean {
    return o.kind === this.kind && o.index === this.index;
  }

  toDOM(): Node {
    return h('span', { class: `vcs-gutter vcs-gutter-${this.kind}` });
  }
}

interface ChangeState {
  ranges: ChangeRange[];
  markers: RangeSet<ChangeMarker>;
}

const setChanges = StateEffect.define<ChangeState>();
const changeField = StateField.define<ChangeState>({
  create: () => ({ ranges: [], markers: RangeSet.empty }),
  update(v, tr) {
    for (const e of tr.effects) if (e.is(setChanges)) return e.value;
    return tr.docChanged ? { ranges: v.ranges, markers: v.markers.map(tr.changes) } : v;
  },
});

/** Index content per open path (null = not in the index: no markers). */
const bases = new Map<string, Text | null>();

function markersFor(doc: Text, ranges: ChangeRange[]): RangeSet<ChangeMarker> {
  const out = [];
  for (let i = 0; i < ranges.length; i++) {
    const r = ranges[i];
    for (let ln = r.from; ln <= Math.min(r.to, doc.lines); ln++) out.push(new ChangeMarker(r.kind, i).range(doc.line(ln).from));
  }
  return RangeSet.of(out, true);
}

function recompute(view: EditorView, path: string): void {
  const base = bases.get(path);
  const ranges = base ? changeRanges(base, view.state.doc) : [];
  view.dispatch({ effects: setChanges.of({ ranges, markers: markersFor(view.state.doc, ranges) }) });
}

function relOf(path: string): string {
  const root = normalize(gitState.peek().status.root);
  const p = normalize(path);
  return root && p.startsWith(root + '/') ? p.slice(root.length + 1) : p;
}

let popup: HTMLElement | null = null;
function closePopup(): void {
  popup?.remove();
  popup = null;
}

function showHunk(view: EditorView, path: string, r: ChangeRange, x: number, y: number): void {
  closePopup();
  const base = bases.get(path);
  if (!base) return;
  const old = r.kind === 'added' ? '' : base.sliceString(base.line(r.baseFrom).from, base.line(Math.min(r.baseTo, base.lines)).to);
  const rollback = (): void => {
    // Chunks are line-aligned: put the base text back in place of the doc's.
    view.dispatch({ changes: { from: r.bFrom, to: Math.min(r.bTo, view.state.doc.length), insert: base.sliceString(r.aFrom, r.aTo) }, userEvent: 'input.rollback' });
    closePopup();
  };
  const stage = async (): Promise<void> => {
    closePopup();
    try {
      await gitApi.applyPatch(rangePatch(relOf(path), base, view.state.doc, r), true, false);
    } catch (err) {
      gitError('Cannot stage the change', err);
    }
  };
  popup = h(
    'div',
    { class: 'vcs-popup', role: 'dialog' },
    h(
      'div',
      { class: 'vcs-popup-bar' },
      h('span', null, r.kind === 'added' ? 'Added lines' : r.kind === 'deleted' ? 'Deleted lines' : 'Changed lines'),
      h('button', { class: 'btn', onClick: rollback }, 'Rollback'),
      h('button', { class: 'btn', onClick: () => void stage() }, 'Stage Hunk'),
      h('button', { class: 'btn', onClick: () => (closePopup(), void openDiff({ path, mode: 'work' })) }, 'Show Diff'),
    ),
    old ? h('pre', { class: 'vcs-popup-old' }, old) : h('div', { class: 'vcs-popup-none' }, 'No previous text (new lines)'),
  );
  document.body.append(popup);
  const rect = popup.getBoundingClientRect();
  popup.style.left = `${Math.min(x, window.innerWidth - rect.width - 8)}px`;
  popup.style.top = `${Math.min(y + 8, window.innerHeight - rect.height - 8)}px`;
  const off = (e: Event): void => {
    if (popup && !popup.contains(e.target as Node)) {
      closePopup();
      document.removeEventListener('mousedown', off, true);
      document.removeEventListener('keydown', esc, true);
    }
  };
  const esc = (e: KeyboardEvent): void => {
    if (e.key === 'Escape') {
      closePopup();
      document.removeEventListener('mousedown', off, true);
      document.removeEventListener('keydown', esc, true);
    }
  };
  setTimeout(() => {
    document.addEventListener('mousedown', off, true);
    document.addEventListener('keydown', esc, true);
  });
}

export function changeGutter(buf: Buffer): Extension {
  if (buf.binary) return [];
  const path = buf.path;
  let timer: ReturnType<typeof setTimeout> | null = null;
  return [
    changeField,
    gutter({
      class: 'cm-vcs-gutter',
      markers: (v) => v.state.field(changeField).markers,
      domEventHandlers: {
        mousedown(view, block, e) {
          const line = view.state.doc.lineAt(block.from).number;
          const r = view.state.field(changeField).ranges.find((x) => line >= x.from && line <= x.to);
          if (!r) return false;
          const me = e as MouseEvent;
          showHunk(view, path, r, me.clientX, me.clientY);
          return true;
        },
      },
    }),
    ViewPlugin.define((view) => {
      queueMicrotask(() => recompute(view, path));
      return {
        update(u) {
          if (!u.docChanged) return;
          if (timer) clearTimeout(timer);
          timer = setTimeout(() => recompute(view, path), 200);
        },
        destroy() {
          if (timer) clearTimeout(timer);
        },
      };
    }),
  ];
}

async function loadBase(path: string): Promise<void> {
  if (!gitState.peek().repo) {
    bases.delete(path);
  } else {
    try {
      const r = await gitApi.show(':0', path);
      bases.set(path, r.found ? Text.of(r.text.split('\n')) : null);
    } catch {
      bases.set(path, null);
    }
  }
  const buf = editors.buffers.get(path);
  if (buf) for (const v of buf.views) if (v.state.field(changeField, false)) recompute(v, path);
}

export function installChangeGutter(): void {
  editors.addExtension(changeGutter);
  editors.hooks.opened.on((buf) => void loadBase(buf.path));
  editors.hooks.closed.on((buf) => bases.delete(buf.path));
  // The index changes on stage/unstage/commit/checkout: reload bases.
  let last = '';
  effect(() => {
    const st = gitState();
    const sig = `${st.repo}|${st.status.head}|${st.status.files.map((f) => `${f.path}:${f.index}`).join(',')}`;
    if (sig === last) return;
    last = sig;
    for (const path of editors.buffers.keys()) void loadBase(path);
  });
}
