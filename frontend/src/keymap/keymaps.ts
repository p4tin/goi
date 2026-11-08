// Built-in keymaps and effective-binding computation (base keymap plus user
// overrides from the `keys` setting).

import { resolveMod } from './chords';

export type KeymapId = 'goi-default' | 'vscode';

/** Bindings for a command: on all platforms, or split by platform. */
export type Entry = string[] | { all?: string[]; mac?: string[]; other?: string[] };

export interface Keymap {
  name: string;
  bindings: Record<string, Entry>;
}

const shared: Record<string, Entry> = {
  'file.save': ['Mod-s'],
  'file.saveAll': ['Mod-Alt-s'],
  'project.openFolder': ['Mod-o'],
  'editor.nextTab': ['Ctrl-Tab'],
  'editor.prevTab': ['Ctrl-Shift-Tab'],
  'editor.find': ['Mod-f'],
  'editor.splitRight': ['Mod-Backslash'],
  'editor.splitDown': ['Mod-k Mod-Backslash'],
  'editor.focusNextPane': ['Mod-k Mod-ArrowRight'],
  'editor.focusPrevPane': ['Mod-k Mod-ArrowLeft'],
  'editor.toggleSoftWrap': ['Alt-z'],
  'nav.recentFiles': ['Mod-e'],
  'view.toggleBottom': ['Mod-j'],
  'settings.open': ['Mod-Comma'],
  'search.findInPath': ['Mod-Shift-f'],
};

export const keymaps: Record<KeymapId, Keymap> = {
  'goi-default': {
    name: 'Goi Default',
    bindings: {
      ...shared,
      'editor.close': { all: ['Mod-w'], other: ['Mod-w', 'Mod-F4'] },
      'editor.replace': ['Mod-r'],
      'editor.gotoLine': ['Mod-l'],
      'editor.reformat': ['Mod-Alt-l'],
      'nav.gotoFile': ['Mod-Shift-o'],
      'nav.gotoAction': ['Mod-Shift-a'],
      'view.toggleLeft': ['Mod-1'],
      'tree.focus': ['Mod-Shift-1'],
      'lang.gotoDefinition': ['Mod-b'],
      'lang.gotoTypeDefinition': ['Mod-Shift-b'],
      'lang.gotoImplementation': ['Mod-Alt-b'],
      'lang.gotoTest': ['Mod-Shift-t'],
      'nav.back': ['Mod-BracketLeft'],
      'nav.forward': ['Mod-BracketRight'],
      'nav.gotoSymbol': ['Mod-Alt-o'],
      'lang.fileStructure': ['Mod-F12'],
      'lang.findUsages': ['Alt-F7'],
      'lang.quickDoc': { mac: ['F1'], other: ['Ctrl-q'] },
      'lang.parameterInfo': ['Mod-p'],
      'lang.rename': ['Shift-F6'],
      'lang.contextActions': ['Alt-Enter'],
      'search.replaceInPath': ['Mod-Shift-r'],
      'run.run': { mac: ['Ctrl-r'], other: ['Shift-F10'] },
      'run.debug': { mac: ['Ctrl-d'], other: ['Shift-F9'] },
      'run.runContext': { mac: ['Ctrl-Shift-r'], other: ['Ctrl-Shift-F10'] },
      'run.debugContext': { mac: ['Ctrl-Shift-d'], other: ['Ctrl-Alt-Shift-F9'] },
      'run.selectConfiguration': { mac: ['Ctrl-Alt-r'], other: ['Alt-Shift-F10'] },
      'run.rerun': ['Mod-F5'],
      'run.stop': ['Mod-F2'],
      'debug.resume': { mac: ['Mod-Alt-r'], other: ['F9'] },
      'debug.stepOver': ['F8'],
      'debug.stepInto': ['F7'],
      'debug.forceStepInto': ['Alt-Shift-F7'],
      'debug.stepOut': ['Shift-F8'],
      'debug.runToCursor': ['Alt-F9'],
      'debug.evaluate': ['Alt-F8'],
      'debug.toggleBreakpoint': ['Mod-F8'],
      'debug.viewBreakpoints': ['Mod-Shift-F8'],
      'view.run': ['Mod-4'],
      'view.debug': ['Mod-5'],
      'terminal.toggle': ['Alt-F12'],
      'git.commit': ['Mod-Alt-k'],
      'git.push': ['Mod-Shift-k'],
      'git.pull': ['Mod-t'],
      'git.branches': ['Ctrl-Shift-Backquote'],
      'view.commit': ['Mod-0'],
      'git.log': ['Mod-9'],
    },
  },
  vscode: {
    name: 'VS Code',
    bindings: {
      ...shared,
      'editor.close': ['Mod-w'],
      'editor.replace': { mac: ['Mod-Alt-f'], other: ['Mod-h'] },
      'editor.gotoLine': ['Ctrl-g'],
      'editor.reformat': ['Shift-Alt-f'],
      'nav.gotoFile': ['Mod-p'],
      'nav.gotoAction': ['Mod-Shift-p'],
      'view.toggleLeft': ['Mod-b'],
      'tree.focus': ['Mod-Shift-e'],
      'lang.gotoDefinition': ['F12'],
      'lang.gotoImplementation': ['Mod-F12'],
      'nav.back': { mac: ['Ctrl-Minus'], other: ['Alt-ArrowLeft'] },
      'nav.forward': { mac: ['Ctrl-Shift-Minus'], other: ['Alt-ArrowRight'] },
      'nav.gotoSymbol': ['Mod-t'],
      'lang.fileStructure': ['Mod-Shift-o'],
      'lang.findUsages': ['Shift-F12'],
      'lang.quickDoc': ['Mod-k Mod-i'],
      'lang.parameterInfo': ['Mod-Shift-Space'],
      'lang.rename': ['F2'],
      'lang.contextActions': ['Mod-Period'],
      'search.replaceInPath': ['Mod-Shift-h'],
      'run.run': ['Ctrl-F5'],
      'debug.resume': ['F5'],
      'run.stop': ['Shift-F5'],
      'debug.restart': ['Mod-Shift-F5'],
      'debug.stepOver': ['F10'],
      'debug.stepInto': ['F11'],
      'debug.stepOut': ['Shift-F11'],
      'debug.toggleBreakpoint': ['F9'],
      'terminal.toggle': ['Ctrl-Backquote'],
      'terminal.new': ['Ctrl-Shift-Backquote'],
      'view.commit': ['Ctrl-Shift-g'],
    },
  },
};

