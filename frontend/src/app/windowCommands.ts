// Window commands: open a folder in a separate Goi window (one project per
// window; an already-open project's window is focused instead).

import { app } from '../lib/api';
import { commands } from '../lib/commands';
import type { Workbench } from './workbench';

export function installWindowCommands(wb: Workbench): void {
  commands.register({
    id: 'project.openInNewWindow',
    title: 'Open Folder in New Window…',
    category: 'File',
    run: async () => {
      const dir = await app.openFolderDialog();
      if (!dir) return;
      try {
        await app.openProject(dir, true);
      } catch (err) {
        wb.notifications.push({ id: 'open-window', level: 'error', title: 'Cannot open window', body: String(err) });
      }
    },
  });
}
