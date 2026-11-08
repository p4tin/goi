// Conversions between CodeMirror offsets and LSP positions. Both count
// UTF-16 code units within a line, so no transcoding is needed.

import type { ChangeSpec, Text } from '@codemirror/state';
import type { Pos, Range, TextEdit } from '../lib/api';

export function toPos(doc: Text, offset: number): Pos {
  const line = doc.lineAt(Math.max(0, Math.min(offset, doc.length)));
  return { line: line.number - 1, character: offset - line.from };
}

/** Offset of p in doc; past-the-end positions clamp to the line or doc end. */
export function toOffset(doc: Text, p: Pos): number {
  if (p.line < 0) return 0;
  if (p.line >= doc.lines) return doc.length;
  const line = doc.line(p.line + 1);
  return Math.min(line.from + Math.max(0, p.character), line.to);
}

export function toOffsets(doc: Text, r: Range): { from: number; to: number } {
  const from = toOffset(doc, r.start);
  const to = toOffset(doc, r.end);
  return { from: Math.min(from, to), to: Math.max(from, to) };
}

export function toRange(doc: Text, from: number, to: number): Range {
  return { start: toPos(doc, from), end: toPos(doc, to) };
}

/** Converts LSP edits (relative to doc) into one CodeMirror change set
 * spec. Throws if two edits overlap. */
export function editsToChanges(doc: Text, edits: readonly TextEdit[]): ChangeSpec[] {
  const changes = edits
    .map((e, i) => ({ ...toOffsets(doc, e.range), insert: e.newText, i }))
    .sort((a, b) => a.from - b.from || a.i - b.i);
  for (let i = 1; i < changes.length; i++) {
    if (changes[i].from < changes[i - 1].to) throw new Error('overlapping edits');
  }
  return changes.map(({ from, to, insert }) => ({ from, to, insert }));
}

/** Applies edits to a string (for previews and tests). */
export function applyEditsToString(doc: Text, edits: readonly TextEdit[]): string {
  let out = '';
  let last = 0;
  const text = doc.toString();
  for (const c of editsToChanges(doc, edits) as { from: number; to: number; insert: string }[]) {
    out += text.slice(last, c.from) + c.insert;
    last = c.to;
  }
  return out + text.slice(last);
}

/** The identifier-ish word around offset (for keyword docs and rename
 * prompts), or null. */
export function wordAt(doc: Text, offset: number): { from: number; to: number; text: string } | null {
  const line = doc.lineAt(offset);
  const s = line.text;
  let a = offset - line.from;
  let b = a;
  const isWord = (c: string): boolean => /[\p{L}\p{N}_]/u.test(c);
  while (a > 0 && isWord(s[a - 1])) a--;
  while (b < s.length && isWord(s[b])) b++;
  if (a === b) return null;
  return { from: line.from + a, to: line.from + b, text: s.slice(a, b) };
}
