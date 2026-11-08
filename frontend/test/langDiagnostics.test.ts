// @vitest-environment happy-dom
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { describe, expect, it } from 'vitest';
import { cmSeverity, countProblems, groupProblems, toCmDiagnostics } from '../src/lang/diagnostics';
import type { Diagnostic } from '../src/lib/api';

const d = (line: number, ch: number, severity: 1 | 2 | 3 | 4, message = 'm', extra: Partial<Diagnostic> = {}): Diagnostic => ({
  range: { start: { line, character: ch }, end: { line, character: ch + 3 } },
  severity,
  message,
  ...extra,
});

describe('problems', () => {
  const map = new Map<string, Diagnostic[]>([
    ['/p/b.go', [d(4, 0, 2), d(1, 0, 2)]],
    ['/p/a.go', [d(9, 0, 3)]],
    ['/p/c.go', [d(3, 2, 2), d(2, 0, 1), d(2, 0, 1)]],
    ['/p/empty.go', []],
  ]);
  it('groups files with errors first and sorts items', () => {
    const g = groupProblems(map);
    expect(g.map((x) => x.path)).toEqual(['/p/c.go', '/p/a.go', '/p/b.go']);
    expect(g[0].items.map((x) => x.severity)).toEqual([1, 1, 2]);
    expect(g[0].errors).toBe(2);
    expect(g[2].items.map((x) => x.range.start.line)).toEqual([1, 4]);
  });
  it('counts', () => {
    expect(countProblems(map)).toEqual({ errors: 2, warnings: 3, total: 6 });
  });
  it('maps severities', () => {
    expect(cmSeverity(1)).toBe('error');
    expect(cmSeverity(4)).toBe('hint');
    expect(cmSeverity(undefined)).toBe('error');
  });
  it('converts to CodeMirror diagnostics with marks', () => {
    const view = new EditorView({ state: EditorState.create({ doc: 'package p\n\nvar unused = 1\n' }) });
    const [cm] = toCmDiagnostics(view, [d(2, 4, 2, 'declared and not used', { source: 'compiler', tags: [1] })]);
    expect(view.state.sliceDoc(cm.from, cm.to)).toBe('unu');
    expect(cm.severity).toBe('warning');
    expect(cm.source).toBe('compiler');
    expect(cm.markClass).toBe('cm-lang-unnecessary');
    const [empty] = toCmDiagnostics(view, [{ range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }, message: 'x' }]);
    expect(empty.to).toBe(1);
    view.destroy();
  });
});
