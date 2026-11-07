// File Structure popup (R4.5) and breadcrumbs (R4.6) from documentSymbol.

import type { Extension } from '@codemirror/state';
import { showPanel } from '@codemirror/view';
import type { EditorView, Panel } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { lang } from '../lib/api';
import type { DocumentSymbol, Pos } from '../lib/api';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { activeTarget, client, info, warn } from './context';
import { openAt } from './navigation';
import { showListPopup } from './popup';
import { toOffsets, toPos } from './positions';

export interface FlatSymbol {
  sym: DocumentSymbol;
  depth: number;
  /** Names of the enclosing symbols. */
  container: string[];
}

/** Depth-first flattening in document order. */
export function flattenSymbols(syms: readonly DocumentSymbol[], depth = 0, container: string[] = []): FlatSymbol[] {
  const out: FlatSymbol[] = [];
  const sorted = [...syms].sort((a, b) => a.range.start.line - b.range.start.line || a.range.start.character - b.range.start.character);
  for (const s of sorted) {
    out.push({ sym: s, depth, container });
    if (s.children?.length) out.push(...flattenSymbols(s.children, depth + 1, [...container, s.name]));
  }
  return out;
}

function contains(s: DocumentSymbol, p: Pos): boolean {
  const { start, end } = s.range;
  const afterStart = p.line > start.line || (p.line === start.line && p.character >= start.character);
  const beforeEnd = p.line < end.line || (p.line === end.line && p.character <= end.character);
  return afterStart && beforeEnd;
}

/** The chain of symbols enclosing p, outermost first. */
export function crumbsAt(syms: readonly DocumentSymbol[], p: Pos): DocumentSymbol[] {
  const chain: DocumentSymbol[] = [];
  let level: readonly DocumentSymbol[] = syms;
  for (;;) {
    const hit = level.find((s) => contains(s, p));
    if (!hit) return chain;
    chain.push(hit);
    level = hit.children ?? [];
  }
}

const KIND_BADGE: Record<number, string> = {
  2: 'mod',
  4: 'pkg',
  5: 'T',
  6: 'm',
  7: 'p',
  8: 'f',
  10: 'E',
  11: 'I',
  12: 'ƒ',
  13: 'v',
  14: 'c',
  22: 'e',
  23: 'S',
  26: 'T',
};

export function kindBadge(kind: number): string {
  return KIND_BADGE[kind] ?? '•';
}

export async function showFileStructure(): Promise<void> {
  const t = await activeTarget();
  if (!t) return;
  let syms: DocumentSymbol[];
  try {
    syms = await lang.documentSymbols(t.buf.path);
  } catch (err) {
    warn('lang.structure', 'Cannot read file structure', String(err));
    return;
  }
  const flat = flattenSymbols(syms);
  if (!flat.length) {
    info('lang.structure', 'No symbols in this file');
    return;
  }
  // Preselect the innermost symbol at the cursor.
  const chain = crumbsAt(syms, t.pos);
  const current = chain[chain.length - 1];
  const initial = Math.max(0, flat.findIndex((f) => f.sym === current));
  const path = t.buf.path;
  showListPopup({
    items: flat,
    title: `Structure of ${t.buf.name}`,
    filter: true,
    initial,
    placeholder: 'Type to filter symbols',
    text: (f) => [...f.container, f.sym.name].join('.'),
    render: (f) =>
      [
        h('span', { style: `padding-left:${f.depth * 14}px` }),
        h('span', { class: 'lp-kind' }, kindBadge(f.sym.kind)),
        h('span', null, f.sym.name),
        f.sym.detail ? h('span', { class: 'lp-detail' }, f.sym.detail) : null,
      ].filter((n): n is HTMLSpanElement => n !== null),
    onPick: (f) => void openAt(path, f.sym.selectionRange, false),
  });
}

// ---- Breadcrumbs

export const CRUMB_REFRESH_MS = 500;

function crumbsPanel(buf: Buffer) {
  return (view: EditorView): Panel => {
    const dom = h('div', { class: 'lang-crumbs', 'aria-label': 'Breadcrumbs' });
    let syms: DocumentSymbol[] = [];
    let timer: ReturnType<typeof setTimeout> | null = null;
    let lastKey = '';

    const render = (): void => {
      const chain = crumbsAt(syms, toPos(view.state.doc, view.state.selection.main.head));
      const key = chain.map((s) => `${s.name}@${s.range.start.line}`).join('/');
      if (key === lastKey) return;
      lastKey = key;
      const nodes: Node[] = [];
      chain.forEach((s, i) => {
        if (i > 0) nodes.push(h('span', { class: 'sep' }, '›'));
        nodes.push(
          h(
            'button',
            {
              title: s.detail || s.name,
              onClick: () => {
                const { from } = toOffsets(view.state.doc, s.selectionRange);
                view.dispatch({ selection: { anchor: from }, scrollIntoView: true });
                view.focus();
              },
            },
            s.name,
          ),
        );
      });
      dom.replaceChildren(...nodes);
    };
    const fetch = (): void => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(async () => {
        timer = null;
        if (!client.live() || !client.tracked(buf.path)) return;
        const version = await client.flush(buf);
        try {
          const next = await lang.documentSymbols(buf.path);
          if (buf.version !== version) return;
          syms = next;
          lastKey = '\0';
          render();
        } catch {
          // keep the previous crumbs
        }
      }, CRUMB_REFRESH_MS);
    };
    // Fetch now and whenever gopls becomes live (e.g. after it starts).
    let wasLive = false;
    const stop = effect(() => {
      const live = ['ready', 'indexing'].includes(client.status().state);
      if (live && !wasLive) fetch();
      wasLive = live;
    });
    return {
      dom,
      update(u) {
        if (u.docChanged) fetch();
        if (u.selectionSet || u.docChanged) render();
      },
      destroy() {
        stop();
        if (timer) clearTimeout(timer);
      },
    };
  };
}

export function breadcrumbsExtension(buf: Buffer): Extension {
  return buf.path.endsWith('.go') ? showPanel.of(crumbsPanel(buf)) : [];
}
