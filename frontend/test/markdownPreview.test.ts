// @vitest-environment happy-dom
// Views are opened with focus:false (see editor.test.ts).
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../src/lib/api', () => ({ on: () => () => {}, Topics: {} }));
vi.mock('../src/editor/install', () => ({ editors: undefined }));
const { renderDiagram } = vi.hoisted(() => ({
  renderDiagram: vi.fn(async (source: string, theme: string) =>
    source.includes('bad') ? { error: 'Parse error on line 1' } : { svg: `<svg xmlns="http://www.w3.org/2000/svg" data-theme="${theme}"><text>${source.replace(/>/g, '&gt;')}</text></svg>` },
  ),
}));
vi.mock('../src/markdown/mermaid', () => ({ renderDiagram }));

import { EditorView } from '@codemirror/view';
import { EditorManager } from '../src/editor/editorManager';
import type { FileMeta, OpenedFile } from '../src/lib/api';
import { nextMode, parseSvg, Preview, reconcile, scrollTarget, toMode } from '../src/markdown/preview';
import type { Mode, PreviewEnv } from '../src/markdown/preview';
import { renderBlocks } from '../src/markdown/render';

// Disposed after each test: a pending debounce must not fire after
// happy-dom is torn down.
const created: Preview[] = [];

async function setup(content: string, o: { mode?: Mode; meta?: Partial<FileMeta>; env?: PreviewEnv } = {}) {
  const m = new EditorManager({
    io: {
      read: async (p) => ({ meta: { path: p, size: 0, modTime: '', binary: false, eol: '\n', bom: false, finalNewline: true, mode: 'normal', ...o.meta }, content }) as OpenedFile,
      write: async (_p, _c, mt) => mt,
    },
    confirmClose: async () => 'cancel',
    onError: () => {},
  });
  document.body.appendChild(m.activePane.peek().el);
  let preview!: Preview;
  // What install.ts's view plugin does.
  m.addExtension(() => EditorView.updateListener.of((u) => u.docChanged && preview.docChanged()));
  m.hooks.viewCreated.on(({ buffer, view, content: el }) => {
    preview = new Preview(buffer, view, el, { mode: o.mode ?? 'split', theme: 'dark', scheme: 'goi-dark', env: o.env });
    created.push(preview);
  });
  await m.open('/p/README.md', { focus: false });
  const view = m.activeView()!;
  return { m, view, preview, content: m.activePane.peek().contents.get('/p/README.md')! };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe('Preview', () => {
  afterEach(() => {
    for (const p of created.splice(0)) p.dispose();
    vi.useRealTimers();
    document.body.replaceChildren();
  });

  it('opens in split mode beside the editor (AC1)', async () => {
    const { preview, content, view } = await setup('# Readme\n\ntext');
    expect(content.dataset.mdMode).toBe('split');
    expect([...content.children].map((c) => c.className.split(' ')[0])).toEqual(['cm-editor', 'md-divider', 'md-preview', 'md-toolbar']);
    expect(view.dom.parentElement).toBe(content);
    expect(preview.body.querySelector('h1')?.textContent).toBe('Readme');
    const pressed = [...content.querySelectorAll('.md-mode')].map((b) => b.getAttribute('aria-pressed'));
    expect(pressed).toEqual(['false', 'true', 'false']);
  });

  it('switches modes from the toolbar and the cycle (AC3)', async () => {
    const { preview, content } = await setup('x', { mode: 'editor' });
    expect(content.dataset.mdMode).toBe('editor');
    expect(preview.body.childElementCount).toBe(0); // not rendered while hidden
    content.querySelector<HTMLElement>('[data-mode="preview"]')!.click();
    expect(content.dataset.mdMode).toBe('preview');
    expect(preview.body.textContent).toBe('x');
    expect(['editor', 'split', 'preview'].map((m) => nextMode(m as Mode))).toEqual(['split', 'preview', 'editor']);
    expect([toMode('preview'), toMode('nope'), toMode(undefined)]).toEqual(['preview', 'split', 'split']);
  });

  it('opens large files in Editor mode without rendering', async () => {
    const { preview, content } = await setup('# big', { meta: { mode: 'noHighlight' } });
    expect(content.dataset.mdMode).toBe('editor');
    expect(preview.body.childElementCount).toBe(0);
    preview.setMode('split');
    expect(preview.body.querySelector('h1')).not.toBeNull();
  });

  it('updates within 300 ms of typing, debounced (AC2)', async () => {
    const { preview, view } = await setup('');
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'requestAnimationFrame', 'cancelAnimationFrame'] });
    for (const ch of '# Hello') view.dispatch({ changes: { from: view.state.doc.length, insert: ch } });
    expect(preview.body.querySelector('h1')).toBeNull();
    vi.advanceTimersByTime(300);
    expect(preview.body.querySelector('h1')?.outerHTML).toMatch(/^<h1 id="hello"[^>]*>Hello<\/h1>$/);
  });

  it('keeps unchanged blocks and re-renders everything on theme changes', async () => {
    const { preview, view } = await setup('# A\n\nfirst\n\nsecond\n');
    const [h1, p1, p2] = [...preview.body.children];
    view.dispatch({ changes: { from: view.state.doc.length - 1, insert: '!' } });
    preview.render();
    const after = [...preview.body.children];
    expect(after[0]).toBe(h1);
    expect(after[1]).toBe(p1);
    expect(after[2]).not.toBe(p2);
    expect(after[2].textContent).toBe('second!');
    preview.setTheme('light', 'goi-light');
    preview.render();
    expect(preview.body.children[0]).not.toBe(h1);
  });

  it('never runs scripts (AC4)', async () => {
    const { preview } = await setup('<script>alert(1)</script>\n');
    expect(preview.body.querySelector('script')).toBeNull();
    expect(preview.body.textContent).toBe('<script>alert(1)</script>');
  });

  it('renders mermaid diagrams with copy actions, and errors in place (AC9, AC10)', async () => {
    const { preview } = await setup('```mermaid\ngraph TD; A-->B\n```\n\n```mermaid\nbad\n```\n\nafter\n');
    await flush();
    const [ok, bad, after] = [...preview.body.children] as HTMLElement[];
    expect(ok.dataset.state).toBe('ok');
    expect(ok.querySelector('svg')?.textContent).toBe('graph TD; A-->B');
    expect([...ok.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Copy source', 'Copy SVG']);
    expect(bad.dataset.state).toBe('error');
    expect(bad.querySelector('code')?.textContent).toBe('bad');
    expect(bad.querySelector('.md-error')?.textContent).toBe('Parse error on line 1');
    expect(after.textContent).toBe('after');
    expect(renderDiagram).toHaveBeenCalledWith('graph TD; A-->B', 'dark');
  });

  it('re-renders diagrams in the new theme (AC11)', async () => {
    const { preview } = await setup('```mermaid\ngraph LR; X\n```\n');
    await flush();
    expect(preview.body.querySelector('svg')?.getAttribute('data-theme')).toBe('dark');
    preview.setTheme('light', 'goi-light');
    preview.render();
    await flush();
    expect(preview.body.querySelector('svg')?.getAttribute('data-theme')).toBe('light');
  });

  it('routes link clicks (R1.7)', async () => {
    const env = { openExternal: vi.fn(), openFile: vi.fn(), readImage: vi.fn(), root: () => '/p' };
    const { preview } = await setup('[web](https://go.dev) [doc](docs/a.md#x) [top](#intro) [bad](javascript:alert(1))\n\n# Intro\n', { env });
    const links = [...preview.body.querySelectorAll<HTMLElement>('a')];
    const clicks = links.map((a) => a.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true })));
    expect(clicks).toEqual([false, false, false, false]); // default prevented
    expect(env.openExternal).toHaveBeenCalledWith('https://go.dev');
    expect(env.openFile).toHaveBeenCalledWith('/p/docs/a.md', 'x');
    expect(env.openFile).toHaveBeenCalledTimes(1);
    expect(preview.scrollToAnchor('intro')).toBe(true);
    expect(preview.scrollToAnchor('missing')).toBe(false);
  });

  it('loads local images through the env and falls back to alt text (R1.6)', async () => {
    const env = {
      openExternal: vi.fn(),
      openFile: vi.fn(),
      readImage: vi.fn(async (p: string) => {
        if (p.endsWith('missing.png')) throw new Error('nope');
        return 'data:image/png;base64,AAAA';
      }),
      root: () => '/p',
    };
    const { preview } = await setup('![logo](img/logo.png) ![gone](img/missing.png) ![x](javascript:y) ![again](img/logo.png) ![web](https://x.io/a.png)\n', { env });
    await flush();
    const imgs = [...preview.body.querySelectorAll('img')];
    expect(imgs.map((i) => i.getAttribute('src'))).toEqual(['data:image/png;base64,AAAA', 'data:image/png;base64,AAAA', 'https://x.io/a.png']);
    expect([...preview.body.querySelectorAll('.md-img-alt')].map((s) => s.textContent)).toEqual(['gone', 'x']);
    expect(env.readImage.mock.calls.map((c) => c[0])).toEqual(['/p/img/logo.png', '/p/img/missing.png']); // cached
    preview.filesChanged(['/p/img/logo.png']);
    expect(env.readImage).toHaveBeenCalledTimes(3); // both logo images reload from one new read
  });

  it('disposes its elements', async () => {
    const { preview, content } = await setup('x');
    preview.dispose();
    expect(content.querySelector('.md-preview, .md-toolbar, .md-divider')).toBeNull();
    expect(content.classList.contains('md-tab')).toBe(false);
  });
});

