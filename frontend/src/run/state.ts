// Shared run state: configurations, the selected configuration and the
// workbench handle.

import type { Workbench } from '../app/workbench';
import type { RunConfig } from '../lib/api';
import { signal } from '../lib/store';

export const configs = signal<RunConfig[]>([]);
/** Name of the configuration shown in the run widget. */
export const selectedName = signal<string>('');

let wb: Workbench | null = null;

export function setRunWorkbench(w: Workbench): void {
  wb = w;
}

export function workbench(): Workbench {
  if (!wb) throw new Error('run: workbench not set');
  return wb;
}

export function selectedConfig(): RunConfig | undefined {
  const name = selectedName();
  return configs().find((c) => c.name.toLowerCase() === name.toLowerCase());
}

export function findConfig(name: string): RunConfig | undefined {
  return configs.peek().find((c) => c.name.toLowerCase() === name.toLowerCase());
}

export function notifyError(id: string, title: string, err: unknown): void {
  wb?.notifications.push({ id, level: 'error', title, body: err instanceof Error ? err.message : String(err) });
}

/** "Process finished with exit code 0 (1.2 s)". */
export function exitLine(code: number | undefined, signal: string | undefined, elapsedMs: number): string {
  const t = elapsedMs >= 60000 ? `${Math.floor(elapsedMs / 60000)} min ${Math.round((elapsedMs % 60000) / 1000)} s` : `${(elapsedMs / 1000).toFixed(1)} s`;
  if (signal) return `Process finished with signal ${signal.toUpperCase()} (${t})`;
  return `Process finished with exit code ${code ?? '?'} (${t})`;
}

/** A short label for a configuration type. */
export const TYPE_LABEL: Record<RunConfig['type'], string> = {
  app: 'Go Build',
  test: 'Go Test',
  remote: 'Go Remote',
  shell: 'Shell Script',
  make: 'Makefile',
};
