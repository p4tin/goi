// Markdown → DOM for the preview, on @lezer/markdown with GFM (tables, task
// lists, strikethrough, autolinks). Builds nodes only (never innerHTML):
// raw HTML shows as text, links carry data-href and images data-src, which
// the preview resolves. Every top-level block is keyed by its source text
// so the preview can keep unchanged nodes between renders.

import { GFM, parser } from '@lezer/markdown';
import type { SyntaxNode } from '@lezer/common';

export interface RenderOptions {
  /** Highlighted children for a fenced block, or null for plain text. */
  highlight?(lang: string, code: string): Node[] | null;
}

export interface Block {
  /** Node type + source text: equal keys render identically. */
  key: string;
  /** 0-based source line where the block starts. */
  line: number;
  el: HTMLElement;
}

const md = parser.configure(GFM);

/** Inline nodes that only delimit content. */
const MARKS = new Set([
  'EmphasisMark',
  'CodeMark',
  'LinkMark',
  'HeaderMark',
  'QuoteMark',
  'StrikethroughMark',
  'TaskMarker',
  'ListMark',
  'LinkTitle',
  'LinkLabel',
  'URL',
  'CodeInfo',
]);

const ENTITIES: Record<string, string> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: ' ',
  copy: '©',
  reg: '®',
  trade: '™',
  mdash: '—',
  ndash: '–',
  hellip: '…',
  laquo: '«',
  raquo: '»',
  middot: '·',
  times: '×',
  rarr: '→',
  larr: '←',
};

export function decodeEntity(s: string): string {
  const m = /^&(?:#(\d+)|#[xX]([0-9a-fA-F]+)|(\w+));$/.exec(s);
  if (!m) return s;
  if (m[3]) return ENTITIES[m[3]] ?? s;
  const code = m[1] ? parseInt(m[1], 10) : parseInt(m[2], 16);
  return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : '�';
}

/** GitHub-style heading id; used counts duplicates (x, x-1, x-2…). */
export function slugify(text: string, used: Map<string, number>): string {
  const base = text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s_-]/gu, '')
    .replace(/\s/g, '-');
  const n = used.get(base);
  used.set(base, (n ?? -1) + 1);
  return n === undefined ? base : `${base}-${n + 1}`;
}

const normLabel = (s: string): string => s.replace(/^\[|\]$/g, '').trim().replace(/\s+/g, ' ').toLowerCase();

const children = (n: SyntaxNode): SyntaxNode[] => {
  const out: SyntaxNode[] = [];
  for (let c = n.firstChild; c; c = c.nextSibling) out.push(c);
  return out;
};

function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls?: string): HTMLElementTagNameMap[K] {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  return e;
}

class Renderer {
  private refs = new Map<string, { url: string; title: string }>();
  private slugs = new Map<string, number>();

  constructor(
    private src: string,
    private o: RenderOptions,
  ) {}

  text(from: number, to: number): string {
    return this.src.slice(from, to);
  }

  collectRefs(doc: SyntaxNode): void {
    for (const n of children(doc)) {
      if (n.name !== 'LinkReference') continue;
      const label = n.getChild('LinkLabel');
      const url = n.getChild('URL');
      const title = n.getChild('LinkTitle');
      if (label && url) {
        const key = normLabel(this.text(label.from, label.to));
        if (!this.refs.has(key)) this.refs.set(key, { url: this.text(url.from, url.to), title: title ? unquote(this.text(title.from, title.to)) : '' });
      }
    }
  }

  block(n: SyntaxNode, tight = false): HTMLElement | null {
    switch (n.name) {
      case 'ATXHeading1':
      case 'ATXHeading2':
      case 'ATXHeading3':
      case 'ATXHeading4':
      case 'ATXHeading5':
      case 'ATXHeading6':
      case 'SetextHeading1':
      case 'SetextHeading2': {
        const level = Number(n.name.slice(-1));
        const h = el(`h${level}` as 'h1');
        this.inline(n, h, n.from, n.to);
        trim(h);
        h.id = slugify(h.textContent ?? '', this.slugs);
        return h;
      }
      case 'Paragraph': {
        const p = el(tight ? 'span' : 'p');
        if (tight) p.className = 'mdp-tight';
        this.inline(n, p, n.from, n.to);
        trim(p);
        return p;
      }
      case 'Blockquote': {
        const q = el('blockquote');
        for (const c of children(n)) {
          const b = this.block(c);
          if (b) q.append(b);
        }
        return q;
      }
      case 'BulletList':
      case 'OrderedList':
        return this.list(n);
      case 'HorizontalRule':
        return el('hr');
      case 'FencedCode':
      case 'CodeBlock':
        return this.code(n);
      case 'Table':
        return this.table(n);
      case 'HTMLBlock':
      case 'ProcessingInstructionBlock': {
        const pre = el('pre', 'mdp-raw');
        pre.textContent = this.text(n.from, n.to);
        return pre;
      }
      case 'CommentBlock':
      case 'LinkReference':
        return null;
      default: {
        if (MARKS.has(n.name)) return null;
        // Unknown block: its text, so nothing is silently lost.
        const p = el('p');
        p.textContent = this.text(n.from, n.to);
        return p;
      }
    }
  }

