// Feature installers, run once at boot after the workbench exists. Each
// feature registers its own commands, panels, status items and layout parts.
// Add one import and one list entry per feature.

import type { Workbench } from './app/workbench';
import { installWindowCommands } from './app/windowCommands';
import { installEditor } from './editor/install';
import { installCloseGuard } from './editor/closeGuard';
import { installDiskSync } from './editor/diskSync';
import { installJournal } from './editor/journal';
import { installLanguages } from './editor/languages';
import { installKeymaps } from './keymap/install';
import { installPalette } from './palette/palette';
import { installKeyHints } from './keymap/hints';
import { installRunGutter } from './editor/runGutter';
import { installSaveFlow } from './editor/saveFlow';
import { installGoStatus } from './views/goStatus';
import { installSettings } from './views/settings/settingsView';
import { installProjectTree } from './views/projectTree';
import { installWelcome } from './views/welcome';

export type Feature = (wb: Workbench) => void;

export const features: Feature[] = [
  installWelcome,
  installWindowCommands,
  installEditor,
  installSaveFlow,
  installJournal,
  installLanguages,
  installRunGutter,
  installDiskSync,
  installCloseGuard,
  installPalette,
  installKeymaps,
  installKeyHints,
  installProjectTree,
  installGoStatus,
  installSettings,
];
