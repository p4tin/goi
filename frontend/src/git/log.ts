// Git Log window (R8): recent commits for the repository or one file, with a
// filter, details and changed files of the selected commit (opening diffs),
// and Cherry-Pick, Revert, Checkout Revision, New Branch Here, Reset and
// Copy Hash.

import { confirmDialog, promptDialog, showDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import { baseName, dirName } from '../editor/buffer';
import { gitApi } from '../lib/api';
import type { GitChangedFile, GitCommit } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { iconButton } from '../run/icons';
import { validBranchName } from './branches';
import { openDiff } from './diffView';
import { gitError, gitInfo, gitState, gitWorkbench } from './state';

const PAGE = 200;

/** "just now", "5 minutes ago", "yesterday", "3 days ago", else the date. */
export function relativeDate(iso: string, now = Date.now()): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return 'just now';
  const m = Math.round(s / 60);
  if (m < 60) return `${m} minute${m === 1 ? '' : 's'} ago`;
  const hrs = Math.round(m / 60);
  if (hrs < 24) return `${hrs} hour${hrs === 1 ? '' : 's'} ago`;
  const d = Math.round(hrs / 24);
  if (d === 1) return 'yesterday';
  if (d < 7) return `${d} days ago`;
  return new Date(t).toISOString().slice(0, 10);
}

/** Commits whose hash, subject, author or refs contain q. */
export function filterCommits(list: readonly GitCommit[], q: string): GitCommit[] {
  const n = q.trim().toLowerCase();
  if (!n) return [...list];
  return list.filter((c) => c.hash.startsWith(n) || c.subject.toLowerCase().includes(n) || c.author.toLowerCase().includes(n) || c.refs.toLowerCase().includes(n));
}

const commits = signal<GitCommit[]>([]);
const selected = signal('');
const pathFilter = signal('');
let exhausted = false;
let loading = false;

async function load(reset: boolean): Promise<void> {
  if (loading || (!reset && exhausted)) return;
  loading = true;
  try {
    const page = await gitApi.log({ path: pathFilter.peek(), skip: reset ? 0 : commits.peek().length, max: PAGE });
    exhausted = page.length < PAGE;
    commits.set(reset ? page : [...commits.peek(), ...page]);
    if (reset && page.length && !page.some((c) => c.hash === selected.peek())) selected.set(page[0].hash);
  } catch (err) {
    gitError('Cannot read the log', err);
  } finally {
    loading = false;
  }
}

export async function showHistory(path: string): Promise<void> {
  pathFilter.set(path);
  gitWorkbench().panels.show('gitlog');
  await load(true);
}

export async function showCommit(hash: string): Promise<void> {
  pathFilter.set('');
  gitWorkbench().panels.show('gitlog');
  await load(true);
  if (!commits.peek().some((c) => c.hash.startsWith(hash))) {
    // Older than the first page: page until found (bounded).
    for (let i = 0; i < 10 && !exhausted && !commits.peek().some((c) => c.hash.startsWith(hash)); i++) await load(false);
  }
  const c = commits.peek().find((x) => x.hash.startsWith(hash));
  if (c) selected.set(c.hash);
}

async function act(label: string, fn: () => Promise<void>, done?: string): Promise<void> {
  try {
    await fn();
    if (done) gitInfo(done);
    await load(true);
  } catch (err) {
    const conflicted = !!gitState.peek().status.op;
    gitError(conflicted ? `${label} stopped on conflicts` : `${label} failed`, conflicted ? 'Resolve them in the Commit window, then Continue.' : err);
    if (conflicted) gitWorkbench().panels.show('commit');
  }
}

