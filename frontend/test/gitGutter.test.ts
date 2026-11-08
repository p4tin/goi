// @vitest-environment happy-dom
import { Text } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { changeRanges, rangePatch } from '../src/git/gutter';

const T = (s: string): Text => Text.of(s.split('\n'));
const base = T('a\nb\nc\nd\ne\nf\ng\nh\n');

describe('change ranges', () => {
  it('detects modified, added and deleted lines', () => {
    const r = changeRanges(base, T('a\nB\nc\nd\nnew\ne\nf\nh\n'));
    expect(r.map((x) => [x.kind, x.from, x.to])).toEqual([
      ['modified', 2, 2],
      ['added', 5, 5],
      ['deleted', 8, 8],
    ]);
    expect(changeRanges(base, base)).toEqual([]);
  });
  it('restores the base text over a chunk (rollback)', () => {
    const doc = T('a\nB\nc\nd\nnew\ne\nf\nh\n');
    let text = doc.toString();
    for (const r of [...changeRanges(base, doc)].reverse()) text = text.slice(0, r.bFrom) + base.sliceString(r.aFrom, r.aTo) + text.slice(r.bTo);
    expect(text).toBe(base.toString());
  });
  it('builds a patch with index context', () => {
    const doc = T('a\nB\nc\nd\ne\nf\ng\nh\n');
    const [r] = changeRanges(base, doc);
    expect(rangePatch('x.txt', base, doc, r)).toBe('diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1,5 +1,5 @@\n a\n-b\n+B\n c\n d\n e\n');
    const added = T('a\nb\nc\nd\ne\nf\ng\nh\nz\n');
    const [ra] = changeRanges(base, added);
    expect(ra.kind).toBe('added');
    expect(rangePatch('x.txt', base, added, ra)).toContain('@@ -6,3 +6,4 @@\n f\n g\n h\n+z\n');
  });
});

describe('range patches apply with git', () => {
  it('stages modified, added and deleted chunks (incl. end of file)', async () => {
    const { execFileSync } = await import('node:child_process');
    const { mkdtempSync, writeFileSync } = await import('node:fs');
    const { tmpdir } = await import('node:os');
    const { join } = await import('node:path');
    try {
      execFileSync('git', ['--version']);
    } catch {
      return; // git not installed
    }
    const cases: [string, string][] = [
      ['modify', 'a\nB\nc\nd\ne\nf\ng\nh\n'],
      ['add-middle', 'a\nb\nc\nX\nd\ne\nf\ng\nh\n'],
      ['add-end', 'a\nb\nc\nd\ne\nf\ng\nh\nz\n'],
      ['delete', 'a\nb\nd\ne\nf\ng\nh\n'],
      ['delete-end', 'a\nb\nc\nd\ne\nf\ng\n'],
      ['add-start', 'Z\na\nb\nc\nd\ne\nf\ng\nh\n'],
    ];
    for (const [name, next] of cases) {
      const dir = mkdtempSync(join(tmpdir(), 'goi-patch-'));
      const git = (...args: string[]): string => execFileSync('git', args, { cwd: dir, env: { ...process.env, GIT_CONFIG_NOSYSTEM: '1', HOME: dir } }).toString();
      git('init', '-q');
      writeFileSync(join(dir, 'x.txt'), base.toString());
      git('add', 'x.txt');
      const doc = T(next);
      const ranges = changeRanges(base, doc);
      expect(ranges.length, name).toBe(1);
      const patch = rangePatch('x.txt', base, doc, ranges[0]);
      writeFileSync(join(dir, 'p.diff'), patch);
      git('apply', '--cached', '--recount', join(dir, 'p.diff'));
      const staged = git('show', ':x.txt');
      expect(staged, `${name}\n${patch}`).toBe(next);
    }
  });
});
