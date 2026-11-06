import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { StringStream } from '@codemirror/language';
import type { StreamParser } from '@codemirror/language';
import { describe, expect, it } from 'vitest';
import { gomodParser, gosumParser } from '../src/editor/lang/gomod';
import { gotmplParser } from '../src/editor/lang/gotmpl';
import { languageIdFor, languageNames, loadLanguage } from '../src/editor/languages';
import type { LangId } from '../src/editor/languages';
import { formatMB, largeFileMessage } from '../src/editor/largeFile';

describe('languageIdFor', () => {
  const cases: [string, LangId][] = [
    ['/p/go.mod', 'gomod'],
    ['/p/go.work', 'gomod'],
    ['/p/go.sum', 'gosum'],
    ['/p/main.go', 'go'],
    ['/p/x_test.go', 'go'],
    ['/p/page.gotmpl', 'gotmpl'],
    ['/p/page.tmpl', 'gotmpl'],
    ['/p/page.gohtml', 'gotmpl'],
    ['/p/Dockerfile', 'dockerfile'],
    ['/p/Containerfile', 'dockerfile'],
    ['/p/api.dockerfile', 'dockerfile'],
    ['/p/Makefile', 'makefile'],
    ['/p/GNUmakefile', 'makefile'],
    ['/p/rules.mk', 'makefile'],
    ['/p/run.sh', 'shell'],
    ['/p/run.bash', 'shell'],
    ['/p/run.zsh', 'shell'],
    ['/home/u/.bashrc', 'shell'],
    ['/home/u/.zshrc', 'shell'],
    ['/home/u/.profile', 'shell'],
    ['/p/a.yaml', 'yaml'],
    ['/p/a.yml', 'yaml'],
    ['/p/a.json', 'json'],
    ['/p/a.jsonc', 'json'],
    ['/p/a.code-workspace', 'json'],
    ['/p/a.toml', 'toml'],
    ['/p/README.md', 'markdown'],
    ['/p/a.markdown', 'markdown'],
    ['/p/q.sql', 'sql'],
    ['/p/a.proto', 'protobuf'],
    ['/p/i.html', 'html'],
    ['/p/i.htm', 'html'],
    ['/p/s.css', 'css'],
    ['/p/s.scss', 'css'],
    ['/p/s.less', 'css'],
    ['/p/a.js', 'javascript'],
    ['/p/a.mjs', 'javascript'],
    ['/p/a.cjs', 'javascript'],
    ['/p/a.jsx', 'javascript'],
    ['/p/a.ts', 'typescript'],
    ['/p/a.tsx', 'typescript'],
    ['/p/a.mts', 'typescript'],
    ['/p/a.xml', 'xml'],
    ['/p/a.svg', 'xml'],
    ['/p/Info.plist', 'xml'],
    ['/p/.env', 'properties'],
    ['/p/.env.local', 'properties'],
    ['/p/a.ini', 'properties'],
    ['/p/a.properties', 'properties'],
    ['/p/.editorconfig', 'properties'],
    ['/p/LICENSE', 'plaintext'],
    ['/p/notes.txt', 'plaintext'],
    ['/p/.gitignore', 'plaintext'],
  ];
  it.each(cases)('%s → %s', (path, want) => {
    expect(languageIdFor(path)).toBe(want);
  });

  it('is case-insensitive and handles Windows paths', () => {
    expect(languageIdFor('C:\\src\\proj\\MAIN.GO')).toBe('go');
    expect(languageIdFor('C:\\src\\proj\\GO.MOD')).toBe('gomod');
    expect(languageIdFor('C:\\src\\proj\\dockerfile')).toBe('dockerfile');
    expect(languageIdFor('C:\\src\\proj\\Config.YML')).toBe('yaml');
  });

  it('maps every sample fixture to a language', () => {
    const dir = join(__dirname, 'samples');
    for (const f of readdirSync(dir)) {
      const id = languageIdFor(join(dir, f));
      if (f === 'notes.txt') expect(id).toBe('plaintext');
      else expect(id, f).not.toBe('plaintext');
    }
  });

  it('has a display name for every id', () => {
    expect(languageNames.go).toBe('Go');
    expect(languageNames.gomod).toBe('go.mod');
    expect(languageNames.plaintext).toBe('Plain Text');
  });
});

