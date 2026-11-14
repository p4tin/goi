import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { formatYAML } from '../src/format/yaml';

const ok = async (text: string, indent = 2): Promise<string> => {
  const r = await formatYAML(text, indent);
  if (!r.ok) throw new Error(`unexpected error ${r.line}:${r.col} ${r.message}`);
  return r.text;
};

const fixtures = new URL('./fixtures/yaml/', import.meta.url);

describe('formatYAML', () => {
  it('reindents and keeps comments, anchors and block scalars (AC12)', async () => {
    const src = [
      '# top',
      'base: &base',
      '    name: x   # eol',
      '    script: |',
      '        echo one',
      '          two',
      '',
      'derived:',
      '    <<: *base',
      '    items:',
      '    - a',
      '    # before b',
      '    - b',
      '',
    ].join('\n');
    expect(await ok(src)).toBe(
      [
        '# top',
        'base: &base',
        '  name: x # eol',
        '  script: |',
        '    echo one',
        '      two',
        '',
        'derived:',
        '  <<: *base',
        '  items:',
        '    - a',
        '    # before b',
        '    - b',
        '',
      ].join('\n'),
    );
  });

  it('never changes values (AC13)', async () => {
    const src = 'enabled: yes\nversion: 1.10\nhex: 0x1F\nnull1: ~\noct: 0o17\nbig: 12345678901234567890\n';
    expect(await ok(src)).toBe(src);
  });

  it('keeps quoting styles, tags and flow collections', async () => {
    const src = "a: 'single'\nb: \"double\"\nc: plain\nd: !Ref foo\ne: !!str 3\nf: [x, y, {z: 1}]\ng: {}\nh: []\n";
    expect(await ok(src)).toBe(src);
  });

  it('keeps folded scalars line by line with their chomping', async () => {
    const src = ['a:', '    f: >-', '        long', '        text', '', '          more', '        end', '    k: >+', '        keep', '', '    z: 1', ''].join('\n');
    expect(await ok(src)).toBe(['a:', '  f: >-', '    long', '    text', '', '      more', '    end', '  k: >+', '    keep', '', '  z: 1', ''].join('\n'));
  });

  it('keeps multiple documents and markers', async () => {
    const src = 'a: 1\n---\n# second\nb:\n    - 2\n...\n';
    expect(await ok(src)).toBe('a: 1\n---\n# second\nb:\n  - 2\n...\n');
  });

  it('uses the indent size and keeps long lines', async () => {
    const long = 'x'.repeat(200);
    expect(await ok(`a:\n  b:\n    - ${long} ${long}\n`, 4)).toBe(`a:\n    b:\n        - ${long} ${long}\n`);
  });

  it('keeps the final newline state; empty input is unchanged', async () => {
    expect(await ok('a:   1')).toBe('a: 1');
    expect(await ok('')).toBe('');
    expect(await ok('  \n')).toBe('  \n');
  });

  it('reports syntax errors (AC14)', async () => {
    const r = await formatYAML('a: [1, 2\n', 2);
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.line).toBeGreaterThanOrEqual(1);
      expect(r.message).not.toContain('\n');
    }
    expect(await formatYAML('a: b\n  c: d\n', 2)).toMatchObject({ ok: false });
  });

  it('fails on Helm templates rather than guessing', async () => {
    const r = await formatYAML('metadata:\n  name: {{ .Release.Name }}\n  {{- include "labels" . | nindent 4 }}\n', 2);
    expect(r.ok).toBe(false);
  });

  it('refuses output that would change the document', async () => {
    const r = await formatYAML('a: 1 # c\n', 2, (out) => out.replace(' # c', ''));
    expect(r).toMatchObject({ ok: false, message: 'formatting would change the document' });
    const v = await formatYAML('a: 1\n', 2, (out) => out.replace('1', '2'));
    expect(v.ok).toBe(false);
  });

  for (const name of readdirSync(fixtures)) {
    it(`formats ${name} idempotently, keeping every comment`, async () => {
      const src = readFileSync(new URL(name, fixtures), 'utf8');
      const out = await ok(src);
      expect(await ok(out)).toBe(out);
      for (const m of src.matchAll(/#\s.*$/gm)) expect(out).toContain(m[0].trim());
      expect(out).not.toMatch(/\t/);
    });
  }
});
