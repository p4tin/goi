import { describe, expect, it } from 'vitest';
import { CommandRegistry } from '../src/lib/commands';

describe('CommandRegistry', () => {
  it('registers, runs and unregisters', async () => {
    const r = new CommandRegistry();
    const off = r.register({ id: 'a', title: 'A', run: (x) => `ran ${x}` });
    expect(await r.run('a', 1)).toBe('ran 1');
    off();
    expect(r.get('a')).toBeUndefined();
  });

  it('rejects duplicate ids', () => {
    const r = new CommandRegistry();
    r.register({ id: 'a', title: 'A', run: () => {} });
    expect(() => r.register({ id: 'a', title: 'A2', run: () => {} })).toThrow(/already registered/);
  });

  it('unknown commands reject', async () => {
    await expect(new CommandRegistry().run('nope')).rejects.toThrow(/unknown command/);
  });

  it('respects when()', async () => {
    const r = new CommandRegistry();
    let enabled = false;
    let ran = 0;
    r.register({ id: 'a', title: 'A', when: () => enabled, run: () => ran++ });
    expect(r.isEnabled('a')).toBe(false);
    await r.run('a');
    enabled = true;
    await r.run('a');
    expect(ran).toBe(1);
  });

  it('notifies listeners on change', () => {
    const r = new CommandRegistry();
    let n = 0;
    r.onDidChange(() => n++);
    const off = r.register({ id: 'a', title: 'A', run: () => {} });
    off();
    expect(n).toBe(2);
  });
});
