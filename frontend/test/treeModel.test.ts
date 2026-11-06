import { describe, expect, it } from 'vitest';
import {
  ancestorsWithin,
  applyFsEvents,
  baseName,
  emptyState,
  isWithin,
  join,
  moveSelection,
  parentOf,
  relativeTo,
  visibleRows,
} from '../src/views/treeModel';
import type { Entry, TreeRoot } from '../src/views/treeModel';

const d = (path: string): Entry => ({ name: baseName(path), path, isDir: true });
const f = (path: string): Entry => ({ name: baseName(path), path, isDir: false });
const roots: TreeRoot[] = [{ path: '/p', label: 'proj', extra: false }];

describe('path helpers', () => {
  it('handle posix paths', () => {
    expect(parentOf('/p/a/b.go')).toBe('/p/a');
    expect(parentOf('/p')).toBe('/');
    expect(baseName('/p/a/')).toBe('a');
    expect(join('/p/a', 'x.go')).toBe('/p/a/x.go');
    expect(join('/', 'x')).toBe('/x');
    expect(relativeTo('/p', '/p/a/b')).toBe('a/b');
  });

  it('handle windows paths', () => {
    expect(parentOf('C:\\p\\a\\b.go')).toBe('C:\\p\\a');
    expect(parentOf('C:\\p')).toBe('C:\\');
    expect(baseName('C:\\p\\a')).toBe('a');
    expect(join('C:\\p', 'x.go')).toBe('C:\\p\\x.go');
    expect(isWithin('C:\\p\\a\\b', 'C:\\p\\a')).toBe(true);
    expect(isWithin('C:\\p\\ab', 'C:\\p\\a')).toBe(false);
    expect(relativeTo('C:\\p', 'C:\\p\\a\\b')).toBe('a/b');
  });

  it('ancestorsWithin lists intermediate dirs top-first', () => {
    expect(ancestorsWithin('/p', '/p/a/b/c.go')).toEqual(['/p/a', '/p/a/b']);
    expect(ancestorsWithin('/p', '/p/c.go')).toEqual([]);
  });
});

describe('visibleRows', () => {
  it('flattens expanded dirs with depth and loading placeholders', () => {
    const s = emptyState();
    s.expanded.add('/p');
    s.expanded.add('/p/a');
    s.expanded.add('/p/b');
    s.children.set('/p', [d('/p/a'), d('/p/b'), f('/p/go.mod')]);
    s.children.set('/p/a', [f('/p/a/x.go')]);
    s.children.set('/p/b', 'loading');
    const rows = visibleRows(roots, s);
    expect(rows.map((r) => [r.name, r.depth, r.loading])).toEqual([
      ['proj', 0, false],
      ['a', 1, false],
      ['x.go', 2, false],
      ['b', 1, false],
      ['loading…', 2, true],
      ['go.mod', 1, false],
    ]);
    expect(rows[0].root).toBe(roots[0]);
    expect(rows[1].expanded).toBe(true);
  });

  it('collapsed root shows only itself', () => {
    expect(visibleRows(roots, emptyState()).length).toBe(1);
  });
});

describe('applyFsEvents', () => {
  const loaded = () => {
    const s = emptyState();
    s.expanded = new Set(['/p', '/p/a', '/p/a/sub']);
    s.children.set('/p', [d('/p/a'), f('/p/x.go')]);
    s.children.set('/p/a', [d('/p/a/sub')]);
    s.children.set('/p/a/sub', [f('/p/a/sub/y.go')]);
    return s;
  };

  it('reloads loaded parents only', () => {
    const { reload } = applyFsEvents(loaded(), [
      { path: '/p/new.go', op: 'create', isDir: false },
      { path: '/p/a/sub/z.go', op: 'write', isDir: false },
      { path: '/p/unloaded/q.go', op: 'create', isDir: false },
    ]);
    expect([...reload].sort()).toEqual(['/p', '/p/a/sub']);
  });

  it('drops removed expanded dirs and descendants and fixes selection', () => {
    const s = loaded();
    s.selected = '/p/a/sub/y.go';
    const { state, reload } = applyFsEvents(s, [{ path: '/p/a', op: 'remove', isDir: true }]);
    expect([...state.expanded]).toEqual(['/p']);
    expect(state.children.has('/p/a')).toBe(false);
    expect(state.children.has('/p/a/sub')).toBe(false);
    expect(state.selected).toBe('/p');
    expect([...reload]).toEqual(['/p']);
    expect(s.expanded.size).toBe(3); // input not mutated
  });

  it('works with windows paths', () => {
    const s = emptyState();
    s.expanded = new Set(['C:\\p', 'C:\\p\\a']);
    s.children.set('C:\\p', [d('C:\\p\\a')]);
    s.children.set('C:\\p\\a', []);
    s.selected = 'C:\\p\\a';
    const { state, reload } = applyFsEvents(s, [{ path: 'C:\\p\\a', op: 'rename', isDir: true }]);
    expect(state.selected).toBe('C:\\p');
    expect([...reload]).toEqual(['C:\\p']);
    expect(state.expanded.has('C:\\p\\a')).toBe(false);
  });
});

describe('moveSelection', () => {
  it('clamps and skips loading rows', () => {
    const s = emptyState();
    s.expanded = new Set(['/p', '/p/a']);
    s.children.set('/p', [d('/p/a'), f('/p/b')]);
    s.children.set('/p/a', 'loading');
    const rows = visibleRows(roots, s);
    expect(moveSelection(rows, null, 1)).toBe('/p');
    expect(moveSelection(rows, '/p/a', 1)).toBe('/p/b');
    expect(moveSelection(rows, '/p/b', 5)).toBe('/p/b');
    expect(moveSelection(rows, '/p', -1)).toBe('/p');
  });
});