async function reset(c: GitCommit): Promise<void> {
  const mode = await showDialog<'soft' | 'mixed' | 'hard' | ''>({
    title: `Reset Current Branch to ${c.short}`,
    body: h(
      'div',
      { class: 'git-reset' },
      h('p', null, h('b', null, 'Soft'), ': keep changes staged.'),
      h('p', null, h('b', null, 'Mixed'), ': keep changes in the working tree, unstaged.'),
      h('p', null, h('b', null, 'Hard'), ': discard all changes after this commit.'),
    ),
    cancelValue: '',
    buttons: [
      { label: 'Cancel', value: '' },
      { label: 'Hard', value: 'hard', danger: true },
      { label: 'Soft', value: 'soft' },
      { label: 'Mixed', value: 'mixed', primary: true },
    ],
  });
  if (!mode) return;
  if (mode === 'hard' && !(await confirmDialog('Hard Reset', 'This discards uncommitted changes and later commits on this branch. Continue?', 'Reset', true))) return;
  await act('Reset', () => gitApi.reset(c.hash, mode), `Reset to ${c.short} (${mode})`);
}

function commitMenu(c: GitCommit, x: number, y: number): void {
  showMenu(x, y, [
    { label: 'Cherry-Pick', run: () => void act('Cherry-pick', () => gitApi.cherryPick(c.hash), `Cherry-picked ${c.short}`) },
    {
      label: 'Revert Commit',
      run: async () => {
        if (await confirmDialog('Revert Commit', `Create a commit that undoes ${c.short} "${c.subject}"?`, 'Revert')) await act('Revert', () => gitApi.revert(c.hash), `Reverted ${c.short}`);
      },
    },
    { separator: true },
    {
      label: 'Checkout Revision',
      run: async () => {
        if (await confirmDialog('Checkout Revision', `Check out ${c.short}? HEAD will be detached.`, 'Checkout')) await act('Checkout', () => gitApi.checkout(c.hash), `HEAD at ${c.short}`);
      },
    },
    {
      label: 'New Branch Here…',
      run: async () => {
        const name = await promptDialog(`New Branch at ${c.short}`, '', validBranchName, 'Branch name');
        if (name) await act('New branch', () => gitApi.checkout(c.hash, name), `Created ${name}`);
      },
    },
    { label: 'Reset Current Branch to Here…', danger: true, run: () => void reset(c) },
    { separator: true },
    { label: 'Copy Hash', run: () => void navigator.clipboard?.writeText(c.hash) },
  ]);
}

