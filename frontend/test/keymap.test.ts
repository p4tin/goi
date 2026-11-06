// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  formatForDisplay,
  hasCommandModifier,
  parseSequence,
  resolveMod,
  stringifyStroke,
  strokeFromEvent,
  toModForm,
} from '../src/keymap/chords';
import { KeyDispatcher } from '../src/keymap/dispatcher';
import type { DispatchEvent } from '../src/keymap/dispatcher';
import { commandBindings, effectiveBindings, findConflicts, keymaps } from '../src/keymap/keymaps';

const ev = (o: Partial<DispatchEvent> & { key: string; code?: string }): DispatchEvent & { prevented: boolean } => {
  const e = {
    code: o.code ?? (o.key.length === 1 && /[a-z]/i.test(o.key) ? `Key${o.key.toUpperCase()}` : o.key),
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    target: document.body,
    prevented: false,
    preventDefault() {
      e.prevented = true;
    },
    stopPropagation() {},
    ...o,
  };
  return e as DispatchEvent & { prevented: boolean };
};

describe('chords', () => {
  it('parses and normalizes modifiers and keys', () => {
    expect(parseSequence('Shift-Mod-S').map(stringifyStroke)).toEqual(['Mod-Shift-s']);
    expect(parseSequence('cmd-option-,').map(stringifyStroke)).toEqual(['Alt-Meta-Comma']);
    expect(parseSequence('Mod-k   Mod-s').map(stringifyStroke)).toEqual(['Mod-k', 'Mod-s']);
    expect(parseSequence('Ctrl-esc').map(stringifyStroke)).toEqual(['Ctrl-Escape']);
    expect(parseSequence('Mod--').map(stringifyStroke)).toEqual(['Mod-Minus']);
    expect(parseSequence('f2').map(stringifyStroke)).toEqual(['F2']);
    expect(() => parseSequence('Hyper-x')).toThrow(/unknown modifier/);
  });

  it('resolves Mod per platform and back', () => {
    expect(resolveMod('Mod-k Mod-Shift-s', true)).toBe('Meta-k Meta-Shift-s');
    expect(resolveMod('Mod-k Mod-Shift-s', false)).toBe('Ctrl-k Ctrl-Shift-s');
    expect(toModForm('Meta-Shift-s', true)).toBe('Mod-Shift-s');
    expect(toModForm('Ctrl-Tab', true)).toBe('Ctrl-Tab');
    expect(toModForm('Ctrl-Shift-s', false)).toBe('Mod-Shift-s');
  });

  it('formats for display', () => {
    expect(formatForDisplay('Mod-Shift-s', true)).toBe('⇧⌘S');
    expect(formatForDisplay('Mod-Shift-s', false)).toBe('Ctrl+Shift+S');
    expect(formatForDisplay('Mod-k Mod-Backslash', false)).toBe('Ctrl+K Ctrl+\\');
    expect(formatForDisplay('Ctrl-Alt-ArrowUp', true)).toBe('⌃⌥↑');
    expect(formatForDisplay('Mod-Comma', true)).toBe('⌘,');
  });

  it('strokeFromEvent uses code for letters (Alt on mac) and ignores modifiers', () => {
    expect(strokeFromEvent(ev({ key: 'ß', code: 'KeyS', altKey: true, metaKey: true }), true)).toBe('Alt-Meta-s');
    expect(strokeFromEvent(ev({ key: 'S', code: 'KeyS', ctrlKey: true, shiftKey: true }), false)).toBe('Ctrl-Shift-s');
    expect(strokeFromEvent(ev({ key: '!', code: 'Digit1', shiftKey: true, metaKey: true }), true)).toBe('Meta-Shift-1');
    expect(strokeFromEvent(ev({ key: '\\', code: 'Backslash', metaKey: true }), true)).toBe('Meta-Backslash');
    expect(strokeFromEvent(ev({ key: 'ArrowRight', code: 'ArrowRight' }), true)).toBe('ArrowRight');
    expect(strokeFromEvent(ev({ key: 'Shift', code: 'ShiftLeft', shiftKey: true }), true)).toBeNull();
    expect(hasCommandModifier('Shift-a')).toBe(false);
    expect(hasCommandModifier('Mod-a')).toBe(true);
  });
});

