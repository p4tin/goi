import { describe, expect, it } from 'vitest';
import { classifyLink, insideRoots, joinPath, resolveImage } from '../src/markdown/links';

const md = '/p/docs/guide/README.md';

describe('links', () => {
  it('joins and normalizes paths', () => {
    expect(joinPath('/p/docs', '../a/./b.md')).toBe('/p/a/b.md');
    expect(joinPath('/p', '../../x')).toBe('/x');
    expect(joinPath('C:\\p\\docs', '..\\img\\a.png')).toBe('C:\\p\\img\\a.png');
    expect(joinPath('C:\\p\\docs', 'img/a.png')).toBe('C:\\p\\docs\\img\\a.png');
  });

  it('classifies link targets', () => {
    expect(classifyLink('https://go.dev/doc', md)).toEqual({ kind: 'external', url: 'https://go.dev/doc' });
    expect(classifyLink('#install-it', md)).toEqual({ kind: 'anchor', id: 'install-it' });
    expect(classifyLink('../api.md#Get%20Started', md)).toEqual({ kind: 'file', path: '/p/docs/api.md', anchor: 'Get Started' });
    expect(classifyLink('my%20notes.md?plain=1', md)).toEqual({ kind: 'file', path: '/p/docs/guide/my notes.md', anchor: undefined });
    expect(classifyLink('/CONTRIBUTING.md', md, '/p')).toEqual({ kind: 'file', path: '/p/CONTRIBUTING.md', anchor: undefined });
    for (const bad of ['javascript:alert(1)', 'file:///etc/passwd', 'data:text/html,x', 'mailto:a@b.c', '', '#']) {
      expect(classifyLink(bad, md)).toEqual({ kind: 'none' });
    }
  });

  it('resolves images', () => {
    expect(resolveImage('https://img.shields.io/x.svg', md)).toEqual({ kind: 'remote', url: 'https://img.shields.io/x.svg' });
    expect(resolveImage('./img/a%20b.png', md)).toEqual({ kind: 'local', path: '/p/docs/guide/img/a b.png' });
    expect(resolveImage('../../logo.png', md)).toEqual({ kind: 'local', path: '/p/logo.png' });
    expect(resolveImage('data:image/png;base64,xx', md)).toEqual({ kind: 'none' });
    expect(resolveImage('javascript:x', md)).toEqual({ kind: 'none' });
  });

  it('checks project roots', () => {
    expect(insideRoots('/p/a.md', ['/p'])).toBe(true);
    expect(insideRoots('/p', ['/p/'])).toBe(true);
    expect(insideRoots('/pp/a.md', ['/p'])).toBe(false);
    expect(insideRoots('C:\\p\\a.md', ['C:\\p'])).toBe(true);
  });
});
