// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { KEYWORD_DOCS, keywordDoc } from '../src/lang/keywords';
import { signatureDom, splitSignature } from '../src/lang/signature';
import type { SignatureInfo } from '../src/lib/api';

const GO_KEYWORDS = 'break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var'.split(' ');

describe('keyword docs', () => {
  it('covers every Go keyword', () => {
    expect(Object.keys(KEYWORD_DOCS).sort()).toEqual([...GO_KEYWORDS].sort());
  });
  it('formats with a heading code block', () => {
    expect(keywordDoc('defer')).toMatch(/^```go\ndefer\n```/);
    expect(keywordDoc('fmt')).toBeNull();
  });
});

describe('signature help', () => {
  const sig: SignatureInfo = {
    label: 'Fprintf(w io.Writer, format string, a ...any) (n int, err error)',
    parameters: [
      { label: '', start: 8, end: 19 },
      { label: 'format string', start: -1, end: -1 },
      { label: 'a ...any', start: -1, end: -1 },
    ],
  };
  it('splits around offset and text labels', () => {
    expect(splitSignature(sig, 0)[1]).toBe('w io.Writer');
    expect(splitSignature(sig, 1)).toEqual(['Fprintf(w io.Writer, ', 'format string', ', a ...any) (n int, err error)']);
    expect(splitSignature(sig, 9)).toEqual([sig.label, '', '']);
  });
  it('renders the active parameter', () => {
    const el = signatureDom({ signatures: [sig], activeSignature: 0, activeParameter: 2 });
    expect(el.querySelector('.active-param')?.textContent).toBe('a ...any');
  });
});
