// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { branchLabel, dirsOf, emptyState, fileKind } from '../src/git/state';
import type { GitFileStatus } from '../src/lib/api';

const f = (path: string, index: string, work: string, extra: Partial<GitFileStatus> = {}): GitFileStatus => ({
  path, rel: path.slice(3), orig: '', index, work, conflict: false, untracked: false, ignored: false, ...extra,
});

describe('git state', () => {
  it('maps porcelain codes to kinds', () => {
    expect(fileKind(f('/r/a', '.', 'M'))).toBe('modified');
    expect(fileKind(f('/r/a', 'M', '.'))).toBe('modified');
    expect(fileKind(f('/r/a', 'A', '.'))).toBe('added');
    expect(fileKind(f('/r/a', 'A', 'M'))).toBe('added');
    expect(fileKind(f('/r/a', 'D', '.'))).toBe('deleted');
    expect(fileKind(f('/r/a', '.', 'D'))).toBe('deleted');
    expect(fileKind(f('/r/a', 'R', '.'))).toBe('renamed');
    expect(fileKind(f('/r/a', '?', '?', { untracked: true }))).toBe('untracked');
    expect(fileKind(f('/r/a', 'U', 'U', { conflict: true }))).toBe('conflict');
  });
  it('marks every changed directory up to the root', () => {
    const dirs = dirsOf([f('/r/a/b/c.go', '.', 'M'), f('/r/x.go', '?', '?', { untracked: true }), f('/r/ign/z', '!', '!', { ignored: true })], '/r');
    expect([...dirs].sort()).toEqual(['/r', '/r/a', '/r/a/b']);
  });
  it('labels branches with ahead/behind and operations', () => {
    const st = { ...emptyState().status, branch: 'main', ahead: 2, behind: 1 };
    expect(branchLabel(st)).toBe('main ↑2 ↓1');
    expect(branchLabel({ ...st, ahead: 0, behind: 0, op: 'merge' })).toBe('main (MERGING)');
    expect(branchLabel({ ...st, branch: '', detached: true, head: 'abcdef123456', ahead: 0, behind: 0 })).toBe('(abcdef1)');
    expect(branchLabel(emptyState().status)).toBe('');
  });
});
