import { describe, expect, it } from 'vitest';
import { descriptor, filterSchema, getByKey, parseIntInput, schema, sections, validate } from '../src/views/settings/schema';

describe('settings schema', () => {
  it('covers every M1 key once, in a known section', () => {
    const keys = schema.map((d) => d.key);
    expect(new Set(keys).size).toBe(keys.length);
    for (const k of [
      'ui.theme', 'ui.scheme', 'ui.keymap', 'editor.fontFamily', 'editor.fontSize', 'editor.tabSize', 'editor.softWrap',
      'editor.maxHighlightSizeKB', 'editor.maxOpenSizeKB', 'go.goroot', 'go.env', 'go.tags', 'go.goos', 'go.goarch',
      'go.cgo', 'go.vendor', 'project.excludes', 'project.contentRoots', 'save.format', 'startup.reopenLastProject',
    ]) {
      expect(keys).toContain(k);
    }
    for (const d of schema) expect(sections).toContain(d.section);
    expect(descriptor('ui.theme')!.layers).toEqual(['user']);
  });

  it('getByKey reads dotted paths', () => {
    const s = { editor: { fontSize: 13 }, go: { env: { GOPROXY: 'x' } } };
    expect(getByKey(s, 'editor.fontSize')).toBe(13);
    expect(getByKey(s, 'go.env')).toEqual({ GOPROXY: 'x' });
    expect(getByKey(s, 'missing.key')).toBeUndefined();
    expect(getByKey(null, 'a')).toBeUndefined();
  });

  it('filterSchema matches title, key and description case-insensitively', () => {
    expect(filterSchema('').length).toBe(schema.length);
    expect(filterSchema('FONT SIZE').map((d) => d.key)).toEqual(['editor.fontSize', 'terminal.fontSize']);
    expect(filterSchema('goproxy').map((d) => d.key)).toEqual(['go.env']);
    expect(filterSchema('save.format').map((d) => d.key)).toEqual(['save.format']);
    expect(filterSchema('zzzz')).toEqual([]);
  });

  it('validate checks types, ranges and options', () => {
    const fs = descriptor('editor.fontSize')!;
    expect(validate(fs, 13)).toBeNull();
    expect(validate(fs, 7)).toMatch(/at least 8/);
    expect(validate(fs, 41)).toMatch(/at most 40/);
    expect(validate(fs, 1.5)).toMatch(/whole number/);
    expect(validate(descriptor('editor.softWrap')!, 'yes')).toMatch(/true or false/);
    expect(validate(descriptor('save.format')!, 'gofmt')).toBeNull();
    expect(validate(descriptor('save.format')!, 'black')).not.toBeNull();
    expect(validate(descriptor('go.cgo')!, '')).toBeNull();
    expect(validate(descriptor('go.tags')!, ['a', 'b'])).toBeNull();
    expect(validate(descriptor('go.tags')!, ['a', ' '])).toMatch(/empty/);
    expect(validate(descriptor('go.env')!, { GOPROXY: 'direct' })).toBeNull();
    expect(validate(descriptor('go.env')!, { 'BAD-NAME': 'x' })).toMatch(/Invalid variable/);
    expect(validate(descriptor('go.env')!, ['x'])).not.toBeNull();
    expect(validate(descriptor('go.goroot')!, '/usr/local/go')).toBeNull();
  });

  it('parseIntInput', () => {
    expect(parseIntInput(' 14 ')).toBe(14);
    expect(Number.isNaN(parseIntInput('1.5'))).toBe(true);
    expect(Number.isNaN(parseIntInput('abc'))).toBe(true);
  });
});
