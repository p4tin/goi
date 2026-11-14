// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest';

vi.mock('../src/editor/install', () => ({ editors: undefined }));

import { langIdForFence } from '../src/editor/languages';
import { createHighlighter } from '../src/markdown/highlight';

describe('fence languages', () => {
  it('maps tags and aliases', () => {
    expect(['go', 'Golang', 'sh', 'console', 'yml', 'ts', 'jsonc', 'proto', 'rs', '', 'text'].map(langIdForFence)).toEqual([
      'go',
      'go',
      'shell',
      'shell',
      'yaml',
      'typescript',
      'json',
      'protobuf',
      null,
      null,
      null,
    ]);
  });
});

describe('createHighlighter', () => {
  it('highlights once the language has loaded', async () => {
    let calls = 0;
    let ready!: () => void;
    const done = new Promise<void>((r) => (ready = r));
    const hl = createHighlighter('goi-dark', () => {
      calls++;
      ready();
    });
    expect(hl('json', '{"a": 1}')).toBeNull();
    await done;
    expect(calls).toBe(1);
    const nodes = hl('json', '{"a": 1}\n[true]')!;
    expect(nodes.map((n) => n.textContent).join('')).toBe('{"a": 1}\n[true]');
    expect(nodes.some((n) => n instanceof HTMLElement && n.className)).toBe(true);
  });

  it('returns null for unknown languages', () => {
    expect(createHighlighter('goi-light', () => {})('nope', 'x')).toBeNull();
  });
});
