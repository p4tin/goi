// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { groupUsages, isWriteUsage } from '../src/lang/usages';
import type { Usage } from '../src/lang/usages';

describe('isWriteUsage', () => {
  const at = (line: string, ident: string): boolean => isWriteUsage(line, line.indexOf(ident) + ident.length);
  it.each([
    ['\tx = 1', 'x', true],
    ['\tx := f()', 'x', true],
    ['\tcount += 2', 'count', true],
    ['\ti++', 'i', true],
    ['\tm[k] = v', 'm', true],
    ['\ta, b = b, a', 'a', true],
    ['\tv, err := f()', 'v', true],
    ['\tif x == y {', 'x', false],
    ['\tif x <= y {', 'x', false],
    ['\tif x != y {', 'x', false],
    ['\ts.field = 1', 's', false],
    ['\tf(x, y)', 'x', false],
    ['\treturn x', 'x', false],
    ['\tx.Close()', 'x', false],
  ])('%s → %s is write: %s', (line, ident, want) => {
    expect(at(line, ident)).toBe(want);
  });
});

describe('groupUsages', () => {
  const u = (path: string, line: number, test = path.endsWith('_test.go')): Usage => ({
    path,
    range: { start: { line, character: 0 }, end: { line, character: 1 } },
    text: '',
    write: false,
    test,
  });
  it('groups production before test, then package and file', () => {
    const g = groupUsages([u('/p/b/x.go', 3), u('/p/a/y_test.go', 1), u('/p/a/z.go', 9), u('/p/a/z.go', 2), u('/p/b/w.go', 1)]);
    expect(g.map((x) => x.kind)).toEqual(['prod', 'test']);
    expect(g[0].count).toBe(4);
    expect(g[0].packages.map((p) => p.dir)).toEqual(['/p/a', '/p/b']);
    expect(g[0].packages[0].files[0].items.map((i) => i.range.start.line)).toEqual([2, 9]);
    expect(g[0].packages[1].files.map((f) => f.path)).toEqual(['/p/b/w.go', '/p/b/x.go']);
    expect(g[1].packages[0].files[0].path).toBe('/p/a/y_test.go');
  });
});
