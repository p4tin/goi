// Shared state for the language feature modules.

import type { Workbench } from '../app/workbench';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import type { EditorView } from '@codemirror/view';
import type { Pos } from '../lib/api';
import { LangClient } from './client';
import { toPos } from './positions';

export const client = new LangClient();

let workbench: Workbench | null = null;

export function setWorkbench(wb: Workbench): void {
  workbench = wb;
}

export function wb(): Workbench {
  if (!workbench) throw new Error('lang: workbench not set');
  return workbench;
}

/** Shows a short info notification (e.g. "No declaration found"). */
export function info(id: string, title: string, body?: string): void {
  workbench?.notifications.push({ id, level: 'info', title, body });
}

export function warn(id: string, title: string, body?: string): void {
  workbench?.notifications.push({ id, level: 'warn', title, body });
}

/** The active Go buffer with its view and cursor position, after flushing
 * pending sync. Explains to the user why nothing happens when gopls isn't
 * available. */
export async function activeTarget(quiet = false): Promise<{ buf: Buffer; view: EditorView; pos: Pos; offset: number; version: number } | null> {
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (!buf || !view || !client.tracked(buf.path)) return null;
  if (!client.live()) {
    if (!quiet) {
      const st = client.status.peek();
      info('lang.notReady', st.state === 'missing' ? 'gopls is not installed' : 'gopls is not ready', st.message || undefined);
    }
    return null;
  }
  const version = await client.flush(buf);
  const offset = view.state.selection.main.head;
  return { buf, view, pos: toPos(view.state.doc, offset), offset, version };
}

/** True if buf is unchanged since version (drop stale responses). */
export function fresh(buf: Buffer, version: number): boolean {
  return buf.version === version && editors.buffers.get(buf.path) === buf;
}
