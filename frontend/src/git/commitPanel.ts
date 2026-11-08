// Commit tool window (R3): Merge Conflicts / Staged / Changes groups with
// per-file and per-group stage, unstage and rollback; a message box with
// Amend; Commit and Commit and Push. The same form opens as a modal dialog.

import { confirmDialog, showDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { project } from '../app/state';
import { baseName, dirName } from '../editor/buffer';
import { editors } from '../editor/install';
import { gitApi } from '../lib/api';
import type { GitFileStatus } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { iconButton } from '../run/icons';
import { runGitOp } from './console';
import { openDiff } from './diffView';
import { fileKind, gitError, gitInfo, gitState, gitWorkbench } from './state';

export type Group = 'conflicts' | 'staged' | 'changes';

export interface Grouped {
  conflicts: GitFileStatus[];
  staged: GitFileStatus[];
  changes: GitFileStatus[];
}

/** Splits files into the panel's groups (a file can be staged and changed). */
export function groupFiles(files: readonly GitFileStatus[]): Grouped {
  const g: Grouped = { conflicts: [], staged: [], changes: [] };
  const byRel = (a: GitFileStatus, b: GitFileStatus): number => a.rel.localeCompare(b.rel);
  for (const f of files) {
    if (f.ignored) continue;
    if (f.conflict) {
      g.conflicts.push(f);
      continue;
    }
    if (!f.untracked && f.index !== '.') g.staged.push(f);
    if (f.untracked || f.work !== '.') g.changes.push(f);
  }
  g.conflicts.sort(byRel);
  g.staged.sort(byRel);
  g.changes.sort(byRel);
  return g;
}

/** Status letter shown for a file in a group. */
export function letter(f: GitFileStatus, group: Group): string {
  if (group === 'conflicts') return 'C';
  if (f.untracked) return 'U';
  const c = group === 'staged' ? f.index : f.work;
  return c === '.' ? 'M' : c;
}

/** The commit form state shared by the panel and the modal dialog. */
const message = signal('');
const amend = signal(false);
let savedMessage = '';

async function setAmend(on: boolean): Promise<void> {
  amend.set(on);
  if (on) {
    savedMessage = message.peek();
    const last = await gitApi.lastMessage().catch(() => '');
    message.set(last);
  } else {
    message.set(savedMessage);
  }
}

/** Commits (optionally pushing after). Returns true on success. */
export async function doCommit(push: boolean): Promise<boolean> {
  const msg = message.peek().trim();
  if (!msg) {
    gitError('Enter a commit message', 'The commit message is empty.');
    return false;
  }
  const g = groupFiles(gitState.peek().status.files);
  if (g.conflicts.length) {
    gitError('Resolve conflicts first', `${g.conflicts.length} file(s) still have merge conflicts.`);
    return false;
  }
  if (!g.staged.length && !amend.peek()) {
    if (!g.changes.length) {
      gitError('Nothing to commit', 'There are no changes.');
      return false;
    }
    const ok = await confirmDialog('No Staged Changes', `Stage all ${g.changes.length} changed file(s) and commit?`, 'Stage All and Commit');
    if (!ok) return false;
    try {
      await gitApi.stage(g.changes.map((f) => f.path));
    } catch (err) {
      gitError('Cannot stage changes', err);
      return false;
    }
  }
  // Save dirty buffers of staged files? Commit uses the index, so unsaved
  // edits are simply not included; warn about them.
  const dirty = editors.dirtyBuffers().filter((b) => gitState.peek().status.files.some((f) => f.path === b.path));
  if (dirty.length) {
    const ok = await confirmDialog('Unsaved Changes', `${dirty.length} changed file(s) have unsaved edits that won’t be committed. Commit anyway?`, 'Commit');
    if (!ok) return false;
  }
  try {
    await gitApi.commit(message.peek(), amend.peek());
  } catch (err) {
    gitError('Commit failed', err);
    return false;
  }
  gitInfo(amend.peek() ? 'Commit amended' : 'Committed', msg.split('\n')[0]);
  message.set('');
  savedMessage = '';
  amend.set(false);
  if (push) await runGitOp('push');
  return true;
}

function commitForm(onDone?: () => void): HTMLElement {
  const area = h('textarea', { class: 'input commit-message', placeholder: 'Commit message (Mod+Enter to commit)', spellcheck: true, rows: 4 });
  area.value = message.peek();
  area.addEventListener('input', () => message.set(area.value));
  area.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      e.stopPropagation();
      void doCommit(e.shiftKey).then((ok) => ok && onDone?.());
    }
  });
  const amendBox = h('input', { type: 'checkbox' });
  amendBox.addEventListener('change', () => void setAmend(amendBox.checked));
  effect(() => {
    if (area.value !== message()) area.value = message();
    amendBox.checked = amend();
  });
  return h(
    'div',
    { class: 'commit-form' },
    area,
    h(
      'div',
      { class: 'commit-actions' },
      h('label', { class: 'rc-check' }, amendBox, ' Amend'),
      h('span', { class: 'commit-spacer' }),
      h('button', { class: 'btn', onClick: () => void doCommit(true).then((ok) => ok && onDone?.()) }, 'Commit and Push…'),
      h('button', { class: 'btn btn-primary', onClick: () => void doCommit(false).then((ok) => ok && onDone?.()) }, 'Commit'),
    ),
  );
}