describe('helpers', () => {
  it('reconcile reuses keyed nodes in order', () => {
    const body = document.createElement('div');
    reconcile(body, renderBlocks('a\n\nb\n\nc\n'));
    const [a, b, c] = [...body.children];
    reconcile(body, renderBlocks('c\n\na\n\nnew\n\nb\n'));
    expect([...body.children].map((e) => e.textContent)).toEqual(['c', 'a', 'new', 'b']);
    expect(body.children[0]).toBe(c);
    expect(body.children[1]).toBe(a);
    expect(body.children[3]).toBe(b);
    expect((body.children[0] as HTMLElement).dataset.line).toBe('0');
  });

  it('scrollTarget interpolates between blocks', () => {
    const anchors = [
      { line: 0, top: 0 },
      { line: 10, top: 200 },
      { line: 20, top: 260 },
    ];
    expect(scrollTarget(anchors, 0)).toBe(0);
    expect(scrollTarget(anchors, 5)).toBe(100);
    expect(scrollTarget(anchors, 15)).toBe(230);
    expect(scrollTarget(anchors, 40)).toBe(260);
    expect(scrollTarget([], 3)).toBe(0);
    expect(scrollTarget([{ line: 4, top: 50 }], 1)).toBe(0);
  });

  it('parseSvg returns an svg element without scripts running', () => {
    const svg = parseSvg('<svg xmlns="http://www.w3.org/2000/svg"><g><text>hi<br>there</text></g></svg>');
    expect(svg?.tagName.toLowerCase()).toBe('svg');
    expect(parseSvg('not svg')).toBeNull();
  });
});
