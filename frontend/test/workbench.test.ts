// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';
import { LayoutStore } from '../src/app/layout';
import { Notifications } from '../src/app/notifications';
import { clampSize, MIN_DOCK, PanelRegistry, renderDock } from '../src/app/panels';
import { StatusBar } from '../src/app/statusbar';

describe('LayoutStore', () => {
  it('restores registered parts from saved layout and debounces saves', async () => {
    vi.useFakeTimers();
    const saved: Record<string, unknown>[] = [];
    const store = new LayoutStore(
      { save: async (l) => void saved.push(l), load: async () => ({ a: 1, other: 'kept' }) },
      100,
    );
    let a = 0;
    store.register<number>('a', { save: () => a, restore: (v) => (a = v) });
    await store.load();
    expect(a).toBe(1);
    let b = 'x';
    store.register<string>('b', { save: () => b, restore: (v) => (b = v) });
    a = 5;
    b = 'y';
    store.scheduleSave();
    store.scheduleSave();
    await vi.advanceTimersByTimeAsync(150);
    expect(saved).toEqual([{ a: 5, b: 'y', other: 'kept' }]);
    vi.useRealTimers();
  });

  it('restores late-registered parts and tolerates bad values', async () => {
    const store = new LayoutStore({ save: async () => {}, load: async () => ({ p: 'v', bad: 1 }) }, 10);
    await store.load();
    let got = '';
    store.register<string>('p', { save: () => got, restore: (v) => (got = v) });
    expect(got).toBe('v');
    expect(() =>
      store.register('bad', {
        save: () => 0,
        restore: () => {
          throw new Error('boom');
        },
      }),
    ).not.toThrow();
  });
});

describe('PanelRegistry', () => {
  it('activates the first panel per dock and toggles/shows', () => {
    const reg = new PanelRegistry();
    reg.register({ id: 'project', title: 'Project', dock: 'left', render: () => {} });
    reg.register({ id: 'problems', title: 'Problems', dock: 'bottom', render: () => {} });
    reg.register({ id: 'notifications', title: 'Notifications', dock: 'bottom', render: () => {} });
    expect(reg.state.peek().left.active).toBe('project');
    expect(reg.state.peek().bottom.active).toBe('problems');
    reg.toggle('left');
    expect(reg.state.peek().left.visible).toBe(false);
    reg.show('notifications');
    expect(reg.state.peek().bottom).toMatchObject({ visible: true, active: 'notifications' });
  });

  it('restore keeps valid active panels and sizes', () => {
    const reg = new PanelRegistry();
    reg.register({ id: 'project', title: 'Project', dock: 'left', render: () => {} });
    reg.restore({ left: { size: 333, visible: false, active: 'gone' } });
    expect(reg.state.peek().left).toEqual({ size: 333, visible: false, active: 'project' });
    expect(reg.state.peek().right.size).toBe(280);
  });

  it('renders panels lazily once and hides inactive ones', () => {
    const reg = new PanelRegistry();
    const renders: string[] = [];
    reg.register({ id: 'a', title: 'A', dock: 'bottom', render: (c) => void (renders.push('a'), (c.textContent = 'A')) });
    reg.register({ id: 'b', title: 'B', dock: 'bottom', render: (c) => void (renders.push('b'), (c.textContent = 'B')) });
    const dock = renderDock(reg, 'bottom');
    dock.update();
    expect(dock.el.hidden).toBe(true); // bottom hidden by default
    reg.patch('bottom', { visible: true });
    dock.update();
    reg.patch('bottom', { active: 'b' });
    dock.update();
    reg.patch('bottom', { active: 'a' });
    dock.update();
    expect(renders).toEqual(['a', 'b']);
    expect(dock.el.querySelectorAll('.dock-tab').length).toBe(2);
    expect((dock.el.querySelector('[data-panel="b"]') as HTMLElement).hidden).toBe(true);
  });

  it('clampSize bounds sizes', () => {
    expect(clampSize(10, 1000)).toBe(MIN_DOCK);
    expect(clampSize(900, 1000)).toBe(700);
    expect(clampSize(300, 1000)).toBe(300);
  });
});

describe('StatusBar', () => {
  it('orders items by side and priority', () => {
    const sb = new StatusBar();
    sb.add({ id: 'b', side: 'left', priority: 2 }).textContent = 'b';
    sb.add({ id: 'a', side: 'left', priority: 1 }).textContent = 'a';
    sb.add({ id: 'z', side: 'right', priority: 1 }).textContent = 'z';
    sb.add({ id: 'y', side: 'right', priority: 2 }).textContent = 'y';
    expect(sb.el.querySelector('.status-left')!.textContent).toBe('ab');
    expect(sb.el.querySelector('.status-right')!.textContent).toBe('yz');
    sb.remove('a');
    expect(sb.el.querySelector('.status-left')!.textContent).toBe('b');
  });
});

describe('Notifications', () => {
  it('keeps newest first and replaces same id', () => {
    const n = new Notifications();
    n.push({ id: 'x', level: 'info', title: 'one' });
    n.push({ id: 'y', level: 'warn', title: 'two' });
    n.push({ id: 'x', level: 'error', title: 'three' });
    expect(n.history.peek().map((r) => r.title)).toEqual(['three', 'two']);
  });

  it('toasts into an attached host', () => {
    const n = new Notifications();
    const host = document.createElement('div');
    n.attachToasts(host);
    n.push({ id: 'e', level: 'error', title: 'Bad', body: 'details', file: '/a/b/go.mod', line: 3 });
    expect(host.querySelector('.toast-error')!.textContent).toContain('go.mod:3');
  });
});
