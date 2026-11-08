// The main window shell: docks around the editor area, a status bar, toasts,
// and the Welcome host shown when no project is open.

import { app, on, Topics } from '../lib/api';
import type { Notification } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { runCloseChecks } from './closeChecks';
import { LayoutStore } from './layout';
import { Notifications, renderNotificationsPanel } from './notifications';
import { PanelRegistry, renderDock, splitter } from './panels';
import type { DockId, PanelsState } from './panels';
import { project } from './state';
import { StatusBar } from './statusbar';

export interface Workbench {
  /** Root of the project window (hidden on the Welcome screen). */
  root: HTMLElement;
  /** Host for the Welcome screen. */
  welcome: HTMLElement;
  /** Editor area. */
  center: HTMLElement;
  panels: PanelRegistry;
  status: StatusBar;
  layout: LayoutStore;
  notifications: Notifications;
}

export function createWorkbench(): Workbench {
  const panels = new PanelRegistry();
  const status = new StatusBar();
  const notifications = new Notifications();
  const layout = new LayoutStore({
    save: (l) => (project.peek() ? app.saveLayout(l) : Promise.resolve()),
    load: () => app.loadLayout<Record<string, unknown>>(),
  });

  const center = h('main', { class: 'editor-area' });
  const main = h('div', { class: 'wb-main' });
  const centerCol = h('div', { class: 'wb-center' });
  const docks = {} as Record<DockId, ReturnType<typeof renderDock>>;
  for (const d of ['left', 'right', 'bottom'] as DockId[]) docks[d] = renderDock(panels, d);

  const width = (): number => main.clientWidth || window.innerWidth;
  const height = (): number => centerCol.clientHeight || window.innerHeight;
  const leftSplit = splitter(panels, 'left', width);
  const rightSplit = splitter(panels, 'right', width);
  const bottomSplit = splitter(panels, 'bottom', height);
  centerCol.append(center, bottomSplit, docks.bottom.el);
  main.append(docks.left.el, leftSplit, centerCol, rightSplit, docks.right.el);

  const toasts = h('div', { class: 'toasts', 'aria-live': 'polite' });
  notifications.attachToasts(toasts);
  const root = h('div', { class: 'wb', hidden: true }, main, status.el);
  const welcome = h('div', { class: 'welcome-host', hidden: true });
  document.body.appendChild(toasts);

  effect(() => {
    const st = panels.state();
    panels.panels();
    for (const d of ['left', 'right', 'bottom'] as DockId[]) {
      docks[d].update();
      const el = docks[d].el;
      if (d === 'bottom') el.style.height = `${st[d].size}px`;
      else el.style.width = `${st[d].size}px`;
    }
    leftSplit.hidden = docks.left.el.hidden;
    rightSplit.hidden = docks.right.el.hidden;
    bottomSplit.hidden = docks.bottom.el.hidden;
    layout.scheduleSave();
  });

  layout.register<PanelsState>('panels', {
    save: () => panels.state.peek(),
    restore: (v) => panels.restore(v),
  });

  on<Notification>(Topics.notify, (n) => notifications.push(n));

  const wb: Workbench = { root, welcome, center, panels, status, layout, notifications };
  installShellCommands(wb);
  installShellPanels(wb);
  return wb;
}

/** Shows the Welcome screen or the workbench as the project opens/closes,
 * restoring the project's saved layout. */
export function routeWorkbench(wb: Workbench): void {
  let lastRoot: string | null = null;
  effect(() => {
    const p = project();
    wb.root.hidden = !p;
    wb.welcome.hidden = !!p;
    document.title = p ? `${p.name} — Goi` : 'Goi';
    if (p && p.root !== lastRoot) {
      lastRoot = p.root;
      void wb.layout.load();
    } else if (!p) {
      lastRoot = null;
    }
  });
}

function installShellCommands(wb: Workbench): void {
  const toggle = (dock: DockId, title: string): void => {
    commands.register({ id: `view.toggle${title}`, title: `Toggle ${title} Panel`, category: 'View', run: () => wb.panels.toggle(dock) });
  };
  toggle('left', 'Left');
  toggle('right', 'Right');
  toggle('bottom', 'Bottom');
  commands.register({
    id: 'view.notifications',
    title: 'Show Notifications',
    category: 'View',
    run: () => wb.panels.show('notifications'),
  });
  commands.register({
    id: 'project.openFolder',
    title: 'Open Folder…',
    category: 'File',
    run: async () => {
      const dir = await app.openFolderDialog();
      if (!dir) return;
      try {
        await app.openProject(dir, !!project.peek());
      } catch (err) {
        wb.notifications.push({ id: 'open', level: 'error', title: 'Cannot open folder', body: String(err) });
      }
    },
  });
  commands.register({
    id: 'project.close',
    title: 'Close Project',
    category: 'File',
    when: () => !!project.peek(),
    run: async () => {
      if (!(await runCloseChecks())) return;
      await wb.layout.flush();
      await app.closeProject();
    },
  });
}

function installShellPanels(wb: Workbench): void {
  wb.panels.register({
    id: 'notifications',
    title: 'Notifications',
    dock: 'bottom',
    order: 90,
    render: (c) => renderNotificationsPanel(wb.notifications, c),
  });
  const name = wb.status.add({ id: 'project', side: 'left', priority: 0, title: 'Project' });
  effect(() => {
    const p = project();
    name.textContent = p ? `${p.name} · ${p.kind}` : '';
  });
}
