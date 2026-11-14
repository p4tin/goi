// A safe Markdown subset rendered to DOM nodes (never innerHTML): headings,
// paragraphs, lists, block quotes, rules, simple tables, fenced code blocks
// (with Copy / Insert actions), inline code, bold, italic, links and file
// locations. An unterminated code fence (still streaming) renders as code.

import { h } from '../lib/dom';

export interface MarkdownOptions {
  /** Opens http(s) links. */
  openLink?: (url: string) => void;
  /** Opens a file location like "pkg/a.go:12". */
  openFile?: (path: string, line?: number) => void;
  /** Code block actions. */
  copy?: (code: string) => void;
  insert?: (code: string) => void;
}

const FILE_RE = /^(?:[A-Za-z]:)?(?:[\w.@~-]*[/\\])*[\w.@-]+\.[A-Za-z0-9]+(?::(\d+))?(?::\d+)?$/;

/** Parses "path:line[:col]" when s looks like a file location. */
export function fileLocation(s: string): { path: string; line?: number } | null {
  const t = s.trim();
  if (!FILE_RE.test(t) || /^\d/.test(t) || !/[/\\]|\.(go|mod|sum|ts|js|md|toml|ya?ml|json|txt|sql|proto)\b/.test(t)) return null;
  const m = /^(.*?)(?::(\d+))?(?::\d+)?$/.exec(t)!;
  return { path: m[1], line: m[2] ? Number(m[2]) : undefined };
}

