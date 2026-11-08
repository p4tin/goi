// Applies workspace edits: open buffers are edited through CodeMirror (one
// undoable transaction per buffer, mirrored to split views by the editor
// manager); other files are rewritten on disk by the backend.

import type { EditorManager } from '../editor/editorManager';
import { editors } from '../editor/install';
import { lang } from '../lib/api';
import type { FileEdit } from '../lib/api';
import { editsToChanges } from './positions';

export interface EditSummary {
  files: number;
  edits: number;
  /** Paths edited on disk (not open in the editor). */
  onDisk: string[];
}

export function summarize(edits: readonly FileEdit[]): { files: number; edits: number } {
  return { files: edits.filter((e) => e.edits.length > 0).length, edits: edits.reduce((n, e) => n + e.edits.length, 0) };
}

export async function applyFileEdits(
  edits: readonly FileEdit[],
  m: EditorManager = editors,
  disk: (edits: FileEdit[]) => Promise<void> = lang.applyFileEdits,
): Promise<EditSummary> {
  const closed: FileEdit[] = [];
  let count = 0;
  let files = 0;
  for (const fe of edits) {
    if (fe.edits.length === 0) continue;
    files++;
    count += fe.edits.length;
    const buf = m.buffers.get(fe.path);
    const view = buf?.views.values().next().value;
    if (!buf || !view) {
      closed.push(fe);
      continue;
    }
    if (buf.readOnly) throw new Error(`${buf.name} is read-only`);
    view.dispatch({ changes: editsToChanges(view.state.doc, fe.edits), userEvent: 'input.lang' });
  }
  if (closed.length) await disk(closed);
  return { files, edits: count, onDisk: closed.map((c) => c.path) };
}
