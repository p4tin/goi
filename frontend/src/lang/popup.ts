// A keyboard-driven list popup anchored at a point (or centered), with an
// optional filter box. Used for the declaration chooser, context actions
// and the file structure.

import { h } from '../lib/dom';

export interface ListPopupOptions<T> {
  items: readonly T[];
  /** Row content. */
  render: (item: T) => Node[];
  /** Text matched by the filter (case-insensitive subsequence). */
  text?: (item: T) => string;
  title?: string;
  filter?: boolean;
  placeholder?: string;
  /** Viewport point for the top-left corner; centered when omitted. */
  at?: { x: number; y: number };
  initial?: number;
  onPick: (item: T) => void;
  onClose?: () => void;
  empty?: string;
}

let current: (() => void) | null = null;

/** Case-insensitive subsequence match (exported for tests). */
export function fuzzyMatch(text: string, q: string): boolean {
  if (!q) return true;
  const t = text.toLowerCase();
  let i = 0;
  for (const c of q.toLowerCase()) {
    i = t.indexOf(c, i);
    if (i < 0) return false;
    i++;
  }
  return true;
}

export function closeListPopup(): void {
  current?.();
}

export function showListPopup<T>(o: ListPopupOptions<T>): () => void {
  current?.();
  const prevFocus = document.activeElement as HTMLElement | null;
  let shown: T[] = [...o.items];
  let selected = Math.max(0, Math.min(o.initial ?? 0, shown.length - 1));

  const input = h('input', { class: 'lp-filter input', type: 'text', spellcheck: false, placeholder: o.placeholder ?? 'Type to filter' });
  const list = h('ul', { class: 'lp-list', role: 'listbox' });
  const root = h(
    'div',
    { class: `lp${o.at ? '' : ' lp-centered'}`, role: 'dialog' },
    o.title ? h('div', { class: 'lp-title' }, o.title) : null,
    o.filter ? input : null,
    list,
  );

  const render = (): void => {
    list.replaceChildren(
      ...(shown.length === 0
        ? [h('li', { class: 'lp-empty' }, o.empty ?? 'Nothing to show')]
        : shown.map((it, i) =>
            h(
              'li',
              {
                class: `lp-row${i === selected ? ' selected' : ''}`,
                role: 'option',
                'aria-selected': String(i === selected),
                onMousemove: () => {
                  if (selected !== i) {
                    selected = i;
                    mark();
                  }
                },
                onMousedown: (e: MouseEvent) => {
                  e.preventDefault();
                  pick(i);
                },
              },
              o.render(it),
            ),
          )),
    );
  };
  const mark = (): void => {
    list.querySelectorAll('.lp-row').forEach((el, i) => {
      el.classList.toggle('selected', i === selected);
      el.setAttribute('aria-selected', String(i === selected));
      if (i === selected) (el as HTMLElement).scrollIntoView?.({ block: 'nearest' });
    });
  };
  const pick = (i: number): void => {
    const it = shown[i];
    if (it === undefined) return;
    close();
    o.onPick(it);
  };
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      if (shown.length) selected = (selected + (e.key === 'ArrowDown' ? 1 : -1) + shown.length) % shown.length;
      mark();
    } else if (e.key === 'Enter') {
      pick(selected);
    } else if (e.key === 'Escape') {
      close();
    } else {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
  };
  const onDown = (e: MouseEvent): void => {
    if (!root.contains(e.target as Node)) close();
  };
  input.addEventListener('input', () => {
    const q = input.value.trim();
    shown = o.items.filter((it) => fuzzyMatch(o.text ? o.text(it) : '', q));
    selected = 0;
    render();
  });

  function close(): void {
    if (current !== close) return;
    current = null;
    root.remove();
    document.removeEventListener('keydown', onKey, true);
    document.removeEventListener('mousedown', onDown, true);
    prevFocus?.focus?.();
    o.onClose?.();
  }

  render();
  document.body.appendChild(root);
  if (o.at) {
    const r = root.getBoundingClientRect();
    root.style.left = `${Math.max(4, Math.min(o.at.x, window.innerWidth - r.width - 8))}px`;
    root.style.top = `${Math.max(4, Math.min(o.at.y, window.innerHeight - r.height - 8))}px`;
  }
  document.addEventListener('keydown', onKey, true);
  document.addEventListener('mousedown', onDown, true);
  current = close;
  if (o.filter) input.focus();
  else root.tabIndex = -1;
  if (!o.filter) root.focus();
  mark();
  return close;
}
