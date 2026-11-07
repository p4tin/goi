// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { crumbsAt, flattenSymbols, kindBadge } from '../src/lang/structure';
import { symbolEntry } from '../src/lang/symbols';
import type { DocumentSymbol } from '../src/lib/api';

const r = (l1: number, l2: number) => ({ start: { line: l1, character: 0 }, end: { line: l2, character: 1 } });
const sym = (name: string, kind: number, l1: number, l2: number, children: DocumentSymbol[] = []): DocumentSymbol => ({
  name,
  kind,
  range: r(l1, l2),
  selectionRange: r(l1, l1),
  children,
});

const tree = [
  sym('main', 12, 20, 30),
  sym('Server', 23, 2, 10, [sym('Addr', 8, 4, 4), sym('mu', 8, 3, 3)]),
  sym('(*Server).Start', 6, 12, 18),
];

describe('structure', () => {
  it('flattens in document order with depth and container', () => {
    const flat = flattenSymbols(tree);
    expect(flat.map((f) => `${'  '.repeat(f.depth)}${f.sym.name}`)).toEqual(['Server', '  mu', '  Addr', '(*Server).Start', 'main']);
    expect(flat[2].container).toEqual(['Server']);
  });
  it('finds the crumb chain at a position', () => {
    expect(crumbsAt(tree, { line: 4, character: 0 }).map((s) => s.name)).toEqual(['Server', 'Addr']);
    expect(crumbsAt(tree, { line: 15, character: 3 }).map((s) => s.name)).toEqual(['(*Server).Start']);
    expect(crumbsAt(tree, { line: 11, character: 0 })).toEqual([]);
  });
  it('badges kinds', () => {
    expect(kindBadge(12)).toBe('ƒ');
    expect(kindBadge(23)).toBe('S');
    expect(kindBadge(999)).toBe('•');
  });
  it('builds palette entries for workspace symbols', () => {
    const e = symbolEntry({ name: 'Start', kind: 6, containerName: 'example.com/m.Server', location: { path: '/p/server.go', range: r(11, 11) } }, () => {});
    expect(e.label).toBe('Start');
    expect(e.detail).toBe('example.com/m.Server · /p/server.go:12');
    expect(e.kind).toBe('m');
  });
});
