// Document formatters for non-Go files (JSON, YAML). Reformat File and
// format on save (saveFlow.ts) pick one by path.

import type { Settings } from '../lib/api';

export interface FormatOptions {
  /** The file's indent unit ("\t" or spaces). */
  indentUnit: string;
  tabSize: number;
  path: string;
}

/** A formatted text, or the first syntax error (1-based line and column). */
export type FormatOutcome = { ok: true; text: string } | { ok: false; line: number; col: number; message: string };

export interface DocFormatter {
  /** Also the diagnostic source of its syntax errors. */
  id: string;
  matches(path: string): boolean;
  /** Whether to format on save. */
  onSave(s: Settings): boolean;
  format(text: string, o: FormatOptions): Promise<FormatOutcome>;
}

const formatters: DocFormatter[] = [];

export function registerFormatter(f: DocFormatter): () => void {
  formatters.push(f);
  return () => {
    const i = formatters.indexOf(f);
    if (i >= 0) formatters.splice(i, 1);
  };
}

export function formatterFor(path: string): DocFormatter | null {
  return formatters.find((f) => f.matches(path)) ?? null;
}

/** 1-based line and column (UTF-16 units) of an offset. */
export function lineCol(text: string, offset: number): { line: number; col: number } {
  let line = 1;
  let start = 0;
  for (let i = text.indexOf('\n'); i >= 0 && i < offset; i = text.indexOf('\n', i + 1)) {
    line++;
    start = i + 1;
  }
  return { line, col: offset - start + 1 };
}
