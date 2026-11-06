// Renders the editor split tree: nested flex containers with draggable
// dividers. Pane elements are re-parented, so their views stay alive.

import { h } from '../lib/dom';
import { effect } from '../lib/store';
import type { EditorManager, Pane, SplitNode } from './editorManager';
import { renderTabs } from './tabs';

const MIN_RATIO = 0.15;

export function clampRatio(r: number): number {
  return Math.min(1 - MIN_RATIO, Math.max(MIN_RATIO, r));
}

export function renderPanes(m: EditorManager, host: HTMLElement, onResize: () => void): void {
  const tabsRendered = new WeakSet<Pane>();

  effect(() => {
    const tree = m.tree();
    for (const p of m.panes()) {
      if (!tabsRendered.has(p)) {
        tabsRendered.add(p);
        renderTabs(m, p);
      }
    }
    host.replaceChildren(build(tree, onResize));
    for (const p of m.panes.peek()) for (const v of p.views.values()) v.requestMeasure();
  });

  effect(() => {
    const active = m.activePane();
    const many = m.panes().length > 1;
    for (const p of m.panes.peek()) p.el.classList.toggle('pane-active', many && p === active);
  });
}

function build(node: SplitNode, onResize: () => void): HTMLElement {
  if (node.kind === 'pane') return node.pane.el;
  const a = build(node.a, onResize);
  const b = build(node.b, onResize);
  const el = h('div', { class: `split split-${node.dir}` });
  const apply = (): void => {
    a.style.flex = `${node.ratio} 1 0`;
    b.style.flex = `${1 - node.ratio} 1 0`;
  };
  apply();
  const divider = h('div', { class: `pane-divider pane-divider-${node.dir}`, role: 'separator' });
  divider.addEventListener('pointerdown', (down: PointerEvent) => {
    down.preventDefault();
    divider.setPointerCapture(down.pointerId);
    const rect = el.getBoundingClientRect();
    const move = (e: PointerEvent): void => {
      const r = node.dir === 'row' ? (e.clientX - rect.left) / rect.width : (e.clientY - rect.top) / rect.height;
      // The tree node is mutated in place: resizing is not a structural change.
      node.ratio = clampRatio(r);
      apply();
    };
    const up = (): void => {
      divider.removeEventListener('pointermove', move);
      divider.removeEventListener('pointerup', up);
      onResize();
    };
    divider.addEventListener('pointermove', move);
    divider.addEventListener('pointerup', up);
  });
  el.append(a, divider, b);
  return el;
}
