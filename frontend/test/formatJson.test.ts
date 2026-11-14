import { describe, expect, it } from 'vitest';
import { formatJSON } from '../src/format/json';

const ok = (text: string, indent = '  '): string => {
  const r = formatJSON(text, indent);
  if (!r.ok) throw new Error(`unexpected error ${r.line}:${r.col} ${r.message}`);
  return r.text;
};

describe('formatJSON', () => {
  it('pretty-prints with tokens kept as written (AC5)', () => {
    expect(ok('{"a":1,"b":[1,2,{"c":12345678901234567890}],"d":{}}')).toBe(
      ['{', '  "a": 1,', '  "b": [', '    1,', '    2,', '    {', '      "c": 12345678901234567890', '    }', '  ],', '  "d": {}', '}'].join('\n'),
    );
  });

  it('keeps numbers, escapes and key order', () => {
    expect(ok('{"z":1.0,"a":-0.5e+10,"s":"\\u00e9\\n\\"q\\"","e":[]}')).toBe(
      '{\n  "z": 1.0,\n  "a": -0.5e+10,\n  "s": "\\u00e9\\n\\"q\\"",\n  "e": []\n}',
    );
  });

  it('uses the indent unit, including tabs', () => {
    expect(ok('[1,[2]]', '\t')).toBe('[\n\t1,\n\t[\n\t\t2\n\t]\n]');
    expect(ok('{"a":{"b":true}}', '    ')).toBe('{\n    "a": {\n        "b": true\n    }\n}');
  });

  it('keeps the final newline state', () => {
    expect(ok('{"a":null}\n')).toBe('{\n  "a": null\n}\n');
    expect(ok('[]')).toBe('[]');
    expect(ok('  "x"  ')).toBe('"x"');
  });

  it('keeps comments (AC7)', () => {
    const src = [
      '// header',
      '{',
      '  "a": 1, // trailing a',
      '      // own line',
      '  "b": /* inline */ 2,',
      '  /* block',
      '     comment */',
      '  "c": [1, /* after comma */ 2]',
      '}',
    ].join('\n');
    const out = ok(src);
    for (const c of ['// header', '// trailing a', '// own line', '/* inline */', '/* block\n     comment */', '/* after comma */']) {
      expect(out).toContain(c);
    }
    expect(out).toBe(
      [
        '// header',
        '{',
        '  "a": 1, // trailing a',
        '  // own line',
        '  "b": /* inline */ 2,',
        '  /* block',
        '     comment */',
        '  "c": [',
        '    1, /* after comma */',
        '    2',
        '  ]',
        '}',
      ].join('\n'),
    );
    // Formatting is idempotent.
    expect(ok(out)).toBe(out);
  });

  it('never joins code onto a line comment', () => {
    expect(ok('[1 // one\n,2]')).toBe('[\n  1 // one\n  ,\n  2\n]');
    expect(ok('{ // c\n}')).toBe('{ // c\n}');
    expect(ok('{\n// only\n}')).toBe('{\n  // only\n}');
  });

  it('keeps trailing commas', () => {
    expect(ok('{"a":[1,2,],}')).toBe('{\n  "a": [\n    1,\n    2,\n  ],\n}');
  });

  it('keeps one blank line between groups', () => {
    expect(ok('{\n"a": 1,\n\n\n"b": 2\n}')).toBe('{\n  "a": 1,\n\n  "b": 2\n}');
    expect(ok('{\n\n"a": 1}')).toBe('{\n  "a": 1\n}');
  });

  it('reports syntax errors with positions (AC6)', () => {
    expect(formatJSON('{"a": }', '  ')).toMatchObject({ ok: false, line: 1, col: 7 });
    expect(formatJSON('{\n  "a": 1\n  "b": 2\n}', '  ')).toMatchObject({ ok: false, line: 3, col: 3 });
    expect(formatJSON('[1, 2', '  ')).toMatchObject({ ok: false, message: 'unexpected end of input' });
    expect(formatJSON('{"a": tru}', '  ')).toMatchObject({ ok: false, line: 1, col: 7 });
    expect(formatJSON('{"a": "x\\q"}', '  ')).toMatchObject({ ok: false, col: 9 });
    expect(formatJSON('{"a": "x', '  ')).toMatchObject({ ok: false, message: 'unterminated string' });
    expect(formatJSON('1 2', '  ')).toMatchObject({ ok: false, col: 3 });
    expect(formatJSON('[,]', '  ')).toMatchObject({ ok: false, col: 2 });
    expect(formatJSON('{"a" 1}', '  ')).toMatchObject({ ok: false, col: 6 });
    expect(formatJSON('{"a": 1 /* x', '  ')).toMatchObject({ ok: false, message: 'unterminated comment' });
  });

  it('leaves empty and comment-only input unchanged', () => {
    expect(ok('')).toBe('');
    expect(ok('  // nothing\n')).toBe('  // nothing\n');
  });

  it('handles deep nesting without recursion', () => {
    const deep = '['.repeat(10000) + ']'.repeat(10000);
    const r = formatJSON(deep, ' ');
    expect(r.ok).toBe(true);
  });
});
