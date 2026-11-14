// Mermaid diagrams for the Markdown preview. Mermaid is large, so it loads on
// the first diagram. It runs in strict mode (no click handlers or scripts,
// labels sanitized) with colors from Goi's theme tokens, one render at a
// time (it keeps global state), and results are cached by theme + source.

import type { MermaidConfig } from 'mermaid';
import { uiThemes } from '../theme/tokens';
import type { UiThemeId } from '../theme/tokens';

export type DiagramResult = { svg: string } | { error: string };

type Mermaid = (typeof import('mermaid'))['default'];

/** The Mermaid configuration for a Goi theme (colors from tokens). */
export function mermaidConfig(theme: UiThemeId, fontFamily = 'sans-serif'): MermaidConfig {
  const t = uiThemes[theme];
  return {
    startOnLoad: false,
    securityLevel: 'strict',
    theme: 'base',
    darkMode: theme !== 'light',
    fontFamily,
    themeVariables: {
      darkMode: theme !== 'light',
      background: t.bg,
      fontFamily,
      primaryColor: t.panelBg,
      primaryTextColor: t.fg,
      primaryBorderColor: t.accent,
      secondaryColor: t.selection,
      secondaryTextColor: t.fg,
      tertiaryColor: t.hover,
      tertiaryTextColor: t.fg,
      lineColor: t.fgMuted,
      textColor: t.fg,
      noteBkgColor: t.inputBg,
      noteTextColor: t.fg,
      noteBorderColor: t.panelBorder,
      errorBkgColor: t.diffRemovedBg,
      errorTextColor: t.error,
    },
  };
}

let loader: Promise<Mermaid> | null = null;
let initialized: UiThemeId | null = null;
let queue: Promise<unknown> = Promise.resolve();
let seq = 0;

const CACHE_SIZE = 64;
const cache = new Map<string, DiagramResult>();

function remember(key: string, r: DiagramResult): DiagramResult {
  cache.delete(key);
  cache.set(key, r);
  if (cache.size > CACHE_SIZE) cache.delete(cache.keys().next().value!);
  return r;
}

/** Renders a diagram to an SVG string (or the error message). */
export function renderDiagram(source: string, theme: UiThemeId): Promise<DiagramResult> {
  const key = `${theme}\u0000${source}`;
  const hit = cache.get(key);
  if (hit) return Promise.resolve(remember(key, hit));
  const job = queue.then(() => render(source, theme)).then((r) => remember(key, r));
  queue = job.catch(() => undefined);
  return job;
}

async function render(source: string, theme: UiThemeId): Promise<DiagramResult> {
  let m: Mermaid;
  try {
    loader ??= import('mermaid').then((mod) => mod.default);
    m = await loader;
  } catch (err) {
    loader = null;
    console.error('load mermaid', err);
    return { error: 'Mermaid failed to load' };
  }
  if (initialized !== theme) {
    m.initialize(mermaidConfig(theme, getComputedStyle(document.body).fontFamily || 'sans-serif'));
    initialized = theme;
  }
  // Mermaid needs real layout: render into an attached, off-screen element.
  const id = `goi-mmd-${++seq}`;
  const host = document.createElement('div');
  host.style.cssText = 'position:fixed;left:-10000px;top:0;width:1000px;visibility:hidden';
  document.body.appendChild(host);
  try {
    const { svg } = await m.render(id, source, host);
    return { svg };
  } catch (err) {
    return { error: err instanceof Error ? err.message : String(err) };
  } finally {
    host.remove();
    document.getElementById(`d${id}`)?.remove();
    document.getElementById(id)?.remove();
  }
}
