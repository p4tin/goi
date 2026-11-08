// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { inLintScope, lintActions } from '../src/lint/lint';
import type { Diagnostic } from '../src/lib/api';

const d = (msg: string, data?: unknown): Diagnostic => ({ range: { start: { line: 1, character: 0 }, end: { line: 1, character: 3 } }, severity: 2, message: msg, source: 'golangci-lint (godot)', data }) as Diagnostic;

describe('lint', () => {
  it('scopes package runs to one directory and project runs recursively', () => {
    expect(inLintScope('/r/calc/a.go', ['/r/calc'], false)).toBe(true);
    expect(inLintScope('/r/calc/sub/a.go', ['/r/calc'], false)).toBe(false);
    expect(inLintScope('/r/calc/sub/a.go', ['/r'], true)).toBe(true);
    expect(inLintScope('/rx/a.go', ['/r'], true)).toBe(false);
    expect(inLintScope('C:\\r\\a.go', ['C:\\r'], false)).toBe(true);
  });
  it('turns suggested edits into preferred quick fixes', () => {
    const edits = [{ path: '/r/a.go', edits: [{ range: { start: { line: 2, character: 0 }, end: { line: 2, character: 5 } }, newText: 'x.' }] }];
    const acts = lintActions([d('Comment should end in a period', { goiLintFix: edits }), d('no fix here')]);
    expect(acts).toHaveLength(1);
    expect(acts[0]).toMatchObject({ kind: 'quickfix', isPreferred: true, goiEdits: edits });
    expect(acts[0].title).toBe('Apply fix: Comment should end in a period (golangci-lint (godot))');
  });
});
