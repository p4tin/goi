import { describe, expect, it } from 'vitest';
import { Buffer } from '../src/editor/buffer';
import { JournalTracker } from '../src/editor/journal';
import type { FileMeta } from '../src/lib/api';

const meta = { path: '/a', mode: 'normal' } as FileMeta;

describe('JournalTracker', () => {
  it('journals dirty buffers once per version', () => {
    const t = new JournalTracker();
    const b = new Buffer('/a', meta, 'x');
    expect(t.due([b])).toEqual([]); // clean
    b.changed(b.saved.replace(0, 1, b.saved.slice(0, 0).append(b.saved)));
    b.dirty.set(true);
    expect(t.due([b])).toEqual([b]);
    t.wrote(b);
    expect(t.due([b])).toEqual([]);
    b.version++;
    expect(t.due([b])).toEqual([b]);
    t.forget('/a');
    expect(t.due([b])).toEqual([b]);
  });
});
