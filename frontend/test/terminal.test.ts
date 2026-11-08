// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { allowedInTerminal } from '../src/keymap/dispatcher';
import { parseOsc7 } from '../src/terminal/terminal';

describe('terminal', () => {
  it('parses OSC 7 working directories', () => {
    expect(parseOsc7('file://host/Users/me/my%20proj')).toBe('/Users/me/my proj');
    expect(parseOsc7('file:///home/u')).toBe('/home/u');
    expect(parseOsc7('file://pc/C:/Users/x')).toBe('C:/Users/x');
    expect(parseOsc7('nonsense')).toBeNull();
  });
  it('lets the shell keep Ctrl keys while passing allowlisted commands', () => {
    expect(allowedInTerminal('run.run', 'Ctrl-r', true)).toBe(false);
    expect(allowedInTerminal('run.debug', 'Ctrl-d', false)).toBe(false);
    expect(allowedInTerminal('terminal.toggle', 'Alt-F12', true)).toBe(true);
    expect(allowedInTerminal('debug.stepOver', 'F8', false)).toBe(true);
    expect(allowedInTerminal('nav.gotoFile', 'Ctrl-Shift-o', false)).toBe(true);
    expect(allowedInTerminal('run.stop', 'Meta-F2', true)).toBe(true);
    expect(allowedInTerminal('editor.find', 'Meta-f', true)).toBe(false);
    expect(allowedInTerminal('view.run', 'Meta-4', true)).toBe(true);
  });
});
