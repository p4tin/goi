// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { enclosingFunction, markerContext, pickContext } from '../src/run/context';
import type { DocumentSymbol, RunMarker } from '../src/lib/api';

const sym = (name: string, kind: number, start: number, end: number, children: DocumentSymbol[] = []): DocumentSymbol =>
  ({ name, kind, detail: '', range: { start: { line: start, character: 0 }, end: { line: end, character: 1 } }, selectionRange: { start: { line: start, character: 5 }, end: { line: start, character: 5 + name.length } }, children }) as DocumentSymbol;

const markers: RunMarker[] = [
  { kind: 'test', name: 'TestA', line: 5 },
  { kind: 'bench', name: 'BenchmarkB', line: 20 },
];

describe('run context', () => {
  it('finds the enclosing function from document symbols', () => {
    const syms = [sym('T', 23, 0, 3, [sym('M', 6, 1, 2)]), sym('TestA', 12, 4, 10), sym('helper', 12, 12, 15)];
    expect(enclosingFunction(syms, 6)).toBe('TestA');
    expect(enclosingFunction(syms, 13)).toBe('helper');
    expect(enclosingFunction(syms, 11)).toBeNull();
  });
  it('picks the marker of the enclosing function', () => {
    expect(pickContext('/p/x_test.go', markers, 7, 'TestA')).toEqual({ path: '/p/x_test.go', kind: 'test', name: 'TestA' });
    expect(pickContext('/p/x_test.go', markers, 21, 'BenchmarkB')).toMatchObject({ kind: 'bench', name: 'BenchmarkB' });
  });
  it('falls back to the file tests outside test functions', () => {
    expect(pickContext('/p/x_test.go', markers, 13, 'helper')).toEqual({ path: '/p/x_test.go', kind: 'file', name: '' });
  });
  it('uses the nearest marker above when gopls is unavailable', () => {
    expect(pickContext('/p/x_test.go', markers, 22, undefined)).toMatchObject({ kind: 'bench', name: 'BenchmarkB' });
  });
  it('runs func main in non-test files', () => {
    const main: RunMarker[] = [{ kind: 'main', name: 'main', line: 3 }];
    expect(pickContext('/p/main.go', main, 10, null)).toEqual(markerContext('/p/main.go', main[0]));
    expect(pickContext('/p/util.go', [], 1, null)).toBeNull();
  });
});