async function rollback(paths: string[]): Promise<void> {
  if (!paths.length) return;
  const ok = await confirmDialog('Rollback Changes', `Discard changes in ${paths.length === 1 ? baseName(paths[0]) : `${paths.length} files`}? New files are moved to the trash.`, 'Rollback', true);
  if (!ok) return;
  try {
    await gitApi.rollback(paths);
  } catch (err) {
    gitError('Rollback failed', err);
  }
}

function fileMenu(f: GitFileStatus, group: Group): MenuItem[] {
  const items: MenuItem[] = [];
  if (group === 'conflicts') {
    items.push(
      { label: 'Resolve…', run: () => void import('./merge').then((m) => m.openMerge(f.path)) },
      { label: 'Accept Yours', run: () => void gitApi.acceptSide(f.path, 'ours').catch((e) => gitError('Accept Yours failed', e)) },
      { label: 'Accept Theirs', run: () => void gitApi.acceptSide(f.path, 'theirs').catch((e) => gitError('Accept Theirs failed', e)) },
      { separator: true },
    );
  } else {
    items.push({ label: 'Show Diff', run: () => void openDiff({ path: f.path, mode: group === 'staged' ? 'staged' : 'work', oldPath: f.orig || undefined }) });
    if (group === 'staged') items.push({ label: 'Unstage', run: () => void gitApi.unstage([f.path]).catch((e) => gitError('Unstage failed', e)) });
    else items.push({ label: 'Stage', run: () => void gitApi.stage([f.path]).catch((e) => gitError('Stage failed', e)) });
    items.push({ label: 'Rollback…', danger: true, run: () => void rollback([f.path]) }, { separator: true });
  }
  items.push({ label: 'Open', disabled: fileKind(f) === 'deleted', run: () => void editors.open(f.path, { focus: true }) });
  if (f.untracked) items.push({ label: 'Add to .gitignore', run: () => void gitApi.addToGitignore([f.path]).catch((e) => gitError('Cannot update .gitignore', e)) });
  if (group !== 'conflicts') items.push({ label: 'Show History', run: () => void import('./log').then((m) => m.showHistory(f.path)) });
  return items;
}

