// "Show Difference" for failed tests (R3.4): extracts expected/actual values
// from testify-style failure output and shows them side by side.

import { MergeView } from '@codemirror/merge';
import { EditorView } from '@codemirror/view';
import { showDialog } from '../app/dialog';
import { h } from '../lib/dom';

export interface ExpectedActual {
  expected: string;
  actual: string;
}

/** testify lines: "<indent>\t<label>\t<content>" (label may be blank). */
const TESTIFY_LINE = /^\s*\t([^\t]*)\t(.*)$/;

/** Strips the testify column layout from output lines. */
function contentLines(output: string): string[] {
  return output.split('\n').map((l) => {
    const m = TESTIFY_LINE.exec(l);
    return m ? m[2] : l.trim();
  });
}

/** Unquotes a Go double-quoted string literal (or returns s unchanged). */
export function unquoteGo(s: string): string {
  const t = s.trim();
  if (t.length >= 2 && t.startsWith('"') && t.endsWith('"')) {
    try {
      return JSON.parse(t.replace(/\\x([0-9a-fA-F]{2})/g, '\\u00$1').replace(/\\'/g, "'")) as string;
    } catch {
      return t;
    }
  }
  if (t.length >= 2 && t.startsWith('`') && t.endsWith('`')) return t.slice(1, -1);
  return s;
}

/** Finds the first expected/actual pair in failure output. */
export function parseExpectedActual(output: string): ExpectedActual | null {
  const lines = contentLines(output);
  const ei = lines.findIndex((l) => /^expected\s*:/.test(l));
  if (ei < 0) return null;
  const ai = lines.findIndex((l, i) => i > ei && /^actual\s*:/.test(l));
  if (ai < 0) return null;
  const stop = (l: string): boolean => l === '' || /^(Diff|Test|Messages|Error Trace|Error)\s*:/.test(l) || /^-{3} /.test(l);
  const take = (from: number, label: RegExp, until: number): string => {
    const first = lines[from].replace(label, '');
    const rest: string[] = [];
    for (let i = from + 1; i < until && !stop(lines[i]); i++) rest.push(lines[i]);
    return [first, ...rest].join('\n');
  };
  let end = ai + 1;
  while (end < lines.length && !stop(lines[end])) end++;
  return {
    expected: unquoteGo(take(ei, /^expected\s*:\s?/, ai)),
    actual: unquoteGo(take(ai, /^actual\s*:\s?/, end)),
  };
}

export async function showDifference(title: string, d: ExpectedActual): Promise<void> {
  const host = h('div', { class: 'diff-host' });
  const head = h('div', { class: 'diff-head' }, h('span', null, 'Expected'), h('span', null, 'Actual'));
  const theme = EditorView.theme({ '&': { height: '100%' }, '.cm-scroller': { fontFamily: 'var(--goi-editor-font, ui-monospace, monospace)' } });
  const mv = new MergeView({
    a: { doc: d.expected, extensions: [EditorView.editable.of(false), EditorView.lineWrapping, theme] },
    b: { doc: d.actual, extensions: [EditorView.editable.of(false), EditorView.lineWrapping, theme] },
    parent: host,
  });
  await showDialog({ title, body: h('div', { class: 'diff-dialog' }, head, host), cancelValue: undefined, buttons: [{ label: 'Close', value: undefined, primary: true }] });
  mv.destroy();
}
