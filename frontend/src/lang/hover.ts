// Hover documentation and Quick Documentation (R6.1–2).

import { StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { hoverTooltip, keymap, showTooltip } from '@codemirror/view';
import type { EditorView } from '@codemirror/view';
import type { Tooltip, TooltipView } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { lang } from '../lib/api';
import { h } from '../lib/dom';
import { editors } from '../editor/install';
import { client, info } from './context';
import { keywordDoc } from './keywords';
import { renderMarkdown } from './markdown';
import { toOffsets, toPos, wordAt } from './positions';

export const HOVER_DELAY_MS = 450;

export function docDom(markdown: string): HTMLElement {
  return h('div', { class: 'lang-doc' }, renderMarkdown(markdown));
}

/** A tooltip view with the shared popup look. CodeMirror makes the view's
 * dom the .cm-tooltip element itself, so the class goes on dom. */
export function langTooltip(content: HTMLElement): TooltipView {
  const dom = document.createElement('div');
  dom.className = 'lang-tooltip';
  dom.append(content);
  return { dom };
}

/** Documentation for the word at pos: gopls hover, else keyword docs. */
async function docAt(buf: Buffer, view: EditorView, pos: number): Promise<{ md: string; from: number; to: number } | null> {
  const word = wordAt(view.state.doc, pos);
  if (!word) return null;
  let md = '';
  let from = word.from;
  let to = word.to;
  if (client.live() && client.tracked(buf.path)) {
    await client.flush(buf);
    try {
      const res = await lang.hover(buf.path, toPos(view.state.doc, pos));
      md = res.markdown;
      if (res.range) ({ from, to } = toOffsets(view.state.doc, res.range));
    } catch {
      md = '';
    }
  }
  if (!md.trim()) md = keywordDoc(word.text) ?? '';
  return md.trim() ? { md, from, to } : null;
}

function hoverExtension(buf: Buffer): Extension {
  return hoverTooltip(
    async (view, pos) => {
      const version = buf.version;
      const d = await docAt(buf, view, pos);
      if (!d || buf.version !== version) return null;
      return { pos: d.from, end: d.to, above: true, create: () => langTooltip(docDom(d.md)) };
    },
    { hoverTime: HOVER_DELAY_MS },
  );
}

// Quick Documentation: a tooltip at the cursor until the cursor moves.
const setQuickDoc = StateEffect.define<Tooltip | null>();

const quickDocField = StateField.define<Tooltip | null>({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setQuickDoc)) return e.value;
    if (value && (tr.docChanged || tr.selection)) return null;
    return value;
  },
  provide: (f) => showTooltip.from(f),
});

const quickDocKeys = keymap.of([
  {
    key: 'Escape',
    run: (view) => {
      if (!view.state.field(quickDocField, false)) return false;
      view.dispatch({ effects: setQuickDoc.of(null) });
      return true;
    },
  },
]);

export async function showQuickDoc(): Promise<void> {
  // Works without gopls too: keyword docs are local.
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (!buf || !view) return;
  const t = { buf, view, offset: view.state.selection.main.head };
  const d = await docAt(t.buf, t.view, t.offset);
  if (!d) {
    info('lang.quickDoc', 'No documentation found');
    return;
  }
  t.view.dispatch({
    effects: setQuickDoc.of({ pos: d.from, above: false, create: () => langTooltip(docDom(d.md)) }),
  });
}

export function hoverExtensions(buf: Buffer): Extension {
  return [hoverExtension(buf), quickDocField, quickDocKeys];
}
