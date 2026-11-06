// @vitest-environment happy-dom
// Views are opened with focus:false: happy-dom dispatches selectionchange
// synchronously, which re-enters CodeMirror's update in a focused view.
import { describe, expect, it, vi } from 'vitest';
import type { FileMeta, OpenedFile } from '../src/lib/api';
import { EditorManager, insertTab, sortPinned, tabLabels } from '../src/editor/editorManager';
import type { CloseChoice } from '../src/editor/editorManager';
import { parseLineCol, resolveScheme } from '../src/editor/install';

function meta(path: string, extra: Partial<FileMeta> = {}): FileMeta {
  return { path, size: 0, modTime: '', binary: false, eol: '\n', bom: false, finalNewline: true, mode: 'normal', ...extra } as FileMeta;
}

function manager(files: Record<string, string>, opts: { choice?: CloseChoice; metas?: Record<string, Partial<FileMeta>> } = {}) {
  const writes: { path: string; content: string }[] = [];
  const errors: string[] = [];
  const m = new EditorManager({
    io: {
      read: async (p) => {
        if (!(p in files)) throw new Error('missing');
        return { meta: meta(p, opts.metas?.[p]), content: files[p] } as OpenedFile;
      },
      write: async (p, content, mt) => {
        writes.push({ path: p, content });
        files[p] = content;
        return { ...mt, size: content.length } as FileMeta;
      },
    },
    confirmClose: vi.fn(async () => opts.choice ?? 'cancel'),
    onError: (t) => errors.push(t),
  });
  document.body.appendChild(m.activePane.peek().el);
  return { m, writes, errors };
}

describe('EditorManager', () => {
  it('opens files into tabs and activates them', async () => {
    const { m } = manager({ '/p/a.go': 'package a\n', '/p/b.go': 'package b\n' });
    await m.open('/p/a.go', { focus: false });
    await m.open('/p/b.go');
    const pane = m.activePane.peek();
    expect(pane.tabs.peek().map((t) => t.path)).toEqual(['/p/a.go', '/p/b.go']);
    expect(m.activePath.peek()).toBe('/p/b.go');
    expect(m.activeView()?.state.doc.toString()).toBe('package b\n');
    await m.open('/p/a.go', { line: 1, col: 9 });
    expect(m.cursor.peek()).toMatchObject({ line: 1, col: 9 });
  });

  it('reports open errors without throwing', async () => {
    const { m, errors } = manager({});
    expect(await m.open('/nope')).toBeNull();
    expect(errors).toHaveLength(1);
  });

  it('tracks dirty state and saves', async () => {
    const { m, writes } = manager({ '/p/a.txt': 'hello' });
    const buf = (await m.open('/p/a.txt', { focus: false }))!;
    const v = m.activeView()!;
    v.dispatch({ changes: { from: 5, insert: ' world' } });
    expect(buf.dirty.peek()).toBe(true);
    v.dispatch({ changes: { from: 5, to: 11 } });
    expect(buf.dirty.peek()).toBe(false); // back to saved content
    v.dispatch({ changes: { from: 0, insert: '>' } });
    expect(await m.save()).toBe(true);
    expect(writes).toEqual([{ path: '/p/a.txt', content: '>hello' }]);
    expect(buf.dirty.peek()).toBe(false);
  });

  it('runs beforeSave hooks that may edit the document', async () => {
    const { m, writes } = manager({ '/p/a.go': 'x' });
    await m.open('/p/a.go', { focus: false });
    m.hooks.beforeSave.on((buf) => {
      const v = [...buf.views][0];
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: 'formatted' } });
    });
    m.activeView()!.dispatch({ changes: { from: 1, insert: 'y' } });
    await m.save();
    expect(writes[0].content).toBe('formatted');
  });

  it('asks before closing a dirty buffer', async () => {
    const { m } = manager({ '/p/a.txt': 'a' }, { choice: 'cancel' });
    await m.open('/p/a.txt', { focus: false });
    m.activeView()!.dispatch({ changes: { from: 0, insert: 'b' } });
    expect(await m.close('/p/a.txt')).toBe(false);
    expect(m.buffers.has('/p/a.txt')).toBe(true);
    expect(await m.close('/p/a.txt', undefined, true)).toBe(true);
    expect(m.buffers.size).toBe(0);
    expect(m.activePath.peek()).toBeNull();
  });

  it('save choice on close writes then closes', async () => {
    const { m, writes } = manager({ '/p/a.txt': 'a' }, { choice: 'save' });
    await m.open('/p/a.txt', { focus: false });
    m.activeView()!.dispatch({ changes: { from: 0, insert: 'b' } });
    expect(await m.close('/p/a.txt')).toBe(true);
    expect(writes).toHaveLength(1);
  });

  it('activates a neighbour after closing the active tab', async () => {
    const { m } = manager({ '/a': '', '/b': '', '/c': '' });
    for (const p of ['/a', '/b', '/c']) await m.open(p);
    await m.open('/b');
    await m.close('/b');
    expect(m.activePath.peek()).toBe('/c');
  });

  it('closeWhere keeps pinned tabs', async () => {
    const { m } = manager({ '/a': '', '/b': '', '/c': '' });
    await m.open('/a', { pinned: true });
    await m.open('/b');
    await m.open('/c');
    await m.closeWhere(m.activePane.peek(), () => true);
    expect(m.activePane.peek().tabs.peek().map((t) => t.path)).toEqual(['/a']);
  });

  it('read-only and binary buffers', async () => {
    const { m } = manager({ '/big.log': 'x', '/img.png': '' }, { metas: { '/big.log': { mode: 'readOnly' }, '/img.png': { binary: true } } });
    const big = (await m.open('/big.log'))!;
    expect(big.readOnly).toBe(true);
    expect(m.activeView()!.state.readOnly).toBe(true);
    expect(await m.save()).toBe(false);
    await m.open('/img.png');
    expect(m.activeView()).toBeNull();
    expect(m.activePane.peek().host.querySelector('.binary-placeholder')).not.toBeNull();
  });

  it('extension factories apply to existing views', async () => {
    const { m } = manager({ '/a': 'x' });
    await m.open('/a');
    let calls = 0;
    m.addExtension(() => {
      calls++;
      return [];
    });
    expect(calls).toBe(1);
  });
});

