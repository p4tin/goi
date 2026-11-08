// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { kindBadge, overlapping, sortActions } from '../src/lang/codeActions';
import { previewRows, validateIdentifier } from '../src/lang/rename';
import type { CodeAction, Diagnostic } from '../src/lib/api';

const rng = (l1: number, c1: number, l2: number, c2: number) => ({ start: { line: l1, character: c1 }, end: { line: l2, character: c2 } });

describe('rename', () => {
  it('validates identifiers', () => {
    expect(validateIdentifier('newName')).toBeNull();
    expect(validateIdentifier('_x1')).toBeNull();
    expect(validateIdentifier('héllo')).toBeNull();
    expect(validateIdentifier('1abc')).toMatch(/valid/);
    expect(validateIdentifier('a-b')).toMatch(/valid/);
    expect(validateIdentifier(' ')).toMatch(/Enter/);
  });
  it('builds preview rows', () => {
    const e = { range: rng(0, 0, 0, 1), newText: 'x' };
    expect(previewRows([{ path: '/b.go', edits: [e, e] }, { path: '/a.go', edits: [e] }, { path: '/c.go', edits: [] }])).toEqual([
      { path: '/a.go', count: 1 },
      { path: '/b.go', count: 2 },
    ]);
  });
});

describe('context actions', () => {
  it('sorts quick-fixes first and drops disabled actions', () => {
    const acts: CodeAction[] = [
      { title: 'Extract function', kind: 'refactor.extract' },
      { title: 'Organize imports', kind: 'source.organizeImports' },
      { title: 'Fix A', kind: 'quickfix' },
      { title: 'Fix B (preferred)', kind: 'quickfix', isPreferred: true },
      { title: 'Nope', kind: 'quickfix', disabled: { reason: 'x' } },
    ];
    expect(sortActions(acts).map((a) => a.title)).toEqual(['Fix B (preferred)', 'Fix A', 'Extract function', 'Organize imports']);
    expect(kindBadge(acts[0])).toBe('ref');
    expect(kindBadge(acts[2])).toBe('fix');
  });
  it('picks overlapping diagnostics', () => {
    const ds: Diagnostic[] = [
      { range: rng(3, 4, 3, 9), message: 'a' },
      { range: rng(5, 0, 6, 2), message: 'b' },
      { range: rng(9, 0, 9, 1), message: 'c' },
    ];
    expect(overlapping(ds, rng(3, 0, 3, 0)).map((d) => d.message)).toEqual(['a']);
    expect(overlapping(ds, rng(6, 1, 6, 1)).map((d) => d.message)).toEqual(['b']);
    expect(overlapping(ds, rng(3, 8, 5, 1)).map((d) => d.message)).toEqual(['a', 'b']);
    expect(overlapping(ds, rng(7, 0, 8, 0))).toEqual([]);
  });
});
