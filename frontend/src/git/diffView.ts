// Diff viewer (R4): side-by-side or unified comparison over the editor area,
// one tab per opened diff. Working-tree diffs can stage or revert a change;
// staged diffs can unstage it (partial staging through git apply).

import { MergeView, unifiedMergeView } from '@codemirror/merge';
import { EditorState } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { EditorView, lineNumbers } from '@codemirror/view';
import { confirmDialog } from '../app/dialog';
import { project } from '../app/state';
import { baseName } from '../editor/buffer';
import { editors } from '../editor/install';
import { languageIdFor, loadLanguage } from '../editor/languages';
import { files, gitApi } from '../lib/api';
import type { GitFileDiff } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { iconButton } from '../run/icons';
import { buildPatch, changedRange } from './patch';
import { gitError, gitState, gitWorkbench } from './state';

export type DiffMode = 'work' | 'staged' | 'head' | 'commit' | 'text';

export interface DiffRequest {
  path: string;
  mode: DiffMode;
  /** Commit hash for mode "commit". */
  commit?: string;
  /** Old path for renames. */
  oldPath?: string;
  /** Mode "text": the left side, and labels for both (the right side is the file as the editor has it). */
  left?: string;
  leftLabel?: string;
  rightLabel?: string;
}

interface DiffTab {
  key: string;
  req: DiffRequest;
  el: HTMLElement;
  tab: HTMLElement;
  body: HTMLElement;
  info: HTMLElement;
  view: MergeView | EditorView | null;
  fd: GitFileDiff | null;
  hunk: number;
  left: string;
  right: string;
  unified: boolean;
}

const tabs = signal<DiffTab[]>([]);
const active = signal('');
let host: HTMLElement | null = null;
let strip: HTMLElement | null = null;
let bodies: HTMLElement | null = null;

const LABELS: Record<DiffMode, [string, string]> = {
  work: ['Staged (index)', 'Working tree'],
  staged: ['HEAD', 'Staged (index)'],
  head: ['HEAD', 'Working tree'],
  commit: ['Parent', 'Commit'],
  text: ['Before', 'Now'],
};

async function readWork(path: string): Promise<string> {
  const buf = editors.buffers.get(path);
  if (buf) return buf.text();
  try {
    return (await files.read(path)).content;
  } catch {
    return '';
  }
}

async function show(rev: string, path: string): Promise<string> {
  const r = await gitApi.show(rev, path);
  return r.found ? r.text : '';
}

/** Loads both sides and the git hunks for a request. */
async function load(req: DiffRequest): Promise<{ left: string; right: string; fd: GitFileDiff | null }> {
  const old = req.oldPath || req.path;
  let left = '';
  let right = '';
  let fds: GitFileDiff[] = [];
  switch (req.mode) {
    case 'work': {
      const idx = await gitApi.show(':0', old);
      left = idx.found ? idx.text : await show('HEAD', old);
      right = await readWork(req.path);
      fds = await gitApi.diff(req.path, false);
      break;
    }
    case 'staged':
      left = await show('HEAD', old);
      right = await show(':0', req.path);
      fds = await gitApi.diff(req.path, true);
      break;
    case 'head':
      left = await show('HEAD', old);
      right = await readWork(req.path);
      fds = await gitApi.diff(req.path, false, 'HEAD');
      break;
    case 'commit':
      left = await show(`${req.commit}^`, old);
      right = await show(req.commit ?? 'HEAD', req.path);
      fds = await gitApi.diff(req.path, false, '', req.commit);
      break;
    case 'text':
      left = req.left ?? '';
      right = await readWork(req.path);
      break;
  }
  return { left, right, fd: fds[0] ?? null };
}

async function extensionsFor(path: string): Promise<Extension[]> {
  const ext: Extension[] = [EditorState.readOnly.of(true), EditorView.editable.of(false), lineNumbers()];
  try {
    ext.push(await loadLanguage(languageIdFor(path)));
  } catch {
    // plain text
  }
  return ext;
}

