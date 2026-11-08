// @vitest-environment happy-dom
import { Text } from '@codemirror/state';
import { undo } from '@codemirror/commands';
import { describe, expect, it, vi } from 'vitest';
import type { FileEdit, FileMeta, OpenedFile } from '../src/lib/api';
import { EditorManager } from '../src/editor/editorManager';
import { Buffer } from '../src/editor/buffer';
import { applyEditsToString, editsToChanges, toOffset, toPos, wordAt } from '../src/lang/positions';
import { parseBlocks, renderMarkdown } from '../src/lang/markdown';
import { isLangBuffer, isLangPath, LangClient, SYNC_DEBOUNCE_MS } from '../src/lang/client';
import type { SyncApi } from '../src/lang/client';
import { applyFileEdits, summarize } from '../src/lang/edits';
import { fuzzyMatch } from '../src/lang/popup';
import { statusLabel } from '../src/lang/status';

function meta(path: string, extra: Partial<FileMeta> = {}): FileMeta {
  return { path, size: 0, modTime: '', binary: false, eol: '\n', bom: false, finalNewline: true, mode: 'normal', ...extra } as FileMeta;
}

const edit = (l1: number, c1: number, l2: number, c2: number, newText: string) => ({
  range: { start: { line: l1, character: c1 }, end: { line: l2, character: c2 } },
  newText,
});

describe('positions', () => {
  const doc = Text.of(['ab', 'c😀d', '']);
  it('converts both ways in UTF-16 units', () => {
    expect(toPos(doc, 0)).toEqual({ line: 0, character: 0 });
    expect(toPos(doc, 3)).toEqual({ line: 1, character: 0 });
    const afterEmoji = doc.line(2).from + 3;
    expect(toPos(doc, afterEmoji)).toEqual({ line: 1, character: 3 });
    expect(toOffset(doc, { line: 1, character: 3 })).toBe(afterEmoji);
  });
  it('clamps past-the-end positions', () => {
    expect(toOffset(doc, { line: 0, character: 99 })).toBe(2);
    expect(toOffset(doc, { line: 9, character: 0 })).toBe(doc.length);
    expect(toOffset(doc, { line: -1, character: 0 })).toBe(0);
  });
  it('turns edits into changes and rejects overlaps', () => {
    const d = Text.of(['package p', '', 'func f() {}']);
    expect(applyEditsToString(d, [edit(2, 5, 2, 6, 'g'), edit(1, 0, 1, 0, 'import "fmt"')])).toBe('package p\nimport "fmt"\nfunc g() {}');
    expect(() => editsToChanges(d, [edit(0, 0, 0, 4, 'x'), edit(0, 2, 0, 5, 'y')])).toThrow(/overlapping/);
  });
  it('finds the word at an offset', () => {
    const d = Text.of(['x := fooBar(1)']);
    expect(wordAt(d, 7)?.text).toBe('fooBar');
    expect(wordAt(d, 4)).toBeNull();
  });
});

describe('markdown', () => {
  it('parses blocks', () => {
    const blocks = parseBlocks('```go\nfunc F()\n```\n\nPara one\ncontinues.\n\n- a\n- b\n\n# Head');
    expect(blocks.map((b) => b.kind)).toEqual(['code', 'para', 'list', 'heading']);
    expect(blocks[1]).toEqual({ kind: 'para', text: 'Para one continues.' });
  });
  it('renders inline markup without HTML injection', () => {
    const el = renderMarkdown('Use `x` and **bold** and *em* see [doc](https://pkg.go.dev/fmt) \\*not em\\* <script>alert(1)</script>');
    expect(el.querySelector('code')?.textContent).toBe('x');
    expect(el.querySelector('strong')?.textContent).toBe('bold');
    expect(el.querySelector('em')?.textContent).toBe('em');
    expect(el.querySelector('a')?.getAttribute('title')).toBe('https://pkg.go.dev/fmt');
    expect(el.querySelector('script')).toBeNull();
    expect(el.textContent).toContain('*not em*');
    expect(el.textContent).toContain('<script>');
  });
  it('keeps snake_case identifiers intact', () => {
    expect(renderMarkdown('call my_func_name now').querySelector('em')).toBeNull();
  });
});

