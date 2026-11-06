// Window close guard (R6.9): the backend prevents the close and asks; we
// resolve unsaved buffers and then confirm.

import { showDialog } from '../app/dialog';
import type { Workbench } from '../app/workbench';
import { app, on, Topics } from '../lib/api';
import { h } from '../lib/dom';
import { editors } from './install';

type Choice = 'save' | 'discard' | 'cancel';

export function installCloseGuard(wb: Workbench): void {
  let asking = false;

  const confirm = async (): Promise<void> => {
    try {
      await wb.layout.flush();
    } catch (err) {
      console.warn('layout flush on close', err);
    }
    await app.confirmClose();
  };

  on(Topics.beforeClose, () => {
    if (asking) return;
    void (async () => {
      const dirty = editors.dirtyBuffers();
      if (dirty.length === 0) return confirm();
      asking = true;
      try {
        const choice = await showDialog<Choice>({
          title: dirty.length === 1 ? 'You have 1 unsaved file' : `You have ${dirty.length} unsaved files`,
          body: h(
            'div',
            null,
            h('p', null, 'Save your changes before closing?'),
            h('ul', null, dirty.map((b) => h('li', { title: b.path }, b.name))),
          ),
          cancelValue: 'cancel',
          buttons: [
            { label: 'Don’t Save', value: 'discard', danger: true },
            { label: 'Cancel', value: 'cancel' },
            { label: 'Save All', value: 'save', primary: true },
          ],
        });
        if (choice === 'cancel') return;
        if (choice === 'save' && !(await editors.saveAll())) return;
        await confirm();
      } finally {
        asking = false;
      }
    })();
  });
}
