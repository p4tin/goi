import { describe, expect, it, vi } from 'vitest';

const set = vi.fn(async () => {});
vi.mock('../src/lib/api', () => ({ settingsApi: { set: (...a: unknown[]) => set(...(a as [])) } }));

const { installThemeCommands, toggledTheme } = await import('../src/theme/commands');
const { resolvedTheme } = await import('../src/app/state');
const { commands } = await import('../src/lib/commands');

describe('theme commands', () => {
  installThemeCommands({} as never);

  it('toggles between light and dark from the resolved theme', async () => {
    expect(toggledTheme('light')).toBe('dark');
    expect(toggledTheme('dark')).toBe('light');
    expect(toggledTheme('hc')).toBe('light');
    resolvedTheme.set('light');
    await commands.run('view.toggleDarkMode');
    expect(set).toHaveBeenLastCalledWith('user', 'ui.theme', 'dark');
  });

  it('sets a specific theme', async () => {
    await commands.run('view.themeAuto');
    expect(set).toHaveBeenLastCalledWith('user', 'ui.theme', 'auto');
    await commands.run('view.themeHighContrast');
    expect(set).toHaveBeenLastCalledWith('user', 'ui.theme', 'hc');
  });
});
