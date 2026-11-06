import { describe, expect, it } from 'vitest';
import { applyChanges, changesFromDiff } from '../src/editor/saveFlow';

describe('changesFromDiff', () => {
  const cases: [string, string][] = [
    ['package m\nfunc  f( ) {  }\n', 'package m\n\nfunc f() {}\n'],
    ['import "os"\n\nfunc f() {}\n', 'func f() {}\n'],
    ['', 'package x\n'],
    ['package x\n', ''],
    ['a\nb\nc\n', 'a\nB\nc\n'],
  ];
  for (const [a, b] of cases) {
    it(`reproduces ${JSON.stringify(b)}`, () => {
      expect(applyChanges(a, changesFromDiff(a, b))).toBe(b);
    });
  }

  it('produces no changes for identical text', () => {
    expect(changesFromDiff('same\n', 'same\n')).toEqual([]);
  });

  it('keeps unchanged regions untouched', () => {
    const head = 'package m\n\n// a long untouched comment block\n';
    const changes = changesFromDiff(`${head}func  f() {}\n`, `${head}func f() {}\n`);
    expect(changes.length).toBeGreaterThan(0);
    for (const c of changes) expect(c.from).toBeGreaterThanOrEqual(head.length);
  });
});