describe('LangClient', () => {
  function fakeApi() {
    const calls: string[] = [];
    const api: SyncApi = {
      didOpen: async (p, v, t) => void calls.push(`open ${p} ${v} ${t}`),
      didChange: async (p, v, t) => void calls.push(`change ${p} ${v} ${t}`),
      didSave: async (p) => void calls.push(`save ${p}`),
      didClose: async (p) => void calls.push(`close ${p}`),
    };
    return { api, calls };
  }
  function buf(path: string, text: string, mode = 'normal'): Buffer {
    return new Buffer(path, meta(path, { mode: mode as FileMeta['mode'] }), text);
  }

  it('recognizes Go-family buffers', () => {
    expect(isLangPath('/p/a.go')).toBe(true);
    expect(isLangPath('/p/go.mod')).toBe(true);
    expect(isLangPath('/p/go.sum')).toBe(false);
    expect(isLangPath('/p/a.md')).toBe(false);
    expect(isLangBuffer(buf('/p/big.go', '', 'noHighlight'))).toBe(false);
  });

  it('opens, debounces changes, flushes, saves and closes', async () => {
    vi.useFakeTimers();
    const { api, calls } = fakeApi();
    const c = new LangClient(api);
    const b = buf('/p/a.go', 'x');
    c.open(b);
    c.open(b);
    expect(calls).toEqual(['open /p/a.go 0 x']);
    b.version = 1;
    c.changed(b);
    b.version = 2;
    c.changed(b);
    expect(calls.length).toBe(1);
    await vi.advanceTimersByTimeAsync(SYNC_DEBOUNCE_MS + 1);
    expect(calls[1]).toBe('change /p/a.go 2 x');
    b.version = 3;
    c.changed(b);
    expect(await c.flush(b)).toBe(3);
    expect(calls[2]).toBe('change /p/a.go 3 x');
    await vi.advanceTimersByTimeAsync(SYNC_DEBOUNCE_MS + 1);
    expect(calls.length).toBe(3);
    c.saved(b);
    await vi.runAllTimersAsync();
    expect(calls[3]).toBe('save /p/a.go');
    c.close(b);
    expect(calls[4]).toBe('close /p/a.go');
    c.open(buf('/p/readme.md', ''));
    expect(calls.length).toBe(5);
    vi.useRealTimers();
  });

  it('re-sends open buffers when gopls becomes live', () => {
    const { api, calls } = fakeApi();
    const c = new LangClient(api);
    c.open(buf('/p/a.go', 'x'));
    c.setStatus({ state: 'missing', message: '', bin: '' });
    expect(c.live()).toBe(false);
    c.setStatus({ state: 'ready', message: '', bin: '' });
    expect(c.live()).toBe(true);
    expect(calls.filter((x) => x.startsWith('open')).length).toBe(2);
    c.setStatus({ state: 'indexing', message: '', bin: '' });
    expect(calls.filter((x) => x.startsWith('open')).length).toBe(2);
  });
});

describe('applyFileEdits', () => {
  it('edits open buffers undoably and sends the rest to disk', async () => {
    const m = new EditorManager({
      io: {
        read: async (p) => ({ meta: meta(p), content: 'package a\n\nfunc f() {}\n' }) as OpenedFile,
        write: async (_p, _c, mt) => mt,
      },
      confirmClose: async () => 'discard',
      onError: () => {},
    });
    await m.open('/p/a.go', { focus: false });
    const disk = vi.fn(async (_e: FileEdit[]) => {});
    const res = await applyFileEdits(
      [
        { path: '/p/a.go', edits: [edit(2, 5, 2, 6, 'g')] },
        { path: '/p/b.go', edits: [edit(0, 0, 0, 0, '// x\n')] },
        { path: '/p/c.go', edits: [] },
      ],
      m,
      disk,
    );
    const buf = m.buffers.get('/p/a.go')!;
    expect(buf.text()).toBe('package a\n\nfunc g() {}\n');
    expect(buf.dirty()).toBe(true);
    expect(disk).toHaveBeenCalledWith([{ path: '/p/b.go', edits: [edit(0, 0, 0, 0, '// x\n')] }]);
    expect(res).toEqual({ files: 2, edits: 2, onDisk: ['/p/b.go'] });
    undo(m.activeView()!);
    expect(buf.text()).toBe('package a\n\nfunc f() {}\n');
  });

  it('summarizes', () => {
    expect(summarize([{ path: 'a', edits: [edit(0, 0, 0, 0, 'x'), edit(1, 0, 1, 0, 'y')] }, { path: 'b', edits: [] }])).toEqual({ files: 1, edits: 2 });
  });
});

describe('popup + status helpers', () => {
  it('fuzzy matches subsequences', () => {
    expect(fuzzyMatch('ServeHTTP', 'shttp')).toBe(true);
    expect(fuzzyMatch('ServeHTTP', 'xyz')).toBe(false);
    expect(fuzzyMatch('anything', '')).toBe(true);
  });
  it('labels states', () => {
    expect(statusLabel({ state: 'ready', message: '', bin: '' })).toBe('gopls');
    expect(statusLabel({ state: 'missing', message: '', bin: '' })).toContain('not installed');
  });
});
