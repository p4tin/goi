// @vitest-environment happy-dom
import { EditorState } from '@codemirror/state';
import { EditorView, lineNumbers } from '@codemirror/view';
import { describe, expect, it } from 'vitest';
import {
  breakpointClass,
  breakpointsExtension,
  describeBreakpoint,
  hitConditionLabel,
  lineBreakpoints,
  newBreakpoint,
  normalize,
  toggleBreakpoint,
  toggleIn,
  updateBreakpoint,
} from '../src/debug/breakpoints';
import type { Buffer } from '../src/editor/buffer';

describe('breakpoint helpers', () => {
  it('toggles and normalizes', () => {
    let l = toggleIn([], 5);
    l = toggleIn(l, 2);
    expect(l.map((b) => b.line)).toEqual([2, 5]);
    expect(toggleIn(l, 5).map((b) => b.line)).toEqual([2]);
    expect(normalize([newBreakpoint(3), newBreakpoint(3), newBreakpoint(0), newBreakpoint(1)]).map((b) => b.line)).toEqual([1, 3]);
  });
  it('labels hit conditions and describes breakpoints', () => {
    expect(hitConditionLabel('3')).toBe('== 3');
    expect(hitConditionLabel('>= 5')).toBe('>= 5');
    const b = { ...newBreakpoint(7), condition: 'i == 2', hitCondition: '3' };
    expect(describeBreakpoint(b)).toBe('Line 7, when i == 2, hit count == 3');
    expect(describeBreakpoint({ ...newBreakpoint(1), enabled: false })).toBe('Line 1, disabled');
    expect(describeBreakpoint(newBreakpoint(2), { verified: false, message: 'no code' })).toBe('Line 2, no code');
  });
  it('picks marker classes', () => {
    expect(breakpointClass(newBreakpoint(1))).toBe('bp');
    expect(breakpointClass({ ...newBreakpoint(1), condition: 'x' })).toBe('bp bp-cond');
    expect(breakpointClass({ ...newBreakpoint(1), logMessage: 'x={x}' })).toBe('bp bp-log');
    expect(breakpointClass({ ...newBreakpoint(1), enabled: false })).toBe('bp bp-disabled');
    expect(breakpointClass(newBreakpoint(1), { verified: false, message: '' })).toBe('bp bp-unverified');
  });
  it('updates and removes through the store', () => {
    lineBreakpoints.set({});
    toggleBreakpoint('/p/a.go', 4);
    updateBreakpoint('/p/a.go', 4, { condition: 'n > 1' });
    expect(lineBreakpoints.peek()['/p/a.go']).toEqual([{ ...newBreakpoint(4), condition: 'n > 1' }]);
    updateBreakpoint('/p/a.go', 4, null);
    expect(lineBreakpoints.peek()['/p/a.go']).toBeUndefined();
  });
});

describe('breakpoint gutter', () => {
  const buf = { path: '/p/main.go', binary: false } as unknown as Buffer;
  const mount = (doc: string): EditorView => {
    const parent = document.createElement('div');
    document.body.appendChild(parent);
    return new EditorView({ state: EditorState.create({ doc, extensions: [lineNumbers(), breakpointsExtension(buf)] }), parent });
  };
  const tick = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));

  it('renders markers and follows edits', async () => {
    lineBreakpoints.set({ '/p/main.go': [newBreakpoint(3)] });
    const view = mount('a\nb\nc\nd\n');
    await tick(0);
    const visible = [...view.dom.querySelectorAll<HTMLElement>('.cm-bp-gutter .bp')].filter((el) => (el.closest('.cm-gutterElement') as HTMLElement).style.visibility !== 'hidden');
    expect(visible.length).toBe(1);
    view.dispatch({ changes: { from: 0, insert: 'new line\n' } });
    await tick(350);
    expect(lineBreakpoints.peek()['/p/main.go'].map((b) => b.line)).toEqual([4]);
    view.destroy();
  });
  it('ignores non-Go buffers', () => {
    expect(breakpointsExtension({ path: '/p/README.md', binary: false } as unknown as Buffer)).toEqual([]);
  });
});
