// Help → About Goi: product version, build, platform and log location.

import { app, openExternal } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { showDialog } from './dialog';
import type { Workbench } from './workbench';

/** "0.1.0" for release builds, "0.1 (dev)" otherwise. */
export function versionLabel(version: string, build: string): string {
  return build && build !== 'dev' ? build : `${version} (dev)`;
}

export async function showAbout(): Promise<void> {
  const info = await app.info();
  const row = (k: string, v: string): HTMLElement => h('div', { class: 'about-row' }, h('span', null, k), h('code', null, v));
  const body = h(
    'div',
    { class: 'about' },
    h('div', { class: 'about-title' }, 'Goi'),
    h('div', { class: 'about-sub' }, 'A cross-platform Go IDE'),
    row('Version', versionLabel(info.version, info.build)),
    row('Platform', `${info.os}/${info.arch}`),
    row('Built with', info.goVersion),
    row('Log file', info.logPath),
  );
  const choice = await showDialog<'site' | 'close'>({
    title: 'About Goi',
    body,
    cancelValue: 'close',
    buttons: [
      { label: 'Project Page', value: 'site' },
      { label: 'Close', value: 'close', primary: true },
    ],
  });
  if (choice === 'site') openExternal('https://github.com/p4tin/goi');
}

export function installAbout(_wb: Workbench): void {
  commands.register({ id: 'help.about', title: 'About Goi', category: 'Help', run: () => showAbout() });
}
