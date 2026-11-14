// YAML reformatter on the `yaml` library (loaded on first use). The failsafe
// schema reads every scalar as a string, so values are printed exactly as
// typed (no yes→true, 1.10 stays 1.10). Folded block scalars keep their
// original line breaks. A round-trip check refuses any output whose values,
// comments, tags or anchors differ from the input.

import type { CST, Document, ParseOptions, DocumentOptions, SchemaOptions } from 'yaml';
import type { FormatOutcome } from '../editor/formatters';

type YamlModule = typeof import('yaml');

let mod: Promise<YamlModule> | null = null;
const load = (): Promise<YamlModule> => (mod ??= import('yaml'));

const PARSE: ParseOptions & DocumentOptions & SchemaOptions = {
  schema: 'failsafe',
  strict: false,
  uniqueKeys: false,
  prettyErrors: true,
  keepSourceTokens: true,
};

interface Folded {
  placeholder: string;
  lines: string[];
}

/** Reformats YAML with indentSize spaces per level. mangle (tests only)
 * alters the output before the round-trip check. */
export async function formatYAML(text: string, indentSize: number, mangle?: (out: string) => string): Promise<FormatOutcome> {
  if (!text.trim()) return { ok: true, text };
  const Y = await load();
  const docs = Y.parseAllDocuments(text, PARSE);
  if (!Array.isArray(docs)) return { ok: true, text }; // empty stream
  for (const d of docs) {
    const e = d.errors[0];
    if (e) {
      const pos = e.linePos?.[0] ?? { line: 1, col: 1 };
      return { ok: false, line: pos.line, col: pos.col, message: e.message.split('\n')[0].replace(/ at line \d+, column \d+:?$/, '') };
    }
  }
  const before = docs.map((d) => fingerprint(Y, d));

  const folded: Folded[] = [];
  const tag = `__goi_folded_${Math.random().toString(36).slice(2)}_`;
  for (const d of docs) {
    Y.visit(d, {
      Scalar(_, n) {
        if (n.type !== 'BLOCK_FOLDED' || typeof n.value !== 'string') return;
        const src = n.srcToken as CST.BlockScalar | undefined;
        if (src?.type !== 'block-scalar') return;
        const header = (src.props.find((p) => p.type === 'block-scalar-header') as CST.SourceToken | undefined)?.source ?? '';
        if (/\d/.test(header)) return; // explicit indentation indicator: keep the library's output
        const lines = src.source.replace(/\n+$/, '').split('\n');
        const min = Math.min(...lines.filter((l) => l.trim()).map((l) => l.length - l.trimStart().length));
        const placeholder = tag + folded.length;
        folded.push({ placeholder, lines: lines.map((l) => (l.trim() ? l.slice(min) : '')) });
        n.value = placeholder + /\n*$/.exec(n.value)![0];
      },
    });
  }

  let out = docs
    .map((d) => d.toString({ indent: Math.max(1, indentSize), indentSeq: true, lineWidth: 0, minContentWidth: 0, flowCollectionPadding: false }))
    .join('');
  for (const f of folded) {
    out = out.replace(new RegExp(`^([ ]*)${f.placeholder}$`, 'm'), (_, ind: string) => f.lines.map((l) => (l ? ind + l : '')).join('\n'));
  }
  if (!text.endsWith('\n')) out = out.replace(/\n$/, '');
  if (mangle) out = mangle(out);

  // Round-trip guard: the output must mean the same thing as the input.
  const again = Y.parseAllDocuments(out, PARSE);
  const after = Array.isArray(again) && again.every((d) => !d.errors.length) ? again.map((d) => fingerprint(Y, d)) : null;
  if (!after || JSON.stringify(after) !== JSON.stringify(before)) {
    return { ok: false, line: 1, col: 1, message: 'formatting would change the document' };
  }
  return { ok: true, text: out };
}

/** Values, comments, tags and anchors of a document, order-insensitive for
 * comments (the printer may attach one to a neighbouring node). */
function fingerprint(Y: YamlModule, d: Document): unknown {
  const comments: string[] = [];
  const tags: string[] = [];
  const anchors: string[] = [];
  const add = (c: string | null | undefined): void => {
    if (c) for (const l of c.split('\n')) if (l.trim()) comments.push(l.trim());
  };
  add(d.commentBefore);
  add(d.comment);
  Y.visit(d, (_, n) => {
    if (Y.isNode(n)) {
      add(n.commentBefore);
      add(n.comment);
      if (n.tag) tags.push(n.tag);
      if ('anchor' in n && n.anchor) anchors.push(n.anchor);
    }
  });
  let value: unknown;
  try {
    value = d.toJS({ maxAliasCount: -1 });
  } catch {
    value = String(d.contents);
  }
  return { value, comments: comments.sort(), tags: tags.sort(), anchors: anchors.sort() };
}
