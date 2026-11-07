// Rename (R9): prepare → prompt → rename → preview (multi-file) → apply.

import { promptDialog, showDialog } from '../app/dialog';
import { lang } from '../lib/api';
import type { FileEdit } from '../lib/api';
import { h } from '../lib/dom';
import { activeTarget, fresh, info, warn } from './context';
import { relPath } from './diagnostics';
import { applyFileEdits, summarize } from './edits';
import { toOffsets, wordAt } from './positions';

const IDENT = /^[\p{L}_][\p{L}\p{N}_]*$/u;

export function validateIdentifier(name: string): string | null {
  if (!name.trim()) return 'Enter a name';
  if (!IDENT.test(name.trim())) return 'Not a valid Go identifier';
  return null;
}

/** Files and edit counts for the preview, sorted by path. */
export function previewRows(edits: readonly FileEdit[]): { path: string; count: number }[] {
  return edits
    .filter((e) => e.edits.length > 0)
    .map((e) => ({ path: e.path, count: e.edits.length }))
    .sort((a, b) => a.path.localeCompare(b.path));
}

async function confirmPreview(oldName: string, newName: string, edits: FileEdit[]): Promise<boolean> {
  const rows = previewRows(edits);
  const s = summarize(edits);
  const body = h(
    'div',
    { class: 'rename-preview' },
    h('p', null, `Rename ${oldName} to ${newName}: ${s.edits} occurrences in ${s.files} files.`),
    h(
      'ul',
      { class: 'rename-files' },
      rows.map((r) => h('li', null, h('span', null, relPath(r.path)), h('span', { class: 'muted' }, ` (${r.count})`))),
    ),
  );
  return showDialog({
    title: 'Rename',
    body,
    cancelValue: false,
    buttons: [
      { label: 'Cancel', value: false },
      { label: 'Refactor', value: true, primary: true },
    ],
  });
}

export async function renameAtCursor(): Promise<void> {
  const t = await activeTarget();
  if (!t) return;
  const version = t.buf.version;
  let prep;
  try {
    prep = await lang.prepareRename(t.buf.path, t.pos);
  } catch (err) {
    warn('lang.rename', 'Cannot rename', String(err));
    return;
  }
  if (!prep.ok || !fresh(t.buf, version)) {
    if (!prep.ok) info('lang.rename', 'This element cannot be renamed');
    return;
  }
  const doc = t.view.state.doc;
  let oldName = prep.placeholder;
  if (!oldName && (prep.range.start.line || prep.range.start.character || prep.range.end.character)) {
    const { from, to } = toOffsets(doc, prep.range);
    oldName = doc.sliceString(from, to);
  }
  if (!oldName) oldName = wordAt(doc, t.offset)?.text ?? '';
  const newName = await promptDialog('Rename', oldName, validateIdentifier, `Rename ${oldName} to:`);
  if (!newName || newName === oldName) return;

  let edits: FileEdit[];
  try {
    edits = await lang.rename(t.buf.path, t.pos, newName);
  } catch (err) {
    warn('lang.rename', `Cannot rename ${oldName}`, String(err));
    return;
  }
  if (!fresh(t.buf, version)) {
    warn('lang.rename', 'The file changed during rename; try again');
    return;
  }
  const s = summarize(edits);
  if (s.edits === 0) {
    info('lang.rename', 'Nothing to rename');
    return;
  }
  if (s.files > 1 && !(await confirmPreview(oldName, newName, edits))) return;
  try {
    const res = await applyFileEdits(edits);
    info('lang.rename', `Renamed ${oldName} to ${newName}`, `${res.edits} occurrences in ${res.files} file${res.files === 1 ? '' : 's'}`);
  } catch (err) {
    warn('lang.rename', 'Rename failed', String(err));
  }
}
