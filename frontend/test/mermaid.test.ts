// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest';

const calls: string[] = [];
const initialize = vi.fn();
const render = vi.fn(async (id: string, source: string, host: HTMLElement) => {
  calls.push(`start ${source}`);
  expect(host.isConnected).toBe(true);
  await new Promise((r) => setTimeout(r, 5));
  calls.push(`end ${source}`);
  if (source.includes('bad')) throw new Error('Parse error on line 1');
  return { svg: `<svg id="${id}"><text>${source}</text></svg>`, diagramType: 'flowchart' };
});
vi.mock('mermaid', () => ({ default: { initialize, render } }));

const { mermaidConfig, renderDiagram } = await import('../src/markdown/mermaid');
const { uiThemes } = await import('../src/theme/tokens');

describe('mermaid', () => {
  beforeEach(() => {
    calls.length = 0;
  });

  it('is strict, manual and themed from tokens', () => {
    const light = mermaidConfig('light');
    expect(light).toMatchObject({ startOnLoad: false, securityLevel: 'strict', theme: 'base', darkMode: false });
    expect(light.themeVariables).toMatchObject({ background: uiThemes.light.bg, primaryTextColor: uiThemes.light.fg, lineColor: uiThemes.light.fgMuted });
    expect(mermaidConfig('hc').darkMode).toBe(true);
    expect(mermaidConfig('dark').themeVariables.background).toBe(uiThemes.dark.bg);
  });

  it('renders one diagram at a time and caches results', async () => {
    const [a, b] = await Promise.all([renderDiagram('graph TD; A-->B', 'dark'), renderDiagram('graph TD; C-->D', 'dark')]);
    expect(calls).toEqual(['start graph TD; A-->B', 'end graph TD; A-->B', 'start graph TD; C-->D', 'end graph TD; C-->D']);
    expect('svg' in a && a.svg).toContain('A-->B');
    expect('svg' in b && b.svg).toContain('C-->D');
    expect(initialize).toHaveBeenCalledTimes(1);
    expect(initialize.mock.calls[0][0]).toMatchObject({ securityLevel: 'strict', startOnLoad: false });
    const n = render.mock.calls.length;
    await renderDiagram('graph TD; A-->B', 'dark');
    expect(render.mock.calls.length).toBe(n);
    // Another theme re-initializes and renders again.
    await renderDiagram('graph TD; A-->B', 'light');
    expect(render.mock.calls.length).toBe(n + 1);
    expect(initialize).toHaveBeenCalledTimes(2);
  });

  it('returns errors and keeps going', async () => {
    const bad = await renderDiagram('bad diagram', 'dark');
    expect(bad).toEqual({ error: 'Parse error on line 1' });
    const ok = await renderDiagram('graph LR; X', 'dark');
    expect('svg' in ok).toBe(true);
    expect(document.body.children.length).toBe(0);
  });
});
