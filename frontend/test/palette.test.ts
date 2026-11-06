// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';

vi.mock('../src/lib/api', () => ({ search: { files: vi.fn() } }));
vi.mock('../src/editor/install', () => ({ editors: undefined }));

import { createDoubleShift } from '../src/palette/doubleShift';
import type { KeyLike } from '../src/palette/doubleShift';
import { actionTitle, highlight, SETTINGS, splitRel } from '../src/palette/palette';
import { fuzzyScore, rankItems } from '../src/palette/rank';
import { pushRecent } from '../src/palette/recentFiles';

describe('fuzzyScore', () => {
  it('returns null for non-matches and is case-insensitive', () => {
    expect(fuzzyScore('xyz', 'File: Save')).toBeNull();
    expect(fuzzyScore('SAVE', 'file: save')).not.toBeNull();
  });

  it('reports positions of matched characters', () => {
    const m = fuzzyScore('fs', 'File: Save')!;
    expect(m.positions).toEqual([0, 6]);
    expect(fuzzyScore('', 'anything')).toEqual({ score: 0, positions: [] });
  });

  it('prefers word starts and consecutive runs', () => {
    const a = fuzzyScore('sa', 'Save All')!.score;
    const b = fuzzyScore('sa', 'Show Notifications')!.score;
    expect(a).toBeGreaterThan(b);
    const camel = fuzzyScore('gl', 'goToLine')!.score;
    const mid = fuzzyScore('gl', 'angle')!.score;
    expect(camel).toBeGreaterThan(mid);
  });
});

describe('rankItems', () => {
  const items = ['File: Save All', 'View: Show Notifications', 'File: Save', 'Editor: Go to Line…'];
  it('orders by score, then shorter text', () => {
    const r = rankItems('save', items, (s) => s).map((x) => x.item);
    expect(r).toEqual(['File: Save', 'File: Save All']);
  });
  it('word-start abbreviations', () => {
    expect(rankItems('gtl', items, (s) => s)[0].item).toBe('Editor: Go to Line…');
    expect(rankItems('sa', items, (s) => s)[0].item).toBe('File: Save'); // ties go to the shorter title
    expect(rankItems('fsal', items, (s) => s)[0].item).toBe('File: Save All');
  });
  it('empty query keeps all items in order', () => {
    expect(rankItems('', items, (s) => s).map((x) => x.item)).toEqual(items);
  });
});

describe('createDoubleShift', () => {
  const run = (seq: [KeyLike['type'], string, number, boolean?][]): number => {
    let n = 0;
    const ds = createDoubleShift(() => n++, 300);
    for (const [type, key, t, repeat] of seq) ds.handle({ type, key, repeat }, t);
    return n;
  };

  it('triggers on two quick Shift taps', () => {
    expect(run([['keydown', 'Shift', 0], ['keyup', 'Shift', 50], ['keydown', 'Shift', 120], ['keyup', 'Shift', 170]])).toBe(1);
  });

  it('ignores key repeat', () => {
    expect(
      run([['keydown', 'Shift', 0], ['keydown', 'Shift', 30, true], ['keyup', 'Shift', 60], ['keydown', 'Shift', 100], ['keyup', 'Shift', 150]]),
    ).toBe(1);
  });

  it('does not trigger when another key intervenes', () => {
    expect(run([['keydown', 'Shift', 0], ['keydown', 'A', 20], ['keyup', 'Shift', 50], ['keydown', 'Shift', 100], ['keyup', 'Shift', 150]])).toBe(0);
    expect(run([['keydown', 'Shift', 0], ['keyup', 'Shift', 50], ['keydown', 'a', 80], ['keydown', 'Shift', 120], ['keyup', 'Shift', 170]])).toBe(0);
  });

  it('does not trigger when too slow', () => {
    expect(run([['keydown', 'Shift', 0], ['keyup', 'Shift', 50], ['keydown', 'Shift', 500], ['keyup', 'Shift', 550]])).toBe(0);
  });

  it('third tap after a trigger starts over', () => {
    expect(
      run([
        ['keydown', 'Shift', 0], ['keyup', 'Shift', 10], ['keydown', 'Shift', 50], ['keyup', 'Shift', 60],
        ['keydown', 'Shift', 100], ['keyup', 'Shift', 110],
      ]),
    ).toBe(1);
  });
});

describe('palette helpers', () => {
  it('pushRecent dedupes, fronts and caps', () => {
    expect(pushRecent(['/a', '/b', '/c'], '/b')).toEqual(['/b', '/a', '/c']);
    expect(pushRecent(['/a', '/b'], '/c', 2)).toEqual(['/c', '/a']);
  });

  it('splitRel', () => {
    expect(splitRel('internal/rpc/app.go')).toEqual({ dir: 'internal/rpc', base: 'app.go', baseStart: 13 });
    expect(splitRel('go.mod')).toEqual({ dir: '', base: 'go.mod', baseStart: 0 });
  });

  it('highlight wraps matched characters with an offset', () => {
    const el = document.createElement('span');
    el.append(...highlight('app.go', [13, 14, 17], 13));
    expect(el.innerHTML).toBe('<mark>ap</mark>p.<mark>g</mark>o');
    const plain = document.createElement('span');
    plain.append(...highlight('dir', []));
    expect(plain.innerHTML).toBe('dir');
  });

  it('actionTitle and settings list', () => {
    expect(actionTitle({ id: 'file.save', title: 'Save', category: 'File', run: () => {} })).toBe('File: Save');
    expect(actionTitle({ id: 'x', title: 'X', run: () => {} })).toBe('X');
    expect(new Set(SETTINGS.map((s) => s.key)).size).toBe(SETTINGS.length);
    expect(SETTINGS.find((s) => s.key === 'editor.fontSize')?.title).toBe('Editor: Font Size');
  });
});
