// UI theme selection: applies tokens as CSS custom properties and follows
// the OS appearance when the setting is 'auto'.

import { signal, type ReadSignal } from '../lib/store';
import { cssVar, uiThemes, uiTokenKeys, type UiThemeId } from './tokens';

export type ThemeSetting = 'auto' | UiThemeId;

export function resolveTheme(setting: ThemeSetting, prefersDark: boolean): UiThemeId {
  if (setting === 'auto') return prefersDark ? 'dark' : 'light';
  return setting;
}

/** Writes the theme's tokens onto the root element and sets data-theme. */
export function applyUiTheme(id: UiThemeId, root: HTMLElement = document.documentElement): void {
  const tokens = uiThemes[id];
  for (const key of uiTokenKeys) root.style.setProperty(cssVar(key), tokens[key]);
  root.dataset.theme = id;
  root.style.colorScheme = id === 'light' ? 'light' : 'dark';
}

export interface ThemeController {
  resolved: ReadSignal<UiThemeId>;
  set(setting: ThemeSetting): void;
  dispose(): void;
}

const darkQuery = '(prefers-color-scheme: dark)';

/** native, when given, receives each new setting so the window chrome can
 * follow it ("auto" must stay "auto" to keep tracking the OS). */
export function initTheme(setting: ThemeSetting, native?: (setting: ThemeSetting) => void): ThemeController {
  const mql = typeof window.matchMedia === 'function' ? window.matchMedia(darkQuery) : null;
  let current = setting;
  native?.(current);
  const resolved = signal<UiThemeId>(resolveTheme(current, mql?.matches ?? false));

  const update = () => {
    const id = resolveTheme(current, mql?.matches ?? false);
    resolved.set(id);
    applyUiTheme(id);
  };
  const onChange = () => {
    if (current === 'auto') update();
  };
  mql?.addEventListener('change', onChange);
  update();

  return {
    resolved,
    set(next) {
      if (next !== current) native?.(next);
      current = next;
      update();
    },
    dispose() {
      mql?.removeEventListener('change', onChange);
    },
  };
}
