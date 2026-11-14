// @vitest-environment happy-dom
// Views are opened with focus:false: happy-dom dispatches selectionchange
// synchronously, which re-enters CodeMirror's update in a focused view.
import { describe, expect, it, vi } from 'vitest';
import type { FileMeta, OpenedFile } from '../src/lib/api';
import { EditorManager, insertTab, leaves, MAX_PANES, Pane, removeLeaf, replaceLeaf, sortPinned, tabLabels } from '../src/editor/editorManager';
import { clampRatio } from '../src/editor/panes';
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

  it('wraps each text tab in a content element that features can extend', async () => {
    const { m } = manager({ '/a.md': '# a', '/b.go': 'package b' });
    const created: HTMLElement[] = [];
    m.hooks.viewCreated.on(({ content, view }) => {
      expect(view.dom.parentElement).toBe(content);
      content.append(Object.assign(document.createElement('div'), { className: 'side' }));
      created.push(content);
    });
    await m.open('/a.md', { focus: false });
    await m.open('/b.go', { focus: false });
    const pane = m.activePane.peek();
    const a = pane.contents.get('/a.md')!;
    const b = pane.contents.get('/b.go')!;
    expect(created).toEqual([a, b]);
    expect(a.parentElement).toBe(pane.host);
    expect(a.classList.contains('tab-content')).toBe(true);
    expect([a.hidden, b.hidden]).toEqual([true, false]);
    // Focus inside a sibling of the editor activates its tab.
    const side = a.querySelector<HTMLElement>('.side')!;
    side.tabIndex = 0;
    side.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    expect(m.activePath.peek()).toBe('/a.md');
    expect([a.hidden, b.hidden]).toEqual([false, true]);
    await m.close('/a.md');
    expect(pane.contents.has('/a.md')).toBe(false);
    expect(a.isConnected).toBe(false);
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

describe('splits', () => {
  it('split shows the same buffer and mirrors edits', async () => {
    const { m } = manager({ '/a.txt': 'abc' });
    await m.open('/a.txt', { focus: false });
    const first = m.activePane.peek();
    const second = (await m.split(first, 'row'))!;
    expect(m.panes.peek()).toEqual([first, second]);
    const v1 = first.views.get('/a.txt')!;
    const v2 = second.views.get('/a.txt')!;
    v1.dispatch({ changes: { from: 3, insert: 'd' } });
    expect(v2.state.doc.toString()).toBe('abcd');
    expect(m.buffers.get('/a.txt')!.dirty.peek()).toBe(true);
    expect(m.buffers.get('/a.txt')!.views.size).toBe(2);
  });

  it('caps at MAX_PANES and removes a pane when its last tab closes', async () => {
    const { m } = manager({ '/a': 'x' });
    await m.open('/a', { focus: false });
    const p1 = m.activePane.peek();
    for (let i = 1; i < MAX_PANES; i++) expect(await m.split(m.panes.peek()[0], i % 2 ? 'row' : 'column')).not.toBeNull();
    expect(await m.split(p1, 'row')).toBeNull();
    expect(m.panes.peek()).toHaveLength(MAX_PANES);
    const last = m.panes.peek()[MAX_PANES - 1];
    await m.close('/a', last);
    expect(m.panes.peek()).toHaveLength(MAX_PANES - 1);
    expect(m.buffers.has('/a')).toBe(true); // still open elsewhere
  });

  it('moveToOtherPane splits when alone and moves the tab', async () => {
    const { m } = manager({ '/a': 'x', '/b': 'y' });
    await m.open('/a', { focus: false });
    await m.open('/b', { focus: false });
    await m.moveToOtherPane();
    const [p1, p2] = m.panes.peek();
    expect(p1.tabs.peek().map((t) => t.path)).toEqual(['/a']);
    expect(p2.tabs.peek().map((t) => t.path)).toEqual(['/b']);
  });

  it('tree helpers', () => {
    const a = new Pane();
    const b = new Pane();
    const c = new Pane();
    let t = replaceLeaf({ kind: 'pane', pane: a }, a, { kind: 'split', dir: 'row', ratio: 0.5, a: { kind: 'pane', pane: a }, b: { kind: 'pane', pane: b } });
    t = replaceLeaf(t, b, { kind: 'split', dir: 'column', ratio: 0.5, a: { kind: 'pane', pane: b }, b: { kind: 'pane', pane: c } });
    expect(leaves(t)).toEqual([a, b, c]);
    const r = removeLeaf(t, b)!;
    expect(leaves(r)).toEqual([a, c]);
    expect(removeLeaf({ kind: 'pane', pane: a }, a)).toBeNull();
    expect(clampRatio(0.01)).toBeCloseTo(0.15);
    expect(clampRatio(0.5)).toBe(0.5);
  });
});
