// Feature installers, run once at boot after the workbench exists. Each
// feature registers its own commands, panels, status items and layout parts.
// Add one import and one list entry per feature.

import type { Workbench } from './app/workbench';
import { installEditor } from './editor/install';
import { installWelcome } from './views/welcome';

export type Feature = (wb: Workbench) => void;

export const features: Feature[] = [installWelcome, installEditor];