  private list(n: SyntaxNode): HTMLElement {
    const ordered = n.name === 'OrderedList';
    const list = el(ordered ? 'ol' : 'ul');
    const items = children(n).filter((c) => c.name === 'ListItem');
    // Loose lists (blank lines between items) wrap item text in paragraphs.
    const tight = !items.slice(0, -1).some((it) => /\n[ \t>]*\n/.test(this.text(it.from, (it.nextSibling ?? it).from)));
    if (ordered && items[0]) {
      const mark = items[0].getChild('ListMark');
      const start = mark ? parseInt(this.text(mark.from, mark.to), 10) : 1;
      if (start !== 1 && Number.isFinite(start)) (list as HTMLOListElement).start = start;
    }
    for (const it of items) {
      const li = el('li');
      for (const c of children(it)) {
        if (c.name === 'ListMark') continue;
        if (c.name === 'Task') {
          li.classList.add('mdp-task');
          const marker = c.getChild('TaskMarker');
          const box = el('input');
          box.type = 'checkbox';
          box.disabled = true;
          box.checked = !!marker && /x/i.test(this.text(marker.from, marker.to));
          const span = el('span', 'mdp-tight');
          this.inline(c, span, marker ? marker.to : c.from, c.to);
          trim(span);
          li.append(box, ' ', span);
          continue;
        }
        const b = this.block(c, tight);
        if (b) li.append(b);
      }
      list.append(li);
    }
    return list;
  }

  private code(n: SyntaxNode): HTMLElement {
    const info = n.getChild('CodeInfo');
    const lang = info ? this.text(info.from, info.to).trim().split(/\s+/)[0].toLowerCase() : '';
    const texts = n.getChildren('CodeText');
    let code = texts.length ? this.text(texts[0].from, texts[texts.length - 1].to) : '';
    if (n.name === 'CodeBlock') code = code.replace(/^ {1,4}/gm, '');
    const pre = el('pre', 'mdp-code');
    const c = el('code');
    if (lang) c.dataset.lang = lang;
    if (lang === 'mermaid') {
      c.textContent = code;
      pre.append(c);
      const box = el('div', 'mdp-mermaid');
      box.dataset.source = code;
      box.append(pre);
      return box;
    }
    const hl = lang ? (this.o.highlight?.(lang, code) ?? null) : null;
    if (hl) c.append(...hl);
    else c.textContent = code;
    pre.append(c);
    return pre;
  }

  private table(n: SyntaxNode): HTMLElement {
    const table = el('table', 'mdp-table');
    const delim = children(n).find((c) => c.name === 'TableDelimiter');
    const aligns = delim
      ? this.text(delim.from, delim.to)
          .replace(/^\s*\|/, '')
          .replace(/\|\s*$/, '')
          .split('|')
          .map((s) => {
            const t = s.trim();
            return t.startsWith(':') && t.endsWith(':') ? 'center' : t.endsWith(':') ? 'right' : t.startsWith(':') ? 'left' : '';
          })
      : [];
    const row = (r: SyntaxNode, tag: 'th' | 'td', width: number): HTMLElement => {
      const tr = el('tr');
      const cells = r.getChildren('TableCell');
      for (let i = 0; i < Math.max(width, cells.length); i++) {
        const cell = el(tag);
        if (aligns[i]) cell.style.textAlign = aligns[i];
        const c = cells[i];
        if (c) {
          this.inline(c, cell, c.from, c.to);
          trim(cell);
        }
        tr.append(cell);
      }
      return tr;
    };
    const header = n.getChild('TableHeader');
    const width = header ? header.getChildren('TableCell').length : 0;
    if (header) {
      const thead = el('thead');
      thead.append(row(header, 'th', width));
      table.append(thead);
    }
    const rows = n.getChildren('TableRow');
    if (rows.length) {
      const tbody = el('tbody');
      for (const r of rows) tbody.append(row(r, 'td', width));
      table.append(tbody);
    }
    return table;
  }

  /** Renders the inline content of n between from and to into parent. */
  inline(n: SyntaxNode, parent: HTMLElement, from: number, to: number): void {
    let pos = from;
    for (const c of children(n)) {
      if (c.to <= from || c.from >= to) continue;
      if (c.from > pos) this.gap(parent, pos, c.from);
      this.inlineNode(c, parent);
      pos = Math.max(pos, c.to);
    }
    if (pos < to) this.gap(parent, pos, to);
  }

  private gap(parent: HTMLElement, from: number, to: number): void {
    let t = this.text(from, to).replace(/\n[ \t]*/g, '\n');
    // After a continuation line's prefix ("  > "), drop its spaces.
    const lineStart = this.src.lastIndexOf('\n', from - 1) + 1;
    if (lineStart > 0 && /^[\s>]*$/.test(this.text(lineStart, from))) t = t.replace(/^[ \t]+/, '');
    if (t) parent.append(t);
  }

