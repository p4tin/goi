// Keymap section of the settings view: every registered command with its
// current bindings, editable (record a new sequence) and resettable.

import { isMac, settings } from '../../app/state';
import { commands } from '../../lib/commands';
import type { Command } from '../../lib/commands';
import { h } from '../../lib/dom';
import { effect, signal } from '../../lib/store';
import type { ReadSignal } from '../../lib/store';
import { formatForDisplay } from '../../keymap/chords';
import { bindings, bindWithConflictCheck, recordSequence, resetBinding } from '../../keymap/install';

export function commandLabel(c: Command): string {
  return c.category ? `${c.category}: ${c.title}` : c.title;
}

/** Commands matching every word of query (label or id), sorted by label. */
export function filterCommands(all: Command[], query: string): Command[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return all
    .filter((c) => {
      const hay = `${commandLabel(c)} ${c.id}`.toLowerCase();
      return words.every((w) => hay.includes(w));
    })
    .sort((a, b) => commandLabel(a).localeCompare(commandLabel(b)));
}

export function renderKeymapEditor(container: HTMLElement, query: ReadSignal<string>): () => void {
  const version = signal(0);
  const offCmds = commands.onDidChange(() => version.update((v) => v + 1));
  const recording = signal<string | null>(null);
  const table = h('table', { class: 'keymap-table' });
  container.append(table);

  const stop = effect(() => {
    version();
    const q = query();
    const b = bindings();
    const keys = settings()?.keys ?? {};
    const rec = recording();
    const mac = isMac();
    const rows = filterCommands(commands.all(), q).map((c) => {
      const seqs = b.byCmd.get(c.id) ?? [];
      const custom = Object.prototype.hasOwnProperty.call(keys, c.id);
      return h(
        'tr',
        { dataset: { command: c.id } },
        h('td', { class: 'keymap-cmd' }, commandLabel(c), custom ? h('span', { class: 'badge badge-custom' }, 'custom') : null),
        h(
          'td',
          { class: 'keymap-keys' },
          rec === c.id
            ? h('span', { class: 'muted' }, 'Press keys… (Esc to cancel)')
            : seqs.length
              ? seqs.map((s) => h('kbd', null, formatForDisplay(s, mac)))
              : h('span', { class: 'muted' }, '—'),
        ),
        h(
          'td',
          { class: 'keymap-actions' },
          h(
            'button',
            {
              class: 'link-btn',
              'aria-label': `Edit shortcut for ${commandLabel(c)}`,
              onClick: async () => {
                recording.set(c.id);
                try {
                  const seq = await recordSequence();
                  if (seq) await bindWithConflictCheck(c.id, seq);
                } finally {
                  recording.set(null);
                }
              },
            },
            'Edit',
          ),
          custom
            ? h(
                'button',
                { class: 'link-btn', 'aria-label': `Reset shortcut for ${commandLabel(c)}`, onClick: () => void resetBinding(c.id) },
                'Reset',
              )
            : null,
        ),
      );
    });
    table.replaceChildren(
      h('thead', null, h('tr', null, h('th', null, 'Command'), h('th', null, 'Shortcut'), h('th', null, ''))),
      h('tbody', null, rows.length ? rows : h('tr', null, h('td', { class: 'muted', colSpan: 3 }, 'No matching commands'))),
    );
  });

  return () => {
    stop();
    offCmds();
  };
}
