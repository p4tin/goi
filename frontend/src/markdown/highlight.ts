// Syntax highlighting for fenced code blocks in the Markdown preview, with
// the editor's parsers and the active scheme's highlight style. Languages
// load lazily: a block renders as plain text until its language arrives,
// then onLoaded asks the preview to render again.

import type { Language } from '@codemirror/language';
import { highlightCode } from '@lezer/highlight';
import { langIdForFence, languageFor } from '../editor/languages';
import type { LangId } from '../editor/languages';
import { schemeHighlightStyle } from '../theme/schemes';
import type { SchemeId } from '../theme/tokens';

const loaded = new Map<LangId, Language | null>();
const loading = new Map<LangId, Promise<void>>();

/** Code longer than this stays plain (highlighting is synchronous). */
const MAX_CODE = 100_000;

/** Returns RenderOptions.highlight for a scheme. The style's classes exist
 * because the Markdown buffer's own editor view uses the same scheme. */
export function createHighlighter(scheme: SchemeId, onLoaded: () => void): (lang: string, code: string) => Node[] | null {
  const style = schemeHighlightStyle(scheme);
  return (tag, code) => {
    const id = langIdForFence(tag);
    if (!id || code.length > MAX_CODE) return null;
    if (!loaded.has(id)) {
      let p = loading.get(id);
      if (!p) {
        p = languageFor(id).then((lang) => {
          loaded.set(id, lang);
          loading.delete(id);
        });
        loading.set(id, p);
      }
      void p.then(onLoaded);
      return null;
    }
    const lang = loaded.get(id);
    if (!lang) return null;
    const out: Node[] = [];
    highlightCode(
      code,
      lang.parser.parse(code),
      style,
      (text, classes) => {
        if (!classes) out.push(document.createTextNode(text));
        else {
          const span = document.createElement('span');
          span.className = classes;
          span.textContent = text;
          out.push(span);
        }
      },
      () => out.push(document.createTextNode('\n')),
    );
    return out;
  };
}
