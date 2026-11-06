// Base CodeMirror extensions shared by every editor view, plus
// settings-driven compartments.

import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import {
  bracketMatching,
  foldGutter,
  foldKeymap,
  foldService,
  indentOnInput,
  indentUnit,
} from '@codemirror/language';
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search';
import { EditorState } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import {
  crosshairCursor,
  drawSelection,
  dropCursor,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
} from '@codemirror/view';

/** Folds a line's indented block when no syntax-based folding applies. */
export const indentFold = foldService.of((state, lineStart) => {
  const line = state.doc.lineAt(lineStart);
  const indent = (s: string): number => {
    let n = 0;
    for (const ch of s) {
      if (ch === ' ') n++;
      else if (ch === '\t') n += state.tabSize;
      else return n;
    }
    return -1; // blank line
  };
  const base = indent(line.text);
  if (base < 0) return null;
  let end = line.number;
  for (let n = line.number + 1; n <= state.doc.lines; n++) {
    const ind = indent(state.doc.line(n).text);
    if (ind === -1) continue;
    if (ind <= base) break;
    end = n;
  }
  if (end === line.number) return null;
  return { from: line.to, to: state.doc.line(end).to };
});

export function baseExtensions(): Extension[] {
  return [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    history(),
    foldGutter(),
    indentFold,
    drawSelection(),
    dropCursor(),
    EditorState.allowMultipleSelections.of(true),
    indentOnInput(),
    bracketMatching(),
    closeBrackets(),
    autocompletion({ activateOnTyping: false }),
    rectangularSelection(),
    crosshairCursor(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    search({ top: true }),
    keymap.of([
      ...closeBracketsKeymap,
      ...defaultKeymap,
      ...searchKeymap,
      ...historyKeymap,
      ...foldKeymap,
      ...completionKeymap,
      indentWithTab,
    ]),
  ];
}

export interface EditorPrefs {
  fontFamily: string;
  fontSize: number;
  tabSize: number;
  softWrap: boolean;
}

export function fontExtension(p: EditorPrefs): Extension {
  return EditorView.theme({
    '&': { fontSize: `${p.fontSize}px` },
    '.cm-scroller': { fontFamily: p.fontFamily, lineHeight: '1.5' },
  });
}

export function tabExtension(p: EditorPrefs): Extension {
  return [EditorState.tabSize.of(p.tabSize), indentUnit.of(' '.repeat(Math.max(1, p.tabSize)))];
}

export function wrapExtension(p: EditorPrefs): Extension {
  return p.softWrap ? EditorView.lineWrapping : [];
}
