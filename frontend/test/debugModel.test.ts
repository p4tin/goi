// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import {
  filterGoroutines,
  formatValue,
  functionStart,
  goroutineLabel,
  groupByFunction,
  inlineValues,
  inProject,
  isByteSlice,
  isUnreadable,
  parseGoroutine,
  stripAddresses,
  truncate,
} from '../src/debug/model';
import { VarTree, varNode } from '../src/debug/variables';

const threads = [
  { id: 1, name: '[Go 1] sync.runtime_SemacquireWaitGroup' },
  { id: 2, name: '[Go 2] runtime.gopark' },
  { id: 3, name: '[Go 3] runtime.gopark' },
  { id: 34, name: '[Go 34] main.work (Thread 8510400)' },
  { id: 36, name: '* [Go 36] main.work (Thread 8509839)' },
];

describe('goroutines', () => {
  it('parses Delve thread names', () => {
    expect(parseGoroutine(threads[4])).toEqual({ id: 36, current: true, func: 'main.work', thread: '8509839', extra: '', name: threads[4].name });
    expect(parseGoroutine(threads[1])).toMatchObject({ id: 2, current: false, func: 'runtime.gopark', thread: '' });
    expect(parseGoroutine({ id: 9, name: 'odd' })).toMatchObject({ id: 9, func: 'odd' });
    expect(parseGoroutine({ id: 5, name: '[Go 5] main.serve label=x' })).toMatchObject({ func: 'main.serve', extra: 'label=x' });
  });
  it('groups by function with the current group first', () => {
    const gs = groupByFunction(threads.map(parseGoroutine));
    expect(gs.map((g) => `${g.func}:${g.items.length}`)).toEqual(['main.work:2', 'runtime.gopark:2', 'sync.runtime_SemacquireWaitGroup:1']);
  });
  it('filters by text and hides, never hiding the current goroutine', () => {
    const all = threads.map(parseGoroutine);
    expect(filterGoroutines(all, 'work', new Set()).map((g) => g.id)).toEqual([34, 36]);
    expect(filterGoroutines(all, '#2', new Set()).map((g) => g.id)).toEqual([2]);
    expect(filterGoroutines(all, '8509839', new Set()).map((g) => g.id)).toEqual([36]);
    expect(filterGoroutines(all, '', new Set([2, 36])).map((g) => g.id)).toEqual([1, 3, 34, 36]);
  });
  it('labels with optional thread ids', () => {
    const g = parseGoroutine(threads[4]);
    expect(goroutineLabel(g, false)).toBe('Goroutine 36 main.work');
    expect(goroutineLabel(g, true)).toBe('Goroutine 36 main.work (thread 8509839)');
  });
});

describe('values', () => {
  it('strips addresses unless shown', () => {
    expect(stripAddresses('(*int)(0x14000112008)')).toBe('(*int)');
    expect(stripAddresses('*main.T {p: 0xc000123456, n: 2}')).toBe('*main.T {p: …, n: 2}');
    expect(formatValue('(*int)(0x14000112008)', { showAddresses: true })).toBe('(*int)(0x14000112008)');
    expect(stripAddresses('0x1f')).toBe('0x1f');
  });
  it('detects unreadable values and byte slices', () => {
    expect(isUnreadable('(unreadable could not read)')).toBe(true);
    expect(isUnreadable('5')).toBe(false);
    expect(isByteSlice('[]uint8')).toBe(true);
    expect(isByteSlice('[]byte')).toBe(true);
    expect(isByteSlice('[16]uint8')).toBe(true);
    expect(isByteSlice('[]int')).toBe(false);
    expect(truncate('a  b\nc', 60)).toBe('a b c');
    expect(truncate('x'.repeat(70), 10)).toBe('xxxxxxxxx…');
  });
  it('knows project frames', () => {
    expect(inProject('/r/p/main.go', '/r/p')).toBe(true);
    expect(inProject('/go/src/runtime/proc.go', '/r/p')).toBe(false);
    expect(inProject(undefined, '/r/p')).toBe(false);
  });
});

describe('inline values', () => {
  const src = [
    'package main', // 1
    '', // 2
    'func work(i int, wg *sync.WaitGroup) {', // 3
    '\tdefer wg.Done()', // 4
    '\tx := i * 2 // i doubled', // 5
    '\tfmt.Println("x is", x, s.x)', // 6
    '}', // 7
  ];
  it('finds the function start', () => {
    expect(functionStart(src, 6)).toBe(3);
    expect(functionStart(src, 1)).toBe(1);
  });
  it('lists mentioned variables per line up to the stop line', () => {
    const values = new Map([
      ['i', '2'],
      ['x', '4'],
      ['wg', '*sync.WaitGroup {…}'],
    ]);
    const got = inlineValues(src, 6, values);
    expect([...got.entries()]).toEqual([
      [3, ['i = 2', 'wg = *sync.WaitGroup {…}']],
      [4, ['wg = *sync.WaitGroup {…}']],
      [5, ['x = 4', 'i = 2']],
      [6, ['x = 4']],
    ]);
    expect(inlineValues(src, 6, new Map()).size).toBe(0);
  });
});

describe('variables tree', () => {
  it('renders nodes with types, hides unreadable values on request', () => {
    let unreadable = true;
    const tree = new VarTree({
      sessionId: () => 's',
      showTypes: () => true,
      showAddresses: () => false,
      showUnreadable: () => unreadable,
      canSetValue: () => false,
      frameId: () => 1,
    });
    const v = (name: string, value: string, type: string) => varNode({ name, value, type, evaluateName: name, variablesReference: 0, namedVariables: 0, indexedVariables: 0 }, 'scope:Locals', 1000);
    tree.setRoots([v('i', '2', 'int'), v('p', '(*int)(0x14000112008)', '*int'), v('bad', '(unreadable could not read)', 'string')]);
    const text = (): string[] => [...tree.el.querySelectorAll('.vt-row')].map((r) => r.textContent ?? '');
    expect(text()).toEqual(['iint=2', 'p*int=(*int)', 'badstring=(unreadable could not read)']);
    unreadable = false;
    tree.render();
    expect(text()).toHaveLength(2);
  });
});
