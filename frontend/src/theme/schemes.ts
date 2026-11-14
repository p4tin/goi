// Editor color schemes as CodeMirror extensions. Colors come straight from
// SchemeTokens so a Compartment can swap schemes at runtime.

import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import type { Extension } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { tags as t } from '@lezer/highlight';
import { schemes, type SchemeId, type UiThemeId } from './tokens';

const highlightStyles = new Map<SchemeId, HighlightStyle>();

/** The scheme's syntax colors, shared by editors and the Markdown preview. */
export function schemeHighlightStyle(id: SchemeId): HighlightStyle {
  let style = highlightStyles.get(id);
  if (!style) {
    const c = schemes[id].syntax;
    style = HighlightStyle.define([
      { tag: [t.keyword, t.controlKeyword, t.moduleKeyword, t.operatorKeyword, t.definitionKeyword], color: c.keyword },
      { tag: [t.string, t.special(t.string), t.regexp, t.character], color: c.string },
      { tag: [t.number, t.integer, t.float], color: c.number },
      { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: c.comment, fontStyle: 'italic' },
      { tag: [t.function(t.variableName), t.function(t.propertyName), t.function(t.definition(t.variableName))], color: c.function },
      { tag: [t.typeName, t.className, t.namespace, t.standard(t.typeName)], color: c.type },
      { tag: [t.variableName, t.propertyName, t.definition(t.variableName)], color: c.variable },
      { tag: [t.bool, t.null, t.atom, t.constant(t.variableName), t.self], color: c.constant },
      { tag: [t.operator, t.compareOperator, t.arithmeticOperator, t.logicOperator], color: c.operator },
      { tag: [t.punctuation, t.bracket, t.separator], color: c.punctuation },
      { tag: [t.tagName], color: c.tag },
      { tag: [t.attributeName], color: c.attribute },
      { tag: [t.invalid], color: c.invalid },
      { tag: [t.heading], color: c.heading, fontWeight: 'bold' },
      { tag: [t.link, t.url], color: c.link, textDecoration: 'underline' },
      { tag: [t.emphasis], fontStyle: 'italic' },
      { tag: [t.strong], fontWeight: 'bold' },
    ]);
    highlightStyles.set(id, style);
  }
  return style;
}

export function schemeExtension(id: SchemeId): Extension {
  const s = schemes[id];
  const e = s.editor;

  const theme = EditorView.theme(
    {
      '&': { color: e.fg, backgroundColor: e.bg },
      '.cm-content': { caretColor: e.cursor },
      '.cm-cursor, .cm-dropCursor': { borderLeftColor: e.cursor },
      '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
        { backgroundColor: e.selection },
      '.cm-activeLine': { backgroundColor: e.lineHighlight },
      '.cm-gutters': { backgroundColor: e.gutterBg, color: e.gutterFg, border: 'none' },
      '.cm-activeLineGutter': { backgroundColor: e.lineHighlight, color: e.fg },
      '&.cm-focused .cm-matchingBracket': { backgroundColor: e.matchBracket },
      '.cm-searchMatch': { backgroundColor: e.matchBracket },
    },
    { dark: s.dark },
  );

  const highlight = schemeHighlightStyle(id);

  return [theme, syntaxHighlighting(highlight)];
}

export function defaultSchemeFor(ui: UiThemeId): SchemeId {
  return ui === 'light' ? 'goi-light' : 'goi-dark';
}
