// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';

vi.mock('../src/lib/api', () => ({
  app: {},
  recentApi: {},
  on: () => () => {},
  Topics: { recentChanged: 'recent.changed' },
}));

const { middleEllipsis, moveSelection, relativeTime } = await import('../src/views/welcome');
const { validateNewProject } = await import('../src/views/newProject');

describe('relativeTime', () => {
  const now = new Date('2026-11-06T12:00:00Z');
  it.each([
    ['2026-11-06T11:59:30Z', 'just now'],
    ['2026-11-06T11:55:00Z', '5 min ago'],
    ['2026-11-06T09:00:00Z', '3 h ago'],
    ['2026-11-04T12:00:00Z', '2 d ago'],
  ])('%s → %s', (then, want) => {
    expect(relativeTime(then, now)).toBe(want);
  });
  it('falls back to a date for old entries and empty for invalid', () => {
    expect(relativeTime('2025-01-01T00:00:00Z', now)).not.toMatch(/ago/);
    expect(relativeTime('nonsense', now)).toBe('');
  });
});

describe('middleEllipsis', () => {
  it('keeps short strings', () => {
    expect(middleEllipsis('/a/b', 10)).toBe('/a/b');
  });
  it('shortens to max with head and tail', () => {
    const out = middleEllipsis('/Users/me/projects/very/long/path/goi', 15);
    expect(out.length).toBe(15);
    expect(out.startsWith('/Users/')).toBe(true);
    expect(out.endsWith('ath/goi')).toBe(true);
    expect(out).toContain('…');
  });
});

describe('moveSelection', () => {
  it.each([
    [-1, 'ArrowDown', 3, 0],
    [-1, 'ArrowUp', 3, 2],
    [0, 'ArrowDown', 3, 1],
    [2, 'ArrowDown', 3, 2],
    [0, 'ArrowUp', 3, 0],
    [1, 'Home', 3, 0],
    [0, 'End', 3, 2],
    [1, 'x', 3, 1],
    [0, 'ArrowDown', 0, -1],
  ])('(%i, %s, %i) → %i', (cur, key, count, want) => {
    expect(moveSelection(cur, key, count)).toBe(want);
  });
});

describe('validateNewProject', () => {
  const ok = { parent: '/tmp', name: 'app', modulePath: 'example.com/app' };
  it('accepts valid input', () => {
    expect(validateNewProject(ok)).toBeNull();
  });
  it.each([
    [{ ...ok, parent: ' ' }, /parent/],
    [{ ...ok, name: '' }, /name/],
    [{ ...ok, name: 'a/b' }, /separators/],
    [{ ...ok, name: '..' }, /separators/],
    [{ ...ok, modulePath: '' }, /module path/],
    [{ ...ok, modulePath: 'a b' }, /spaces/],
  ])('rejects %o', (input, re) => {
    expect(validateNewProject(input)).toMatch(re);
  });
});
