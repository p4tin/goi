// Recently activated files, most recent first, persisted per project.

import type { Workbench } from '../app/workbench';
import { effect, signal } from '../lib/store';
import type { ReadSignal } from '../lib/store';

export const MAX_RECENT_FILES = 50;

const list = signal<string[]>([]);

/** Recently activated file paths, most recent first. */
export const recentFiles: ReadSignal<string[]> = list;

/** Moves path to the front, deduplicating and capping the list. */
export function pushRecent(paths: string[], path: string, max = MAX_RECENT_FILES): string[] {
  return [path, ...paths.filter((p) => p !== path)].slice(0, max);
}

export function noteRecent(path: string): void {
  list.update((ps) => pushRecent(ps, path));
}

/** Tracks activations of activePath and persists the list as the
 * 'recentFiles' layout part. */
export function installRecentFiles(wb: Workbench, activePath: ReadSignal<string | null>): void {
  effect(() => {
    const p = activePath();
    if (p) {
      noteRecent(p);
      wb.layout.scheduleSave();
    }
  });
  wb.layout.register<string[]>('recentFiles', {
    save: () => list.peek(),
    restore: (v) => {
      if (!Array.isArray(v)) return;
      const restored = v.filter((x): x is string => typeof x === 'string');
      // Keep anything activated since startup in front of the restored list.
      list.update((cur) => [...cur, ...restored.filter((p) => !cur.includes(p))].slice(0, MAX_RECENT_FILES));
    },
  });
}
