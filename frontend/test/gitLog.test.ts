// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { blameLabel } from '../src/git/blame';
import { filterCommits, relativeDate } from '../src/git/log';
import type { GitCommit } from '../src/lib/api';

const now = Date.parse('2026-11-08T12:00:00Z');
const c = (hash: string, subject: string, author: string, refs = ''): GitCommit => ({ hash, short: hash.slice(0, 7), parents: [], author, email: '', date: '', subject, body: '', refs });

describe('git log helpers', () => {
  it('formats relative dates', () => {
    expect(relativeDate('2026-11-08T11:59:40Z', now)).toBe('just now');
    expect(relativeDate('2026-11-08T11:55:00Z', now)).toBe('5 minutes ago');
    expect(relativeDate('2026-11-08T09:00:00Z', now)).toBe('3 hours ago');
    expect(relativeDate('2026-11-07T10:00:00Z', now)).toBe('yesterday');
    expect(relativeDate('2026-11-04T12:00:00Z', now)).toBe('4 days ago');
    expect(relativeDate('2026-09-01T12:00:00Z', now)).toBe('2026-09-01');
    expect(relativeDate('garbage', now)).toBe('garbage');
  });
  it('filters by hash prefix, subject, author and refs', () => {
    const list = [c('abc123', 'Fix login', 'Ada'), c('def456', 'Add tests', 'Linus', 'tag: v1.0'), c('0a1b2c', 'Refactor', 'ada lovelace')];
    expect(filterCommits(list, 'abc').map((x) => x.hash)).toEqual(['abc123']);
    expect(filterCommits(list, 'ADA').map((x) => x.hash)).toEqual(['abc123', '0a1b2c']);
    expect(filterCommits(list, 'v1.0').map((x) => x.hash)).toEqual(['def456']);
    expect(filterCommits(list, '')).toHaveLength(3);
  });
  it('labels blame lines', () => {
    expect(blameLabel({ line: 1, hash: 'a'.repeat(40), author: 'Ada Lovelace', date: '2026-10-01T10:00:00Z', summary: 's' })).toBe('aaaaaaa Ada Lovelace 2026-10-01');
    expect(blameLabel({ line: 1, hash: '0'.repeat(40), author: 'Not Committed Yet', date: '', summary: '' })).toBe('Not committed yet');
    expect(blameLabel({ line: 1, hash: 'b'.repeat(40), author: 'A Very Long Author Name', date: '2026-10-01', summary: '' })).toBe('bbbbbbb A Very Long A… 2026-10-01');
  });
});
