import { describe, expect, it } from 'vitest';
import { batch, computed, effect, signal } from '../src/lib/store';

describe('store', () => {
  it('signal get/set/update', () => {
    const s = signal(1);
    s.set(2);
    s.update((v) => v + 1);
    expect(s()).toBe(3);
  });

  it('effect re-runs on dependency change and stops after dispose', () => {
    const s = signal(0);
    const seen: number[] = [];
    const dispose = effect(() => {
      seen.push(s());
    });
    s.set(1);
    s.set(1); // equal value: no re-run
    dispose();
    s.set(2);
    expect(seen).toEqual([0, 1]);
  });

  it('effect cleanup runs before re-run', () => {
    const s = signal(0);
    const log: string[] = [];
    effect(() => {
      const v = s();
      log.push(`run ${v}`);
      return () => log.push(`clean ${v}`);
    });
    s.set(1);
    expect(log).toEqual(['run 0', 'clean 0', 'run 1']);
  });

  it('tracks dynamic dependencies', () => {
    const flag = signal(true);
    const a = signal('a');
    const b = signal('b');
    const seen: string[] = [];
    effect(() => {
      seen.push(flag() ? a() : b());
    });
    flag.set(false);
    a.set('A'); // no longer a dependency
    b.set('B');
    expect(seen).toEqual(['a', 'b', 'B']);
  });

  it('computed derives values', () => {
    const s = signal(2);
    const doubled = computed(() => s() * 2);
    s.set(5);
    expect(doubled()).toBe(10);
  });

  it('batch coalesces effect runs', () => {
    const a = signal(0);
    const b = signal(0);
    let runs = 0;
    effect(() => {
      a();
      b();
      runs++;
    });
    batch(() => {
      a.set(1);
      b.set(1);
    });
    expect(runs).toBe(2);
  });

  it('peek does not track', () => {
    const s = signal(0);
    let runs = 0;
    effect(() => {
      s.peek();
      runs++;
    });
    s.set(1);
    expect(runs).toBe(1);
  });
});
