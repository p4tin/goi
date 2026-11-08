// Launching saved configurations, with hooks the debugger plugs into.

import { run } from '../lib/api';
import type { RunConfig } from '../lib/api';
import { findConfig, notifyError, selectedName } from './state';

/** Hooks other features (the debugger) use to take part in launches. */
export const runHooks: {
  /** Handles a debug launch failure (e.g. Delve missing); true if handled. */
  debugError?: (cfg: RunConfig, err: unknown) => Promise<boolean>;
  /** Stops the active debug session; true if there was one. */
  stopDebug?: () => Promise<boolean>;
} = {};

/** Launches a saved configuration. */
export async function launch(name: string, debug: boolean): Promise<void> {
  const cfg = findConfig(name);
  selectedName.set(name);
  try {
    await run.run(name, debug);
  } catch (err) {
    if (debug && cfg && runHooks.debugError && (await runHooks.debugError(cfg, err))) return;
    notifyError('run', `Cannot ${debug ? 'debug' : 'run'} ${name}`, err);
  }
}
