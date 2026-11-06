// Status bar with left and right item groups ordered by priority.

import { h } from '../lib/dom';

export interface StatusItem {
  id: string;
  side: 'left' | 'right';
  /** Lower renders closer to the outer edge. */
  priority?: number;
  title?: string;
  onClick?: () => void;
}

export class StatusBar {
  readonly el: HTMLElement;
  private left = h('div', { class: 'status-left' });
  private right = h('div', { class: 'status-right' });
  private items = new Map<string, { item: StatusItem; el: HTMLElement }>();

  constructor() {
    this.el = h('footer', { class: 'statusbar' }, this.left, this.right);
  }

  /** Adds an item and returns its element for the caller to fill. */
  add(item: StatusItem): HTMLElement {
    this.remove(item.id);
    const el = h(item.onClick ? 'button' : 'span', {
      class: `status-item${item.onClick ? ' clickable' : ''}`,
      title: item.title,
      dataset: { status: item.id },
      onClick: item.onClick,
    });
    this.items.set(item.id, { item, el });
    this.layout();
    return el;
  }

  remove(id: string): void {
    const it = this.items.get(id);
    if (it) {
      it.el.remove();
      this.items.delete(id);
    }
  }

  private layout(): void {
    const sorted = [...this.items.values()].sort((a, b) => (a.item.priority ?? 100) - (b.item.priority ?? 100));
    this.left.replaceChildren(...sorted.filter((x) => x.item.side === 'left').map((x) => x.el));
    this.right.replaceChildren(...sorted.filter((x) => x.item.side === 'right').reverse().map((x) => x.el));
  }
}
