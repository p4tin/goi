// Feature installers, run once at boot after the workbench exists. Each
// feature registers its own commands, panels, status items and layout parts.
// Add one import and one list entry per core feature. The app's frontend
// pack (src/pack.ts) adds its own features after named core features.

import type { Workbench } from './app/workbench';
import { packFeatures, type PackFeature } from './pack';
import { installAbout } from './app/about';
import { installMenuBar } from './app/menuBar';
import { installWindowCommands } from './app/windowCommands';
import { installEditor } from './editor/install';
import { installCloseGuard } from './editor/closeGuard';
import { installDiskSync } from './editor/diskSync';
import { installJournal } from './editor/journal';
import { installLanguages } from './editor/languages';
import { installMarkdownPreview } from './markdown/install';
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

/** Core features in boot order, each with an id pack features can anchor to. */
export const coreFeatures: Array<[id: string, install: Feature]> = [
  ['welcome', installWelcome],
  ['windowCommands', installWindowCommands],
  ['editor', installEditor],
  ['saveFlow', installSaveFlow],
  ['formatters', installFormatters],
  ['journal', installJournal],
  ['languages', installLanguages],
  ['markdownPreview', installMarkdownPreview],
  ['runGutter', installRunGutter],
  ['diskSync', installDiskSync],
  ['closeGuard', installCloseGuard],
  ['lang', installLang],
  ['run', installRun],
  ['debug', installDebug],
  ['terminal', installTerminal],
  ['lint', installLint],
  ['goMod', installGoMod],
  ['palette', installPalette],
  ['keymaps', installKeymaps],
  ['keyHints', installKeyHints],
  ['projectTree', installProjectTree],
  // After the project tree, so Project stays the left dock's default tab.
  ['git', installGit],
  // After git: agent diffs open in the git diff viewer.
  ['agent', installAgent],
  ['goStatus', installGoStatus],
  ['settings', installSettings],
  ['themeCommands', installThemeCommands],
  ['about', installAbout],
  ['menuBar', installMenuBar],
];

/** Inserts each pack feature right after the feature named by its `after`
 * (core or an earlier pack feature). An unknown anchor is a build mistake. */
export function composeFeatures(core: Array<[string, Feature]>, extra: PackFeature[]): Array<[string, Feature]> {
  const out = [...core];
  for (const f of extra) {
    const i = out.findIndex(([id]) => id === f.after);
    if (i < 0) throw new Error(`pack feature ${f.id}: no feature "${f.after}" to follow`);
    out.splice(i + 1, 0, [f.id, f.install]);
  }
  return out;
}

export const featureOrder = composeFeatures(coreFeatures, packFeatures);
export const features: Feature[] = featureOrder.map(([, install]) => install);
