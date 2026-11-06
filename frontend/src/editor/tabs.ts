// Tab strip for a pane: activation, close, pin, drag reorder, context menu.

import { showMenu } from '../app/menu';
import { files } from '../lib/api';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import type { EditorManager, Pane } from './editorManager';
import { tabLabels } from './editorManager';

export function renderTabs(m: EditorManager, pane: Pane): () => void {
  let dragFrom = -1;
  return effect(() => {
    const tabs = pane.tabs();
    const active = pane.active();
    const labels = tabLabels(tabs.map((t) => t.path));
    pane.tabBar.replaceChildren(
      ...tabs.map((t, i) => {
        const buf = m.buffers.get(t.path);
        const dirty = buf?.dirty() ?? false;
        const el = h(
          'div',
          {
            class: `tab${t.path === active ? ' active' : ''}${t.pinned ? ' pinned' : ''}${dirty ? ' dirty' : ''}`,
            role: 'tab',
            title: t.path,
            draggable: true,
            'aria-selected': String(t.path === active),
          },
          t.pinned ? h('span', { class: 'tab-pin', 'aria-label': 'Pinned' }, '•') : null,
          h('span', { class: 'tab-label' }, labels.get(t.path) ?? t.path),
          h(
            'button',
            {
              class: 'tab-close icon-btn',
              title: 'Close',
              tabIndex: -1,
              onClick: (e: MouseEvent) => {
                e.stopPropagation();
                void m.close(t.path, pane);
              },
            },
            dirty ? '●' : '×',
          ),
        );
        el.addEventListener('mousedown', (e) => {
          if (e.button === 0) {
            m.activate(pane, t.path);
            queueMicrotask(() => pane.views.get(t.path)?.focus());
          }
        });
        el.addEventListener('auxclick', (e) => {
          if (e.button === 1) void m.close(t.path, pane);
        });
        el.addEventListener('contextmenu', (e) => {
          e.preventDefault();
          tabMenu(m, pane, t.path, i, e.clientX, e.clientY);
        });
        el.addEventListener('dragstart', (e) => {
          dragFrom = i;
          e.dataTransfer?.setData('text/plain', t.path);
        });
        el.addEventListener('dragover', (e) => e.preventDefault());
        el.addEventListener('drop', (e) => {
          e.preventDefault();
          if (dragFrom >= 0 && dragFrom !== i) m.moveTab(pane, dragFrom, i);
          dragFrom = -1;
        });
        return el;
      }),
    );
  });
}

function tabMenu(m: EditorManager, pane: Pane, path: string, index: number, x: number, y: number): void {
  const pinned = pane.tabs.peek().find((t) => t.path === path)?.pinned;
  showMenu(x, y, [
    { label: 'Close', run: () => void m.close(path, pane) },
    { label: 'Close Others', run: () => void m.closeWhere(pane, (t) => t.path !== path) },
    { label: 'Close Tabs to the Right', run: () => void m.closeWhere(pane, (_, i) => i > index) },
    { label: 'Close All', run: () => void m.closeWhere(pane, () => true) },
    { separator: true },
    { label: pinned ? 'Unpin Tab' : 'Pin Tab', run: () => m.togglePin(pane, path) },
    { separator: true },
    { label: 'Copy Path', run: () => void files.copyPath(path) },
    { label: 'Reveal in File Manager', run: () => void files.reveal(path) },
  ]);
}
