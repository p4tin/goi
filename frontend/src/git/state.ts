// Git status store shared by the git features.

import type { Workbench } from '../app/workbench';
import type { GitFileStatus, GitState } from '../lib/api';
import { computed, signal } from '../lib/store';

export const emptyState = (): GitState => ({
  repo: false,
  status: { root: '', branch: '', head: '', upstream: '', ahead: 0, behind: 0, detached: false, initial: false, op: '', files: [] },
});

export const gitState = signal<GitState>(emptyState());

export type FileKind = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked' | 'conflict' | 'ignored';

/** What a file's status means for display. Work-tree state wins over the
 * index (a staged-then-edited file shows as modified). */
export function fileKind(f: GitFileStatus): FileKind {
  if (f.conflict) return 'conflict';
  if (f.untracked) return 'untracked';
  if (f.ignored) return 'ignored';
  if (f.work === 'D' || (f.work === '.' && f.index === 'D')) return 'deleted';
  if (f.index === 'A' && (f.work === '.' || f.work === 'M')) return 'added';
  if (f.index === 'R' || f.index === 'C') return 'renamed';
  return 'modified';
}

export const KIND_LABEL: Record<FileKind, string> = {
  modified: 'modified',
  added: 'added',
  deleted: 'deleted',
  renamed: 'renamed',
  untracked: 'unversioned',
  conflict: 'merge conflict',
  ignored: 'ignored',
};

/** Status by absolute path. */
export const byPath = computed(() => {
  const m = new Map<string, GitFileStatus>();
  for (const f of gitState().status.files) m.set(normalize(f.path), f);
  return m;
});

/** Directories (absolute) containing changed files, up to the repo root. */
export const changedDirs = computed(() => dirsOf(gitState().status.files, gitState().status.root));

export function normalize(p: string): string {
  return p.replace(/\\/g, '/').replace(/\/$/, '');
}

export function dirsOf(files: readonly GitFileStatus[], root: string): Set<string> {
  const out = new Set<string>();
  const r = normalize(root);
  for (const f of files) {
    if (f.ignored) continue;
    let d = normalize(f.path);
    for (;;) {
      const i = d.lastIndexOf('/');
      if (i <= 0) break;
      d = d.slice(0, i);
      if (r && !(d + '/').startsWith(r + '/')) break;
      if (out.has(d)) break;
      out.add(d);
      if (d === r) break;
    }
  }
  return out;
}

export function statusOf(path: string): GitFileStatus | undefined {
  return byPath.peek().get(normalize(path));
}

export function isRepo(): boolean {
  return gitState.peek().repo;
}

let wb: Workbench | null = null;
export function setGitWorkbench(w: Workbench): void {
  wb = w;
}
export function gitWorkbench(): Workbench {
  if (!wb) throw new Error('git: workbench not set');
  return wb;
}

export function gitError(title: string, err: unknown): void {
  wb?.notifications.push({ id: `git-${title}`, level: 'error', title, body: err instanceof Error ? err.message : String(err) });
}

export function gitInfo(title: string, body = ''): void {
  wb?.notifications.push({ id: `git-${title}`, level: 'info', title, body });
}

export const OP_LABEL: Record<string, string> = { merge: 'MERGING', rebase: 'REBASING', 'cherry-pick': 'CHERRY-PICKING', revert: 'REVERTING' };

/** "main ↑1 ↓2 (MERGING)" style branch label. */
export function branchLabel(st: GitState['status']): string {
  if (!st.branch && !st.detached && !st.head) return '';
  let s = st.detached ? `(${st.head.slice(0, 7)})` : st.branch;
  if (st.ahead) s += ` ↑${st.ahead}`;
  if (st.behind) s += ` ↓${st.behind}`;
  if (st.op) s += ` (${OP_LABEL[st.op] ?? st.op.toUpperCase()})`;
  return s;
}
