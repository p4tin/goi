// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { blockSides, conflictBlocks, resolveAll, resolveBlock } from '../src/git/conflicts';

const text = 'top\n<<<<<<< HEAD\nmine\n=======\nyours\n>>>>>>> feature\nmiddle\n<<<<<<< HEAD\na\n||||||| base\nb\n=======\nc\n>>>>>>> feature\nend\n<<<<<<< dangling\n';

describe('conflict blocks', () => {
  it('finds blocks, including diff3 bases, and skips unterminated ones', () => {
    const b = conflictBlocks(text);
    expect(b.map((x) => [x.start, x.base, x.mid, x.end])).toEqual([
      [1, -1, 3, 5],
      [7, 9, 11, 13],
    ]);
    expect(b[0].theirsLabel).toBe('feature');
    expect(blockSides(text, b[1])).toEqual({ ours: ['a'], base: ['b'], theirs: ['c'] });
  });
  it('resolves one block or all of them', () => {
    const [first] = conflictBlocks(text);
    expect(resolveBlock(text, first, 'theirs').startsWith('top\nyours\nmiddle\n')).toBe(true);
    expect(resolveBlock(text, first, 'both').startsWith('top\nmine\nyours\nmiddle\n')).toBe(true);
    expect(resolveAll(text, 'ours')).toBe('top\nmine\nmiddle\na\nend\n<<<<<<< dangling\n');
  });
});
