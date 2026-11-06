// Context / popup menus positioned at a point, with keyboard navigation.

import { h } from '../lib/dom';

export type MenuItem =
  | { label: string; hint?: string; disabled?: boolean; danger?: boolean; run: () => void }
  | { separator: true };

let open: { close(): void } | null = null;

export function showMenu(x: number, y: number, items: MenuItem[]): void {
  open?.close();
  const buttons: HTMLButtonElement[] = [];
  const menu = h(
    'div',
    { class: 'menu', role: 'menu' },
    items.map((it) => {
      if ('separator' in it) return h('div', { class: 'menu-sep', role: 'separator' });
      const b = h(
        'button',
        {
          class: `menu-item${it.danger ? ' danger' : ''}`,
          role: 'menuitem',
          disabled: !!it.disabled,
          onClick: () => {
            close();
            it.run();
          },
        },
        h('span', null, it.label),
        it.hint ? h('span', { class: 'menu-hint' }, it.hint) : null,
      );
      buttons.push(b);
      return b;
    }),
  );
  document.body.appendChild(menu);
  const r = menu.getBoundingClientRect();
  menu.style.left = `${Math.min(x, window.innerWidth - r.width - 4)}px`;
  menu.style.top = `${Math.min(y, window.innerHeight - r.height - 4)}px`;

  const enabled = (): HTMLButtonElement[] => buttons.filter((b) => !b.disabled);
  const onKey = (e: KeyboardEvent): void => {
    const list = enabled();
    const i = list.indexOf(document.activeElement as HTMLButtonElement);
    if (e.key === 'Escape') close();
    else if (e.key === 'ArrowDown') list[(i + 1) % list.length]?.focus();
    else if (e.key === 'ArrowUp') list[(i - 1 + list.length) % list.length]?.focus();
    else return;
    e.preventDefault();
    e.stopPropagation();
  };
  const onDown = (e: MouseEvent): void => {
    if (!menu.contains(e.target as Node)) close();
  };
  function close(): void {
    menu.remove();
    document.removeEventListener('keydown', onKey, true);
    document.removeEventListener('mousedown', onDown, true);
    window.removeEventListener('blur', close);
    if (open?.close === close) open = null;
  }
  document.addEventListener('keydown', onKey, true);
  document.addEventListener('mousedown', onDown, true);
  window.addEventListener('blur', close);
  open = { close };
  enabled()[0]?.focus();
}