describe('tab helpers', () => {
  it('insertTab after active and keeps pinned first', () => {
    const ts = [
      { path: '/p', pinned: true },
      { path: '/a', pinned: false },
      { path: '/b', pinned: false },
    ];
    expect(insertTab(ts, { path: '/n', pinned: false }, '/a').map((t) => t.path)).toEqual(['/p', '/a', '/n', '/b']);
    expect(sortPinned([{ path: '/x', pinned: false }, { path: '/y', pinned: true }]).map((t) => t.path)).toEqual(['/y', '/x']);
  });

  it('tabLabels disambiguates duplicate names', () => {
    const l = tabLabels(['/r/a/main.go', '/r/b/main.go', '/r/c/util.go']);
    expect(l.get('/r/a/main.go')).toBe('main.go — a');
    expect(l.get('/r/c/util.go')).toBe('util.go');
  });

  it('parseLineCol', () => {
    expect(parseLineCol('12')).toEqual({ line: 12, col: 1 });
    expect(parseLineCol(' 3:7 ')).toEqual({ line: 3, col: 7 });
    expect(parseLineCol('3,7')).toEqual({ line: 3, col: 7 });
    expect(parseLineCol('x')).toBeNull();
  });
});

describe('resolveScheme', () => {
  it('follows the UI theme for auto and unknown values', () => {
    expect(resolveScheme('auto', 'light')).toBe('goi-light');
    expect(resolveScheme('auto', 'dark')).toBe('goi-dark');
    expect(resolveScheme('bogus', 'hc')).toBe('goi-dark');
    expect(resolveScheme('goi-ember', 'light')).toBe('goi-ember');
  });
});
