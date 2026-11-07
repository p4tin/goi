// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { applyUiTheme, initTheme, resolveTheme } from '../src/theme/theme';
import { defaultSchemeFor, schemeExtension } from '../src/theme/schemes';
import {
  schemeEditorKeys,
  schemes,
  syntaxKeys,
  uiThemes,
  uiTokenKeys,
  type SchemeId,
  type UiThemeId,
} from '../src/theme/tokens';

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  const ch = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2];
}

export function contrast(a: string, b: string): number {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

const hex = /^#[0-9a-f]{6}$/;

describe('resolveTheme', () => {
  it.each([
    ['auto', true, 'dark'],
    ['auto', false, 'light'],
    ['light', true, 'light'],
    ['dark', false, 'dark'],
    ['hc', false, 'hc'],
  ] as const)('%s prefersDark=%s → %s', (setting, dark, want) => {
    expect(resolveTheme(setting, dark)).toBe(want);
  });
});

describe('tokens', () => {
  it('UI themes define every key as #rrggbb', () => {
    for (const [id, tokens] of Object.entries(uiThemes)) {
      for (const k of uiTokenKeys) expect(tokens[k], `${id}.${k}`).toMatch(hex);
      expect(Object.keys(tokens).sort()).toEqual([...uiTokenKeys].sort());
    }
  });

  it('schemes define every key as #rrggbb', () => {
    for (const [id, s] of Object.entries(schemes)) {
      for (const k of schemeEditorKeys) expect(s.editor[k], `${id}.editor.${k}`).toMatch(hex);
      for (const k of syntaxKeys) expect(s.syntax[k], `${id}.syntax.${k}`).toMatch(hex);
    }
  });
});

describe('contrast (WCAG)', () => {
  it('UI themes: fg on bg ≥ 4.5', () => {
    for (const [id, t] of Object.entries(uiThemes)) {
      expect(contrast(t.fg, t.bg), id).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('high contrast: all primary text pairs ≥ 4.5', () => {
    const t = uiThemes.hc;
    const pairs: [string, string, string][] = [
      ['fg/bg', t.fg, t.bg],
      ['fg/panelBg', t.fg, t.panelBg],
      ['statusFg/statusBg', t.statusFg, t.statusBg],
      ['accentFg/accent', t.accentFg, t.accent],
    ];
    for (const [name, a, b] of pairs) expect(contrast(a, b), name).toBeGreaterThanOrEqual(4.5);
  });

  it('schemes: fg ≥ 4.5, syntax ≥ 3.0, comment ≥ 2.5 on editor bg', () => {
    for (const [id, s] of Object.entries(schemes)) {
      const bg = s.editor.bg;
      expect(contrast(s.editor.fg, bg), `${id} fg`).toBeGreaterThanOrEqual(4.5);
      for (const k of syntaxKeys) {
        const min = k === 'comment' ? 2.5 : 3.0;
        expect(contrast(s.syntax[k], bg), `${id}.${k}`).toBeGreaterThanOrEqual(min);
      }
    }
  });

  it('reports minimum contrast per theme', () => {
    const report: Record<string, number> = {};
    for (const [id, t] of Object.entries(uiThemes)) {
      report[`ui:${id}`] = Math.min(contrast(t.fg, t.bg), contrast(t.fg, t.panelBg), contrast(t.statusFg, t.statusBg), contrast(t.accentFg, t.accent));
    }
    for (const [id, s] of Object.entries(schemes)) {
      report[`scheme:${id}`] = Math.min(...syntaxKeys.map((k) => contrast(s.syntax[k], s.editor.bg)));
    }
    console.log('min contrast', Object.fromEntries(Object.entries(report).map(([k, v]) => [k, +v.toFixed(2)])));
    expect(Object.keys(report)).toHaveLength(7);
  });
});

describe('applyUiTheme / initTheme', () => {
  it('writes CSS vars and data-theme', () => {
    applyUiTheme('hc');
    const root = document.documentElement;
    expect(root.style.getPropertyValue('--goi-bg')).toBe(uiThemes.hc.bg);
    expect(root.style.getPropertyValue('--goi-fg-muted')).toBe(uiThemes.hc.fgMuted);
    expect(root.dataset.theme).toBe('hc');
  });

  it('initTheme set() switches live', () => {
    const c = initTheme('light');
    expect(c.resolved()).toBe('light');
    c.set('dark');
    expect(c.resolved()).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    c.dispose();
  });

  it('initTheme reports setting changes to the native window', () => {
    const seen: string[] = [];
    const c = initTheme('auto', (s) => seen.push(s));
    c.set('dark');
    c.set('dark');
    c.set('hc');
    c.set('auto');
    expect(seen).toEqual(['auto', 'dark', 'hc', 'auto']);
    c.dispose();
  });
});

describe('schemes', () => {
  it('builds an extension for each scheme', () => {
    for (const id of Object.keys(schemes) as SchemeId[]) expect(schemeExtension(id)).toBeTruthy();
  });

  it.each([
    ['light', 'goi-light'],
    ['dark', 'goi-dark'],
    ['hc', 'goi-dark'],
  ] as [UiThemeId, SchemeId][])('default scheme for %s is %s', (ui, want) => {
    expect(defaultSchemeFor(ui)).toBe(want);
  });
});
