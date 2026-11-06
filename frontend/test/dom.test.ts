// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { Disposables, h } from '../src/lib/dom';

describe('h', () => {
  it('builds elements with props, listeners and children', () => {
    let clicks = 0;
    const el = h('button', { class: 'btn', title: 'Go', dataset: { id: '7' }, onClick: () => clicks++ }, 'Run ', 1, null, [
      h('span', null, 'x'),
    ]);
    el.click();
    expect(el.className).toBe('btn');
    expect(el.getAttribute('title')).toBe('Go');
    expect(el.dataset.id).toBe('7');
    expect(el.textContent).toBe('Run 1x');
    expect(clicks).toBe(1);
  });

  it('sets boolean and property values', () => {
    const input = h('input', { disabled: true, value: 'v' });
    expect(input.disabled).toBe(true);
    expect(input.value).toBe('v');
  });
});

describe('Disposables', () => {
  it('runs in reverse order once', () => {
    const d = new Disposables();
    const log: number[] = [];
    d.add(() => log.push(1));
    d.add(() => log.push(2));
    d.dispose();
    d.dispose();
    expect(log).toEqual([2, 1]);
  });
});
