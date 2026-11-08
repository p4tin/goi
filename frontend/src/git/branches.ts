// Branches popup (R5) and stash (R7): checkout (tracking remote branches),
// new/rename/delete, merge and rebase, compare; Stash Changes and the
// stashes list. A checkout blocked by local changes offers Stash and
// Checkout.

import { confirmDialog, promptDialog, showDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { gitApi } from '../lib/api';
import type { GitBranch, GitStash } from '../lib/api';
import { h } from '../lib/dom';
import { showListPopup } from '../lang/popup';
import { runGitOp } from './console';
import { gitError, gitInfo, gitState } from './state';

type Entry = { kind: 'action'; label: string; run: () => void; hint?: string } | { kind: 'header'; label: string } | { kind: 'branch'; b: GitBranch };

/** Popup entries: actions, then local and remote branches (current first). */
export function branchEntries(branches: readonly GitBranch[]): { local: GitBranch[]; remote: GitBranch[] } {
  const local = branches.filter((b) => !b.remote).sort((a, b) => Number(b.current) - Number(a.current) || a.name.localeCompare(b.name));
  const remote = branches.filter((b) => b.remote).sort((a, b) => a.name.localeCompare(b.name));
  return { local, remote };
}

/** Git's check-ref-format rules, simplified: returns an error or null. */
export function validBranchName(name: string): string | null {
  const n = name.trim();
  if (!n) return 'Enter a branch name';
  if (/[\s~^:?*[\\]|\.\.|@\{|^[-/.]|[/.]$|\.lock$|\/\//.test(n)) return 'Not a valid branch name';
  return null;
}

async function checkout(ref: string, newBranch = ''): Promise<void> {
  try {
    await gitApi.checkout(ref, newBranch);
    gitInfo(`Checked out ${newBranch || ref}`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    if (/would be overwritten|commit your changes or stash them/i.test(msg)) {
      const choice = await showDialog<'stash' | 'cancel'>({
        title: 'Checkout Blocked by Local Changes',
        body: h('div', null, h('p', null, 'Your local changes would be overwritten by this checkout.'), h('pre', { class: 'git-error-pre' }, msg)),
        cancelValue: 'cancel',
        buttons: [
          { label: 'Cancel', value: 'cancel' },
          { label: 'Stash and Checkout', value: 'stash', primary: true },
        ],
      });
      if (choice !== 'stash') return;
      try {
        await gitApi.stash(`Goi: before checkout ${newBranch || ref}`, true);
        await gitApi.checkout(ref, newBranch);
        gitInfo(`Checked out ${newBranch || ref}`, 'Your changes are in the stash.');
      } catch (e) {
        gitError('Checkout failed', e);
      }
      return;
    }
    gitError('Checkout failed', err);
  }
}

async function newBranch(from = ''): Promise<void> {
  const name = await promptDialog(from ? `New Branch from ${from}` : 'New Branch', '', validBranchName, 'Branch name');
  if (name) await checkout(from, name);
}

async function mergeOrRebase(kind: 'merge' | 'rebase', ref: string): Promise<void> {
  try {
    if (kind === 'merge') await gitApi.merge(ref);
    else await gitApi.rebase(ref);
    gitInfo(kind === 'merge' ? `Merged ${ref}` : `Rebased onto ${ref}`);
  } catch (err) {
    if (gitState.peek().status.op) {
      gitError(`${kind === 'merge' ? 'Merge' : 'Rebase'} stopped on conflicts`, 'Resolve them in the Commit window, then Continue.');
      const { gitWorkbench } = await import('./state');
      gitWorkbench().panels.show('commit');
    } else {
      gitError(`${kind === 'merge' ? 'Merge' : 'Rebase'} failed`, err);
    }
  }
}

async function deleteBranch(name: string): Promise<void> {
  if (!(await confirmDialog('Delete Branch', `Delete the local branch "${name}"?`, 'Delete', true))) return;
  try {
    await gitApi.deleteBranch(name, false);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    if (!/not fully merged/i.test(msg)) {
      gitError('Cannot delete branch', err);
      return;
    }
    if (!(await confirmDialog('Branch Not Merged', `"${name}" has commits that aren’t merged into the current branch. Delete it anyway?`, 'Force Delete', true))) return;
    await gitApi.deleteBranch(name, true).catch((e) => gitError('Cannot delete branch', e));
  }
}

function branchMenu(b: GitBranch, x: number, y: number): void {
  const items: MenuItem[] = [];
  if (!b.current) items.push({ label: 'Checkout', run: () => void checkout(b.name) });
  items.push({ label: 'New Branch from Here…', run: () => void newBranch(b.name) });
  if (!b.current) {
    items.push(
      { separator: true },
      { label: `Merge '${b.name}' into Current`, run: () => void mergeOrRebase('merge', b.name) },
      { label: `Rebase Current onto '${b.name}'`, run: () => void mergeOrRebase('rebase', b.name) },
      { label: 'Compare with Current', run: () => void compare(b.name) },
    );
  }
  if (!b.remote) {
    items.push({ separator: true }, {
      label: 'Rename…',
      run: async () => {
        const to = await promptDialog('Rename Branch', b.name, validBranchName, 'New name');
        if (to && to !== b.name) await gitApi.renameBranch(b.name, to).catch((e) => gitError('Cannot rename branch', e));
      },
    });
    if (!b.current) items.push({ label: 'Delete', danger: true, run: () => void deleteBranch(b.name) });
  }
  showMenu(x, y, items);
}

async function compare(ref: string): Promise<void> {
  // Files that differ between HEAD and ref, each opening a diff of ref's version.
  try {
    const log = await gitApi.log({ ref: `HEAD..${ref}`, max: 50 });
    const body = h(
      'div',
      { class: 'git-compare' },
      h('p', null, log.length ? `${log.length} commit(s) in ${ref} that aren’t in the current branch:` : `${ref} has no commits that aren’t in the current branch.`),
      h('ul', { class: 'git-compare-list' }, log.map((c) => h('li', null, h('code', null, c.short), ` ${c.subject} — ${c.author}`))),
    );
    await showDialog({ title: `Compare with ${ref}`, body, cancelValue: undefined, buttons: [{ label: 'Close', value: undefined, primary: true }] });
  } catch (err) {
    gitError('Cannot compare', err);
  }
}

export async function showBranches(): Promise<void> {
  let branches: GitBranch[];
  try {
    branches = await gitApi.branches();
  } catch (err) {
    gitError('Cannot list branches', err);
    return;
  }
  const { local, remote } = branchEntries(branches);
  const st = gitState.peek().status;
  const entries: Entry[] = [
    { kind: 'action', label: 'New Branch…', run: () => void newBranch() },
    { kind: 'action', label: 'Stash Changes…', run: () => void stashChanges() },
    { kind: 'action', label: 'Stashes…', run: () => void showStashes() },
    { kind: 'action', label: 'Fetch', run: () => void runGitOp('fetch') },
    { kind: 'action', label: 'Pull', run: () => void runGitOp('pull'), hint: st.behind ? `↓${st.behind}` : '' },
    { kind: 'action', label: 'Push', run: () => void runGitOp('push'), hint: st.ahead ? `↑${st.ahead}` : '' },
  ];
  if (local.length) entries.push({ kind: 'header', label: 'Local Branches' }, ...local.map((b): Entry => ({ kind: 'branch', b })));
  if (remote.length) entries.push({ kind: 'header', label: 'Remote Branches' }, ...remote.map((b): Entry => ({ kind: 'branch', b })));
  let at = { x: window.innerWidth / 2 - 200, y: window.innerHeight / 4 };
  showListPopup<Entry>({
    items: entries,
    title: st.detached ? `HEAD detached at ${st.head.slice(0, 7)}` : `Branch: ${st.branch}`,
    filter: true,
    placeholder: 'Filter branches and actions',
    text: (e) => (e.kind === 'branch' ? e.b.name : e.kind === 'action' ? e.label : ''),
    render: (e) => {
      if (e.kind === 'header') return [h('span', { class: 'git-pop-header' }, e.label)];
      if (e.kind === 'action') return [h('span', { class: 'lp-kind' }, '›'), h('span', null, e.label), e.hint ? h('span', { class: 'lp-detail' }, e.hint) : h('span')];
      return [
        h('span', { class: 'lp-kind' }, e.b.current ? '★' : e.b.remote ? '☁' : '⎇'),
        h('span', { class: e.b.current ? 'git-current-branch' : '' }, e.b.name),
        h('span', { class: 'lp-detail' }, e.b.upstream ? `→ ${e.b.upstream}` : e.b.subject),
      ];
    },
    onPick: (e) => {
      if (e.kind === 'action') e.run();
      else if (e.kind === 'branch') branchMenu(e.b, at.x + 40, at.y + 40);
    },
  });
  const pop = document.querySelector('.lp') as HTMLElement | null;
  if (pop) {
    const r = pop.getBoundingClientRect();
    at = { x: r.left, y: r.top };
  }
}

export async function stashChanges(): Promise<void> {
  const msg = h('input', { class: 'input', type: 'text', placeholder: 'Message (optional)', spellcheck: false });
  const untracked = h('input', { type: 'checkbox', checked: true });
  const ok = await showDialog({
    title: 'Stash Changes',
    body: h('div', { class: 'bp-form' }, h('label', null, 'Message'), msg, h('label', { class: 'rc-check' }, untracked, ' Include untracked files')),
    focus: msg,
    cancelValue: false,
    buttons: [
      { label: 'Cancel', value: false },
      { label: 'Stash', value: true, primary: true },
    ],
  });
  if (!ok) return;
  try {
    await gitApi.stash(msg.value.trim(), untracked.checked);
    gitInfo('Changes stashed');
  } catch (err) {
    gitError('Cannot stash', err);
  }
}

export async function showStashes(): Promise<void> {
  let list: GitStash[];
  try {
    list = await gitApi.stashes();
  } catch (err) {
    gitError('Cannot list stashes', err);
    return;
  }
  if (!list.length) {
    gitInfo('No stashes');
    return;
  }
  showListPopup<GitStash>({
    items: list,
    title: 'Stashes',
    filter: true,
    text: (s) => s.message,
    render: (s) => [h('span', { class: 'lp-kind' }, `{${s.index}}`), h('span', null, s.message), h('span', { class: 'lp-detail' }, s.date.slice(0, 16).replace('T', ' '))],
    onPick: (s) => {
      showMenu(window.innerWidth / 2 - 80, window.innerHeight / 3, [
        { label: 'Apply', run: () => void gitApi.stashApply(s.index, false).then(() => gitInfo('Stash applied')).catch((e) => gitError('Cannot apply stash', e)) },
        { label: 'Pop', run: () => void gitApi.stashApply(s.index, true).then(() => gitInfo('Stash popped')).catch((e) => gitError('Cannot pop stash', e)) },
        { label: 'Show Diff', run: () => void showStashDiff(s) },
        { separator: true },
        {
          label: 'Drop',
          danger: true,
          run: async () => {
            if (await confirmDialog('Drop Stash', `Delete stash "${s.message}"?`, 'Drop', true)) await gitApi.stashDrop(s.index).catch((e) => gitError('Cannot drop stash', e));
          },
        },
      ]);
    },
  });
}

async function showStashDiff(s: GitStash): Promise<void> {
  try {
    const files = await gitApi.stashDiff(s.index);
    const body = h(
      'div',
      { class: 'git-compare' },
      h('ul', { class: 'git-compare-list' }, files.map((f) => h('li', null, h('code', null, f.newPath || f.oldPath), ` (${f.hunks.length} change${f.hunks.length === 1 ? '' : 's'})`))),
      h('pre', { class: 'git-stash-patch' }, files.map((f) => [...f.header, ...f.hunks.flatMap((x) => [x.header, ...x.lines])].join('\n')).join('\n')),
    );
    await showDialog({ title: `Stash: ${s.message}`, body, cancelValue: undefined, buttons: [{ label: 'Close', value: undefined, primary: true }] });
  } catch (err) {
    gitError('Cannot show stash', err);
  }
}
