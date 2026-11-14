// Format on save (R8) and Reformat File: Go through the backend, JSON/YAML
// through registered formatters (formatters.ts), each as one minimal
// undoable edit. Syntax errors never block saving; they show as a diagnostic
// and a status message.

import { setDiagnostics } from '@codemirror/lint';
import type { Diagnostic } from '@codemirror/lint';
import { indentUnit } from '@codemirror/language';
import { diff } from '@codemirror/merge';
import type { EditorView } from '@codemirror/view';
import { settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { editorApi } from '../lib/api';
import type { FormatMode, FormatResult, Settings } from '../lib/api';
import { commands } from '../lib/commands';
import type { Buffer } from './buffer';
import { formatterFor } from './formatters';
import type { FormatOutcome } from './formatters';
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

/** Shows a status message (warn: as a warning). */
export type Say = (text: string, warn: boolean) => void;

/** Formats a buffer as one minimal undoable edit: Go through the backend
 * (mode, or the save.format setting), other files through their registered
 * formatter. Errors leave the document unchanged, with a diagnostic. */
export async function formatBuffer(buf: Buffer, say: Say, mode?: FormatMode): Promise<void> {
  const views = [...buf.views];
  const view = views[0];
  if (!view || buf.readOnly) return;
  const before = view.state.doc.toString();
  const t0 = performance.now();
  let outcome: FormatOutcome;
  let source: string;
  try {
    if (buf.path.endsWith('.go')) {
      source = 'gofmt';
      const res: FormatResult = mode ? await editorApi.formatWith(buf.path, before, mode) : await editorApi.format(buf.path, before);
      const se = res.syntaxError;
      outcome = se ? { ok: false, line: se.line, col: se.col, message: se.msg } : { ok: true, text: res.changed ? res.content : before };
    } else {
      const f = formatterFor(buf.path);
      if (!f) return;
      source = f.id;
      outcome = await f.format(before, { indentUnit: view.state.facet(indentUnit), tabSize: view.state.tabSize, path: buf.path });
    }
  } catch (err) {
    say(`Format failed: ${String(err)}`, true);
    return;
  }
  console.debug(`format ${buf.name}: ${Math.round(performance.now() - t0)} ms`);
  if (!outcome.ok) {
    if (outcome.kind === 'unsafe') {
      say(`Not formatted: ${outcome.message}`, true);
      return;
    }
    const o = outcome;
    for (const v of views) v.dispatch(setDiagnostics(v.state, [diagnosticAt(v, o.line, o.col, o.message, source)]));
    say(`Not formatted: syntax error at ${o.line}:${o.col}`, true);
    return;
  }
  for (const v of views) v.dispatch(setDiagnostics(v.state, []));
  // The document may have changed while formatting.
  if (outcome.text === before || view.state.doc.toString() !== before) return;
  const changes = changesFromDiff(before, outcome.text);
  if (changes.length) view.dispatch({ changes, userEvent: 'format' });
}

/** Whether saving path formats it first. */
export function formatsOnSave(path: string, s: Settings | null): boolean {
  if (path.endsWith('.go')) return (s?.save.format ?? 'goimports') !== 'none';
  const f = formatterFor(path);
  return !!(f && s && f.onSave(s));
}

/** Whether Reformat File applies to path. */
export function canReformat(path: string): boolean {
  return path.endsWith('.go') || !!formatterFor(path);
}

export function installSaveFlow(wb: Workbench): void {
  const status = wb.status.add({ id: 'save-status', side: 'left', priority: 50 });
  status.hidden = true;
  let timer: ReturnType<typeof setTimeout> | null = null;
  const say: Say = (text, warn) => {
    status.textContent = text;
    status.classList.toggle('status-warn', warn);
    status.hidden = false;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => (status.hidden = true), STATUS_MS);
  };

  editors.hooks.beforeSave.on(async (buf) => {
    if (formatsOnSave(buf.path, settings.peek())) await formatBuffer(buf, say);
  });

  commands.register({
    id: 'editor.reformat',
    title: 'Reformat File',
    category: 'Editor',
    when: () => {
      const path = editors.activeBuffer()?.path;
      return !!path && canReformat(path);
    },
    run: async () => {
      const buf = editors.activeBuffer();
      if (!buf) return;
      const fmt = (settings.peek()?.save.format ?? 'goimports') as FormatMode;
      await formatBuffer(buf, say, fmt === 'none' ? 'gofmt' : fmt);
    },
  });
}

function diagnosticAt(view: EditorView, line: number, col: number, message: string, source: string): Diagnostic {
  const doc = view.state.doc;
  const l = doc.line(Math.min(Math.max(1, line), doc.lines));
  const from = Math.min(l.from + Math.max(0, col - 1), l.to);
  return { from, to: Math.min(from + 1, l.to), severity: 'error', message, source };
}
