// Code completion from gopls (R3). Items keep gopls's ranking; typing more
// filters locally unless gopls reports an incomplete list. Snippets become
// CodeMirror tab-stop snippets; items that also add imports are inserted in
// one transaction so the whole completion is a single undo step.

import { autocompletion, snippet, startCompletion } from '@codemirror/autocomplete';
import type { Completion, CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import type { Extension } from '@codemirror/state';
import { EditorView, ViewPlugin } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { lang } from '../lib/api';
import type { CompletionItem } from '../lib/api';
import { client } from './context';
import { renderMarkdown } from './markdown';
import { editsToChanges, toOffset, toPos } from './positions';

const KIND: Record<number, string> = {
  1: 'text',
  2: 'method',
  3: 'function',
  4: 'function',
  5: 'property',
  6: 'variable',
  7: 'class',
  8: 'interface',
  9: 'namespace',
  10: 'property',
  12: 'constant',
  13: 'enum',
  14: 'keyword',
  15: 'text',
  20: 'constant',
  21: 'constant',
  22: 'class',
  25: 'type',
};

export function kindType(kind: number | undefined): string | undefined {
  return kind === undefined ? undefined : KIND[kind];
}

interface Field {
  index: number;
  text: string;
  /** Offset in the plain text. */
  at: number;
}

/** Parses an LSP snippet into plain text plus fields. Nested placeholders
 * flatten to their text; choices take the first option; variables are
 * dropped. $0 marks the final cursor. */
export function parseSnippet(src: string): { text: string; fields: Field[]; final: number | null } {
  let text = '';
  const fields: Field[] = [];
  let final: number | null = null;
  let i = 0;

  const readPlaceholderBody = (): string => {
    // after "${N:" until the matching "}"; nested ${…} contribute their text
    let out = '';
    let depth = 0;
    while (i < src.length) {
      const c = src[i];
      if (c === '\\' && i + 1 < src.length) {
        out += src[i + 1];
        i += 2;
        continue;
      }
      if (c === '$' && src[i + 1] === '{') {
        depth++;
        i += 2;
        while (i < src.length && /[\d]/.test(src[i])) i++;
        if (src[i] === ':') i++;
        continue;
      }
      if (c === '}') {
        i++;
        if (depth === 0) return out;
        depth--;
        continue;
      }
      out += c;
      i++;
    }
    return out;
  };

  while (i < src.length) {
    const c = src[i];
    if (c === '\\' && i + 1 < src.length) {
      text += src[i + 1];
      i += 2;
      continue;
    }
    if (c === '$') {
      const simple = /^\$(\d+)/.exec(src.slice(i));
      if (simple) {
        const n = Number(simple[1]);
        if (n === 0) final = text.length;
        else fields.push({ index: n, text: '', at: text.length });
        i += simple[0].length;
        continue;
      }
      const braced = /^\$\{(\d+)(:|\||\})/.exec(src.slice(i));
      if (braced) {
        const n = Number(braced[1]);
        i += braced[0].length;
        let body = '';
        if (braced[2] === ':') body = readPlaceholderBody();
        else if (braced[2] === '|') {
          const end = src.indexOf('|}', i);
          body = (end >= 0 ? src.slice(i, end) : '').split(',')[0] ?? '';
          i = end >= 0 ? end + 2 : src.length;
        }
        if (n === 0) final = text.length;
        else fields.push({ index: n, text: body, at: text.length });
        text += body;
        continue;
      }
      const variable = /^\$\{?[A-Za-z_]\w*\}?/.exec(src.slice(i));
      if (variable) {
        i += variable[0].length;
        continue;
      }
    }
    text += c;
    i++;
  }
  return { text, fields, final };
}

/** Converts an LSP snippet to a CodeMirror snippet template, or null when
 * it can't be expressed (field text containing braces). */
export function toCmTemplate(src: string): string | null {
  const { text, fields, final } = parseSnippet(src);
  if (fields.some((f) => /[{}]/.test(f.text))) return null;
  const sorted = [...fields].sort((a, b) => a.at - b.at || a.index - b.index);
  let out = '';
  let last = 0;
  for (const f of sorted) {
    out += escapeTemplate(text.slice(last, f.at)) + `\${${f.index}:${f.text}}`;
    last = f.at + f.text.length;
  }
  out += escapeTemplate(text.slice(last));
  if (final !== null && final < text.length && sorted.length === 0) {
    // only a final cursor: express it as an empty field
    return escapeTemplate(text.slice(0, final)) + '${}' + escapeTemplate(text.slice(final));
  }
  return out;
}

function escapeTemplate(s: string): string {
  // "${" and "#{" start fields in CodeMirror templates.
  return s.replace(/([$#])\{/g, '$1\\{');
}

/** Plain insertion for a snippet: text and the cursor offset within it. */
export function snippetToPlain(src: string): { text: string; cursor: number } {
  const { text, fields, final } = parseSnippet(src);
  const first = [...fields].sort((a, b) => a.index - b.index)[0];
  if (first) return { text, cursor: first.at + first.text.length };
  return { text, cursor: final ?? text.length };
}

/** CodeMirror boost spreading gopls's order over [-99, 99]. */
export function boostFor(index: number, count: number): number {
  if (count <= 1) return 99;
  return Math.round(99 - (index * 198) / (count - 1));
}

function applyItem(view: EditorView, it: CompletionItem, from: number, to: number): void {
  const doc = view.state.doc;
  let start = from;
  let end = to;
  if (it.textEdit) {
    start = toOffset(doc, it.textEdit.range.start);
    end = Math.max(to, toOffset(doc, it.textEdit.range.end));
  }
  const raw = it.textEdit?.newText ?? it.insertText ?? it.label;
  const isSnippet = it.insertTextFormat === 2;
  const extra = it.additionalTextEdits ?? [];

  if (isSnippet && extra.length === 0) {
    const tmpl = toCmTemplate(raw);
    if (tmpl !== null) {
      snippet(tmpl)(view, { label: it.label }, start, end);
      return;
    }
  }
  const { text, cursor } = isSnippet ? snippetToPlain(raw) : { text: raw, cursor: raw.length };
  let changes;
  try {
    changes = [...editsToChanges(doc, extra), { from: start, to: end, insert: text }];
  } catch {
    changes = [{ from: start, to: end, insert: text }];
  }
  const set = view.state.changes(changes);
  const head = set.mapPos(start, -1) + cursor;
  view.dispatch({ changes: set, selection: { anchor: head }, userEvent: 'input.complete', scrollIntoView: true });
}

export function toOptions(items: readonly CompletionItem[]): Completion[] {
  return items.map((it, i) => ({
    label: it.label,
    detail: it.detail || undefined,
    type: kindType(it.kind),
    boost: boostFor(i, items.length),
    info: it.documentation ? () => renderMarkdown(it.documentation ?? '') : undefined,
    apply: (view: EditorView, _c: Completion, from: number, to: number) => applyItem(view, it, from, to),
    ...(it.deprecated || it.tags?.includes(1) ? { class: 'cm-completion-deprecated' } : {}),
  }));
}

function source(buf: Buffer) {
  return async (ctx: CompletionContext): Promise<CompletionResult | null> => {
    if (!client.live() || !client.tracked(buf.path)) return null;
    const word = ctx.matchBefore(/[\p{L}\p{N}_]*$/u);
    const before = ctx.state.sliceDoc(Math.max(0, ctx.pos - 1), ctx.pos);
    if (!ctx.explicit && !word?.text && before !== '.') return null;
    await client.flush(buf);
    if (ctx.aborted) return null;
    let res;
    try {
      res = await lang.completion(buf.path, toPos(ctx.state.doc, ctx.pos));
    } catch {
      return null;
    }
    if (ctx.aborted || res.items.length === 0) return null;
    const first = res.items.find((i) => i.textEdit)?.textEdit;
    const from = first ? toOffset(ctx.state.doc, first.range.start) : (word?.from ?? ctx.pos);
    return {
      from: Math.min(from, ctx.pos),
      options: toOptions(res.items),
      validFor: res.isIncomplete ? undefined : /^[\p{L}\p{N}_]*$/u,
    };
  };
}

/** Opens completion while typing identifiers or after ".". */
const typingTrigger = ViewPlugin.define((view) => ({
  update(u) {
    if (!u.docChanged) return;
    for (const tr of u.transactions) {
      if (!tr.isUserEvent('input.type')) continue;
      let last = '';
      tr.changes.iterChanges((_fa, _ta, _fb, _tb, ins) => {
        if (ins.length) last = ins.sliceString(ins.length - 1);
      });
      if (/[\p{L}\p{N}_.]/u.test(last)) {
        queueMicrotask(() => startCompletion(view));
        return;
      }
    }
  },
}));

export function completionExtension(buf: Buffer): Extension {
  return [
    autocompletion({ override: [source(buf)], icons: true, maxRenderedOptions: 200 }),
    typingTrigger,
    EditorView.theme({ '.cm-completion-deprecated .cm-completionLabel': { textDecoration: 'line-through' } }),
  ];
}
