// @vitest-environment happy-dom
import { Text } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { blockAt, completionAt, docURL, moduleRefs } from '../src/mod/gomod';

const doc = Text.of(['module example.com/x', '', 'go 1.22', '', 'require (', '\tgithub.com/a/b v1.2.0', '\tgolang.org/x/mod v0.20.0 // indirect', ')', '', 'require github.com/c/d v0.1.0', '', 'replace github.com/a/b => ../b', 'replace example.com/old v1.0.0 => example.com/new v1.1.0']);

describe('go.mod parsing', () => {
  it('knows the block of a line', () => {
    expect(blockAt(doc, 6)).toBe('require');
    expect(blockAt(doc, 7)).toBe('require');
    expect(blockAt(doc, 10)).toBe('');
  });
  it('finds module references for links', () => {
    expect(moduleRefs('\tgithub.com/a/b v1.2.0', 'require')).toEqual([{ path: 'github.com/a/b', version: 'v1.2.0', pathFrom: 1, pathTo: 15 }]);
    expect(moduleRefs('require github.com/c/d v0.1.0', '')[0]).toMatchObject({ path: 'github.com/c/d', version: 'v0.1.0', pathFrom: 8 });
    const rep = moduleRefs('replace github.com/a/b => ../b', '');
    expect(rep.map((r) => [r.path, r.local ?? ''])).toEqual([
      ['github.com/a/b', ''],
      ['../b', '../b'],
    ]);
    expect(moduleRefs('replace example.com/old v1.0.0 => example.com/new v1.1.0', '').map((r) => `${r.path}@${r.version}`)).toEqual(['example.com/old@v1.0.0', 'example.com/new@v1.1.0']);
    expect(moduleRefs('go 1.22', '')).toEqual([]);
    expect(moduleRefs('module example.com/x', '')).toEqual([]);
    expect(moduleRefs('require (', '')).toEqual([]);
  });
  it('decides what to complete', () => {
    expect(completionAt('\tgithub.com/st', 14, 'require')).toEqual({ kind: 'path', from: 1, prefix: 'github.com/st' });
    expect(completionAt('\tgithub.com/a/b v1.', 19, 'require')).toEqual({ kind: 'version', from: 16, prefix: 'v1.', module: 'github.com/a/b' });
    expect(completionAt('require github.com/c/d ', 23, '')).toEqual({ kind: 'version', from: 23, prefix: '', module: 'github.com/c/d' });
    expect(completionAt('replace a/b => example.com/n', 28, '')).toEqual({ kind: 'path', from: 15, prefix: 'example.com/n' });
    expect(completionAt('go 1.2', 6, '')).toBeNull();
    expect(completionAt('\tgithub.com/a/b v1.2.0 // ind', 29, 'require')).toBeNull();
  });
  it('builds documentation links', () => {
    expect(docURL({ path: 'github.com/a/b', version: 'v1.2.0', pathFrom: 0, pathTo: 0 })).toBe('https://pkg.go.dev/github.com/a/b@v1.2.0');
  });
});
