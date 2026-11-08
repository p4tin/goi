// Git console (R6): output of fetch/pull/push in a bottom tool window, with
// Stop, plus notifications for the outcome (and Pull and Push after a
// rejected push).

import { addCloseCheck } from '../app/closeChecks';
import { confirmDialog, showDialog } from '../app/dialog';
import { gitApi, on, Topics } from '../lib/api';
import type { GitOpEvent, GitOutputEvent } from '../lib/api';
import { h } from '../lib/dom';
import { signal } from '../lib/store';
import { Console } from '../run/console';
import { iconButton } from '../run/icons';
import { gitError, gitInfo, gitWorkbench } from './state';

const running = signal<GitOpEvent[]>([]);
let con: Console | null = null;
let root: HTMLElement | null = null;
let stopBtn: HTMLButtonElement | null = null;

/** Runs a network operation and shows its output. */
export async function runGitOp(kind: 'fetch' | 'pull' | 'push', force = false): Promise<void> {
  try {
    if (kind === 'fetch') await gitApi.fetch();
    else if (kind === 'pull') await gitApi.pull();
    else await gitApi.push(force);
  } catch (err) {
    gitError(`Cannot ${kind}`, err);
  }
}

async function onRejected(): Promise<void> {
  const choice = await showDialog<'pull' | 'force' | 'cancel'>({
    title: 'Push Rejected',
    body: 'The remote has commits you don’t have. Pull them first, then push again.',
    cancelValue: 'cancel',
    buttons: [
      { label: 'Force Push (with lease)', value: 'force', danger: true },
      { label: 'Cancel', value: 'cancel' },
      { label: 'Pull and Push', value: 'pull', primary: true },
    ],
  });
  if (choice === 'force') return runGitOp('push', true);
  if (choice !== 'pull') return;
  const id = await gitApi.pull().catch((err) => {
    gitError('Cannot pull', err);
    return '';
  });
  if (!id) return;
  const off = on<GitOpEvent>(Topics.gitOp, (ev) => {
    if (ev.opId !== id || ev.state === 'running') return;
    off();
    if (ev.state === 'done') void runGitOp('push');
  });
}

export function installGitConsole(): void {
  addCloseCheck(async () => {
    const ops = running.peek();
    if (!ops.length) return true;
    return confirmDialog('Git Operation Running', `${ops.map((o) => o.title).join(', ')} is still running. Stop it and close?`, 'Stop and Close', true);
  });
  con = new Console({ maxLines: 5000 });
  stopBtn = iconButton('stop', 'Stop', () => {
    for (const op of running.peek()) void gitApi.stopOp(op.opId).catch(() => {});
  }, 'tb-stop');
  stopBtn.disabled = true;
  const tools = h('div', { class: 'run-tools' }, stopBtn, iconButton('clear', 'Clear', () => con?.clear()), iconButton('wrap', 'Soft-wrap', () => con?.setSoftWrap(!con.softWrap)));
  root = h('div', { class: 'run-window' }, h('div', { class: 'run-bodies' }, h('div', { class: 'run-tab-body' }, tools, h('div', { class: 'run-content' }, con.el))));

  on<GitOutputEvent>(Topics.gitOutput, (ev) => {
    for (const c of ev.chunks) con?.append(c.stream, c.text);
  });
  on<GitOpEvent>(Topics.gitOp, (ev) => {
    if (ev.state === 'running') {
      running.set([...running.peek().filter((o) => o.opId !== ev.opId), ev]);
      gitWorkbench().panels.show('git');
    } else {
      running.set(running.peek().filter((o) => o.opId !== ev.opId));
      con?.append('system', ev.state === 'done' ? `${ev.title} finished\n\n` : `${ev.title} failed\n\n`);
      if (ev.state === 'done') gitInfo(`${ev.title} finished`);
      else if (ev.rejected) void onRejected();
      else gitError(`${ev.title} failed`, ev.error || 'See the Git console for details');
    }
    if (stopBtn) stopBtn.disabled = running.peek().length === 0;
  });
}

export function renderGitConsole(container: HTMLElement): void {
  if (root) container.append(root);
}

export function gitBusy(): boolean {
  return running.peek().length > 0;
}
