// @vitest-environment node
// Feature order: pack features go right after their anchor, and every
// pack's anchors exist among the core features (the order decides dock
// default tabs, see CLAUDE.md).
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { composeFeatures } from '../src/pack';

const src = join(process.cwd(), 'src');

const coreIds = (): string[] => {
  const text = readFileSync(join(src, 'features.ts'), 'utf8');
  const src2 = text;
  const body = src2.slice(src2.indexOf('export const coreFeatures'), src2.indexOf('];', src2.indexOf('export const coreFeatures')));
  return [...body.matchAll(/\['(\w+)', \w+\]/g)].map((m) => m[1]);
};

describe('composeFeatures', () => {
  it('inserts pack features after their anchor', () => {
    const f = () => {};
    const out = composeFeatures(
      [['a', f], ['b', f], ['c', f]],
      [{ id: 'x', after: 'b', install: f }, { id: 'y', after: 'x', install: f }],
    );
    expect(out.map(([id]) => id)).toEqual(['a', 'b', 'x', 'y', 'c']);
  });

  it('rejects an unknown anchor', () => {
    expect(() => composeFeatures([['a', () => {}]], [{ id: 'x', after: 'nope', install: () => {} }])).toThrow(/nope/);
  });

  it('keeps Project before Commit (git) in the core order', () => {
    const ids = coreIds();
    expect(ids.indexOf('projectTree')).toBeLessThan(ids.indexOf('git'));
    expect(ids.indexOf('git')).toBeLessThan(ids.indexOf('agent'));
  });

  it("anchors every pack's features to core features", () => {
    const ids = new Set(coreIds());
    for (const p of readdirSync(join(src, 'packs'))) {
      const text = readFileSync(join(src, 'packs', p, 'index.ts'), 'utf8');
      const own = [...text.matchAll(/id: '(\w+)'/g)].map((m) => m[1]);
      for (const m of text.matchAll(/after: '(\w+)'/g)) {
        expect(ids.has(m[1]) || own.includes(m[1]), `${p}: anchor ${m[1]}`).toBe(true);
      }
    }
  });
});
