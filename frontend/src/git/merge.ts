// 3-way merge editor (R10.2): Yours (stage 2) | Result (editable, starts
// from the working file with conflict markers) | Theirs (stage 3). Each
// conflict can take Yours, Theirs or Both; Apply saves and stages the result.

import { EditorState, StateEffect, StateField } from '@codemirror/state';
import type { Extension, Range } from '@codemirror/state';
import { Decoration, EditorView, lineNumbers } from '@codemirror/view';
import type { DecorationSet } from '@codemirror/view';
import { confirmDialog, showDialog } from '../app/dialog';
import { baseName } from '../editor/buffer';
import { editors } from '../editor/install';
import { languageIdFor, loadLanguage } from '../editor/languages';
import { files, gitApi } from '../lib/api';
import { h } from '../lib/dom';
import { conflictBlocks, resolveAll, resolveBlock } from './conflicts';
import type { ConflictBlock, Side } from './conflicts';
import { gitError, gitInfo } from './state';

const refresh = StateEffect.define<null>();

/** Highlights conflict regions (ours / base / theirs) in the result. */
const conflictHighlight = StateField.define<DecorationSet>({
  create: (s) => build(s.doc.toString(), s.doc),
  update(v, tr) {
    if (tr.docChanged || tr.effects.some((e) => e.is(refresh))) return build(tr.state.doc.toString(), tr.state.doc);
    return v;
  },
  provide: (f) => EditorView.decorations.from(f),
});

function build(text: string, doc: EditorState['doc']): DecorationSet {
  const decos: Range<Decoration>[] = [];
  for (const b of conflictBlocks(text)) {
    const oursEnd = b.base >= 0 ? b.base : b.mid;
    for (let i = b.start; i <= b.end; i++) {
      if (i + 1 > doc.lines) break;
      let cls = 'cm-conflict-marker';
      if (i > b.start && i < oursEnd) cls = 'cm-conflict-ours';
      else if (b.base >= 0 && i > b.base && i < b.mid) cls = 'cm-conflict-base';
      else if (i > b.mid && i < b.end) cls = 'cm-conflict-theirs';
      decos.push(Decoration.line({ class: cls }).range(doc.line(i + 1).from));
    }
  }
  return Decoration.set(decos, true);
}

async function langExt(path: string): Promise<Extension[]> {
  try {
    return [await loadLanguage(languageIdFor(path))];
  } catch {
    return [];
  }
}

async function readWork(path: string): Promise<string> {
  const buf = editors.buffers.get(path);
  if (buf) return buf.text();
  return (await files.read(path)).content;
}

async function saveResult(path: string, text: string): Promise<void> {
  const buf = editors.buffers.get(path);
  if (buf) {
    const v = buf.views.values().next().value as EditorView | undefined;
    if (v) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: text }, userEvent: 'input.merge' });
      await editors.save(path);
      return;
    }
  }
  const f = await files.read(path);
  await files.write(path, text, f.meta);
}