async function render(t: DiffTab): Promise<void> {
  const ext = await extensionsFor(t.req.path);
  if (t.view) t.view.destroy();
  t.body.replaceChildren();
  if (t.unified) {
    t.view = new EditorView({
      parent: t.body,
      state: EditorState.create({ doc: t.right, extensions: [...ext, unifiedMergeView({ original: t.left, mergeControls: false, gutter: true })] }),
    });
  } else {
    const [l, r] = t.req.mode === 'text' ? [t.req.leftLabel ?? LABELS.text[0], t.req.rightLabel ?? LABELS.text[1]] : LABELS[t.req.mode];
    t.body.append(h('div', { class: 'diff-sides' }, h('span', null, l), h('span', null, r)));
    const holder = h('div', { class: 'diff-mv' });
    t.body.append(holder);
    t.view = new MergeView({ a: { doc: t.left, extensions: ext }, b: { doc: t.right, extensions: ext }, parent: holder, gutter: true, highlightChanges: true });
  }
  updateInfo(t);
}

function rightView(t: DiffTab): EditorView | null {
  if (!t.view) return null;
  return t.view instanceof MergeView ? t.view.b : t.view;
}

function updateInfo(t: DiffTab): void {
  const n = t.fd?.hunks.length ?? 0;
  const actions: HTMLElement[] = [];
  if (n && t.req.mode === 'work') {
    actions.push(
      h('button', { class: 'btn', onClick: () => void hunkAction(t, 'stage') }, 'Stage Change'),
      h('button', { class: 'btn', onClick: () => void hunkAction(t, 'revert') }, 'Revert Change'),
    );
  } else if (n && t.req.mode === 'staged') {
    actions.push(h('button', { class: 'btn', onClick: () => void hunkAction(t, 'unstage') }, 'Unstage Change'));
  }
  const summary = t.req.mode === 'text' ? (t.left === t.right ? 'No differences' : '') : n ? `Change ${t.hunk + 1} of ${n}` : t.fd?.binary ? 'Binary file' : 'No differences';
  t.info.replaceChildren(
    h('span', { class: 'diff-count' }, summary),
    ...actions,
  );
}

function goto(t: DiffTab, i: number): void {
  const n = t.fd?.hunks.length ?? 0;
  if (!n) return;
  t.hunk = (i + n) % n;
  const v = rightView(t);
  if (v && t.fd) {
    const r = changedRange(t.fd.hunks[t.hunk]);
    const line = v.state.doc.line(Math.min(r.from, v.state.doc.lines));
    v.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) });
  }
  updateInfo(t);
}

async function hunkAction(t: DiffTab, kind: 'stage' | 'unstage' | 'revert'): Promise<void> {
  if (!t.fd) return;
  try {
    if (kind === 'revert') {
      const buf = editors.buffers.get(t.req.path);
      if (buf?.dirty.peek()) {
        gitError('Save the file first', 'Revert works on the saved file.');
        return;
      }
      if (!(await confirmDialog('Revert Change', 'Discard this change in the working tree?', 'Revert', true))) return;
    }
    const patch = buildPatch(t.fd, [t.hunk]);
    if (kind === 'stage') await gitApi.applyPatch(patch, true, false);
    else if (kind === 'unstage') await gitApi.applyPatch(patch, true, true);
    else await gitApi.applyPatch(patch, false, true);
  } catch (err) {
    gitError(`Cannot ${kind} the change`, err);
    return;
  }
  await reload(t);
}

async function reload(t: DiffTab): Promise<void> {
  const keep = t.hunk;
  try {
    const d = await load(t.req);
    Object.assign(t, d);
  } catch (err) {
    gitError('Cannot load diff', err);
    return;
  }
  t.hunk = Math.min(keep, Math.max(0, (t.fd?.hunks.length ?? 1) - 1));
  await render(t);
  goto(t, t.hunk);
}

