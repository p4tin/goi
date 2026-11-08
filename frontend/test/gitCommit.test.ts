// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { groupFiles, letter } from '../src/git/commitPanel';
import { buildPatch, changedRange, hunkAtLine, hunkStats } from '../src/git/patch';
import type { GitFileDiff, GitFileStatus } from '../src/lib/api';

const f = (rel: string, index: string, work: string, extra: Partial<GitFileStatus> = {}): GitFileStatus => ({
  path: '/r/' + rel, rel, orig: '', index, work, conflict: false, untracked: false, ignored: false, ...extra,
});

describe('commit panel grouping', () => {
  it('splits staged, changes and conflicts (a file can be in both)', () => {
    const g = groupFiles([f('b.go', 'M', 'M'), f('a.go', 'A', '.'), f('c.go', '.', 'M'), f('n.txt', '?', '?', { untracked: true }), f('x.go', 'U', 'U', { conflict: true }), f('i', '!', '!', { ignored: true })]);
    expect(g.staged.map((x) => x.rel)).toEqual(['a.go', 'b.go']);
    expect(g.changes.map((x) => x.rel)).toEqual(['b.go', 'c.go', 'n.txt']);
    expect(g.conflicts.map((x) => x.rel)).toEqual(['x.go']);
  });
  it('shows the group-specific letter', () => {
    expect(letter(f('a', 'A', 'M'), 'staged')).toBe('A');
    expect(letter(f('a', 'A', 'M'), 'changes')).toBe('M');
    expect(letter(f('a', '?', '?', { untracked: true }), 'changes')).toBe('U');
    expect(letter(f('a', 'U', 'U', { conflict: true }), 'conflicts')).toBe('C');
  });
});

const fd: GitFileDiff = {
  oldPath: 'x.go',
  newPath: 'x.go',
  header: ['diff --git a/x.go b/x.go', 'index 1..2 100644', '--- a/x.go', '+++ b/x.go'],
  binary: false,
  hunks: [
    { oldStart: 1, oldLines: 3, newStart: 1, newLines: 3, header: '@@ -1,3 +1,3 @@', lines: [' a', '-b', '+B', ' c'] },
    { oldStart: 20, oldLines: 2, newStart: 20, newLines: 1, header: '@@ -20,2 +20,1 @@', lines: [' y', '-z'] },
  ],
};

describe('patches', () => {
  it('builds a patch with only the chosen hunks', () => {
    expect(buildPatch(fd, [1])).toBe('diff --git a/x.go b/x.go\nindex 1..2 100644\n--- a/x.go\n+++ b/x.go\n@@ -20,2 +20,1 @@\n y\n-z\n');
    expect(() => buildPatch(fd, [])).toThrow();
    expect(() => buildPatch({ ...fd, binary: true }, [0])).toThrow();
  });
  it('finds hunks and changed ranges by line', () => {
    expect(hunkAtLine(fd.hunks, 2)).toBe(0);
    expect(hunkAtLine(fd.hunks, 20)).toBe(1);
    expect(hunkAtLine(fd.hunks, 10)).toBe(-1);
    expect(changedRange(fd.hunks[0])).toEqual({ from: 2, to: 2 });
    expect(changedRange(fd.hunks[1])).toEqual({ from: 21, to: 21 });
    expect(hunkStats(fd)).toEqual({ added: 1, removed: 2 });
  });
});