export async function openMerge(path: string): Promise<void> {
  let ours: string;
  let theirs: string;
  let work: string;
  try {
    const [o, t] = await Promise.all([gitApi.show(':2', path), gitApi.show(':3', path)]);
    ours = o.found ? o.text : '';
    theirs = t.found ? t.text : '';
    work = await readWork(path);
  } catch (err) {
    gitError('Cannot open the merge editor', err);
    return;
  }
  const lang = await langExt(path);
  const ro = [EditorState.readOnly.of(true), EditorView.editable.of(false), lineNumbers(), ...lang];
  const left = h('div', { class: 'merge-pane' });
  const mid = h('div', { class: 'merge-pane' });
  const right = h('div', { class: 'merge-pane' });
  const yours = new EditorView({ parent: left, state: EditorState.create({ doc: ours, extensions: ro }) });
  const result = new EditorView({ parent: mid, state: EditorState.create({ doc: work, extensions: [lineNumbers(), ...lang, conflictHighlight, EditorView.updateListener.of((u) => u.docChanged && update())] }) });
  const theirsView = new EditorView({ parent: right, state: EditorState.create({ doc: theirs, extensions: ro }) });

  let current = 0;
  const info = h('span', { class: 'merge-info' });
  const blocks = (): ConflictBlock[] => conflictBlocks(result.state.doc.toString());
  const reveal = (): void => {
    const bs = blocks();
    if (!bs.length) return;
    current = Math.max(0, Math.min(current, bs.length - 1));
    const line = result.state.doc.line(Math.min(bs[current].start + 1, result.state.doc.lines));
    result.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) });
  };
  const update = (): void => {
    const n = blocks().length;
    info.textContent = n ? `Conflict ${Math.min(current + 1, n)} of ${n}` : 'All conflicts resolved';
    for (const b of actionBtns) b.disabled = n === 0;
  };
  const accept = (side: Side): void => {
    const bs = blocks();
    if (!bs.length) return;
    const text = resolveBlock(result.state.doc.toString(), bs[Math.min(current, bs.length - 1)], side);
    result.dispatch({ changes: { from: 0, to: result.state.doc.length, insert: text }, userEvent: 'input.merge' });
    reveal();
  };
  const acceptAll = (side: Side): void => {
    result.dispatch({ changes: { from: 0, to: result.state.doc.length, insert: resolveAll(result.state.doc.toString(), side) }, userEvent: 'input.merge' });
  };
  const actionBtns = [
    h('button', { class: 'btn', onClick: () => accept('ours') }, '← Accept Yours'),
    h('button', { class: 'btn', onClick: () => accept('both') }, 'Accept Both'),
    h('button', { class: 'btn', onClick: () => accept('theirs') }, 'Accept Theirs →'),
  ];
  const nav = (d: number): void => {
    const n = blocks().length;
    if (!n) return;
    current = (current + d + n) % n;
    reveal();
    update();
  };
  const bar = h(
    'div',
    { class: 'merge-bar' },
    h('button', { class: 'btn', title: 'Previous conflict', onClick: () => nav(-1) }, '↑'),
    h('button', { class: 'btn', title: 'Next conflict', onClick: () => nav(1) }, '↓'),
    info,
    ...actionBtns,
    h('span', { class: 'commit-spacer' }),
    h('button', { class: 'btn', onClick: () => acceptAll('ours') }, 'Accept All Yours'),
    h('button', { class: 'btn', onClick: () => acceptAll('theirs') }, 'Accept All Theirs'),
  );
  const body = h(
    'div',
    { class: 'merge-dialog' },
    bar,
    h('div', { class: 'merge-heads' }, h('span', null, 'Yours (current branch)'), h('span', null, 'Result'), h('span', null, 'Theirs (incoming)')),
    h('div', { class: 'merge-panes' }, left, mid, right),
  );
  update();
  queueMicrotask(reveal);
  for (;;) {
    const choice = await showDialog<'apply' | 'cancel'>({
      title: `Merge Conflicts: ${baseName(path)}`,
      body,
      cancelValue: 'cancel',
      buttons: [
        { label: 'Cancel', value: 'cancel' },
        { label: 'Apply', value: 'apply', primary: true },
      ],
    });
    if (choice === 'cancel') break;
    const text = result.state.doc.toString();
    if (conflictBlocks(text).length && !(await confirmDialog('Unresolved Conflicts', 'The result still has conflict markers. Save and mark the file resolved anyway?', 'Save Anyway', true))) continue;
    try {
      await saveResult(path, text);
      await gitApi.stage([path]);
      gitInfo(`${baseName(path)} resolved`);
    } catch (err) {
      gitError('Cannot save the merge result', err);
      continue;
    }
    break;
  }
  yours.destroy();
  result.destroy();
  theirsView.destroy();
}
