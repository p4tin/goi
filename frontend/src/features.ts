// Feature installers, run once at boot after the workbench exists. Each
// feature registers its own commands, panels, status items and layout parts.
// Add one import and one list entry per feature.

import type { Workbench } from './app/workbench';
import { installAbout } from './app/about';
import { installMenuBar } from './app/menuBar';
import { installWindowCommands } from './app/windowCommands';
import { installEditor } from './editor/install';
import { installCloseGuard } from './editor/closeGuard';
import { installDiskSync } from './editor/diskSync';
import { installJournal } from './editor/journal';
import { installLanguages } from './editor/languages';
import { installLang } from './lang/install';
import { installKeymaps } from './keymap/install';
import { installPalette } from './palette/palette';
import { installKeyHints } from './keymap/hints';
import { installRunGutter } from './editor/runGutter';
import { installRun } from './run/install';
import { installDebug } from './debug/install';
import { installTerminal } from './terminal/terminal';
import { installAgent } from './agent/install';
import { installGit } from './git/install';
import { installLint } from './lint/lint';
import { installGoMod } from './mod/gomod';
import { installSaveFlow } from './editor/saveFlow';
import { installFormatters } from './format/install';
import { installGoStatus } from './views/goStatus';
import { installSettings } from './views/settings/settingsView';
import { installThemeCommands } from './theme/commands';
import { installProjectTree } from './views/projectTree';
import { installWelcome } from './views/welcome';

export type Feature = (wb: Workbench) => void;

export const features: Feature[] = [
  installWelcome,
  installWindowCommands,
  installEditor,
  installSaveFlow,
  installFormatters,
  installJournal,
  installLanguages,
  installRunGutter,
  installDiskSync,
  installCloseGuard,
  installLang,
  installRun,
  installDebug,
  installTerminal,
  installLint,
  installGoMod,
  installPalette,
  installKeymaps,
  installKeyHints,
  installProjectTree,
  // After the project tree, so Project stays the left dock's default tab.
  installGit,
  // After git: agent diffs open in the git diff viewer.
  installAgent,
  installGoStatus,
  installSettings,
  installThemeCommands,
  installAbout,
  installMenuBar,
];