export function renderCommitPanel(container: HTMLElement): () => void {
  const list = h('div', { class: 'commit-list', role: 'tree', tabindex: '0' });
  const banner = h('div', { class: 'commit-banner', hidden: true });
  const empty = h('div', { class: 'results-empty' });
  const toolbar = h(
    'div',
    { class: 'commit-toolbar' },
    iconButton('rerun', 'Refresh', () => void gitApi.refresh()),
    iconButton('plus', 'Stage all changes', () => {
      const g = groupFiles(gitState.peek().status.files);
      void gitApi.stage(g.changes.map((f) => f.path)).catch((e) => gitError('Stage failed', e));
    }),
    iconButton('minus', 'Unstage all', () => {
      const g = groupFiles(gitState.peek().status.files);
      void gitApi.unstage(g.staged.map((f) => f.path)).catch((e) => gitError('Unstage failed', e));
    }),
    iconButton('clear', 'Rollback all changes…', () => {
      const g = groupFiles(gitState.peek().status.files);
      void rollback([...new Set([...g.staged, ...g.changes].map((f) => f.path))]);
    }),
  );
  container.append(h('div', { class: 'commit-panel' }, toolbar, banner, list, empty, commitForm()));
  const collapsed = new Set<Group>();

  const row = (f: GitFileStatus, group: Group): HTMLElement => {
    const k = fileKind(f);
    const action =
      group === 'staged'
        ? iconButton('minus', 'Unstage', () => void gitApi.unstage([f.path]).catch((e) => gitError('Unstage failed', e)), 'commit-row-action')
        : group === 'changes'
          ? iconButton('plus', 'Stage', () => void gitApi.stage([f.path]).catch((e) => gitError('Stage failed', e)), 'commit-row-action')
          : iconButton('settings', 'Resolve…', () => void import('./merge').then((m) => m.openMerge(f.path)), 'commit-row-action');
    const el = h(
      'div',
      { class: `commit-row vcs-${k}`, role: 'treeitem', title: f.path, dataset: { path: f.path, group } },
      h('span', { class: 'commit-letter' }, letter(f, group)),
      h('span', { class: 'commit-name' }, baseName(f.path)),
      h('span', { class: 'commit-dir' }, dirName(f.rel) === f.rel ? '' : dirName(f.rel)),
      action,
    );
    return el;
  };

  list.addEventListener('click', (e) => {
    const r = (e.target as HTMLElement).closest('.commit-row') as HTMLElement | null;
    if (r?.dataset.path) {
      const group = r.dataset.group as Group;
      const f = gitState.peek().status.files.find((x) => x.path === r.dataset.path);
      if (!f) return;
      if (group === 'conflicts') void import('./merge').then((m) => m.openMerge(f.path));
      else void openDiff({ path: f.path, mode: group === 'staged' ? 'staged' : 'work', oldPath: f.orig || undefined });
      return;
    }
    const head = (e.target as HTMLElement).closest('.commit-group') as HTMLElement | null;
    if (head?.dataset.group) {
      const g = head.dataset.group as Group;
      if (collapsed.has(g)) collapsed.delete(g);
      else collapsed.add(g);
      gitState.set({ ...gitState.peek() });
    }
  });
  list.addEventListener('dblclick', (e) => {
    const r = (e.target as HTMLElement).closest('.commit-row') as HTMLElement | null;
    if (r?.dataset.path) void editors.open(r.dataset.path, { focus: true });
  });
  list.addEventListener('contextmenu', (e) => {
    const r = (e.target as HTMLElement).closest('.commit-row') as HTMLElement | null;
    if (!r?.dataset.path) return;
    const f = gitState.peek().status.files.find((x) => x.path === r.dataset.path);
    if (!f) return;
    e.preventDefault();
    showMenu(e.clientX, e.clientY, fileMenu(f, r.dataset.group as Group));
  });

  return effect(() => {
    const st = gitState();
    const p = project();
    if (!p || !st.repo) {
      list.replaceChildren();
      banner.hidden = true;
      empty.hidden = false;
      empty.replaceChildren(h('div', null, p ? 'The project is not in a git repository.' : 'No project'));
      if (p) empty.append(h('button', { class: 'btn', onClick: () => void import('../lib/commands').then((m) => m.commands.run('git.init')) }, 'Initialize Repository…'));
      return;
    }
    banner.hidden = !st.status.op;
    if (st.status.op) {
      const op = st.status.op;
      banner.replaceChildren(
        h('span', null, `${op[0].toUpperCase()}${op.slice(1)} in progress`),
        h('button', { class: 'btn', onClick: () => void gitApi.continueOp().catch((e) => gitError('Continue failed', e)) }, 'Continue'),
      );
      if (op === 'rebase' || op === 'cherry-pick') banner.append(h('button', { class: 'btn', onClick: () => void gitApi.skipOp().catch((e) => gitError('Skip failed', e)) }, 'Skip'));
      banner.append(h('button', { class: 'btn btn-danger', onClick: () => void gitApi.abortOp().catch((e) => gitError('Abort failed', e)) }, 'Abort'));
    }
    const g = groupFiles(st.status.files);
    const rows: HTMLElement[] = [];
    const section = (group: Group, title: string, files: GitFileStatus[]): void => {
      if (!files.length) return;
      const open = !collapsed.has(group);
      rows.push(h('div', { class: 'commit-group', dataset: { group } }, h('span', null, open ? '▾' : '▸'), h('span', null, title), h('span', { class: 'count' }, String(files.length))));
      if (open) for (const f of files) rows.push(row(f, group));
    };
    section('conflicts', 'Merge Conflicts', g.conflicts);
    section('staged', 'Staged', g.staged);
    section('changes', 'Changes', g.changes);
    list.replaceChildren(...rows);
    empty.hidden = rows.length > 0;
    empty.textContent = rows.length ? '' : 'No changes';
  });
}

/** The commit form in a modal dialog. */
export async function openCommitDialog(): Promise<void> {
  const g = groupFiles(gitState.peek().status.files);
  let close: (() => void) | null = null;
  const form = commitForm(() => close?.());
  const summary = h('div', { class: 'commit-summary' }, `${g.staged.length} staged, ${g.changes.length} unstaged`);
  const dialog = showDialog({ title: 'Commit Changes', body: h('div', { class: 'commit-modal' }, summary, form), cancelValue: undefined, buttons: [{ label: 'Close', value: undefined }] });
  close = () => document.querySelector('.dialog-backdrop')?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
  queueMicrotask(() => (form.querySelector('textarea') as HTMLTextAreaElement | null)?.focus());
  await dialog;
}

export function installCommitPanel(): void {
  const wb = gitWorkbench();
  wb.panels.register({ id: 'commit', title: 'Commit', dock: 'left', order: 2, render: (c) => renderCommitPanel(c) });
}

/** Focuses the commit message box. */
export function focusCommit(): void {
  gitWorkbench().panels.show('commit');
  requestAnimationFrame(() => (document.querySelector('.commit-panel .commit-message') as HTMLTextAreaElement | null)?.focus());
}