function tokens<S>(parser: StreamParser<S>, lines: string[]): [string, string | null][] {
  const state = parser.startState!(2);
  const out: [string, string | null][] = [];
  for (const line of lines) {
    const s = new StringStream(line, 4, 2);
    while (!s.eol()) {
      const t = parser.token(s, state);
      if (s.pos === s.start) throw new Error(`no progress at ${s.pos} in ${line}`);
      if (t) out.push([s.current(), t]);
      s.start = s.pos;
    }
  }
  return out;
}

describe('gomod tokenizer', () => {
  it('tokenizes directives, paths, versions, comments, replace arrows', () => {
    const t = tokens(gomodParser, [
      'module example.com/m',
      'go 1.26.0',
      'require (',
      '\texample.com/dep v1.2.3-pre+meta // indirect',
      ')',
      'replace example.com/a => ../a',
      'tool example.com/require',
    ]);
    expect(t).toContainEqual(['module', 'keyword']);
    expect(t).toContainEqual(['example.com/m', 'variableName']);
    expect(t).toContainEqual(['go', 'keyword']);
    expect(t).toContainEqual(['1.26.0', 'number']);
    expect(t).toContainEqual(['require', 'keyword']);
    expect(t).toContainEqual(['v1.2.3-pre+meta', 'number']);
    expect(t).toContainEqual(['// indirect', 'comment']);
    expect(t).toContainEqual(['=>', 'operator']);
    expect(t).toContainEqual(['tool', 'keyword']);
    // A keyword-looking word that is not first on the line stays a path.
    expect(t).toContainEqual(['example.com/require', 'variableName']);
  });

  it('tokenizes go.sum lines', () => {
    const t = tokens(gosumParser, ['example.com/dep v1.2.3/go.mod h1:abc+/=']);
    expect(t).toEqual([
      ['example.com/dep', 'variableName'],
      ['v1.2.3', 'number'],
      ['/go.mod', 'meta'],
      ['h1:abc+/=', 'string'],
    ]);
  });
});

describe('gotmpl tokenizer', () => {
  it('styles actions only', () => {
    const t = tokens(gotmplParser, ['<p>{{- if .User.Name -}}Hi {{ $x := "a" | printf "%s" 3 }}{{/* c */}}{{ end }}</p>']);
    expect(t).toContainEqual(['{{-', 'meta']);
    expect(t).toContainEqual(['if', 'keyword']);
    expect(t).toContainEqual(['.User', 'propertyName']);
    expect(t).toContainEqual(['.Name', 'propertyName']);
    expect(t).toContainEqual(['-}}', 'meta']);
    expect(t).toContainEqual(['$x', 'variableName']);
    expect(t).toContainEqual([':=', 'operator']);
    expect(t).toContainEqual(['"a"', 'string']);
    expect(t).toContainEqual(['|', 'operator']);
    expect(t).toContainEqual(['printf', 'variableName.function']);
    expect(t).toContainEqual(['3', 'number']);
    expect(t).toContainEqual(['/* c */', 'comment']);
    expect(t).toContainEqual(['end', 'keyword']);
    expect(t.some(([text]) => text.includes('<p>') || text.includes('Hi'))).toBe(false);
  });

  it('carries multi-line comments across lines', () => {
    const t = tokens(gotmplParser, ['{{/* start', 'still comment */}}', 'text']);
    expect(t).toEqual([
      ['{{', 'meta'],
      ['/* start', 'comment'],
      ['still comment */', 'comment'],
      ['}}', 'meta'],
    ]);
  });
});

describe('loadLanguage', () => {
  const ids = Object.keys(languageNames) as LangId[];
  it.each(ids)('loads %s', async (id) => {
    const ext = await loadLanguage(id);
    expect(ext).toBeDefined();
    if (id !== 'plaintext') expect(ext).not.toEqual([]);
    expect(await loadLanguage(id)).toBe(ext); // cached
  });
});

describe('largeFileMessage', () => {
  const buf = (mode: string, size: number, binary = false) => ({ meta: { mode, size } as never, binary });
  it('describes degraded modes', () => {
    expect(formatMB(3 * 1024 * 1024)).toBe('3.0 MB');
    expect(largeFileMessage(buf('normal', 10))).toBeNull();
    expect(largeFileMessage(buf('noHighlight', 3 * 1024 * 1024))).toBe('Large file (3.0 MB): syntax highlighting disabled');
    expect(largeFileMessage(buf('readOnly', 25 * 1024 * 1024))).toBe('Very large file (25.0 MB): opened read-only');
    expect(largeFileMessage(buf('readOnly', 1, true))).toBeNull();
  });
});
