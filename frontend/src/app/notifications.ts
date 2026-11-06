// Notifications: toasts plus a history list shown in the Notifications panel.

import type { Notification } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';

export interface NotificationRecord extends Notification {
  at: number;
}

const MAX_HISTORY = 200;
const INFO_TIMEOUT_MS = 6000;

export class Notifications {
  readonly history = signal<NotificationRecord[]>([]);
  private toastHost: HTMLElement | null = null;

  /** Records a notification; toasts it when a host is attached. Duplicates
   * of the same id replace the earlier record. */
  push(n: Notification): void {
    const rec: NotificationRecord = { ...n, at: Date.now() };
    this.history.update((list) => [rec, ...list.filter((x) => x.id !== n.id || !n.id)].slice(0, MAX_HISTORY));
    if (this.toastHost) this.toast(rec);
  }

  clear(): void {
    this.history.set([]);
  }

  attachToasts(host: HTMLElement): void {
    this.toastHost = host;
  }

  private toast(n: NotificationRecord): void {
    const close = (): void => el.remove();
    const el = h(
      'div',
      { class: `toast toast-${n.level}`, role: n.level === 'error' ? 'alert' : 'status' },
      h('div', { class: 'toast-title' }, n.title),
      n.body ? h('div', { class: 'toast-body' }, n.body) : null,
      n.file ? h('button', { class: 'link-btn', onClick: () => openLocation(n) }, location(n)) : null,
      h('button', { class: 'toast-close icon-btn', title: 'Dismiss', onClick: close }, '×'),
    );
    this.toastHost!.querySelectorAll(`[data-id="${CSS.escape(n.id)}"]`).forEach((x) => x.remove());
    el.dataset.id = n.id;
    this.toastHost!.prepend(el);
    if (n.level !== 'error') setTimeout(close, INFO_TIMEOUT_MS);
  }
}

function location(n: Notification): string {
  const name = n.file!.split(/[\\/]/).pop();
  return n.line ? `${name}:${n.line}` : name!;
}

function openLocation(n: Notification): void {
  if (commands.get('editor.open')) void commands.run('editor.open', n.file, n.line);
}

/** Renders the notifications history into a panel container. */
export function renderNotificationsPanel(store: Notifications, container: HTMLElement): () => void {
  const list = h('ul', { class: 'notif-list' });
  const header = h('div', { class: 'panel-toolbar' }, h('button', { class: 'link-btn', onClick: () => store.clear() }, 'Clear all'));
  container.append(header, list);
  const redraw = (): void => {
    const items = store.history();
    list.replaceChildren(
      ...(items.length === 0
        ? [h('li', { class: 'muted' }, 'No notifications')]
        : items.map((n) =>
            h(
              'li',
              { class: `notif notif-${n.level}` },
              h('span', { class: 'notif-time' }, new Date(n.at).toLocaleTimeString()),
              h('strong', null, n.title),
              n.body ? h('span', { class: 'muted' }, ` — ${n.body}`) : null,
              n.file ? h('button', { class: 'link-btn', onClick: () => openLocation(n) }, location(n)) : null,
            ),
          )),
    );
  };
  return effect(redraw);
}
