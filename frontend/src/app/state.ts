// Shared application state, kept current from backend topics.

import { app, on, settingsApi, Topics, workspaceApi } from '../lib/api';
import type { AppInfo, Project, Settings, SettingsChanged } from '../lib/api';
import { signal } from '../lib/store';
import type { UiThemeId } from '../theme/tokens';

export const appInfo = signal<AppInfo | null>(null);
export const project = signal<Project | null>(null);
export const settings = signal<Settings | null>(null);
/** The UI theme in effect after resolving 'auto'. */
export const resolvedTheme = signal<UiThemeId>('dark');

let started = false;

/** Loads initial state and subscribes to backend updates. Idempotent. */
export async function startState(): Promise<void> {
  if (started) return;
  started = true;
  on<Project | null>(Topics.workspaceChanged, (p) => project.set(p ?? null));
  on<SettingsChanged>(Topics.settingsChanged, (e) => settings.set(e.effective));
  const [info, proj, eff] = await Promise.all([app.info(), workspaceApi.project(), settingsApi.effective()]);
  appInfo.set(info);
  project.set(proj);
  settings.set(eff);
}

/** True on macOS, used for Cmd vs Ctrl labels. */
export function isMac(): boolean {
  return (appInfo.peek()?.os ?? navigator.platform.toLowerCase()).startsWith('darwin') || /mac/i.test(navigator.platform);
}
