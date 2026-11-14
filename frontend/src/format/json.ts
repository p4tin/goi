// JSON pretty-printer that keeps every token as written: numbers, string
// escapes and key order are untouched, comments (// and /* */) and trailing
// commas are kept. One grammar serves .json, .jsonc and .code-workspace.

import { lineCol } from '../editor/formatters';
import type { FormatOutcome } from '../editor/formatters';

type TokType = '{' | '}' | '[' | ']' | ':' | ',' | 'str' | 'num' | 'lit' | 'lc' | 'bc';

interface Tok {
  type: TokType;
  text: string;
  start: number;
  /** Newlines between the previous token and this one. */
  nl: number;
}

class SyntaxErr extends Error {
  constructor(
    readonly offset: number,
    msg: string,
  ) {
    super(msg);
  }
}

const NUMBER = /-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/y;
const LITERAL = /true|false|null/y;
const HEX4 = /^[0-9a-fA-F]{4}$/;

function lex(text: string): Tok[] {
  const toks: Tok[] = [];
  let i = 0;
  let nl = 0;
  const n = text.length;
  while (i < n) {
    const c = text[i];
    if (c === '\n') {
      nl++;
      i++;
      continue;
    }
    if (c === ' ' || c === '\t' || c === '\r' || c === '﻿') {
      i++;
      continue;
    }
    const start = i;
    if ('{}[]:,'.includes(c)) {
      toks.push({ type: c as TokType, text: c, start, nl });
      i++;
    } else if (c === '"') {
      i++;
      for (;;) {
        if (i >= n || text[i] === '\n') throw new SyntaxErr(start, 'unterminated string');
        const ch = text[i];
        if (ch === '"') break;
        if (ch === '\\') {
          const e = text[i + 1];
          if (e === 'u') {
            if (!HEX4.test(text.slice(i + 2, i + 6))) throw new SyntaxErr(i, 'invalid \\u escape');
            i += 6;
          } else if (e !== undefined && '"\\/bfnrt'.includes(e)) {
            i += 2;
          } else {
            throw new SyntaxErr(i, 'invalid escape');
          }
          continue;
        }
        if (ch.charCodeAt(0) < 0x20) throw new SyntaxErr(i, 'control character in string');
        i++;
      }
      i++;
      toks.push({ type: 'str', text: text.slice(start, i), start, nl });
    } else if (c === '/' && text[i + 1] === '/') {
      let end = text.indexOf('\n', i);
      if (end < 0) end = n;
      toks.push({ type: 'lc', text: text.slice(start, end).trimEnd(), start, nl });
      i = end;
    } else if (c === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2);
      if (end < 0) throw new SyntaxErr(start, 'unterminated comment');
      i = end + 2;
      toks.push({ type: 'bc', text: text.slice(start, i), start, nl });
    } else {
      NUMBER.lastIndex = i;
      LITERAL.lastIndex = i;
      const m = NUMBER.exec(text) ?? LITERAL.exec(text);
      const after = m ? text[i + m[0].length] : undefined;
      if (!m || (after !== undefined && /[\w.+-]/.test(after))) throw new SyntaxErr(start, `unexpected '${c}'`);
      toks.push({ type: /[tfn]/.test(c) ? 'lit' : 'num', text: m[0], start, nl });
      i += m[0].length;
    }
    nl = 0;
  }
  return toks;
}

type Expect = 'value' | 'key' | 'colon' | 'commaOrClose' | 'end';

/** Pretty-prints JSON with indent per level. */
export function formatJSON(text: string, indent: string): FormatOutcome {
  try {
    return { ok: true, text: format(text, indent) };
  } catch (err) {
    if (!(err instanceof SyntaxErr)) throw err;
    return { ok: false, ...lineCol(text, err.offset), message: err.message };
  }
}

function format(text: string, indent: string): string {
  const toks = lex(text);
  if (!toks.some((t) => t.type !== 'lc' && t.type !== 'bc')) return text;

  // Grammar: an explicit stack, so deep nesting can't overflow.
  const stack: ('{' | '[')[] = [];
  let expect: Expect = 'value';
  let closeOk = false;
  const afterValue = (): void => {
    expect = stack.length ? 'commaOrClose' : 'end';
    closeOk = false;
  };
  const unexpected = (t: Tok): never => {
    throw new SyntaxErr(t.start, `unexpected '${t.text.slice(0, 20)}'`);
  };

  // Printer: output lines; brk means the next token starts a new line; sep
  // joins the next token to a non-empty current line.
  const lines: string[] = [''];
  let brk = false;
  let sep = '';
  let afterOpen = false;
  const cur = (): string => lines[lines.length - 1];
  const place = (s: string, depth: number, nl: number): void => {
    if (brk && !(lines.length === 1 && lines[0] === '')) {
      if (nl >= 2 && !afterOpen && cur().trim() !== '') lines.push('');
      lines.push(indent.repeat(depth) + s);
    } else {
      lines[lines.length - 1] = cur() + (cur().trim() ? sep : '') + s;
    }
    brk = false;
    sep = '';
    afterOpen = false;
  };

  for (let i = 0; i < toks.length; i++) {
    const t = toks[i];
    const depth = stack.length;
    switch (t.type) {
      case 'lc':
      case 'bc': {
        const next = toks[i + 1];
        if (t.nl === 0 && cur().trim() !== '') {
          // Trailing comment: stays at the end of its line.
          lines[lines.length - 1] = cur() + ' ' + t.text;
          if (t.type === 'lc') brk = true;
          else if (!brk) sep = ' ';
        } else {
          brk = true;
          const open = afterOpen;
          place(t.text, depth, t.nl);
          afterOpen = open;
          if (t.type === 'lc' || !next || next.nl > 0) brk = true;
          else sep = ' ';
        }
        continue;
      }
      case '{':
      case '[': {
        if (expect !== 'value') unexpected(t);
        place(t.text, depth, t.nl);
        const close = t.type === '{' ? '}' : ']';
        if (toks[i + 1]?.type === close) {
          lines[lines.length - 1] = cur() + close;
          i++;
          afterValue();
        } else {
          stack.push(t.type);
          expect = t.type === '{' ? 'key' : 'value';
          closeOk = true;
          brk = true;
          afterOpen = true;
        }
        continue;
      }
      case '}':
      case ']': {
        if ((expect !== 'commaOrClose' && !closeOk) || stack[depth - 1] !== (t.type === '}' ? '{' : '[')) unexpected(t);
        stack.pop();
        brk = true;
        place(t.text, depth - 1, 0);
        afterValue();
        continue;
      }
      case 'str':
        if (expect === 'key') {
          expect = 'colon';
          closeOk = false;
        } else if (expect === 'value') afterValue();
        else unexpected(t);
        place(t.text, depth, t.nl);
        continue;
      case 'num':
      case 'lit':
        if (expect !== 'value') unexpected(t);
        afterValue();
        place(t.text, depth, t.nl);
        continue;
      case ':':
        if (expect !== 'colon') unexpected(t);
        expect = 'value';
        place(':', depth, 0);
        sep = ' ';
        continue;
      case ',':
        if (expect !== 'commaOrClose') unexpected(t);
        expect = stack[depth - 1] === '{' ? 'key' : 'value';
        closeOk = true;
        place(',', depth, 0);
        brk = true;
        continue;
    }
  }
  if (expect !== 'end') throw new SyntaxErr(text.trimEnd().length, 'unexpected end of input');
  const out = lines.join('\n');
  return text.endsWith('\n') ? out + '\n' : out;
}
