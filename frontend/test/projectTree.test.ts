// @vitest-environment happy-dom
import { beforeAll, describe, expect, it, vi } from 'vitest';

const listing: Record<string, { name: string; path: string; isDir: boolean }[]> = {
  '/p': [
    { name: 'cmd', path: '/p/cmd', isDir: true },
    { name: 'main.go', path: '/p/main.go', isDir: false },
  ],
  '/p/cmd': [{ name: 'tool.go', path: '/p/cmd/tool.go', isDir: false }],
};

vi.mock('../src/lib/api', () => ({
  app: {},
  workspaceApi: {},
  files: { list: async (dir: string) => listing[dir] ?? [] },
  on: () => () => {},
  Topics: { fsChanged: 'fs.changed' },
}));

const { installProjectTree } = await import('../src/views/projectTree');
const { project } = await import('../src/app/state');
const { commands } = await import('../src/lib/commands');

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

// Real mouse input: the button comes up on whatever element is at the row by
// then, and click/dblclick go to the common ancestor of the press and release
// targets (as browsers do).
function realClick(container: HTMLElement, path: string, count: 1 | 2): void {
  const at = (): HTMLElement => container.querySelector<HTMLElement>(`.tree-row[title="${path}"]`)!;
  const common = (a: Node, b: Node): Node => {
    for (let n: Node | null = a; n; n = n.parentNode) if (n.contains(b)) return n;
    return container;
  };
  let target: Node = container;
  for (let i = 1; i <= count; i++) {
    const down = at();
    down.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, detail: i }));
    const up = at();
    up.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, detail: i }));
    target = down.isConnected ? common(down, up) : up.parentNode ?? container;
    target.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: i }));
  }
  if (count === 2) target.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, detail: 2 }));
}

describe('project tree mouse input', () => {
  const container = document.createElement('div');
  const opened: unknown[] = [];

  beforeAll(async () => {
    // Give the virtualizer a viewport so it renders rows.
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, get: () => 600 });
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 });
    document.body.append(container);

    let render: ((c: HTMLElement) => void) | null = null;
    const wb = {
      panels: { register: (p: { render: (c: HTMLElement) => void }) => (render = p.render), show: () => {} },
      layout: { register: () => {}, scheduleSave: () => {} },
      notifications: { push: () => {} },
    };
    installProjectTree(wb as never);
    commands.register({ id: 'editor.open', title: 'Open', run: (p?: unknown) => void opened.push(p) });
    project.set({ root: '/p', name: 'p', contentRoots: ['/p'] } as never);
    render!(container);
    await flush();
    await flush();
  });

  it('keeps the pressed row element through mousedown', () => {
    const before = container.querySelector('.tree-row[title="/p/main.go"]');
    expect(before).not.toBeNull();
    before!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    expect(container.querySelector('.tree-row[title="/p/main.go"]')).toBe(before);
  });

  it('double-clicking a file opens it', () => {
    realClick(container, '/p/main.go', 2);
    expect(opened).toContain('/p/main.go');
  });

  it('double-clicking a folder expands it', async () => {
    realClick(container, '/p/cmd', 2);
    await flush();
    expect(container.querySelector('.tree-row[title="/p/cmd/tool.go"]')).not.toBeNull();
  });
});
