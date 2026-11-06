// Stream highlighters for go.mod / go.work and go.sum files.

import { StreamLanguage } from '@codemirror/language';
import type { StreamParser } from '@codemirror/language';

const KEYWORDS = new Set(['module', 'go', 'toolchain', 'require', 'replace', 'exclude', 'retract', 'use', 'godebug', 'tool']);

// Semantic version with optional pre-release/build suffix, as used in module
// requirements (v1.2.3, v0.0.0-20240101000000-abcdef123456, v2.0.0+incompatible).
const VERSION = /^v\d+(?:\.\d+){0,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?/;

function quoted(stream: Parameters<StreamParser<unknown>['token']>[0]): void {
  const q = stream.next();
  while (!stream.eol()) {
    const c = stream.next();
    if (q === '"' && c === '\\') {
      stream.next();
      continue;
    }
    if (c === q) return;
  }
}

/** True when the current token is the first one on its line. */
function firstOnLine(stream: Parameters<StreamParser<unknown>['token']>[0]): boolean {
  return stream.string.slice(0, stream.start).trim() === '';
}

export const gomodParser: StreamParser<Record<string, never>> = {
  name: 'gomod',
  startState: () => ({}),
  token(stream) {
    if (stream.eatSpace()) return null;
    if (stream.match('//')) {
      stream.skipToEnd();
      return 'comment';
    }
    if (stream.match('=>')) return 'operator';
    if (stream.match(/^[()]/)) return 'paren';
    const ch = stream.peek();
    if (ch === '"' || ch === '`') {
      quoted(stream);
      return 'string';
    }
    if (stream.match(VERSION)) return 'number';
    if (stream.match(/^\d+(?:\.\d+)*(?:rc\d+|beta\d+)?/)) return 'number';
    if (stream.match(/^[^\s()"`]+/)) {
      const w = stream.current();
      if (KEYWORDS.has(w) && firstOnLine(stream)) return 'keyword';
      return 'variableName';
    }
    stream.next();
    return null;
  },
  languageData: { commentTokens: { line: '//' } },
};

export const gosumParser: StreamParser<Record<string, never>> = {
  name: 'gosum',
  startState: () => ({}),
  token(stream) {
    if (stream.eatSpace()) return null;
    if (stream.match(/^h\d+:\S+/)) return 'string';
    if (stream.match(VERSION)) return 'number';
    if (stream.match('/go.mod')) return 'meta';
    if (stream.match(/^\S+/)) return 'variableName';
    stream.next();
    return null;
  },
};

export const gomod = StreamLanguage.define(gomodParser);
export const gosum = StreamLanguage.define(gosumParser);
