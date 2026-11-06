// Theme commands for the palette and View menu: pick a UI theme or toggle
// dark mode. They write ui.theme in the user layer; main.ts applies it.

import { resolvedTheme } from '../app/state';
import type { Workbench } from '../app/workbench';
import { settingsApi } from '../lib/api';
import { commands } from '../lib/commands';
import type { ThemeSetting } from './theme';

const choices: [id: string, title: string, setting: ThemeSetting][] = [
  ['view.themeDark', 'Theme: Dark', 'dark'],
  ['view.themeLight', 'Theme: Light', 'light'],
  ['view.themeAuto', 'Theme: Match System', 'auto'],
  ['view.themeHighContrast', 'Theme: High Contrast', 'hc'],
];

export function setTheme(setting: ThemeSetting): Promise<void> {
  return settingsApi.set('user', 'ui.theme', setting);
}

/** The theme to switch to when toggling dark mode from the current one. */
export function toggledTheme(current: string): ThemeSetting {
  return current === 'light' ? 'dark' : 'light';
}

export function installThemeCommands(_wb: Workbench): void {
  commands.register({
    id: 'view.toggleDarkMode',
    title: 'Toggle Dark Mode',
    category: 'View',
    run: () => setTheme(toggledTheme(resolvedTheme.peek())),
  });
  for (const [id, title, setting] of choices) {
    commands.register({ id, title, category: 'View', run: () => setTheme(setting) });
  }
}
