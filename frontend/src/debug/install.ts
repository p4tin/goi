// Debugger feature (M3): breakpoints, the Debug tool window, stepping and
// inspection commands, and editor decorations while stopped.

import type { Workbench } from '../app/workbench';
import { installBreakpoints } from './breakpoints';
import './debug.css';

export function installDebug(wb: Workbench): void {
  installBreakpoints(wb);
}
