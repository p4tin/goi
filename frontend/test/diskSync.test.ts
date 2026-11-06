// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';

// diskSync imports install.ts, which pulls Wails bindings; stub the API.
vi.mock('../src/lib/api', () => ({ files: {}, on: () => () => {}, Topics: {} }));

import { ChangeSet, Text } from '@codemirror/state';
import { EditorManager } from '../src/editor/editorManager';
import { applyDisk, decideExternalChange, minimalChanges } from '../src/editor/diskSync';
import type { FileMeta, OpenedFile } from '../src/lib/api';

const meta = (path: string, extra: Partial<FileMeta> = {}): FileMeta =>
  ({ path, size: 5, modTime: 't1', binary: false, eol: '\n', bom: false, finalNewline: true, mode: 'normal', ...extra }) as FileMeta;

describe('decideExternalChange', () => {
  const base = { dirty: false, savedText: 'hello', savedMeta: { modTime: 't1', size: 5 } };
  it('ignores our own writes (same meta or same content)', () => {
    expect(decideExternalChange(base, { content: 'other', modTime: 't1', size: 5 })).toBe('ignore');
    expect(decideExternalChange(base, { content: 'hello', modTime: 't2', size: 5 })).toBe('ignore');
  });
  it('reloads clean buffers and conflicts dirty ones', () => {
    const disk = { content: 'changed', modTime: 't2', size: 7 };
    expect(decideExternalChange(base, disk)).toBe('reload');
    expect(decideExternalChange({ ...base, dirty: true }, disk)).toBe('conflict');
  });
  it('reports deletion', () => {
    expect(decideExternalChange(base, 'deleted')).toBe('deleted');
    expect(decideExternalChange({ ...base, dirty: true }, 'deleted')).toBe('deleted');
  });
});

describe('minimalChanges', () => {
  it('produces changes that turn old into new', () => {
    const cases: [string, string][] = [
      ['a\nb\nc\n', 'a\nB\nc\n'],
      ['', 'new'],
      ['remove all', ''],
      ['same', 'same'],
      ['func f() {}\n', 'package x\n\nfunc f() {}\n'],
    ];
    for (const [a, b] of cases) {
      const cs = ChangeSet.of(minimalChanges(a, b), a.length);
      expect(cs.apply(Text.of(a.split('\n'))).toString()).toBe(b);
    }
    expect(minimalChanges('same', 'same')).toEqual([]);
  });
});

describe('applyDisk', () => {
  it('updates the view minimally, keeps the cursor and clears dirty', async () => {
    const m = new EditorManager({
      io: { read: async (p) => ({ meta: meta(p), content: 'line1\nline2\nline3' }) as OpenedFile, write: async (_p, _c, mt) => mt },
      confirmClose: async () => 'cancel',
      onError: () => {},
    });
    document.body.appendChild(m.activePane.peek().el);
    const buf = (await m.open('/p/a.txt', { focus: false, line: 3, col: 3 }))!;
    const view = m.activeView()!;
    const head = view.state.selection.main.head;
    applyDisk(buf, 'LINE1\nline2\nline3', meta('/p/a.txt', { modTime: 't2' }));
    expect(view.state.doc.toString()).toBe('LINE1\nline2\nline3');
    expect(view.state.selection.main.head).toBe(head);
    expect(buf.dirty.peek()).toBe(false);
    expect(buf.meta.modTime).toBe('t2');
  });

  it('updates a buffer with no view', () => {
    const buf = { views: new Set(), markSaved: vi.fn() } as unknown as Parameters<typeof applyDisk>[0];
    applyDisk(buf, 'x\ny', meta('/p/b'));
    expect((buf.markSaved as ReturnType<typeof vi.fn>).mock.calls[0][0].toString()).toBe('x\ny');
  });
});
