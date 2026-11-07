// @vitest-environment happy-dom
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { history, undo } from '@codemirror/commands';
import { describe, expect, it } from 'vitest';
import type { Completion } from '@codemirror/autocomplete';
import { boostFor, kindType, parseSnippet, snippetToPlain, toCmTemplate, toOptions } from '../src/lang/completion';
import type { CompletionItem } from '../src/lib/api';

const r = (l1: number, c1: number, l2: number, c2: number) => ({ start: { line: l1, character: c1 }, end: { line: l2, character: c2 } });

describe('snippets', () => {
  it('parses fields, escapes, nesting, choices and $0', () => {
    expect(parseSnippet('Println(${1:a ...any})')).toEqual({ text: 'Println(a ...any)', fields: [{ index: 1, text: 'a ...any', at: 8 }], final: null });
    expect(parseSnippet('f($1, $2)$0').fields.map((f) => f.at)).toEqual([2, 4]);
    expect(parseSnippet('f($1, $2)$0').final).toBe(5);
    expect(parseSnippet('${1:outer ${2:inner}}').text).toBe('outer inner');
    expect(parseSnippet('${1|one,two|}').text).toBe('one');
    expect(parseSnippet('a\\$b\\}c').text).toBe('a$b}c');
    expect(parseSnippet('x${TM_SELECTED_TEXT}y').text).toBe('xy');
  });
  it('converts to CodeMirror templates', () => {
    expect(toCmTemplate('Println(${1:a ...any})')).toBe('Println(${1:a ...any})');
    expect(toCmTemplate('f($1, $2)')).toBe('f(${1:}, ${2:})');
    expect(toCmTemplate('T{$0}')).toBe('T{${}}');
    expect(toCmTemplate('${1:m map[string]struct\\{\\}}')).toBeNull();
    expect(toCmTemplate('s := "${x}"')).toBe('s := ""'); // variables are dropped
    expect(toCmTemplate('a${1:b}#{c')).toBe('a${1:b}#\\{c');
  });
  it('gives a plain fallback with a cursor', () => {
    expect(snippetToPlain('f(${1:x}, ${2:y})')).toEqual({ text: 'f(x, y)', cursor: 3 });
    expect(snippetToPlain('T{$0}')).toEqual({ text: 'T{}', cursor: 2 });
    expect(snippetToPlain('plain')).toEqual({ text: 'plain', cursor: 5 });
  });
});

describe('ranking and kinds', () => {
  it('spreads boosts over gopls order', () => {
    expect(boostFor(0, 5)).toBe(99);
    expect(boostFor(4, 5)).toBe(-99);
    expect(boostFor(0, 1)).toBe(99);
    expect(boostFor(1, 3)).toBe(0);
  });
  it('maps kinds', () => {
    expect(kindType(3)).toBe('function');
    expect(kindType(9)).toBe('namespace');
    expect(kindType(undefined)).toBeUndefined();
  });
});

describe('applying items', () => {
  function view(doc: string, cursor: number) {
    return new EditorView({ state: EditorState.create({ doc, selection: { anchor: cursor }, extensions: [history()] }) });
  }
  function apply(v: EditorView, item: CompletionItem, from: number, to: number) {
    const opt = toOptions([item])[0] as Completion & { apply: (v: EditorView, c: Completion, f: number, t: number) => void };
    opt.apply(v, opt, from, to);
  }

  it('adds the import and the text in one undo step', () => {
    const doc = 'package main\n\nfunc main() {\n\tstr\n}\n';
    const v = view(doc, doc.indexOf('str') + 3);
    apply(
      v,
      {
        label: 'strings',
        kind: 9,
        textEdit: { range: r(3, 1, 3, 4), newText: 'strings' },
        additionalTextEdits: [{ range: r(1, 0, 1, 0), newText: 'import "strings"\n' }],
      },
      doc.indexOf('str'),
      doc.indexOf('str') + 3,
    );
    expect(v.state.doc.toString()).toBe('package main\nimport "strings"\n\nfunc main() {\n\tstrings\n}\n');
    expect(v.state.selection.main.head).toBe(v.state.doc.toString().indexOf('strings\n}') + 7);
    undo(v);
    expect(v.state.doc.toString()).toBe(doc);
    v.destroy();
  });

  it('inserts snippets with the cursor in the first field', () => {
    const doc = 'fmt.Pr';
    const v = view(doc, 6);
    apply(v, { label: 'Println', insertTextFormat: 2, textEdit: { range: r(0, 4, 0, 6), newText: 'Println(${1:a ...any})' } }, 4, 6);
    expect(v.state.doc.toString()).toBe('fmt.Println(a ...any)');
    const sel = v.state.selection.main;
    expect(v.state.sliceDoc(sel.from, sel.to)).toBe('a ...any');
    v.destroy();
  });

  it('extends the replaced range to what the user typed since the request', () => {
    const doc = 'fmt.Prin';
    const v = view(doc, 8);
    apply(v, { label: 'Println', textEdit: { range: r(0, 4, 0, 6), newText: 'Println' } }, 4, 8);
    expect(v.state.doc.toString()).toBe('fmt.Println');
    v.destroy();
  });

  it('marks deprecated items and attaches docs lazily', () => {
    const [opt] = toOptions([{ label: 'Old', deprecated: true, documentation: 'Deprecated: use New.' }]);
    expect((opt as Completion & { class?: string }).class).toBe('cm-completion-deprecated');
    expect(typeof opt.info).toBe('function');
  });
});
