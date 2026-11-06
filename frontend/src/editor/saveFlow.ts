// Format on save (R8): formats Go buffers before they are written, as one
// minimal undoable edit. Syntax errors never block saving; they show as a
// diagnostic and a status message.

import { setDiagnostics } from '@codemirror/lint';
import type { Diagnostic } from '@codemirror/lint';
import { diff } from '@codemirror/merge';
import type { EditorView } from '@codemirror/view';
import { settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { editorApi } from '../lib/api';
import type { FormatMode, FormatResult } from '../lib/api';
import { commands } from '../lib/commands';
import type { Buffer } from './buffer';
import { editors } from './install';

export interface TextChange {
  from: number;
  to: number;
  insert: string;
}

/** Minimal changes (positions in oldText) that turn oldText into newText. */
export function changesFromDiff(oldText: string, newText: string): TextChange[] {
  if (oldText === newText) return [];
  return diff(oldText, newText).map((c) => ({ from: c.fromA, to: c.toA, insert: newText.slice(c.fromB, c.toB) }));
}

/** Applies changes (non-overlapping, ascending, positions in the original). */
export function applyChanges(text: string, changes: TextChange[]): string {
  let out = '';
  let pos = 0;
  for (const c of changes) {
    out += text.slice(pos, c.from) + c.insert;
    pos = c.to;
  }
  return out + text.slice(pos);
}

const STATUS_MS = 5000;

export function installSaveFlow(wb: Workbench): void {
  const status = wb.status.add({ id: 'save-status', side: 'left', priority: 50 });
  status.hidden = true;
  let timer: ReturnType<typeof setTimeout> | null = null;
  const say = (text: string, warn: boolean): void => {
    status.textContent = text;
    status.classList.toggle('status-warn', warn);
    status.hidden = false;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => (status.hidden = true), STATUS_MS);
  };

  const run = async (buf: Buffer, mode?: FormatMode): Promise<void> => {
    const views = [...buf.views];
    const view = views[0];
    if (!view || buf.readOnly || !buf.path.endsWith('.go')) return;
    const before = view.state.doc.toString();
    const t0 = performance.now();
    let res: FormatResult;
    try {
      res = mode ? await editorApi.formatWith(buf.path, before, mode) : await editorApi.format(buf.path, before);
    } catch (err) {
      say(`Format failed: ${String(err)}`, true);
      return;
    }
    console.debug(`format ${buf.name}: ${Math.round(performance.now() - t0)} ms`);
    if (res.syntaxError) {
      const se = res.syntaxError;
      for (const v of views) v.dispatch(setDiagnostics(v.state, [diagnosticAt(v, se.line, se.col, se.msg)]));
      say(`Not formatted: syntax error at ${se.line}:${se.col}`, true);
      return;
    }
    for (const v of views) v.dispatch(setDiagnostics(v.state, []));
    // The document may have changed while the backend was formatting.
    if (!res.changed || view.state.doc.toString() !== before) return;
    const changes = changesFromDiff(before, res.content);
    if (changes.length) view.dispatch({ changes, userEvent: 'format' });
  };

  editors.hooks.beforeSave.on(async (buf) => {
    const fmt = settings.peek()?.save.format ?? 'goimports';
    if (fmt === 'none') return;
    await run(buf);
  });

  commands.register({
    id: 'editor.reformat',
    title: 'Reformat File',
    category: 'Editor',
    when: () => !!editors.activeBuffer()?.path.endsWith('.go'),
    run: async () => {
      const buf = editors.activeBuffer();
      if (!buf) return;
      const fmt = (settings.peek()?.save.format ?? 'goimports') as FormatMode;
      await run(buf, fmt === 'none' ? 'gofmt' : fmt);
    },
  });
}

function diagnosticAt(view: EditorView, line: number, col: number, message: string): Diagnostic {
  const doc = view.state.doc;
  const l = doc.line(Math.min(Math.max(1, line), doc.lines));
  const from = Math.min(l.from + Math.max(0, col - 1), l.to);
  return { from, to: Math.min(from + 1, l.to), severity: 'error', message, source: 'gofmt' };
}
