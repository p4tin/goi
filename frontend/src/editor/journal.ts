// Crash journal: every 2 s, unsaved buffers that changed since the last
// write are journaled by the backend; on reopening a project after a crash
// the user is offered to restore them (R6.10).

import { showDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { app } from '../lib/api';
import type { JournalEntry } from '../lib/api';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import type { Buffer } from './buffer';
import { baseName } from './buffer';
import type { EditorManager } from './editorManager';
import { editors } from './install';

export const JOURNAL_INTERVAL_MS = 2000;

/** Tracks which buffer versions were journaled; returns the buffers due. */
export class JournalTracker {
  private written = new Map<string, number>();

  due(buffers: Iterable<Buffer>): Buffer[] {
    const out: Buffer[] = [];
    for (const b of buffers) {
      if (b.dirty.peek() && this.written.get(b.path) !== b.version) out.push(b);
    }
    return out;
  }

  wrote(b: Buffer): void {
    this.written.set(b.path, b.version);
  }

  forget(path: string): void {
    this.written.delete(path);
  }
}

export function installJournal(wb: Workbench): void {
  const m = editors;
  const tracker = new JournalTracker();

  setInterval(() => {
    if (!project.peek()) return;
    for (const b of tracker.due(m.buffers.values())) {
      tracker.wrote(b);
      app.journalPut(b.path, b.text(), String(b.meta.modTime ?? '')).catch((err: unknown) => console.warn('journal put', err));
    }
  }, JOURNAL_INTERVAL_MS);

  const drop = (b: Buffer): void => {
    tracker.forget(b.path);
    app.journalDrop(b.path).catch((err: unknown) => console.warn('journal drop', err));
  };
  m.hooks.saved.on(drop);
  m.hooks.closed.on(drop);

  let offeredFor: string | null = null;
  effect(() => {
    const p = project();
    if (!p || p.root === offeredFor) return;
    offeredFor = p.root;
    void offerRestore(wb, m);
  });
}

async function offerRestore(wb: Workbench, m: EditorManager): Promise<void> {
  let entries: JournalEntry[];
  try {
    entries = (await app.pendingRestore()) ?? [];
  } catch {
    return;
  }
  if (entries.length === 0) return;
  const restore = await showDialog({
    title: 'Restore unsaved changes?',
    body: h(
      'div',
      null,
      h('p', null, `Goi didn’t shut down cleanly. ${entries.length} file${entries.length > 1 ? 's have' : ' has'} unsaved changes:`),
      h('ul', null, entries.map((e) => h('li', { title: e.path }, baseName(e.path)))),
    ),
    cancelValue: false,
    buttons: [
      { label: 'Discard', value: false, danger: true },
      { label: 'Restore', value: true, primary: true },
    ],
  });
  for (const e of entries) {
    if (!restore) {
      void app.journalDrop(e.path);
      continue;
    }
    const buf = await m.open(e.path, { focus: false });
    if (!buf) {
      wb.notifications.push({ id: `restore:${e.path}`, level: 'warn', title: `Could not restore ${baseName(e.path)}`, body: 'The file no longer exists.' });
      continue;
    }
    if (buf.text() !== e.content) m.replaceDoc(buf, e.content, true);
  }
}