describe('keymaps', () => {
  it('every built-in sequence parses', () => {
    for (const km of Object.values(keymaps)) {
      for (const isMac of [true, false]) {
        for (const seqs of commandBindings(km === keymaps.vscode ? 'vscode' : 'goi-default', {}, isMac).values()) {
          for (const s of seqs) expect(() => parseSequence(s)).not.toThrow();
        }
      }
    }
  });

  it('applies platform entries and overrides', () => {
    const goiOther = commandBindings('goi-default', {}, false);
    expect(goiOther.get('editor.close')).toEqual(['Ctrl-w', 'Ctrl-F4']);
    expect(commandBindings('goi-default', {}, true).get('editor.close')).toEqual(['Meta-w']);
    expect(commandBindings('vscode', {}, true).get('editor.replace')).toEqual(['Alt-Meta-f']);
    expect(commandBindings('vscode', {}, false).get('editor.replace')).toEqual(['Ctrl-h']);

    const eff = effectiveBindings('goi-default', { 'file.save': [], 'my.cmd': ['Mod-Shift-y'], bad: ['Hyper-q'] }, true);
    expect([...eff.values()]).not.toContain('file.save');
    expect(eff.get('Meta-Shift-y')).toBe('my.cmd');
    expect(eff.get('Meta-k Meta-Backslash')).toBe('editor.splitDown');
  });

  it('unknown keymap ids fall back to goi-default', () => {
    expect(commandBindings('nope' as 'vscode', {}, true).get('editor.gotoLine')).toEqual(['Meta-l']);
  });

  it('findConflicts detects equal and chord-prefix collisions', () => {
    const b = effectiveBindings('goi-default', {}, true);
    expect(findConflicts(b, 'x', 'Meta-s')).toEqual(['file.save']);
    expect(findConflicts(b, 'file.save', 'Meta-s')).toEqual([]);
    // Meta-k is a prefix of chords
    expect(findConflicts(b, 'x', 'Meta-k').sort()).toEqual(['editor.focusNextPane', 'editor.focusPrevPane', 'editor.splitDown']);
    // A chord whose first stroke is an existing single binding
    expect(findConflicts(b, 'x', 'Meta-s Meta-x')).toEqual(['file.save']);
  });
});

describe('KeyDispatcher', () => {
  let runs: string[];
  let hints: (string | null)[];
  let d: KeyDispatcher;
  const map = effectiveBindings('goi-default', {}, true);

  beforeEach(() => {
    vi.useFakeTimers();
    runs = [];
    hints = [];
    d = new KeyDispatcher({
      bindings: () => map,
      canRun: (id) => id !== 'editor.find',
      run: (id) => runs.push(id),
      isMac: () => true,
      onPending: (h) => hints.push(h),
    });
  });
  afterEach(() => vi.useRealTimers());

  it('runs a full match and consumes the event', () => {
    const e = ev({ key: 's', metaKey: true });
    expect(d.handle(e)).toBe(true);
    expect(e.prevented).toBe(true);
    expect(runs).toEqual(['file.save']);
  });

  it('passes through unbound keys and disabled commands', () => {
    expect(d.handle(ev({ key: 'x' }))).toBe(false);
    expect(d.handle(ev({ key: 'f', metaKey: true }))).toBe(false);
    expect(runs).toEqual([]);
  });

  it('waits on a chord prefix, then runs the chord', () => {
    expect(d.handle(ev({ key: 'k', metaKey: true }))).toBe(true);
    expect(d.pendingSequence).toBe('Meta-k');
    expect(hints[0]).toContain('⌘K');
    expect(d.handle(ev({ key: '\\', code: 'Backslash', metaKey: true }))).toBe(true);
    expect(runs).toEqual(['editor.splitDown']);
    expect(hints.at(-1)).toBeNull();
  });

  it('resets after the timeout', () => {
    d.handle(ev({ key: 'k', metaKey: true }));
    vi.advanceTimersByTime(1600);
    expect(d.pendingSequence).toBe('');
    d.handle(ev({ key: '\\', code: 'Backslash', metaKey: true }));
    expect(runs).toEqual(['editor.splitRight']);
  });

  it('swallows a non-matching continuation and resets', () => {
    d.handle(ev({ key: 'k', metaKey: true }));
    const e = ev({ key: 'q' });
    expect(d.handle(e)).toBe(true);
    expect(runs).toEqual([]);
    expect(d.pendingSequence).toBe('');
  });

  it('skips plain keys in text fields but not modified ones', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    expect(d.handle(ev({ key: 'e', target: input }))).toBe(false);
    expect(d.handle(ev({ key: 's', metaKey: true, target: input }))).toBe(true);
    expect(runs).toEqual(['file.save']);
  });

  it('handles keys inside CodeMirror and ignores dialogs', () => {
    const cm = document.createElement('div');
    cm.className = 'cm-editor';
    const content = document.createElement('div');
    content.contentEditable = 'true';
    cm.appendChild(content);
    const dialog = document.createElement('div');
    dialog.className = 'dialog';
    const field = document.createElement('input');
    dialog.appendChild(field);
    document.body.append(cm, dialog);
    expect(d.handle(ev({ key: 's', metaKey: true, target: content }))).toBe(true);
    expect(d.handle(ev({ key: 's', metaKey: true, target: field }))).toBe(false);
    expect(runs).toEqual(['file.save']);
  });
});
