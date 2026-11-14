// @vitest-environment happy-dom
// Views are opened with focus:false (see editor.test.ts).
import { describe, expect, it, vi } from 'vitest';

vi.mock('../src/lib/api', () => ({ editorApi: {}, on: () => () => {}, Topics: {} }));
vi.mock('../src/editor/install', () => ({ editors: undefined }));

import { undo } from '@codemirror/commands';
import { indentUnit } from '@codemirror/language';
import { diagnosticCount, forEachDiagnostic } from '@codemirror/lint';
import type { Diagnostic } from '@codemirror/lint';
import { Prec } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';
import { EditorManager } from '../src/editor/editorManager';
import { registerFormatter } from '../src/editor/formatters';
import { canReformat, formatBuffer, formatsOnSave } from '../src/editor/saveFlow';
import { jsonFormatter, yamlFormatter } from '../src/format/install';
import type { FileMeta, OpenedFile, Settings } from '../src/lib/api';

registerFormatter(jsonFormatter);
registerFormatter(yamlFormatter);

async function open(path: string, content: string, extra: Partial<FileMeta> = {}) {
  const m = new EditorManager({
    io: {
      read: async (p) => ({ meta: { path: p, size: 0, modTime: '', binary: false, eol: '\n', bom: false, finalNewline: true, mode: 'normal', ...extra }, content }) as OpenedFile,
      write: async (_p, _c, mt) => mt,
    },
    confirmClose: async () => 'cancel',
    onError: () => {},
  });
  document.body.appendChild(m.activePane.peek().el);
  m.setPrefs({ fontFamily: 'monospace', fontSize: 13, tabSize: 2, softWrap: false });
  const buf = (await m.open(path, { focus: false }))!;
  const said: string[] = [];
  return { m, buf, view: m.activeView()!, said, say: (t: string) => said.push(t) };
}

const diags = (v: EditorView): Diagnostic[] => {
  const out: Diagnostic[] = [];
  forEachDiagnostic(v.state, (d) => out.push(d));
  return out;
};

describe('formatBuffer', () => {
  it('formats JSON as one undoable change (AC5, AC8)', async () => {
    const src = '{"a":1,"b":[1,2,{"c":12345678901234567890}],"d":{}}';
    const { buf, view, said, say } = await open('/p/x.json', src);
    await formatBuffer(buf, say);
    expect(view.state.doc.toString()).toBe(
      '{\n  "a": 1,\n  "b": [\n    1,\n    2,\n    {\n      "c": 12345678901234567890\n    }\n  ],\n  "d": {}\n}',
    );
    expect(said).toEqual([]);
    expect(undo(view)).toBe(true);
    expect(view.state.doc.toString()).toBe(src);
  });

  it('uses the file indent unit', async () => {
    const { m, buf, view, say } = await open('/p/x.jsonc', '{"a":[1]}');
    m.addExtension(() => Prec.high(indentUnit.of('\t')));
    await formatBuffer(buf, say);
    expect(view.state.doc.toString()).toBe('{\n\t"a": [\n\t\t1\n\t]\n}');
    const y = await open('/p/x.yaml', 'a:\n - b\n');
    y.m.addExtension(() => Prec.high(indentUnit.of('\t')));
    await formatBuffer(y.buf, y.say);
    expect(y.view.state.doc.toString()).toBe('a:\n  - b\n'); // tabs → tabSize spaces
  });

  it('leaves the document unchanged on a syntax error, with a diagnostic (AC6)', async () => {
    const { buf, view, said, say } = await open('/p/x.json', '{"a": }');
    await formatBuffer(buf, say);
    expect(view.state.doc.toString()).toBe('{"a": }');
    expect(said).toEqual(['Not formatted: syntax error at 1:7']);
    expect(diagnosticCount(view.state)).toBe(1);
    expect(diags(view)[0]).toMatchObject({ from: 6, source: 'json', severity: 'error' });
    // Fixing it and formatting again clears the diagnostic.
    view.dispatch({ changes: { from: 6, insert: '1' } });
    await formatBuffer(buf, say);
    expect(diagnosticCount(view.state)).toBe(0);
  });

  it('reformats YAML as one undoable change (AC8, AC13)', async () => {
    const src = 'a:\n    enabled: yes\n    version: 1.10\n    list:\n    - x\n';
    const { buf, view, say } = await open('/p/c.yml', src);
    await formatBuffer(buf, say);
    expect(view.state.doc.toString()).toBe('a:\n  enabled: yes\n  version: 1.10\n  list:\n    - x\n');
    undo(view);
    expect(view.state.doc.toString()).toBe(src);
  });

  it('reports invalid YAML (AC14)', async () => {
    const { buf, view, said, say } = await open('/p/c.yaml', 'a: [1, 2\n');
    await formatBuffer(buf, say);
    expect(view.state.doc.toString()).toBe('a: [1, 2\n');
    expect(said[0]).toMatch(/^Not formatted: syntax error at \d+:\d+$/);
    expect(diags(view)[0]?.source).toBe('yaml');
  });

  it('does nothing for read-only buffers or files without a formatter', async () => {
    const ro = await open('/p/x.json', '{"a":1}', { mode: 'readOnly' });
    await formatBuffer(ro.buf, ro.say);
    expect(ro.view.state.doc.toString()).toBe('{"a":1}');
    const txt = await open('/p/x.txt', '{"a":1}');
    await formatBuffer(txt.buf, txt.say);
    expect(txt.view.state.doc.toString()).toBe('{"a":1}');
  });
});

describe('format on save and Reformat File', () => {
  const s = (json: boolean, yaml: boolean, format = 'goimports') =>
    ({ save: { format }, json: { formatOnSave: json }, yaml: { formatOnSave: yaml } }) as unknown as Settings;

  it('follows the per-language settings', () => {
    expect(formatsOnSave('/p/a.json', s(false, false))).toBe(false);
    expect(formatsOnSave('/p/a.json', s(true, false))).toBe(true);
    expect(formatsOnSave('/p/a.yaml', s(true, false))).toBe(false);
    expect(formatsOnSave('/p/a.yml', s(false, true))).toBe(true);
    expect(formatsOnSave('/p/a.go', s(false, false))).toBe(true);
    expect(formatsOnSave('/p/a.go', s(false, false, 'none'))).toBe(false);
    expect(formatsOnSave('/p/a.txt', s(true, true))).toBe(false);
  });

  it('offers Reformat File for Go, JSON and YAML', () => {
    expect(['/p/a.go', '/p/a.json', '/p/.vscode/x.code-workspace', '/p/a.yml', '/p/README.md'].map(canReformat)).toEqual([true, true, true, true, false]);
  });
});
