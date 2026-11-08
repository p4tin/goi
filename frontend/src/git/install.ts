// Git feature (M4): status store, project tree colors, branch widget, Git
// console, and git commands. Other git modules plug in from here.

import { confirmDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { gitApi, on, Topics } from '../lib/api';
import type { GitState } from '../lib/api';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { treeDecorators, treeDecorationsVersion } from '../views/projectTree';
import { focusCommit, installCommitPanel, openCommitDialog } from './commitPanel';
import { installGitConsole, renderGitConsole, runGitOp } from './console';
import { installDiffRefresh, openDiff } from './diffView';
import { editors } from '../editor/install';
import { branchLabel, byPath, changedDirs, emptyState, fileKind, gitError, gitState, KIND_LABEL, normalize, setGitWorkbench } from './state';
import './git.css';

export function installGit(wb: Workbench): void {
  setGitWorkbench(wb);
  installGitConsole();
  wb.panels.register({ id: 'git', title: 'Git', dock: 'bottom', order: 12, render: (c) => renderGitConsole(c) });

  on<GitState>(Topics.gitStatus, (st) => gitState.set(st));
  effect(() => {
    if (!project()) {
      gitState.set(emptyState());
      return;
    }
    void gitApi.state().then((st) => gitState.set(st)).catch(() => {});
  });
  // Pick up changes made outside Goi (terminal, other tools).
  window.addEventListener('focus', () => {
    if (project.peek() && gitState.peek().repo) void gitApi.refresh().catch(() => {});
  });

  // Project tree colors.
  treeDecorators.push((row) => {
    if (!gitState.peek().repo) return null;
    if (row.isDir) return changedDirs.peek().has(normalize(row.path)) ? { cls: 'vcs-dir-changed' } : null;
    const f = byPath.peek().get(normalize(row.path));
    if (!f) return null;
    const k = fileKind(f);
    return { cls: `vcs-${k}`, title: KIND_LABEL[k] };
  });
  effect(() => {
    gitState();
    treeDecorationsVersion.update((v) => v + 1);
  });

  // Branch widget.
  const el = wb.status.add({ id: 'git', side: 'right', priority: 35, title: 'Git branch', onClick: () => {
    const id = gitState.peek().repo ? 'git.branches' : 'git.init';
    if (commands.get(id)) void commands.run(id);
  } });
  effect(() => {
    const st = gitState();
    el.hidden = !project();
    if (!st.repo) {
      el.textContent = project() ? 'No Git' : '';
      el.title = 'The project is not in a git repository. Click to initialize one.';
      return;
    }
    el.textContent = `⎇ ${branchLabel(st.status) || '(no branch)'}`;
    el.title = st.status.upstream ? `${st.status.branch} → ${st.status.upstream}` : st.status.branch || 'Detached HEAD';
  });

  const repo = (): boolean => !!project.peek() && gitState.peek().repo;
  commands.register({ id: 'git.refresh', title: 'Refresh Git Status', category: 'Git', when: repo, run: () => gitApi.refresh() });
  commands.register({ id: 'git.fetch', title: 'Fetch', category: 'Git', when: repo, run: () => runGitOp('fetch') });
  commands.register({ id: 'git.pull', title: 'Pull…', category: 'Git', when: repo, run: () => runGitOp('pull') });
  commands.register({ id: 'git.push', title: 'Push…', category: 'Git', when: repo, run: () => runGitOp('push') });
  commands.register({ id: 'view.git', title: 'Show Git Console', category: 'View', run: () => wb.panels.show('git') });
  commands.register({
    id: 'git.init',
    title: 'Initialize Git Repository',
    category: 'Git',
    when: () => !!project.peek() && !gitState.peek().repo,
    run: async () => {
      if (!(await confirmDialog('Initialize Git Repository', `Create a git repository in ${project.peek()?.root}?`, 'Create'))) return;
      try {
        await gitApi.init();
      } catch (err) {
        gitError('Cannot initialize repository', err);
      }
    },
  });

  installCommitPanel();
  installDiffRefresh();
  commands.register({ id: 'git.commit', title: 'Commit…', category: 'Git', when: repo, run: () => focusCommit() });
  commands.register({ id: 'git.commitModal', title: 'Commit (Modal)…', category: 'Git', when: repo, run: () => openCommitDialog() });
  commands.register({ id: 'view.commit', title: 'Show Commit Window', category: 'View', run: () => wb.panels.show('commit') });
  const activeFile = (): string => editors.activeBuffer()?.path ?? '';
  commands.register({
    id: 'git.showDiff',
    title: 'Show Diff (Working Tree vs Staged)',
    category: 'Git',
    when: () => repo() && !!activeFile(),
    run: () => openDiff({ path: activeFile(), mode: 'work' }),
  });
  commands.register({
    id: 'git.compareWithHead',
    title: 'Compare with HEAD',
    category: 'Git',
    when: () => repo() && !!activeFile(),
    run: () => openDiff({ path: activeFile(), mode: 'head' }),
  });

}
