import { describe, expect, it, vi } from 'vitest';

vi.mock('../src/lib/api', () => ({
  menuApi: { set: async () => {} },
  settingsApi: {},
  on: () => () => {},
  Topics: { menuCommand: 'menu.command' },
}));

const { buildMenuSpecs, toAccelerator } = await import('../src/app/menuBar');
const { CommandRegistry } = await import('../src/lib/commands');

describe('toAccelerator', () => {
  it.each([
    ['Meta-1', true, 'cmdorctrl+1'],
    ['Meta-Shift-1', true, 'cmdorctrl+shift+1'],
    ['Ctrl-Tab', true, 'ctrl+tab'],
    ['Meta-Alt-s', true, 'cmdorctrl+optionoralt+s'],
    ['Meta-Comma', true, 'cmdorctrl+,'],
    ['Meta-Backslash', true, 'cmdorctrl+\\'],
    ['Ctrl-Shift-o', false, 'cmdorctrl+shift+o'],
    ['Shift-Alt-f', false, 'optionoralt+shift+f'],
    ['F4', false, 'f4'],
    ['Ctrl-ArrowRight', false, 'cmdorctrl+right'],
  ])('%s (mac=%s) → %s', (seq, mac, want) => {
    expect(toAccelerator(seq, mac)).toBe(want);
  });

  it('skips chords and Meta outside macOS', () => {
    expect(toAccelerator('Meta-k Meta-Backslash', true)).toBe('');
    expect(toAccelerator('Meta-1', false)).toBe('');
  });
});

describe('buildMenuSpecs', () => {
  const reg = new CommandRegistry();
  for (const [id, title] of [
    ['a.one', 'One'],
    ['a.two', 'Two'],
    ['b.three', 'Three'],
  ]) reg.register({ id, title, run: () => {} });

  it('skips unknown commands, tidies separators, drops empty menus, adds shortcuts', () => {
    const specs = buildMenuSpecs(
      [
        { label: 'A', items: ['-', 'a.one', '-', '-', 'missing', '-', 'a.two', '-'] },
        { label: 'Empty', items: ['missing', '-'] },
        { label: 'B', items: ['b.three'] },
      ],
      reg,
      new Map([
        ['a.one', ['Meta-k Meta-1', 'Meta-1']],
        ['b.three', ['Meta-k Meta-3']],
      ]),
      true,
    );
    expect(specs.map((s) => s.label)).toEqual(['A', 'B']);
    expect(specs[0].items).toEqual([
      { label: 'One', command: 'a.one', accel: 'cmdorctrl+1', separator: false },
      { label: '', command: '', accel: '', separator: true },
      { label: 'Two', command: 'a.two', accel: '', separator: false },
    ]);
    expect(specs[1].items[0].accel).toBe('');
  });
});

describe('runFrom', () => {
  it('drops a second run of the same command from another source', async () => {
    const reg = new CommandRegistry();
    let n = 0;
    reg.register({ id: 'x', title: 'X', run: () => void n++ });
    await reg.runFrom('key', 'x');
    await reg.runFrom('menu', 'x');
    expect(n).toBe(1);
    await reg.runFrom('menu', 'x');
    expect(n).toBe(2);
  });
});