export function renderLog(container: HTMLElement): () => void {
  const filter = h('input', { class: 'input git-log-filter', type: 'search', placeholder: 'Filter by text, author or hash', spellcheck: false });
  const pathChip = h('span', { class: 'git-log-path' });
  const list = h('div', { class: 'git-log-list', role: 'listbox', tabindex: '0' });
  const details = h('div', { class: 'git-log-details' });
  const head = h(
    'div',
    { class: 'git-log-head' },
    filter,
    pathChip,
    iconButton('rerun', 'Refresh', () => void load(true)),
  );
  container.append(h('div', { class: 'git-log' }, h('div', { class: 'git-log-left' }, head, list), details));
  filter.addEventListener('input', () => commits.set([...commits.peek()]));

  list.addEventListener('click', (e) => {
    const r = (e.target as HTMLElement).closest('.git-log-row') as HTMLElement | null;
    if (r?.dataset.hash) selected.set(r.dataset.hash);
    if ((e.target as HTMLElement).closest('.git-log-more')) void load(false);
  });
  list.addEventListener('contextmenu', (e) => {
    const r = (e.target as HTMLElement).closest('.git-log-row') as HTMLElement | null;
    const c = commits.peek().find((x) => x.hash === r?.dataset.hash);
    if (!c) return;
    e.preventDefault();
    selected.set(c.hash);
    commitMenu(c, e.clientX, e.clientY);
  });
  list.addEventListener('keydown', (e) => {
    const shown = filterCommits(commits.peek(), filter.value);
    const i = shown.findIndex((c) => c.hash === selected.peek());
    if (e.key === 'ArrowDown' && i < shown.length - 1) selected.set(shown[i + 1].hash);
    else if (e.key === 'ArrowUp' && i > 0) selected.set(shown[i - 1].hash);
    else return;
    e.preventDefault();
  });
  list.addEventListener('scroll', () => {
    if (list.scrollTop + list.clientHeight > list.scrollHeight - 200 && !filter.value) void load(false);
  });

  let filesSeq = 0;
  const stopList = effect(() => {
    const shown = filterCommits(commits(), filter.value);
    const sel = selected();
    const p = pathFilter();
    pathChip.hidden = !p;
    pathChip.replaceChildren();
    if (p) pathChip.append(`History of ${baseName(p)} `, h('button', { class: 'git-chip-x', title: 'Show all', onClick: () => void showHistory('') }, '×'));
    const rows = shown.map((c) =>
      h(
        'div',
        { class: `git-log-row${c.hash === sel ? ' selected' : ''}`, dataset: { hash: c.hash }, title: `${c.hash}\n${c.author} <${c.email}>\n${c.date}` },
        h('span', { class: 'git-log-hash' }, c.short),
        h('span', { class: 'git-log-subject' }, c.refs ? h('span', { class: 'git-log-refs' }, c.refs) : null, c.subject),
        h('span', { class: 'git-log-author' }, c.author),
        h('span', { class: 'git-log-date' }, relativeDate(c.date)),
      ),
    );
    if (!exhausted && !filter.value && rows.length) rows.push(h('div', { class: 'git-log-more' }, 'Load more…'));
    if (!rows.length) rows.push(h('div', { class: 'results-empty' }, gitState.peek().repo ? 'No commits' : 'Not a git repository'));
    list.replaceChildren(...rows);
  });

  const stopDetails = effect(() => {
    const c = commits().find((x) => x.hash === selected());
    if (!c) {
      details.replaceChildren(h('div', { class: 'results-empty' }, 'Select a commit'));
      return;
    }
    const seq = ++filesSeq;
    const filesBox = h('div', { class: 'git-log-files' }, h('div', { class: 'results-empty' }, 'Loading files…'));
    details.replaceChildren(
      h('div', { class: 'git-log-meta' }, h('code', null, c.hash), h('div', null, `${c.author} <${c.email}>`), h('div', null, new Date(Date.parse(c.date)).toLocaleString()), c.refs ? h('div', { class: 'git-log-refs' }, c.refs) : null),
      h('div', { class: 'git-log-message' }, h('b', null, c.subject), c.body ? h('pre', null, c.body) : null),
      filesBox,
    );
    void gitApi
      .commitFiles(c.hash)
      .then((files: GitChangedFile[]) => {
        if (seq !== filesSeq) return;
        filesBox.replaceChildren(
          h('div', { class: 'git-log-files-head' }, `${files.length} file${files.length === 1 ? '' : 's'} changed`),
          ...files.map((f) =>
            h(
              'div',
              { class: `git-log-file vcs-${f.status === 'A' ? 'added' : f.status === 'D' ? 'deleted' : 'modified'}`, title: f.path, onClick: () => void openDiff({ path: f.path, mode: 'commit', commit: c.hash, oldPath: f.orig || undefined }) },
              h('span', { class: 'commit-letter' }, f.status),
              h('span', { class: 'commit-name' }, baseName(f.path)),
              h('span', { class: 'commit-dir' }, dirName(f.rel) === f.rel ? '' : dirName(f.rel)),
            ),
          ),
        );
      })
      .catch((err) => {
        if (seq === filesSeq) filesBox.replaceChildren(h('div', { class: 'results-empty' }, String(err)));
      });
  });

  void load(true);
  // Reload when HEAD moves (commit, checkout, pull).
  let lastHead = '';
  const stopHead = effect(() => {
    const st = gitState();
    const key = `${st.repo}|${st.status.head}|${st.status.branch}`;
    if (key !== lastHead) {
      lastHead = key;
      void load(true);
    }
  });
  return () => {
    stopList();
    stopDetails();
    stopHead();
  };
}

export function installLog(): void {
  gitWorkbench().panels.register({ id: 'gitlog', title: 'Git Log', dock: 'bottom', order: 13, render: (c) => renderLog(c) });
}
