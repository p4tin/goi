// gopls status bar item: state at a glance; click for Restart / Install.

import { showMenu } from '../app/menu';
import type { Workbench } from '../app/workbench';
import { lang, on, Topics } from '../lib/api';
import type { LangState, LangStatus } from '../lib/api';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { client } from './context';

const LABEL: Record<LangState, string> = {
  starting: 'gopls: starting…',
  indexing: 'gopls: indexing…',
  ready: 'gopls',
  error: 'gopls: error',
  stopped: 'gopls: stopped',
  missing: 'gopls: not installed',
  disabled: 'gopls: off',
  installing: 'gopls: installing…',
};

export function statusLabel(st: LangStatus): string {
  return LABEL[st.state] ?? `gopls: ${st.state}`;
}

export function installLangStatus(wb: Workbench): void {
  const el = wb.status.add({
    id: 'lang',
    side: 'right',
    priority: 40,
    title: 'Go language server',
    onClick: () => {
      const r = el.getBoundingClientRect();
      const st = client.status.peek();
      showMenu(r.left, r.top - 8 - 32 * 3, [
        { label: st.message || statusLabel(st), disabled: true, run: () => {} },
        { separator: true },
        { label: 'Restart gopls', disabled: st.state === 'missing' || st.state === 'disabled', run: () => void commands.run('lang.restart') },
        { label: 'Install gopls', disabled: st.state === 'installing', run: () => void commands.run('lang.installGopls') },
      ]);
    },
  });
  effect(() => {
    const st = client.status();
    el.textContent = statusLabel(st);
    el.title = st.message ? `Go language server: ${st.message}` : 'Go language server';
    el.dataset.state = st.state;
  });
  on<LangStatus>(Topics.langStatus, (st) => client.setStatus(st));
  void lang
    .status()
    .then((st) => client.setStatus(st))
    .catch(() => {});

  commands.register({
    id: 'lang.restart',
    title: 'Restart gopls',
    category: 'Go',
    run: () => lang.restart().catch((err: unknown) => wb.notifications.push({ id: 'lang.restart', level: 'error', title: 'Cannot restart gopls', body: String(err) })),
  });
  commands.register({
    id: 'lang.installGopls',
    title: 'Install gopls',
    category: 'Go',
    run: () => lang.install().catch((err: unknown) => wb.notifications.push({ id: 'gopls.install', level: 'error', title: 'Cannot install gopls', body: String(err) })),
  });

  // Offer installation once per session when gopls is missing.
  let offered = false;
  effect(() => {
    const st = client.status();
    if (st.state === 'missing' && !offered) {
      offered = true;
      wb.notifications.push({
        id: 'gopls.missing',
        level: 'warn',
        title: 'gopls is not installed',
        body: 'Code completion, navigation and diagnostics need gopls. Click "gopls: not installed" in the status bar, or run "Install gopls" from Find Action.',
      });
    }
  });
}
