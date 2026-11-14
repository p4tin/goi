// Native menu bar: menus are lists of command ids; labels come from the
// command registry and shortcuts from the active keymap. The backend builds
// the native menu (adding the macOS app/Edit/Window menus) and reports picks
// on menu.command.

import { bindings } from '../keymap/install';
import { parseSequence } from '../keymap/chords';
import { menuApi, on, Topics } from '../lib/api';
import type { MenuCommandEvent, MenuSpec } from '../lib/api';
import type { CommandRegistry } from '../lib/commands';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { isMac } from './state';
import type { Workbench } from './workbench';

/** A menu definition: '-' is a separator. */
export interface MenuDef {
  label: string;
  items: string[];
}

export const MENUS: MenuDef[] = [
  {
    label: 'File',
    items: [
      'project.new', 'project.openFolder', 'project.openInNewWindow', '-',
      'file.save', 'file.saveAll', '-',
      'editor.close', 'editor.closeAll', 'project.close', '-',
      'settings.open', 'keymap.switch',
    ],
  },
  {
    label: 'Code',
    items: [
      'editor.find', 'editor.replace', 'search.findInPath', 'search.replaceInPath', '-',
      'lang.contextActions', 'lang.quickDoc', 'lang.parameterInfo', '-',
      'lang.findUsages', 'lang.rename', '-',
      'editor.reformat', 'editor.toggleSoftWrap', '-',
      'lang.restart', 'lang.installGopls',
    ],
  },
  {
    label: 'View',
    items: [
      'view.toggleLeft', 'view.toggleBottom', 'view.toggleRight', 'tree.focus', 'view.notifications', '-',
      'view.problems', 'view.usages', 'view.find', 'view.run', 'view.debug', 'terminal.toggle', 'terminal.new', 'view.commit', 'view.git', 'agent.show', '-',
      'editor.splitRight', 'editor.splitDown', 'editor.closePane', 'editor.focusNextPane', 'editor.focusPrevPane', '-',
      'view.toggleDarkMode', 'view.themeDark', 'view.themeLight', 'view.themeAuto', 'view.themeHighContrast',
    ],
  },
  {
    label: 'Navigate',
    items: [
      'nav.searchEverywhere', 'nav.gotoFile', 'nav.gotoSymbol', 'nav.gotoAction', 'nav.recentFiles', '-',
      'lang.gotoDefinition', 'lang.gotoTypeDefinition', 'lang.gotoImplementation', 'lang.gotoTest', 'lang.fileStructure', '-',
      'nav.back', 'nav.forward', '-',
      'editor.nextTab', 'editor.prevTab',
    ],
  },
  {
    label: 'Run',
    items: [
      'run.run', 'run.debug', 'run.runContext', 'run.debugContext', 'run.rerun', 'run.stop', '-',
      'run.selectConfiguration', 'run.editConfigurations', 'run.saveConfiguration', '-',
      'debug.resume', 'debug.pause', 'debug.stepOver', 'debug.stepInto', 'debug.forceStepInto', 'debug.stepOut',
      'debug.runToCursor', 'debug.setNextStatement', 'debug.evaluate', 'debug.addWatch', '-',
      'debug.toggleBreakpoint', 'debug.editBreakpoint', 'debug.viewBreakpoints', 'debug.muteBreakpoints', '-',
      'debug.installDlv',
    ],
  },
  {
    label: 'Git',
    items: [
      'git.commit', 'git.commitModal', 'git.push', 'git.pull', 'git.fetch', '-',
      'git.branches', 'git.stash', 'git.stashes', '-',
      'git.showDiff', 'git.compareWithHead', 'git.annotate', 'git.fileHistory', 'git.log', '-',
      'git.refresh', 'git.init',
    ],
  },
  {
    label: 'Tools',
    items: [
      'lint.runProject', 'lint.runPackage', 'lint.install', '-',
      'mod.tidy', 'mod.download', 'mod.vendor', 'mod.init', '-',
      'mod.workInit', 'mod.workUse', 'mod.workSync', '-',
      'agent.newSession', 'agent.askSelection', 'agent.stop', 'agent.install', 'agent.copyMcpConfig',
    ],
  },
  { label: 'Help', items: ['nav.gotoAction', '-', 'help.about'] },
];

const KEY_NAMES: Record<string, string> = {
  ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right', Enter: 'return', Escape: 'escape',
  Backspace: 'backspace', Delete: 'delete', Tab: 'tab', Space: 'space', Home: 'home', End: 'end',
  PageUp: 'page up', PageDown: 'page down', Comma: ',', Period: '.', Slash: '/', Backslash: '\\',
  Backquote: '`', BracketLeft: '[', BracketRight: ']', Semicolon: ';', Quote: "'", Equal: '=', Minus: '-',
};

/** Wails accelerator ("cmdorctrl+shift+1") for a resolved key sequence, or
 * "" when the menu can't show it (chords, Meta outside macOS, odd keys). */
export function toAccelerator(seq: string, mac: boolean): string {
  const strokes = parseSequence(seq);
  if (strokes.length !== 1) return '';
  const s = strokes[0];
  const key = KEY_NAMES[s.key] ?? (/^F\d{1,2}$/.test(s.key) || s.key.length === 1 ? s.key.toLowerCase() : '');
  if (!key) return '';
  const mods: string[] = [];
  if (mac) {
    if (s.meta || s.mod) mods.push('cmdorctrl');
    if (s.ctrl) mods.push('ctrl');
  } else {
    if (s.meta) return '';
    if (s.ctrl || s.mod) mods.push('cmdorctrl');
  }
  if (s.alt) mods.push('optionoralt');
  if (s.shift) mods.push('shift');
  return [...mods, key].join('+');
}

/** Menu specs for the registered commands: unknown ids are skipped, and
 * separators never lead, trail or repeat; empty menus are dropped. */
export function buildMenuSpecs(
  defs: MenuDef[],
  registry: Pick<CommandRegistry, 'get'>,
  byCmd: Map<string, string[]>,
  mac: boolean,
): MenuSpec[] {
  const out: MenuSpec[] = [];
  for (const def of defs) {
    const items: MenuSpec['items'] = [];
    let pendingSep = false;
    for (const id of def.items) {
      if (id === '-') {
        pendingSep = items.length > 0;
        continue;
      }
      const cmd = registry.get(id);
      if (!cmd) continue;
      if (pendingSep) items.push({ label: '', command: '', accel: '', separator: true });
      pendingSep = false;
      const seq = byCmd.get(id)?.find((s) => toAccelerator(s, mac) !== '');
      items.push({ label: cmd.title, command: id, accel: seq ? toAccelerator(seq, mac) : '', separator: false });
    }
    if (items.length) out.push({ label: def.label, items });
  }
  return out;
}

export function installMenuBar(_wb: Workbench): void {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let last = '';
  const push = (): void => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      const specs = buildMenuSpecs(MENUS, commands, bindings.peek().byCmd, isMac());
      const key = JSON.stringify(specs);
      if (key === last) return;
      last = key;
      menuApi.set(specs).catch((err: unknown) => console.warn('menu', err));
    }, 50);
  };
  effect(() => {
    bindings();
    push();
  });
  commands.onDidChange(push);
  on<MenuCommandEvent>(Topics.menuCommand, (e) => {
    if (!commands.get(e.command)) return;
    commands.runFrom('menu', e.command).catch((err: unknown) => console.error('command', e.command, err));
  });
}
