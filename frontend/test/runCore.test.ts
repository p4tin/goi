// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';
import { AnsiParser, applySgr, color256, stripAnsi } from '../src/run/ansi';
import { Console } from '../src/run/console';
import { bestCandidate, findLocations, joinPath } from '../src/run/links';
import { envToText, splitList, textToEnv, uniqueName } from '../src/run/configDialog';
import { exitLine } from '../src/run/state';

describe('ansi', () => {
  it('parses SGR colors and attributes into segments', () => {
    const p = new AnsiParser();
    const segs = p.feed('plain \x1b[1;31mred bold\x1b[0m end');
    expect(segs).toEqual([
      { text: 'plain ', style: {} },
      { text: 'red bold', style: { bold: true, fg: 'p1' } },
      { text: ' end', style: {} },
    ]);
  });
  it('keeps style and split escapes across chunks', () => {
    const p = new AnsiParser();
    expect(p.feed('a\x1b[3')).toEqual([{ text: 'a', style: {} }]);
    expect(p.feed('2mgreen')).toEqual([{ text: 'green', style: { fg: 'p2' } }]);
    expect(p.feed(' still')).toEqual([{ text: ' still', style: { fg: 'p2' } }]);
  });
  it('handles 256 and true colors, bright colors and resets', () => {
    expect(applySgr({}, [38, 5, 196]).fg).toBe(color256(196));
    expect(color256(196)).toBe('rgb(255,0,0)');
    expect(color256(244)).toBe('rgb(128,128,128)');
    expect(color256(9)).toBe('p9');
    expect(applySgr({}, [48, 2, 1, 2, 3]).bg).toBe('rgb(1,2,3)');
    expect(applySgr({}, [94]).fg).toBe('p12');
    expect(applySgr({ fg: 'p1', bold: true }, [39, 22])).toEqual({});
  });
  it('drops cursor movement, OSC and other controls', () => {
    expect(stripAnsi('\x1b[2K\x1b[1Gdone\x1b]0;title\x07!\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\')).toBe('done!link');
    expect(stripAnsi('ab\bc\x07')).toBe('ac');
  });
});

describe('links', () => {
  it('finds Go locations in compiler, test and panic output', () => {
    expect(findLocations('./main.go:12:5: undefined: x')).toEqual([{ start: 0, end: 14, path: './main.go', line: 12, col: 5 }]);
    expect(findLocations('    calc_test.go:9: expected failure')[0]).toMatchObject({ path: 'calc_test.go', line: 9, col: 1 });
    expect(findLocations('\t/home/u/src/app/main.go:42 +0x1d')[0]).toMatchObject({ path: '/home/u/src/app/main.go', line: 42 });
    expect(findLocations('C:\\src\\app\\x.go:3')[0]).toMatchObject({ path: 'C:\\src\\app\\x.go', line: 3 });
    expect(findLocations('see http://host/x.go:80 for details')).toEqual([]);
    expect(findLocations('main.go:0')).toEqual([]);
  });
  it('joins and normalizes relative paths', () => {
    expect(joinPath('/p/root', './pkg/../cmd/x.go')).toBe('/p/root/cmd/x.go');
    expect(joinPath('/p/root', '/abs/x.go')).toBe('/abs/x.go');
    expect(joinPath('C:\\p', 'a\\b.go')).toBe('C:\\p\\a\\b.go');
  });
  it('prefers candidates under the base directory', () => {
    const c = ['/r/a/calc_test.go', '/r/calc/calc_test.go', '/r/x/other.go'];
    expect(bestCandidate('calc_test.go', c, '/r/calc')).toBe('/r/calc/calc_test.go');
    expect(bestCandidate('a/calc_test.go', c, '/r/calc')).toBe('/r/a/calc_test.go');
    expect(bestCandidate('nope.go', c, '/r')).toBeNull();
  });
});

describe('console', () => {
  const lines = (c: Console): string[] => [...c.body.children].map((l) => l.textContent ?? '');

  it('renders lines with stream and ANSI classes', () => {
    const c = new Console({ maxLines: 1000 });
    c.append('stdout', 'one\n\x1b[32mtwo');
    c.append('stdout', ' more\n');
    c.append('stderr', 'oops\n');
    expect(lines(c)).toEqual(['one', 'two more', 'oops']);
    expect(c.body.querySelector('.ansi-fg-p2')?.textContent).toBe('two');
    expect(c.body.querySelector('.con-stderr')?.textContent).toBe('oops');
    expect(c.lines).toBe(3);
  });
  it('overwrites the current line on carriage return', () => {
    const c = new Console({ maxLines: 1000 });
    c.append('stdout', 'progress 10%\r');
    c.append('stdout', 'progress 50%\rprogress 100%\ndone\n');
    expect(lines(c)).toEqual(['progress 100%', 'done']);
  });
  it('caps the number of lines', () => {
    const c = new Console({ maxLines: 100 });
    for (let i = 0; i < 250; i++) c.append('stdout', `line ${i}\n`);
    expect(c.lines).toBe(100);
    expect(lines(c)[0]).toBe('line 150');
  });
  it('links locations and reports clicks', () => {
    const onLocation = vi.fn();
    const c = new Console({ maxLines: 100, onLocation });
    c.append('stdout', 'panic at ./main.go:7:2 here\n');
    const a = c.body.querySelector('a.con-link') as HTMLElement;
    expect(a.textContent).toBe('./main.go:7:2');
    a.click();
    expect(onLocation).toHaveBeenCalledWith(expect.objectContaining({ path: './main.go', line: 7, col: 2 }));
  });
  it('sends typed input and echoes it', () => {
    const onInput = vi.fn();
    const c = new Console({ maxLines: 100, onInput });
    c.setInputEnabled(true);
    const input = c.el.querySelector('.con-input') as HTMLInputElement;
    input.value = 'hello';
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    expect(onInput).toHaveBeenCalledWith('hello');
    expect(lines(c)).toEqual(['hello']);
  });
  it('searches lines', () => {
    const c = new Console({ maxLines: 100 });
    c.append('stdout', 'alpha\nbeta\nalphabet\n');
    expect(c.search('ALPHA')).toBe(2);
    expect(c.body.querySelectorAll('.con-match').length).toBe(2);
    expect(c.body.querySelector('.con-match-current')?.textContent).toBe('alpha');
    c.nextMatch(1);
    expect(c.body.querySelector('.con-match-current')?.textContent).toBe('alphabet');
    expect(c.search('')).toBe(0);
  });
});

describe('configuration helpers', () => {
  it('round-trips environment text', () => {
    expect(textToEnv('A=1\n# c\nB = two\nbad\nC=x=y')).toEqual({ A: '1', B: ' two', C: 'x=y' });
    expect(envToText({ A: '1', B: '2' })).toBe('A=1\nB=2');
  });
  it('splits lists and makes unique names', () => {
    expect(splitList(' a, b  c,')).toEqual(['a', 'b', 'c']);
    expect(uniqueName('App', ['app', 'App (2)'])).toBe('App (3)');
    expect(uniqueName('New', ['App'])).toBe('New');
  });
  it('formats the exit line', () => {
    expect(exitLine(0, '', 1234)).toBe('Process finished with exit code 0 (1.2 s)');
    expect(exitLine(-1, 'interrupt', 500)).toBe('Process finished with signal INTERRUPT (0.5 s)');
    expect(exitLine(1, '', 125000)).toBe('Process finished with exit code 1 (2 min 5 s)');
  });
});
