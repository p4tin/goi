// Debugger feature (M3): breakpoints, the Debug tool window, stepping and
// inspection commands, editor decorations while stopped, and the Delve
// install offer.

import { confirmDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { debug, on, Topics } from '../lib/api';
import type { DlvStatus, RunConfig } from '../lib/api';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { launch, runHooks } from '../run/launch';
import { installBreakpoints } from './breakpoints';
import { actions, addWatch, installDebugWindow, renderDebugWindow, targetTab, watches } from './debugWindow';
import { openEvaluate } from './evaluate';
import { installDebugDecorations } from './inline';
import './debug.css';

/** Offers to install Delve when a debug launch fails because it's missing,
 * then retries the launch. */
async function offerInstall(wb: Workbench, cfg: RunConfig, err: unknown): Promise<boolean> {
  const msg = err instanceof Error ? err.message : String(err);
  if (!/dlv not found/i.test(msg)) return false;
  const ok = await confirmDialog(
    'Delve Is Not Installed',
    'Debugging Go needs Delve (dlv). Install it now? Goi runs "go install github.com/go-delve/delve/cmd/dlv@latest" into its tools folder.',
    'Install Delve',
  );
  if (!ok) return true;
  const off = on<DlvStatus>(Topics.debugInstall, (st) => {
    if (st.installing) return;
    off();
    if (st.found) {
      wb.notifications.push({ id: 'dlv-install', level: 'info', title: 'Delve installed', body: st.path });
      void launch(cfg.name, true);
    } else {
      wb.notifications.push({ id: 'dlv-install', level: 'error', title: 'Cannot install Delve', body: st.message });
    }
  });
  try {
    wb.notifications.push({ id: 'dlv-install', level: 'info', title: 'Installing Delve…', body: 'go install github.com/go-delve/delve/cmd/dlv@latest' });
    await debug.installDlv();
  } catch (e) {
    off();
    wb.notifications.push({ id: 'dlv-install', level: 'error', title: 'Cannot install Delve', body: String(e) });
  }
  return true;
}

export function installDebug(wb: Workbench): void {
  installBreakpoints(wb);
  installDebugDecorations();

  const evaluate = (t = targetTab()): void => {
    if (t) void openEvaluate(t);
  };
  installDebugWindow((t) => evaluate(t));
  wb.panels.register({ id: 'debug', title: 'Debug', dock: 'bottom', order: 6, render: (c) => renderDebugWindow(c) });

  runHooks.debugError = (cfg, err) => offerInstall(wb, cfg, err);
  runHooks.stopDebug = async () => {
    const t = targetTab();
    if (!t) return false;
    await actions.stop();
    return true;
  };

  wb.layout.register<{ watches?: string[] }>('debug', {
    save: () => ({ watches: watches.peek() }),
    restore: (v) => watches.set(Array.isArray(v?.watches) ? v.watches.filter((w) => typeof w === 'string') : []),
  });
  effect(() => {
    watches();
    wb.layout.scheduleSave();
  });

  const stopped = (): boolean => targetTab()?.state?.state === 'stopped';
  const live = (): boolean => !!targetTab();
  const cmd = (id: string, title: string, run: () => unknown, when: () => boolean = stopped): void => {
    commands.register({ id, title, category: 'Run', when, run });
  };
  // Resume, or start debugging the selected configuration when no session
  // is live (VS Code's F5).
  cmd(
    'debug.resume',
    'Resume Program',
    () => (live() ? actions.resume() : commands.run('run.debug')),
    () => !!project.peek() && (stopped() || !live()),
  );
  cmd('debug.pause', 'Pause Program', actions.pause, () => live() && !stopped());
  cmd('debug.stepOver', 'Step Over', actions.stepOver);
  cmd('debug.stepInto', 'Step Into', actions.stepInto);
  cmd('debug.forceStepInto', 'Force Step Into', actions.forceStepInto);
  cmd('debug.stepOut', 'Step Out', actions.stepOut);
  cmd('debug.runToCursor', 'Run to Cursor', actions.runToCursor);
  cmd('debug.setNextStatement', 'Set Next Statement', actions.setNextStatement);
  cmd('debug.evaluate', 'Evaluate Expression…', () => evaluate());
  cmd('debug.restart', 'Restart Debug Session', actions.restart, live);
  cmd('debug.stopSession', 'Stop Debug Session', actions.stop, live);
  commands.register({
    id: 'debug.addWatch',
    title: 'Add to Watches',
    category: 'Run',
    when: () => !!project.peek(),
    run: async () => {
      const { editors } = await import('../editor/install');
      const view = editors.activeView();
      const sel = view ? view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to).trim() : '';
      if (sel && !sel.includes('\n')) {
        addWatch(sel);
        return;
      }
      const { promptDialog } = await import('../app/dialog');
      const expr = await promptDialog('Add Watch', '', undefined, 'Expression');
      if (expr) addWatch(expr);
    },
  });
  commands.register({ id: 'debug.installDlv', title: 'Install Delve (dlv)', category: 'Run', when: () => !!project.peek(), run: () => debug.installDlv() });
  commands.register({ id: 'view.debug', title: 'Show Debug Window', category: 'View', run: () => wb.panels.show('debug') });
}