  private inlineNode(n: SyntaxNode, parent: HTMLElement): void {
    switch (n.name) {
      case 'Emphasis':
      case 'StrongEmphasis':
      case 'Strikethrough': {
        const e = el(n.name === 'Emphasis' ? 'em' : n.name === 'StrongEmphasis' ? 'strong' : 'del');
        this.inline(n, e, n.from, n.to);
        parent.append(e);
        return;
      }
      case 'InlineCode': {
        const marks = n.getChildren('CodeMark');
        const inner = this.text(marks[0]?.to ?? n.from, marks[1]?.from ?? n.to).replace(/\n/g, ' ');
        const c = el('code');
        c.textContent = /^ .*[^ ].* $/.test(inner) ? inner.slice(1, -1) : inner;
        parent.append(c);
        return;
      }
      case 'Link':
        parent.append(this.link(n));
        return;
      case 'Image':
        parent.append(this.image(n));
        return;
      case 'Autolink':
      case 'URL': {
        const u = n.name === 'URL' ? n : n.getChild('URL');
        const url = u ? this.text(u.from, u.to) : '';
        const a = anchor(/^www\./i.test(url) ? `http://${url}` : url.includes('@') && !url.includes(':') ? `mailto:${url}` : url);
        a.textContent = url;
        parent.append(a);
        return;
      }
      case 'Escape':
        parent.append(this.text(n.from + 1, n.to));
        return;
      case 'Entity':
        parent.append(decodeEntity(this.text(n.from, n.to)));
        return;
      case 'HardBreak':
        parent.append(el('br'));
        return;
      case 'HTMLTag':
        parent.append(this.text(n.from, n.to));
        return;
      case 'Comment':
      case 'ProcessingInstruction':
        return;
      default:
        if (MARKS.has(n.name)) return;
        this.inline(n, parent, n.from, n.to);
    }
  }

  /** Label range and destination of a link or image. */
  private target(n: SyntaxNode): { from: number; to: number; url: string; title: string } {
    const marks = n.getChildren('LinkMark');
    const open = marks[0];
    const close = marks.find((m) => this.text(m.from, m.to) === ']');
    const from = open ? open.to : n.from;
    const to = close ? close.from : n.to;
    const url = n.getChild('URL');
    const title = n.getChild('LinkTitle');
    if (url) return { from, to, url: this.text(url.from, url.to).replace(/^<|>$/g, ''), title: title ? unquote(this.text(title.from, title.to)) : '' };
    const label = n.getChild('LinkLabel');
    const ref = this.refs.get(normLabel(label ? this.text(label.from, label.to) : this.text(from, to)));
    return { from, to, url: ref?.url ?? '', title: ref?.title ?? '' };
  }

  private link(n: SyntaxNode): HTMLElement {
    const t = this.target(n);
    const a = anchor(t.url);
    if (t.title) a.title = t.title;
    this.inline(n, a, t.from, t.to);
    return a;
  }

  private image(n: SyntaxNode): HTMLElement {
    const t = this.target(n);
    const img = el('img');
    const alt = el('span');
    this.inline(n, alt, t.from, t.to);
    img.alt = alt.textContent ?? '';
    if (t.url) img.dataset.src = t.url;
    if (t.title) img.title = t.title;
    return img;
  }
}

function anchor(url: string): HTMLAnchorElement {
  const a = el('a');
  a.href = '#';
  a.dataset.href = url;
  if (url) a.title = url;
  return a;
}

function unquote(s: string): string {
  return s.length >= 2 && /^["'(]/.test(s) ? s.slice(1, -1) : s;
}

/** Trims whitespace at the edges of an element's text. */
function trim(e: HTMLElement): void {
  const first = e.firstChild;
  if (first?.nodeType === Node.TEXT_NODE) first.textContent = first.textContent!.replace(/^\s+/, '');
  const last = e.lastChild;
  if (last?.nodeType === Node.TEXT_NODE) last.textContent = last.textContent!.replace(/\s+$/, '');
}

function lineStarts(text: string): number[] {
  const out = [0];
  for (let i = text.indexOf('\n'); i >= 0; i = text.indexOf('\n', i + 1)) out.push(i + 1);
  return out;
}

function lineOf(starts: number[], pos: number): number {
  let lo = 0;
  let hi = starts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (starts[mid] <= pos) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

/** Renders text into top-level blocks. */
export function renderBlocks(text: string, o: RenderOptions = {}): Block[] {
  const tree = md.parse(text);
  const r = new Renderer(text, o);
  const doc = tree.topNode;
  r.collectRefs(doc);
  const starts = lineStarts(text);
  const out: Block[] = [];
  for (const n of children(doc)) {
    const e = r.block(n);
    if (!e) continue;
    const line = lineOf(starts, n.from);
    e.dataset.line = String(line);
    // Heading ids depend on earlier duplicates, so they are part of the key.
    out.push({ key: `${n.name}\u0000${e.id}\u0000${text.slice(n.from, n.to)}`, line, el: e });
  }
  return out;
}
