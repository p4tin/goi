// Welcome screen: open/new project actions and the recent projects list.

import './welcome.css';
import { appInfo, project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { app, on, recentApi, Topics } from '../lib/api';
import type { RecentEntry } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { newProjectDialog } from './newProject';

export const MAX_RECENT = 20;

/** "just now", "5 min ago", "3 h ago", "2 d ago", else a locale date. */
export function relativeTime(then: Date | string, now: Date = new Date()): string {
  const t = typeof then === 'string' ? new Date(then) : then;
  const s = Math.round((now.getTime() - t.getTime()) / 1000);
  if (Number.isNaN(s)) return '';
  if (s < 60) return 'just now';
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ago`;
  const hr = Math.floor(m / 60);
  if (hr < 24) return `${hr} h ago`;
  const d = Math.floor(hr / 24);
  if (d < 30) return `${d} d ago`;
  return t.toLocaleDateString();
}

/** Shortens s to at most max chars by replacing the middle with "…". */
export function middleEllipsis(s: string, max: number): string {
  if (s.length <= max || max < 5) return s;
  const keep = max - 1;
  const head = Math.ceil(keep / 2);
  const tail = Math.floor(keep / 2);
  return `${s.slice(0, head)}…${s.slice(s.length - tail)}`;
}

/** Moves a selection index for Up/Down; -1 means nothing selected. */
export function moveSelection(current: number, key: string, count: number): number {
  if (count === 0) return -1;
  if (key === 'ArrowDown') return current < 0 ? 0 : Math.min(current + 1, count - 1);
  if (key === 'ArrowUp') return current < 0 ? count - 1 : Math.max(current - 1, 0);
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  return current;
}

export function installWelcome(wb: Workbench): void {
  const recent = signal<RecentEntry[]>([]);
  const selected = signal(-1);

  const refresh = async (): Promise<void> => {
    try {
      const list = await recentApi.list();
      recent.set(list.slice(0, MAX_RECENT));
      selected.set(-1);
    } catch (err) {
      console.warn('recent list', err);
    }
  };

  const open = async (e: RecentEntry): Promise<void> => {
    if (!e.exists) return;
    try {
      await app.openProject(e.path, !!project.peek());
    } catch (err) {
      wb.notifications.push({ id: 'welcome.open', level: 'error', title: 'Cannot open project', body: String(err) });
    }
  };

  const remove = async (e: RecentEntry): Promise<void> => {
    try {
      await recentApi.remove(e.path);
      await refresh();
    } catch (err) {
      wb.notifications.push({ id: 'welcome.remove', level: 'error', title: 'Cannot remove project', body: String(err) });
    }
  };

  commands.register({
    id: 'project.new',
    title: 'New Project…',
    category: 'File',
    run: async () => {
      const dir = await newProjectDialog();
      if (!dir) return;
      try {
        await app.openProject(dir, !!project.peek());
      } catch (err) {
        wb.notifications.push({ id: 'welcome.new', level: 'error', title: 'Cannot open new project', body: String(err) });
      }
    },
  });

  const version = h('div', { class: 'welcome-version muted' });
  const list = h('ul', { class: 'recent-list', role: 'listbox', tabindex: 0, 'aria-label': 'Recent projects' });
  const empty = h('p', { class: 'muted recent-empty' }, 'No recent projects');

  const view = h(
    'div',
    { class: 'welcome' },
    h(
      'section',
      { class: 'welcome-actions' },
      h('h1', { class: 'welcome-title' }, 'Goi'),
      version,
      h('button', { class: 'btn btn-primary', onClick: () => void commands.run('project.openFolder') }, 'Open Folder…'),
      h('button', { class: 'btn', onClick: () => void commands.run('project.new') }, 'New Project…'),
    ),
    h('section', { class: 'welcome-recent' }, h('h2', { class: 'welcome-subtitle' }, 'Recent Projects'), list, empty),
  );
  wb.welcome.append(view);

  list.addEventListener('keydown', (ev) => {
    const items = recent.peek();
    if (ev.key === 'Enter') {
      const e = items[selected.peek()];
      if (e) void open(e);
    } else if (ev.key === 'Delete' || ev.key === 'Backspace') {
      const e = items[selected.peek()];
      if (e) void remove(e);
    } else {
      const next = moveSelection(selected.peek(), ev.key, items.length);
      if (next === selected.peek()) return;
      selected.set(next);
    }
    ev.preventDefault();
  });

  effect(() => {
    const info = appInfo();
    version.textContent = info ? `Version ${info.version}` : '';
  });

  effect(() => {
    const items = recent();
    const sel = selected();
    empty.hidden = items.length > 0;
    list.hidden = items.length === 0;
    list.replaceChildren(
      ...items.map((e, i) => {
        const removeBtn = h(
          'button',
          {
            class: 'icon-btn recent-remove',
            title: 'Remove from list',
            'aria-label': `Remove ${e.name}`,
            onClick: (ev: Event) => {
              ev.stopPropagation();
              void remove(e);
            },
          },
          '×',
        );
        const row = h(
          'li',
          {
            class: `recent-row${e.exists ? '' : ' missing'}${i === sel ? ' selected' : ''}`,
            role: 'option',
            'aria-selected': String(i === sel),
            'aria-disabled': String(!e.exists),
            title: e.exists ? e.path : `${e.path} (not found)`,
            onClick: () => {
              selected.set(i);
              void open(e);
            },
          },
          h(
            'div',
            { class: 'recent-main' },
            h('span', { class: 'recent-name' }, e.name),
            e.kind ? h('span', { class: 'recent-kind' }, e.kind) : null,
            h('span', { class: 'recent-when muted' }, relativeTime(e.openedAt)),
          ),
          h('div', { class: 'recent-path muted' }, middleEllipsis(e.path, 64)),
          e.exists
            ? removeBtn
            : h(
                'button',
                {
                  class: 'link-btn recent-missing-remove',
                  onClick: (ev: Event) => {
                    ev.stopPropagation();
                    void remove(e);
                  },
                },
                'Remove',
              ),
        );
        if (i === sel) queueMicrotask(() => row.scrollIntoView?.({ block: 'nearest' }));
        return row;
      }),
    );
  });

  on(Topics.recentChanged, () => void refresh());
  effect(() => {
    if (!project()) void refresh();
  });
}
