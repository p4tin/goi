// Tool-window docks (left, right, bottom). Each dock shows one active panel
// from those registered for it, can be resized and collapsed.

import { h } from '../lib/dom';
import { signal, untracked } from '../lib/store';
import type { Signal } from '../lib/store';

export type DockId = 'left' | 'right' | 'bottom';

export interface Panel {
  id: string;
  title: string;
  dock: DockId;
  /** Lower sorts first in the dock's tab strip. */
  order?: number;
  /** Renders into container once, on first activation. Returns a disposer. */
  render(container: HTMLElement): void | (() => void);
}

export interface DockState {
  size: number;
  visible: boolean;
  active: string | null;
}

export type PanelsState = Record<DockId, DockState>;

export const defaultPanelsState = (): PanelsState => ({
  left: { size: 260, visible: true, active: null },
  right: { size: 280, visible: false, active: null },
  bottom: { size: 220, visible: false, active: null },
});

export const MIN_DOCK = 120;

/** Clamps a dock size to [MIN_DOCK, 70% of available]. */
export function clampSize(size: number, available: number): number {
  return Math.round(Math.max(MIN_DOCK, Math.min(size, available * 0.7)));
}

export class PanelRegistry {
  readonly state: Signal<PanelsState> = signal(defaultPanelsState());
  readonly panels: Signal<Panel[]> = signal<Panel[]>([]);

  register(panel: Panel): () => void {
    this.panels.update((ps) => [...ps.filter((p) => p.id !== panel.id), panel]);
    const st = this.state.peek()[panel.dock];
    if (!st.active) this.patch(panel.dock, { active: panel.id });
    return () => this.panels.update((ps) => ps.filter((p) => p !== panel));
  }

  inDock(dock: DockId): Panel[] {
    return this.panels()
      .filter((p) => p.dock === dock)
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100) || a.title.localeCompare(b.title));
  }

  patch(dock: DockId, change: Partial<DockState>): void {
    this.state.update((s) => ({ ...s, [dock]: { ...s[dock], ...change } }));
  }

  toggle(dock: DockId): void {
    this.patch(dock, { visible: !this.state.peek()[dock].visible });
  }

  /** Shows the dock holding panelId and activates it. */
  show(panelId: string): void {
    const p = this.panels.peek().find((x) => x.id === panelId);
    if (p) this.patch(p.dock, { visible: true, active: p.id });
  }

  restore(saved: Partial<PanelsState>): void {
    const base = defaultPanelsState();
    for (const dock of ['left', 'right', 'bottom'] as DockId[]) {
      const s = saved[dock];
      if (s && typeof s.size === 'number') base[dock] = { ...base[dock], ...s };
    }
    // Keep a valid active panel per dock.
    for (const dock of ['left', 'right', 'bottom'] as DockId[]) {
      const ids = this.panels.peek().filter((p) => p.dock === dock).map((p) => p.id);
      if (base[dock].active && !ids.includes(base[dock].active!) && ids.length) base[dock].active = ids[0];
      if (!base[dock].active && ids.length) base[dock].active = ids[0];
    }
    this.state.set(base);
  }
}

/** Builds the DOM for one dock. Returns the element and an update function. */
export function renderDock(reg: PanelRegistry, dock: DockId): { el: HTMLElement; update(): void } {
  const tabs = h('div', { class: 'dock-tabs', role: 'tablist' });
  const hide = h('button', { class: 'dock-hide icon-btn', title: 'Hide', onClick: () => reg.toggle(dock) }, '–');
  const body = h('div', { class: 'dock-body' });
  const el = h('section', { class: `dock dock-${dock}`, dataset: { dock } }, h('header', { class: 'dock-header' }, tabs, hide), body);
  const mounted = new Map<string, HTMLElement>();

  function update(): void {
    const st = reg.state()[dock];
    const panels = reg.inDock(dock);
    el.hidden = !st.visible || panels.length === 0;
    tabs.replaceChildren(
      ...panels.map((p) =>
        h(
          'button',
          {
            class: `dock-tab${p.id === st.active ? ' active' : ''}`,
            role: 'tab',
            'aria-selected': String(p.id === st.active),
            onClick: () => reg.patch(dock, { active: p.id }),
          },
          p.title,
        ),
      ),
    );
    for (const p of panels) {
      let c = mounted.get(p.id);
      if (!c && p.id === st.active && st.visible) {
        c = h('div', { class: 'panel', dataset: { panel: p.id } });
        body.appendChild(c);
        mounted.set(p.id, c);
        const target = c;
        untracked(() => p.render(target));
      }
      if (c) c.hidden = p.id !== st.active;
    }
  }
  return { el, update };
}

/** A drag handle that resizes a dock. */
export function splitter(reg: PanelRegistry, dock: DockId, available: () => number): HTMLElement {
  const vertical = dock === 'bottom';
  const el = h('div', { class: `splitter ${vertical ? 'splitter-h' : 'splitter-v'}`, role: 'separator' });
  el.addEventListener('pointerdown', (down: PointerEvent) => {
    down.preventDefault();
    el.setPointerCapture(down.pointerId);
    const start = vertical ? down.clientY : down.clientX;
    const startSize = reg.state.peek()[dock].size;
    const move = (e: PointerEvent): void => {
      const delta = (vertical ? e.clientY : e.clientX) - start;
      const sign = dock === 'left' ? 1 : -1;
      reg.patch(dock, { size: clampSize(startSize + sign * delta, available()) });
    };
    const up = (): void => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', up);
  });
  return el;
}
