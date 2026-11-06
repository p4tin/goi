// Keymap feature: global dispatcher, effective bindings from settings, the
// rebinding API used by the settings UI, and key hints for the palette.

import { confirmDialog, showDialog } from '../app/dialog';
import { isMac, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { settingsApi } from '../lib/api';
import { commands } from '../lib/commands';
import { effect, signal } from '../lib/store';
import { formatForDisplay, resolveMod, strokeFromEvent, toModForm } from './chords';
import { KeyDispatcher } from './dispatcher';
import { collides, commandBindings, findConflicts, isKeymapId, keymaps } from './keymaps';
import type { KeymapId } from './keymaps';

interface Current {
  keymap: KeymapId;
  bySeq: Map<string, string>;
  byCmd: Map<string, string[]>;
}

/** The effective bindings, recomputed whenever settings change. */
export const bindings = signal<Current>({ keymap: 'goi-default', bySeq: new Map(), byCmd: new Map() });

function compute(keymap: unknown, overrides: Record<string, string[]> | undefined): Current {
  const id: KeymapId = isKeymapId(keymap) ? keymap : 'goi-default';
  const byCmd = commandBindings(id, overrides, isMac());
  const bySeq = new Map<string, string>();
  for (const [cmd, seqs] of byCmd) for (const s of seqs) bySeq.set(s, cmd);
  return { keymap: id, bySeq, byCmd };
}

/** Display form of the command's first binding, or "". */
export function keyHintFor(commandId: string): string {
  const seq = bindings.peek().byCmd.get(commandId)?.[0];
  return seq ? formatForDisplay(seq, isMac()) : '';
}

/** Replaces a command's bindings in the user layer. Sequences may use Mod. */
export async function setBinding(commandId: string, sequences: string[]): Promise<void> {
  await settingsApi.set('user', `keys.${commandId}`, sequences.map((s) => toModForm(s, isMac())));
}

/** Restores the keymap's default bindings for a command. */
export async function resetBinding(commandId: string): Promise<void> {
  await settingsApi.set('user', `keys.${commandId}`, null);
}

function commandLabel(id: string): string {
  const c = commands.get(id);
  return c ? (c.category ? `${c.category}: ${c.title}` : c.title) : id;
}

/** Binds seq to commandId, asking first when it collides with other
 * commands; on confirmation the colliding sequences are unbound from them.
 * Returns false if the user cancelled. */
export async function bindWithConflictCheck(commandId: string, seq: string): Promise<boolean> {
  const mac = isMac();
  const resolved = resolveMod(seq, mac);
  const cur = bindings.peek();
  const conflicts = findConflicts(cur.bySeq, commandId, resolved);
  if (conflicts.length) {
    const ok = await confirmDialog(
      'Shortcut already in use',
      `${formatForDisplay(resolved, mac)} is already bound to ${conflicts.map(commandLabel).join(', ')}. Replace?`,
      'Replace',
    );
    if (!ok) return false;
    for (const other of conflicts) {
      const kept = (cur.byCmd.get(other) ?? []).filter((s) => !collides(s, resolved));
      await setBinding(other, kept);
    }
  }
  await setBinding(commandId, [resolved]);
  return true;
}

/** Captures the next key sequence (up to two strokes; a second stroke must
 * follow within 1 s). Escape as the first stroke cancels. */
export function recordSequence(): Promise<string | null> {
  return new Promise((resolve) => {
    const strokes: string[] = [];
    let timer: ReturnType<typeof setTimeout> | null = null;
    const finish = (v: string | null): void => {
      window.removeEventListener('keydown', onKey, true);
      if (timer) clearTimeout(timer);
      resolve(v);
    };
    const onKey = (e: KeyboardEvent): void => {
      const s = strokeFromEvent(e, isMac());
      if (!s) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      if (strokes.length === 0 && s === 'Escape') return finish(null);
      strokes.push(s);
      if (timer) clearTimeout(timer);
      if (strokes.length >= 2) return finish(strokes.join(' '));
      timer = setTimeout(() => finish(strokes.join(' ')), 1000);
    };
    window.addEventListener('keydown', onKey, true);
  });
}

export function installKeymaps(wb: Workbench): void {
  effect(() => {
    const s = settings();
    bindings.set(compute(s?.ui.keymap, s?.keys));
  });

  const hint = wb.status.add({ id: 'keymap-chord', side: 'left', priority: 90 });
  hint.hidden = true;
  const dispatcher = new KeyDispatcher({
    bindings: () => bindings.peek().bySeq,
    canRun: (id) => commands.isEnabled(id),
    run: (id) => {
      commands.runFrom('key', id).catch((err: unknown) => console.error('command', id, err));
    },
    isMac,
    onPending: (text) => {
      hint.hidden = text === null;
      hint.textContent = text ?? '';
    },
  });
  document.addEventListener('keydown', (e) => dispatcher.handle(e), true);

  commands.register({
    id: 'keymap.switch',
    title: 'Switch Keymap…',
    category: 'Preferences',
    run: async () => {
      const current = bindings.peek().keymap;
      const ids = Object.keys(keymaps) as KeymapId[];
      const choice = await showDialog<KeymapId | null>({
        title: 'Switch Keymap',
        body: `Current: ${keymaps[current].name}`,
        cancelValue: null,
        buttons: [
          { label: 'Cancel', value: null },
          ...ids.map((id) => ({ label: keymaps[id].name, value: id, primary: id === current })),
        ],
      });
      if (choice && choice !== current) await settingsApi.set('user', 'ui.keymap', choice);
    },
  });
}
