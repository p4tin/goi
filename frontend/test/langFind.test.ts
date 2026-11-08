// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { groupMatches, previewParts, replaceInText, replacementFor, toJsReplacement } from '../src/lang/findInPath';
import type { FindQuery, TextMatch } from '../src/lib/api';

const q = (over: Partial<FindQuery>): FindQuery => ({ pattern: '', regex: false, caseSensitive: false, wholeWord: false, mask: '', replace: '', scope: '', buffers: [], ...over });
const m = (path: string, line: number, col: number, end: number, preview = '', previewStart = 0): TextMatch => ({ path, line, col, end, preview, previewStart });

describe('find in path helpers', () => {
  it('groups matches by file in order', () => {
    const g = groupMatches([m('/a', 1, 0, 1), m('/b', 2, 0, 1), m('/a', 5, 0, 1)]);
    expect(g.map((x) => [x.path, x.items.length])).toEqual([['/a', 2], ['/b', 1]]);
  });
  it('splits previews around the match', () => {
    expect(previewParts(m('/a', 1, 10, 13, '\tfunc() { Foo() }'))).toEqual(['func() { ', 'Foo', '() }']);
    expect(previewParts(m('/a', 1, 105, 108, 'xxabcyy', 103))).toEqual(['xx', 'abc', 'yy']);
  });
  it('converts Go replacement templates', () => {
    expect(toJsReplacement('$2=$1')).toBe('$2=$1');
    expect(toJsReplacement('${1}x')).toBe('$1x');
    expect(toJsReplacement('${name}!')).toBe('$<name>!');
    expect(toJsReplacement('cost $$5')).toBe('cost $$5');
  });
  it('computes replacements', () => {
    expect(replacementFor(q({ replace: '$1' }), 'foo')).toBe('$1');
    expect(replacementFor(q({ regex: true, pattern: '(\\w+)=(\\w+)', replace: '$2=$1' }), 'a=b')).toBe('b=a');
    expect(replacementFor(q({ regex: true, pattern: 'foo', replace: 'X' }), 'FOO')).toBe('X');
  });
  it('replaces listed spans in text, right to left per line', () => {
    const text = 'foo foo\nbar\nfoo';
    const out = replaceInText(text, [m('/a', 1, 0, 3), m('/a', 1, 4, 7), m('/a', 3, 0, 3)], q({ pattern: 'foo', replace: 'baz' }));
    expect(out).toBe('baz baz\nbar\nbaz');
  });
});