const INLINE =
  /(`+)([\s\S]*?[^`])\1(?!`)|\*\*([^*]+?)\*\*|__([^_]+?)__|\*([^*\s][^*]*?)\*|(?<![\w])_([^_\s][^_]*?)_(?![\w])|\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g;

/** Renders inline Markdown into parent. */
export function renderInline(text: string, parent: HTMLElement, o: MarkdownOptions = {}): void {
  let last = 0;
  // A fresh regex per call: renderInline recurses for bold/italic/links.
  const re = new RegExp(INLINE.source, 'g');
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) parent.append(text.slice(last, m.index));
    last = m.index + m[0].length;
    if (m[2] !== undefined) {
      const code = m[2].replace(/^ (.*) $/, '$1');
      const loc = o.openFile ? fileLocation(code) : null;
      if (loc) {
        parent.append(h('a', { class: 'md-file', href: '#', title: 'Open in editor', onClick: (e: Event) => { e.preventDefault(); o.openFile!(loc.path, loc.line); } }, h('code', null, code)));
      } else {
        parent.append(h('code', null, code));
      }
    } else if (m[3] !== undefined || m[4] !== undefined) {
      const b = h('strong');
      renderInline(m[3] ?? m[4], b, o);
      parent.append(b);
    } else if (m[5] !== undefined || m[6] !== undefined) {
      const i = h('em');
      renderInline(m[5] ?? m[6], i, o);
      parent.append(i);
    } else if (m[7] !== undefined) {
      parent.append(link(m[7], m[8], o));
    } else if (m[9] !== undefined) {
      parent.append(link(m[9], m[9], o, true));
    }
  }
  if (last < text.length) parent.append(text.slice(last));
}

function link(label: string, url: string, o: MarkdownOptions, plain = false): HTMLElement {
  const a = h('a', { href: '#', title: url });
  if (plain) a.append(label);
  else renderInline(label, a, o);
  a.addEventListener('click', (e) => {
    e.preventDefault();
    if (/^https?:\/\//i.test(url)) o.openLink?.(url);
    else {
      const loc = fileLocation(url.replace(/^file:\/\//, ''));
      if (loc) o.openFile?.(loc.path, loc.line);
    }
  });
  return a;
}

function codeBlock(lang: string, code: string, o: MarkdownOptions): HTMLElement {
  const actions = h('div', { class: 'md-code-actions' });
  if (lang) actions.append(h('span', { class: 'md-code-lang' }, lang));
  if (o.copy) actions.append(h('button', { class: 'md-code-btn', title: 'Copy', onClick: () => o.copy!(code) }, 'Copy'));
  if (o.insert) actions.append(h('button', { class: 'md-code-btn', title: 'Insert at the cursor', onClick: () => o.insert!(code) }, 'Insert'));
  return h('div', { class: 'md-code' }, actions, h('pre', null, h('code', { dataset: lang ? { lang } : {} }, code)));
}

const isRule = (l: string): boolean => /^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(l);
const listItem = (l: string): { ordered: boolean; text: string; indent: number } | null => {
  const m = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/.exec(l);
  return m ? { ordered: /\d/.test(m[2]), text: m[3], indent: m[1].length } : null;
};
const isTableRow = (l: string): boolean => /^\s*\|.*\|\s*$/.test(l);
const cells = (l: string): string[] => l.trim().replace(/^\||\|$/g, '').split('|').map((c) => c.trim());

/** Renders Markdown into a new element. */
export function renderMarkdown(text: string, o: MarkdownOptions = {}): HTMLElement {
  const root = h('div', { class: 'md' });
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  let i = 0;
  let para: string[] = [];
  const flush = (): void => {
    if (para.length) {
      const p = h('p');
      renderInline(para.join('\n'), p, o);
      root.append(p);
      para = [];
    }
  };
  while (i < lines.length) {
    const line = lines[i];
    const fence = /^\s*(```+|~~~+)\s*([\w+#.-]*)/.exec(line);
    if (fence) {
      flush();
      const close = fence[1];
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trim().startsWith(close)) body.push(lines[i++]);
      i++; // closing fence (or past the end while streaming)
      root.append(codeBlock(fence[2], body.join('\n'), o));
      continue;
    }
    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      flush();
      const el = h(`h${Math.min(heading[1].length + 2, 6)}` as 'h3', { class: 'md-h' });
      renderInline(heading[2].replace(/\s+#+\s*$/, ''), el, o);
      root.append(el);
      i++;
      continue;
    }
    if (isRule(line)) {
      flush();
      root.append(h('hr'));
      i++;
      continue;
    }
    if (/^\s*>/.test(line)) {
      flush();
      const quote: string[] = [];
      while (i < lines.length && /^\s*>/.test(lines[i])) quote.push(lines[i++].replace(/^\s*>\s?/, ''));
      const bq = h('blockquote');
      bq.append(...renderMarkdown(quote.join('\n'), o).childNodes);
      root.append(bq);
      continue;
    }
    if (isTableRow(line) && i + 1 < lines.length && /^\s*\|?\s*:?-{2,}/.test(lines[i + 1])) {
      flush();
      const head = cells(line);
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && isTableRow(lines[i])) rows.push(cells(lines[i++]));
      const tr = (cs: string[], tag: 'th' | 'td'): HTMLElement =>
        h('tr', null, cs.map((c) => {
          const el = h(tag);
          renderInline(c, el, o);
          return el;
        }));
      root.append(h('table', { class: 'md-table' }, h('thead', null, tr(head, 'th')), h('tbody', null, rows.map((r) => tr(r, 'td')))));
      continue;
    }
    const item = listItem(line);
    if (item) {
      flush();
      const list = h(item.ordered ? 'ol' : 'ul');
      while (i < lines.length) {
        const it = listItem(lines[i]);
        if (!it) {
          // Continuation lines of the previous item.
          if (lines[i].trim() && /^\s{2,}/.test(lines[i]) && list.lastElementChild) {
            list.lastElementChild.append('\n' + lines[i].trim());
            i++;
            continue;
          }
          break;
        }
        const li = h('li');
        if (it.indent >= 2) li.classList.add('md-nested');
        const task = /^\[([ xX])\]\s+(.*)$/.exec(it.text);
        if (task) {
          li.classList.add('md-task');
          li.append(h('span', { class: `md-check${task[1] === ' ' ? '' : ' done'}` }, task[1] === ' ' ? '☐' : '☑'), ' ');
          renderInline(task[2], li, o);
        } else {
          renderInline(it.text, li, o);
        }
        list.append(li);
        i++;
      }
      root.append(list);
      continue;
    }
    if (!line.trim()) {
      flush();
      i++;
      continue;
    }
    para.push(line);
    i++;
  }
  flush();
  return root;
}