function ensureHost(): void {
  if (host) return;
  strip = h('div', { class: 'diff-tabs' });
  bodies = h('div', { class: 'diff-bodies' });
  host = h('div', { class: 'diff-host-area', hidden: true }, strip, bodies);
  const center = gitWorkbench().center;
  center.style.position = 'relative';
  center.append(host);
  host.addEventListener('keydown', (e) => {
    const t = tabs.peek().find((x) => x.key === active.peek());
    if (!t) return;
    if (e.key === 'F7') {
      e.preventDefault();
      e.stopPropagation();
      goto(t, t.hunk + (e.shiftKey ? -1 : 1));
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeDiff(t.key);
    }
  });
  effect(() => {
    const list = tabs();
    const a = active();
    if (host) host.hidden = list.length === 0;
    for (const t of list) {
      t.tab.classList.toggle('active', t.key === a);
      t.el.hidden = t.key !== a;
    }
  });
  // Diffs belong to the project.
  effect(() => {
    if (!project()) for (const t of tabs.peek()) closeDiff(t.key);
  });
}

export function closeDiff(key: string): void {
  const t = tabs.peek().find((x) => x.key === key);
  if (!t) return;
  t.view?.destroy();
  t.tab.remove();
  t.el.remove();
  const rest = tabs.peek().filter((x) => x.key !== key);
  tabs.set(rest);
  if (active.peek() === key) active.set(rest[rest.length - 1]?.key ?? '');
}

export function closeAllDiffs(): void {
  for (const t of tabs.peek()) closeDiff(t.key);
}

/** Opens (or focuses) a diff tab. */
export async function openDiff(req: DiffRequest): Promise<void> {
  ensureHost();
  const key = `${req.mode}:${req.commit ?? ''}:${req.path}`;
  const existing = tabs.peek().find((t) => t.key === key);
  if (existing) {
    active.set(key);
    await reload(existing);
    return;
  }
  let data: { left: string; right: string; fd: GitFileDiff | null };
  try {
    data = await load(req);
  } catch (err) {
    gitError('Cannot load diff', err);
    return;
  }
  const info = h('div', { class: 'diff-info' });
  const body = h('div', { class: 'diff-body' });
  const t: DiffTab = { key, req, el: h('div'), tab: h('div'), body, info, view: null, hunk: 0, unified: false, ...data };
  const modeBtn = iconButton('wrap', 'Toggle unified / side-by-side', async () => {
    t.unified = !t.unified;
    modeBtn.classList.toggle('on', t.unified);
    await render(t);
    goto(t, t.hunk);
  });
  const title = `${baseName(req.path)}${req.mode === 'commit' ? ` @ ${req.commit?.slice(0, 7)}` : req.mode === 'staged' ? ' (staged)' : req.mode === 'text' && req.leftLabel ? ` (${req.leftLabel})` : ''}`;
  t.el = h(
    'div',
    { class: 'diff-tab-body', tabindex: '-1' },
    h(
      'div',
      { class: 'diff-toolbar' },
      iconButton('up', 'Previous change (Shift+F7)', () => goto(t, t.hunk - 1)),
      iconButton('down', 'Next change (F7)', () => goto(t, t.hunk + 1)),
      modeBtn,
      iconButton('rerun', 'Refresh', () => void reload(t)),
      info,
      h('span', { class: 'diff-path' }, req.path),
    ),
    body,
  );
  t.tab = h(
    'div',
    { class: 'run-tab', onClick: () => active.set(key), title: req.path },
    h('span', { class: 'run-tab-label' }, `Diff: ${title}`),
    iconButton('close', 'Close diff', () => closeDiff(key), 'run-tab-close'),
  );
  strip!.append(t.tab);
  bodies!.append(t.el);
  tabs.set([...tabs.peek(), t]);
  active.set(key);
  await render(t);
  goto(t, 0);
  t.el.focus();
}

/** Refreshes open working-tree diffs after status changes. */
export function installDiffRefresh(): void {
  let last = '';
  effect(() => {
    const st = gitState();
    const sig = JSON.stringify(st.status.files.map((f) => [f.path, f.index, f.work]));
    if (sig === last) return;
    last = sig;
    for (const t of tabs.peek()) if (t.req.mode !== 'commit') void reload(t);
  });
}
