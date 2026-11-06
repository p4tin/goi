// Welcome screen (placeholder; T14 implements recent projects and New Project).

import type { Workbench } from '../app/workbench';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';

export function installWelcome(wb: Workbench): void {
  wb.welcome.append(
    h(
      'div',
      { class: 'welcome' },
      h('h1', null, 'Goi'),
      h('button', { class: 'btn btn-primary', onClick: () => void commands.run('project.openFolder') }, 'Open Folder…'),
    ),
  );
}
