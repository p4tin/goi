// Parameter Info (R6.3): the signature of the enclosing call with the
// active parameter highlighted. Opens on "(" / "," or Mod-P and follows the
// cursor until it leaves the call.

import { StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { keymap, showTooltip, ViewPlugin } from '@codemirror/view';
import type { EditorView, Tooltip } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { lang } from '../lib/api';
import type { SignatureHelp, SignatureInfo } from '../lib/api';
import { h } from '../lib/dom';
import { client, info } from './context';
import { langTooltip } from './hover';
import { renderMarkdown } from './markdown';
import { toPos } from './positions';

/** Splits a signature label around the active parameter. */
export function splitSignature(sig: SignatureInfo, active: number): [string, string, string] {
  const p = sig.parameters?.[active];
  if (!p) return [sig.label, '', ''];
  let start = p.start;
  let end = p.end;
  if (start < 0 || end < 0) {
    start = p.label ? sig.label.indexOf(p.label) : -1;
    end = start >= 0 ? start + p.label.length : -1;
  }
  if (start < 0 || end > sig.label.length || end < start) return [sig.label, '', ''];
  return [sig.label.slice(0, start), sig.label.slice(start, end), sig.label.slice(end)];
}

export function signatureDom(help: SignatureHelp): HTMLElement {
  const sig = help.signatures[help.activeSignature] ?? help.signatures[0];
  const active = sig.activeParameter ?? help.activeParameter ?? 0;
  const [a, b, c] = splitSignature(sig, active);
  const param = sig.parameters?.[active];
  return h(
    'div',
    { class: 'lang-signature-wrap' },
    h('div', { class: 'lang-signature' }, a, b ? h('span', { class: 'active-param' }, b) : null, c),
    param?.documentation ? h('div', { class: 'lang-signature-doc' }, renderMarkdown(param.documentation)) : null,
    sig.documentation ? h('div', { class: 'lang-signature-doc' }, renderMarkdown(sig.documentation)) : null,
  );
}

const setSignature = StateEffect.define<Tooltip | null>();

const signatureField = StateField.define<Tooltip | null>({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setSignature)) return e.value;
    if (value && tr.docChanged) return { ...value, pos: tr.changes.mapPos(value.pos) };
    return value;
  },
  provide: (f) => showTooltip.from(f),
});

let seq = 0;

async function refresh(buf: Buffer, view: EditorView, explicit: boolean): Promise<void> {
  if (!client.live() || !client.tracked(buf.path)) return;
  const mine = ++seq;
  await client.flush(buf);
  const head = view.state.selection.main.head;
  let help: SignatureHelp | null = null;
  try {
    help = await lang.signatureHelp(buf.path, toPos(view.state.doc, head));
  } catch {
    help = null;
  }
  if (mine !== seq || view.state.selection.main.head !== head) return;
  if (!help || help.signatures.length === 0) {
    if (view.state.field(signatureField, false)) view.dispatch({ effects: setSignature.of(null) });
    if (explicit) info('lang.paramInfo', 'No parameter info here');
    return;
  }
  const h0 = help;
  view.dispatch({ effects: setSignature.of({ pos: head, above: true, create: () => langTooltip(signatureDom(h0)) }) });
}

/** Mod-P: show parameter info at the cursor. */
export function showParameterInfo(): void {
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (buf && view) void refresh(buf, view, true);
}

export function signatureExtension(buf: Buffer): Extension {
  let timer: ReturnType<typeof setTimeout> | null = null;
  return [
    signatureField,
    keymap.of([
      {
        key: 'Escape',
        run: (view) => {
          if (!view.state.field(signatureField, false)) return false;
          view.dispatch({ effects: setSignature.of(null) });
          return true;
        },
      },
    ]),
    ViewPlugin.define((view) => ({
      update(u) {
        const open = !!u.state.field(signatureField, false);
        let trigger = false;
        for (const tr of u.transactions) {
          if (!tr.isUserEvent('input.type')) continue;
          tr.changes.iterChanges((_a, _b, _c, _d, ins) => {
            const s = ins.toString();
            if (s.includes('(') || s.includes(',')) trigger = true;
          });
        }
        if (trigger || (open && (u.docChanged || u.selectionSet))) {
          if (timer) clearTimeout(timer);
          timer = setTimeout(() => void refresh(buf, view, false), trigger ? 0 : 120);
        }
      },
      destroy() {
        if (timer) clearTimeout(timer);
      },
    })),
  ];
}
