// Language server status bar item: state at a glance; click for Restart /
// Install. The server's name comes from the pack (Goi: gopls).

import { showMenu } from '../app/menu';
import type { Workbench } from '../app/workbench';
import { lang, on, Topics } from '../lib/api';
import type { LangState, LangStatus } from '../lib/api';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { capabilities } from '../app/state';
import { client } from './context';

/** The pack's language server name (Goi: "gopls"). */
export function serverName(): string {
  return capabilities().languageServer || 'Language server';
}

const SUFFIX: Record<LangState, string> = {
  starting: ': starting…',
  indexing: ': indexing…',
  ready: '',
  error: ': error',
  stopped: ': stopped',
  missing: ': not installed',
  disabled: ': off',
  installing: ': installing…',
};

export function statusLabel(st: LangStatus, name = serverName()): string {
  return name + (SUFFIX[st.state] ?? `: ${st.state}`);
}

export function installLangStatus(wb: Workbench): void {
  const name = serverName();
  const el = wb.status.add({
    id: 'lang',
    side: 'right',
    priority: 40,
    title: 'Language server',
    onClick: () => {
      const r = el.getBoundingClientRect();
      const st = client.status.peek();
      showMenu(r.left, r.top - 8 - 32 * 3, [
        { label: st.message || statusLabel(st), disabled: true, run: () => {} },
        { separator: true },
        { label: `Restart ${name}`, disabled: st.state === 'missing' || st.state === 'disabled', run: () => void commands.run('lang.restart') },
        { label: `Install ${name}`, disabled: st.state === 'installing', run: () => void commands.run('lang.installServer') },
      ]);
    },
  });
  effect(() => {
    const st = client.status();
    el.textContent = statusLabel(st);
    el.title = st.message ? `Language server: ${st.message}` : 'Language server';
    el.dataset.state = st.state;
  });
  on<LangStatus>(Topics.langStatus, (st) => client.setStatus(st));
  void lang
    .status()
    .then((st) => client.setStatus(st))
    .catch(() => {});

  commands.register({
    id: 'lang.restart',
    title: `Restart ${name}`,
    category: 'Code',
    run: () => lang.restart().catch((err: unknown) => wb.notifications.push({ id: 'lang.restart', level: 'error', title: `Cannot restart ${name}`, body: String(err) })),
  });
  commands.register({
    id: 'lang.installServer',
    title: `Install ${name}`,
    category: 'Code',
    run: () => lang.install().catch((err: unknown) => wb.notifications.push({ id: 'lang.install', level: 'error', title: `Cannot install ${name}`, body: String(err) })),
  });

  // Offer installation once per session when the server is missing.
  let offered = false;
  effect(() => {
    const st = client.status();
    if (st.state === 'missing' && !offered) {
      offered = true;
      wb.notifications.push({
        id: 'lang.missing',
        level: 'warn',
        title: `${name} is not installed`,
        body: `Code completion, navigation and diagnostics need ${name}. Click "${name}: not installed" in the status bar, or run "Install ${name}" from Find Action.`,
      });
    }
  });
}
