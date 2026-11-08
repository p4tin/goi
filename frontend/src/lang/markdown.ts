// A small, safe Markdown renderer for documentation popups. It builds DOM
// nodes with textContent only (never innerHTML), so server text cannot
// inject markup. Supported: fenced code, headings, lists, paragraphs,
// inline code, **bold**, *emphasis*/_emphasis_, [links](url), \escapes.

import { openExternal } from '../lib/api';
import { h } from '../lib/dom';

type Block =
  | { kind: 'code'; lang: string; text: string }
  | { kind: 'heading'; level: number; text: string }
  | { kind: 'list'; items: string[] }
  | { kind: 'para'; text: string }
  | { kind: 'rule' };

/** Splits Markdown into blocks (exported for tests). */
export function parseBlocks(md: string): Block[] {
  const lines = md.replace(/\r\n?/g, '\n').split('\n');
  const out: Block[] = [];
  let para: string[] = [];
  let list: string[] | null = null;
  const flush = (): void => {
    if (para.length) out.push({ kind: 'para', text: para.join(' ') });
    para = [];
    if (list) out.push({ kind: 'list', items: list });
    list = null;
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fence = /^\s*(```|~~~)\s*([\w+-]*)\s*$/.exec(line);
    if (fence) {
      flush();
      const body: string[] = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith(fence[1]); i++) body.push(lines[i]);
      out.push({ kind: 'code', lang: fence[2], text: body.join('\n') });
      continue;
    }
    if (/^\s*$/.test(line)) {
      flush();
      continue;
    }
    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      flush();
      out.push({ kind: 'rule' });
      continue;
    }
    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      flush();
      out.push({ kind: 'heading', level: heading[1].length, text: heading[2] });
      continue;
    }
    const item = /^\s*(?:[-*+]|\d+[.)])\s+(.*)$/.exec(line);
    if (item) {
      if (para.length) {
        out.push({ kind: 'para', text: para.join(' ') });
        para = [];
      }
      (list ??= []).push(item[1]);
      continue;
    }
    if (/^ {4,}|\t/.test(line) && !para.length && !list) {
      // indented code block
      const body = [line.replace(/^( {4}|\t)/, '')];
      while (i + 1 < lines.length && /^( {4,}|\t)/.test(lines[i + 1])) body.push(lines[++i].replace(/^( {4}|\t)/, ''));
      out.push({ kind: 'code', lang: '', text: body.join('\n') });
      continue;
    }
    if (list) {
      list[list.length - 1] += ' ' + line.trim();
      continue;
    }
    para.push(line.trim());
  }
  flush();
  return out;
}

/** Renders inline Markdown into nodes. */
export function renderInline(text: string): Node[] {
  const out: Node[] = [];
  let buf = '';
  const flush = (): void => {
    if (buf) out.push(document.createTextNode(buf));
    buf = '';
  };
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    if (c === '\\' && i + 1 < text.length && /[\\`*_[\]()#<>!.+-]/.test(text[i + 1])) {
      buf += text[i + 1];
      i += 2;
      continue;
    }
    if (c === '`') {
      const end = text.indexOf('`', i + 1);
      if (end > i) {
        flush();
        out.push(h('code', null, text.slice(i + 1, end)));
        i = end + 1;
        continue;
      }
    }
    if (c === '[') {
      const m = /^\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)/.exec(text.slice(i));
      if (m) {
        flush();
        const url = m[2];
        const a = h('a', { href: '#', title: url }, renderInline(m[1]));
        a.addEventListener('click', (e) => {
          e.preventDefault();
          openExternal(url);
        });
        out.push(a);
        i += m[0].length;
        continue;
      }
    }
    if ((c === '*' || c === '_') && text[i + 1] === c) {
      const end = text.indexOf(c + c, i + 2);
      if (end > i + 2) {
        flush();
        out.push(h('strong', null, renderInline(text.slice(i + 2, end))));
        i = end + 2;
        continue;
      }
    }
    if ((c === '*' || c === '_') && text[i + 1] !== ' ' && (c === '*' || i === 0 || !/\w/.test(text[i - 1]))) {
      const end = text.indexOf(c, i + 1);
      if (end > i + 1 && (c === '*' || end + 1 >= text.length || !/\w/.test(text[end + 1]))) {
        flush();
        out.push(h('em', null, renderInline(text.slice(i + 1, end))));
        i = end + 1;
        continue;
      }
    }
    buf += c;
    i++;
  }
  flush();
  return out;
}

/** Renders Markdown into a container element. */
export function renderMarkdown(md: string): HTMLElement {
  const root = h('div', { class: 'md' });
  for (const b of parseBlocks(md)) {
    switch (b.kind) {
      case 'code':
        root.append(h('pre', { class: 'md-code', dataset: { lang: b.lang } }, h('code', null, b.text)));
        break;
      case 'heading':
        root.append(h(`h${Math.min(6, b.level + 2)}` as 'h3', { class: 'md-h' }, renderInline(b.text)));
        break;
      case 'list':
        root.append(h('ul', null, b.items.map((it) => h('li', null, renderInline(it)))));
        break;
      case 'rule':
        root.append(h('hr'));
        break;
      default:
        root.append(h('p', null, renderInline(b.text)));
    }
  }
  return root;
}
