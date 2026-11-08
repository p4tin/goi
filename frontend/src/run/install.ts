// Run feature (M3): run configurations, the run widget, the Run tool window
// and run commands.

import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { on, run, Topics } from '../lib/api';
import type { RunConfig } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import { openConfigDialog } from './configDialog';
import { icon, iconButton } from './icons';
import { activeTab, liveTab, renderRunWindow, rerunTab, setTestViewFactory, stopTab } from './runWindow';
import { createTestView } from './testView';
import { configs, notifyError, selectedConfig, selectedName, setRunWorkbench, TYPE_LABEL } from './state';
import { launch, runHooks } from './launch';
import './run.css';

/** Runs or debugs the selected configuration, asking to pick one first. */
async function launchSelected(debug: boolean): Promise<void> {
  const c = selectedConfig();
  if (c) return launch(c.name, debug);
  if (!configs.peek().length) return openConfigDialog();
  chooseConfig(window.innerWidth - 320, window.innerHeight - 40, debug);
}

function chooseConfig(x: number, y: number, launchDebug?: boolean): void {
  const items: MenuItem[] = [];
  for (const c of configs.peek()) {
    items.push({
      label: `${c.temporary ? '◌ ' : ''}${c.name}`,
      hint: TYPE_LABEL[c.type],
      run: () => {
        selectedName.set(c.name);
        if (launchDebug !== undefined) void launch(c.name, launchDebug);
      },
    });
  }
  if (items.length) items.push({ separator: true });
  items.push({ label: 'Edit Configurations…', run: () => void openConfigDialog() });
  const sel = selectedConfig();
  if (sel?.temporary) items.push({ label: `Save '${sel.name}' Configuration`, run: () => void saveSelected() });
  showMenu(x, y, items);
}

async function saveSelected(): Promise<void> {
  const c = selectedConfig();
  if (!c?.temporary) return;
  try {
    const saved = await run.saveConfig({ ...c, temporary: false, created: 0 }, c.name);
    configs.set(await run.configs());
    selectedName.set(saved.name);
  } catch (err) {
    notifyError('run-save', 'Cannot save configuration', err);
  }
}

async function stopAny(): Promise<void> {
  if (runHooks.stopDebug && (await runHooks.stopDebug())) return;
  const t = liveTab();
  if (t) await stopTab(t);
}

function installWidget(wb: Workbench): void {
  const el = wb.status.add({ id: 'run', side: 'right', priority: 40, title: 'Run configuration' });
  const name = h('span', { class: 'rw-name' });
  const pick = h('button', { class: 'rw-pick', title: 'Select run configuration', type: 'button' }, name, icon('chevron'));
  pick.addEventListener('click', (e) => {
    e.stopPropagation();
    const r = pick.getBoundingClientRect();
    chooseConfig(r.left, r.top - 8 - Math.min(400, (configs.peek().length + 3) * 26));
  });
  const runBtn = iconButton('run', 'Run', () => void launchSelected(false), 'rw-run');
  const debugBtn = iconButton('debug', 'Debug', () => void launchSelected(true), 'rw-debug');
  const stopBtn = iconButton('stop', 'Stop', () => void stopAny(), 'rw-stop');
  el.classList.add('run-widget');
  el.append(pick, runBtn, debugBtn, stopBtn);
  effect(() => {
    const p = project();
    const c = selectedConfig();
    el.hidden = !p;
    name.textContent = c ? c.name : configs().length ? 'Select configuration' : 'Add configuration…';
    debugBtn.disabled = !!c && !(c.type === 'app' || c.type === 'test' || c.type === 'remote');
    runBtn.disabled = !!c && c.type === 'remote';
  });
}

export function installRun(wb: Workbench): void {
  setRunWorkbench(wb);
  setTestViewFactory(createTestView);
  wb.panels.register({ id: 'run', title: 'Run', dock: 'bottom', order: 5, render: (c) => renderRunWindow(c) });
  installWidget(wb);

  const refresh = async (): Promise<void> => {
    if (!project.peek()) {
      configs.set([]);
      return;
    }
    try {
      configs.set(await run.configs());
    } catch {
      configs.set([]);
    }
  };
  effect(() => {
    project();
    void refresh();
  });
  on<RunConfig[]>(Topics.runConfigs, (list) => configs.set(list));

  wb.layout.register<{ selected?: string }>('run', {
    save: () => ({ selected: selectedName.peek() }),
    restore: (v) => {
      if (v && typeof v.selected === 'string') selectedName.set(v.selected);
    },
  });
  effect(() => {
    selectedName();
    wb.layout.scheduleSave();
  });

  const hasProject = (): boolean => !!project.peek();
  commands.register({ id: 'run.run', title: 'Run', category: 'Run', when: hasProject, run: () => launchSelected(false) });
  commands.register({ id: 'run.debug', title: 'Debug', category: 'Run', when: hasProject, run: () => launchSelected(true) });
  commands.register({ id: 'run.stop', title: 'Stop', category: 'Run', when: hasProject, run: () => stopAny() });
  commands.register({ id: 'run.rerun', title: 'Rerun', category: 'Run', when: () => !!activeTab(), run: () => rerunTab() });
  commands.register({
    id: 'run.editConfigurations',
    title: 'Edit Configurations…',
    category: 'Run',
    when: hasProject,
    run: () => openConfigDialog(),
  });
  commands.register({
    id: 'run.selectConfiguration',
    title: 'Select Run Configuration…',
    category: 'Run',
    when: hasProject,
    run: () => chooseConfig(window.innerWidth / 2 - 150, window.innerHeight / 3),
  });
  commands.register({
    id: 'run.saveConfiguration',
    title: 'Save Temporary Configuration',
    category: 'Run',
    when: () => !!selectedConfig()?.temporary,
    run: () => saveSelected(),
  });
  commands.register({ id: 'view.run', title: 'Show Run Window', category: 'View', run: () => wb.panels.show('run') });
}
