// Output console for runs and debug sessions (R2.2): ANSI-styled lines,
// stderr/system styling, clickable source locations, a line cap, soft wrap,
// search, and an input line feeding the process's stdin.

import { h } from '../lib/dom';
import { AnsiParser } from './ansi';
import type { Segment } from './ansi';
import { findLocations } from './links';
import type { LocationMatch } from './links';

export type ConsoleStream = 'stdout' | 'stderr' | 'system' | 'stdin' | 'console';

export interface ConsoleOptions {
  maxLines: number;
  /** Called with a line the user typed (without newline). */
  onInput?: (text: string) => void;
  /** Opens a clicked location. */
  onLocation?: (loc: LocationMatch) => void;
}

export class Console {
  readonly el: HTMLElement;
  readonly body: HTMLElement;
  private input: HTMLInputElement;
  private searchBox: HTMLInputElement;
  private searchInfo: HTMLElement;
  private searchBar: HTMLElement;
  private parsers = new Map<ConsoleStream, AnsiParser>();
  private current: HTMLElement | null = null;
  private pendingCR = false;
  private lineCount = 0;
  private matches: HTMLElement[] = [];
  private matchIndex = -1;
  private stick = true;

  constructor(private opts: ConsoleOptions) {
    this.body = h('div', { class: 'con-body', role: 'log', tabindex: '0' });
    this.body.addEventListener('scroll', () => {
      const b = this.body;
      this.stick = b.scrollTop + b.clientHeight >= b.scrollHeight - 4;
    });
    this.body.addEventListener('click', (e) => {
      const a = (e.target as HTMLElement).closest('a.con-link') as HTMLElement | null;
      if (!a || !this.opts.onLocation) return;
      e.preventDefault();
      this.opts.onLocation({ start: 0, end: 0, path: a.dataset.path!, line: Number(a.dataset.line), col: Number(a.dataset.col) });
    });
    this.input = h('input', { class: 'con-input', type: 'text', spellcheck: false, placeholder: 'Input for the program (Enter sends)', 'aria-label': 'Program input' });
    this.input.addEventListener('keydown', (e) => {
      if (e.key !== 'Enter') return;
      e.preventDefault();
      const v = this.input.value;
      this.input.value = '';
      this.append('stdin', v + '\n');
      this.opts.onInput?.(v);
    });
    this.input.hidden = !opts.onInput;
    this.searchBox = h('input', { class: 'input con-search-input', type: 'search', placeholder: 'Search output', spellcheck: false, 'aria-label': 'Search output' });
    this.searchInfo = h('span', { class: 'con-search-info' });
    this.searchBox.addEventListener('input', () => this.search(this.searchBox.value));
    this.searchBox.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        this.nextMatch(e.shiftKey ? -1 : 1);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        this.toggleSearch(false);
      }
    });
    this.searchBar = h('div', { class: 'con-search', hidden: true }, this.searchBox, this.searchInfo);
    this.el = h('div', { class: 'con' }, this.searchBar, this.body, this.input);
  }

  /** Shows or hides the input line. */
  setInputEnabled(on: boolean): void {
    this.input.hidden = !on || !this.opts.onInput;
  }

  setSoftWrap(on: boolean): void {
    this.el.classList.toggle('con-wrap', on);
  }

  get softWrap(): boolean {
    return this.el.classList.contains('con-wrap');
  }

  get lines(): number {
    return this.lineCount + (this.current ? 1 : 0);
  }

  text(): string {
    return this.body.innerText ?? this.body.textContent ?? '';
  }

  clear(): void {
    this.body.replaceChildren();
    this.current = null;
    this.pendingCR = false;
    this.lineCount = 0;
    this.parsers.clear();
    this.matches = [];
    this.matchIndex = -1;
    this.searchInfo.textContent = '';
  }

  scrollToEnd(): void {
    this.stick = true;
    this.body.scrollTop = this.body.scrollHeight;
  }

  append(stream: ConsoleStream, text: string): void {
    let p = this.parsers.get(stream);
    if (!p) {
      p = new AnsiParser();
      this.parsers.set(stream, p);
    }
    const segs: Segment[] = stream === 'system' || stream === 'stdin' ? [{ text, style: {} }] : p.feed(text);
    for (const seg of segs) this.write(stream, seg);
    this.trim();
    if (this.stick) this.body.scrollTop = this.body.scrollHeight;
  }

  private write(stream: ConsoleStream, seg: Segment): void {
    const parts = seg.text.split('\n');
    parts.forEach((part, i) => {
      if (i > 0) this.endLine();
      if (!part) return;
      const cr = part.lastIndexOf('\r');
      if (cr >= 0) {
        if (cr < part.length - 1 && this.current) this.current.replaceChildren();
        this.pendingCR = cr === part.length - 1;
        part = part.slice(cr + 1);
        if (!part) return;
      } else if (this.pendingCR && this.current) {
        this.current.replaceChildren();
        this.pendingCR = false;
      }
      this.line().appendChild(this.span(stream, seg, part));
    });
  }

  private line(): HTMLElement {
    if (!this.current) {
      this.current = h('div', { class: 'con-line' });
      this.body.appendChild(this.current);
    }
    return this.current;
  }

  private span(stream: ConsoleStream, seg: Segment, text: string): HTMLElement {
    const s = seg.style;
    const cls = [`con-${stream}`];
    let fg = s.fg;
    let bg = s.bg;
    if (s.inverse) [fg, bg] = [bg ?? 'default-bg', fg ?? 'default-fg'];
    const style: string[] = [];
    if (fg) {
      if (fg.startsWith('p')) cls.push(`ansi-fg-${fg}`);
      else if (fg === 'default-fg') cls.push('ansi-fg-default');
      else if (fg === 'default-bg') cls.push('ansi-fg-bg');
      else style.push(`color:${fg}`);
    }
    if (bg) {
      if (bg.startsWith('p')) cls.push(`ansi-bg-${bg}`);
      else if (bg === 'default-fg') cls.push('ansi-bg-fg');
      else if (bg === 'default-bg') cls.push('ansi-bg-default');
      else style.push(`background:${bg}`);
    }
    if (s.bold) cls.push('ansi-bold');
    if (s.dim) cls.push('ansi-dim');
    if (s.italic) cls.push('ansi-italic');
    if (s.underline) cls.push('ansi-underline');
    if (s.strike) cls.push('ansi-strike');
    return h('span', { class: cls.join(' '), style: style.length ? style.join(';') : undefined }, text);
  }

  /** Completes the current line: links its locations. */
  private endLine(): void {
    const line = this.line();
    this.linkify(line);
    this.current = null;
    this.pendingCR = false;
    this.lineCount++;
  }

  private linkify(line: HTMLElement): void {
    if (!this.opts.onLocation) return;
    for (const span of [...line.children] as HTMLElement[]) {
      const text = span.textContent ?? '';
      const locs = findLocations(text);
      if (!locs.length) continue;
      const nodes: (Node | string)[] = [];
      let at = 0;
      for (const l of locs) {
        if (l.start > at) nodes.push(text.slice(at, l.start));
        nodes.push(
          h('a', { class: 'con-link', href: '#', dataset: { path: l.path, line: String(l.line), col: String(l.col) }, title: `Open ${l.path}:${l.line}` }, text.slice(l.start, l.end)),
        );
        at = l.end;
      }
      if (at < text.length) nodes.push(text.slice(at));
      span.replaceChildren(...nodes);
    }
  }

  private trim(): void {
    const max = Math.max(100, this.opts.maxLines);
    let extra = this.lineCount - max;
    while (extra > 0 && this.body.firstElementChild && this.body.firstElementChild !== this.current) {
      this.body.firstElementChild.remove();
      this.lineCount--;
      extra--;
    }
  }

  // ---- Search

  toggleSearch(show?: boolean): void {
    const on = show ?? this.searchBar.hidden;
    this.searchBar.hidden = !on;
    if (on) {
      this.searchBox.focus();
      this.searchBox.select();
      if (this.searchBox.value) this.search(this.searchBox.value);
    } else {
      this.search('');
      this.body.focus();
    }
  }

  /** Marks lines containing q (case-insensitive) and selects the first. */
  search(q: string): number {
    for (const m of this.matches) m.classList.remove('con-match', 'con-match-current');
    this.matches = [];
    this.matchIndex = -1;
    const needle = q.toLowerCase();
    if (needle) {
      for (const line of this.body.children as HTMLCollectionOf<HTMLElement>) {
        if ((line.textContent ?? '').toLowerCase().includes(needle)) {
          line.classList.add('con-match');
          this.matches.push(line);
        }
      }
    }
    if (this.matches.length) this.nextMatch(1);
    else this.searchInfo.textContent = needle ? 'No matches' : '';
    return this.matches.length;
  }

  nextMatch(dir: 1 | -1): void {
    if (!this.matches.length) return;
    this.matches[this.matchIndex]?.classList.remove('con-match-current');
    this.matchIndex = (this.matchIndex + dir + this.matches.length) % this.matches.length;
    const m = this.matches[this.matchIndex];
    m.classList.add('con-match-current');
    m.scrollIntoView?.({ block: 'nearest' });
    this.stick = false;
    this.searchInfo.textContent = `${this.matchIndex + 1} of ${this.matches.length}`;
  }
}
