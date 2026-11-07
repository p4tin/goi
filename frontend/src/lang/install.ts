// Language intelligence feature (M2): wires gopls-backed modules into the
// editor through hooks and extension factories.

import { EditorView } from '@codemirror/view';
import type { Workbench } from '../app/workbench';
import { editors } from '../editor/install';
import { on, Topics } from '../lib/api';
import type { ApplyEditEvent } from '../lib/api';
import { client, setWorkbench } from './context';
import { applyFileEdits } from './edits';
import { installLangStatus } from './status';
import './lang.css';

export function installLang(wb: Workbench): void {
  setWorkbench(wb);
  const m = editors;

  m.hooks.opened.on((buf) => client.open(buf));
  m.hooks.closed.on((buf) => client.close(buf));
  m.hooks.saved.on((buf) => client.saved(buf));
  m.addExtension((buf) => EditorView.updateListener.of((u) => {
    if (u.docChanged) client.changed(buf);
  }));
  // Buffers opened before this feature installed (layout restore).
  for (const buf of m.buffers.values()) client.open(buf);

  installLangStatus(wb);

  on<ApplyEditEvent>(Topics.langApplyEdit, (e) => {
    applyFileEdits(e.edits).catch((err: unknown) =>
      wb.notifications.push({ id: 'lang.applyEdit', level: 'error', title: e.label ? `Cannot apply “${e.label}”` : 'Cannot apply edit', body: String(err) }),
    );
  });
}
