// Stream highlighter for Go text/html templates: only {{ … }} actions are
// styled; text outside actions is left plain.

import { StreamLanguage } from '@codemirror/language';
import type { StreamParser } from '@codemirror/language';

const KEYWORDS = new Set(['if', 'else', 'end', 'range', 'with', 'define', 'template', 'block', 'break', 'continue']);
const ATOMS = new Set(['nil', 'true', 'false']);

export interface GotmplState {
  inAction: boolean;
  inComment: boolean;
}

export const gotmplParser: StreamParser<GotmplState> = {
  name: 'gotmpl',
  startState: () => ({ inAction: false, inComment: false }),
  copyState: (s) => ({ ...s }),
  token(stream, state) {
    if (!state.inAction) {
      if (stream.match(/^\{\{-?/)) {
        state.inAction = true;
        return 'meta';
      }
      if (stream.skipTo('{{')) {
        if (stream.pos === stream.start) stream.next();
      } else {
        stream.skipToEnd();
      }
      return null;
    }

    if (state.inComment) {
      if (stream.skipTo('*/')) {
        stream.match('*/');
        state.inComment = false;
      } else {
        stream.skipToEnd();
      }
      return 'comment';
    }
    if (stream.eatSpace()) return null;
    if (stream.match(/^-?\}\}/)) {
      state.inAction = false;
      return 'meta';
    }
    if (stream.match('/*')) {
      state.inComment = true;
      if (stream.skipTo('*/')) {
        stream.match('*/');
        state.inComment = false;
      } else {
        stream.skipToEnd();
      }
      return 'comment';
    }
    const ch = stream.peek();
    if (ch === '"' || ch === '`' || ch === "'") {
      const q = stream.next();
      while (!stream.eol()) {
        const c = stream.next();
        if (q !== '`' && c === '\\') {
          stream.next();
          continue;
        }
        if (c === q) break;
      }
      return 'string';
    }
    if (stream.match(/^-?\d+(?:\.\d+)?/)) return 'number';
    if (stream.match(/^:?=/) || stream.match('|')) return 'operator';
    if (stream.match(/^\$[\w]*/)) return 'variableName';
    if (stream.match(/^\.[A-Za-z_]\w*/) || stream.match('.')) return 'propertyName';
    if (stream.match(/^[()]/)) return 'paren';
    if (stream.match(/^[A-Za-z_]\w*/)) {
      const w = stream.current();
      if (KEYWORDS.has(w)) return 'keyword';
      if (ATOMS.has(w)) return 'atom';
      return 'variableName.function';
    }
    stream.next();
    return null;
  },
  languageData: { commentTokens: { block: { open: '{{/*', close: '*/}}' } } },
};

export const gotmpl = StreamLanguage.define(gotmplParser);