export function isKeymapId(id: unknown): id is KeymapId {
  return id === 'goi-default' || id === 'vscode';
}

function entryFor(e: Entry, isMac: boolean): string[] {
  if (Array.isArray(e)) return e;
  return (isMac ? e.mac : e.other) ?? e.all ?? [];
}

/** Command → resolved sequences, after applying overrides (an empty array
 * unbinds; a missing key keeps the base bindings). */
export function commandBindings(
  id: KeymapId,
  overrides: Record<string, string[] | null | undefined> | undefined,
  isMac: boolean,
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  const base = keymaps[isKeymapId(id) ? id : 'goi-default'].bindings;
  for (const [cmd, e] of Object.entries(base)) out.set(cmd, entryFor(e, isMac).map((s) => resolveMod(s, isMac)));
  for (const [cmd, seqs] of Object.entries(overrides ?? {})) {
    if (!Array.isArray(seqs)) continue;
    const resolved: string[] = [];
    for (const s of seqs) {
      try {
        resolved.push(resolveMod(s, isMac));
      } catch {
        // Ignore malformed user bindings.
      }
    }
    out.set(cmd, resolved);
  }
  return out;
}

/** Resolved sequence → command. Later commands win on duplicates. */
export function effectiveBindings(
  id: KeymapId,
  overrides: Record<string, string[] | null | undefined> | undefined,
  isMac: boolean,
): Map<string, string> {
  const out = new Map<string, string>();
  for (const [cmd, seqs] of commandBindings(id, overrides, isMac)) for (const s of seqs) out.set(s, cmd);
  return out;
}

/** Commands other than commandId whose bindings equal seq or are a chord
 * prefix of it (or have it as a prefix). seq must be resolved. */
export function findConflicts(bindings: Map<string, string>, commandId: string, seq: string): string[] {
  const out = new Set<string>();
  for (const [s, cmd] of bindings) {
    if (cmd === commandId) continue;
    if (s === seq || seq.startsWith(`${s} `) || s.startsWith(`${seq} `)) out.add(cmd);
  }
  return [...out];
}

/** True if a and b collide (equal, or one is a chord prefix of the other). */
export function collides(a: string, b: string): boolean {
  return a === b || a.startsWith(`${b} `) || b.startsWith(`${a} `);
}
