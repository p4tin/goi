// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { decodeEntity, renderBlocks, slugify } from '../src/markdown/render';

const html = (text: string): string => {
  const root = document.createElement('div');
  for (const b of renderBlocks(text)) root.append(b.el);
  return root.innerHTML;
};
const one = (text: string): HTMLElement => renderBlocks(text)[0].el;

describe('renderBlocks', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders headings with GitHub-style ids (AC2)', () => {
    const h = one('# Hello');
    expect(h.outerHTML).toBe('<h1 id="hello" data-line="0">Hello</h1>');
    const bs = renderBlocks('## A *b* `c`\n\nSetext\n---\n\n## A b c\n');
    expect(bs.map((b) => [b.el.tagName, b.el.id])).toEqual([
      ['H2', 'a-b-c'],
      ['H2', 'setext'],
      ['H2', 'a-b-c-1'],
    ]);
    expect(bs[0].el.innerHTML).toBe('A <em>b</em> <code>c</code>');
  });

  it('renders inline formatting', () => {
    expect(one('**b** _i_ ~~s~~ a\\*b &amp; &#65; &copy; x  \ny').innerHTML).toBe(
      '<strong>b</strong> <em>i</em> <del>s</del> a*b &amp; A © x<br>y',
    );
  });

  it('renders links with data-href only, and images without src', () => {
    const p = one('[doc](docs/a.md#x "T") <https://a.io> https://b.io ![logo *x*](img/l.png "L")');
    const links = [...p.querySelectorAll('a')];
    expect(links.map((a) => [a.getAttribute('href'), a.dataset.href, a.textContent])).toEqual([
      ['#', 'docs/a.md#x', 'doc'],
      ['#', 'https://a.io', 'https://a.io'],
      ['#', 'https://b.io', 'https://b.io'],
    ]);
    expect(links[0].title).toBe('T');
    const img = p.querySelector('img')!;
    expect(img.getAttribute('src')).toBeNull();
    expect([img.alt, img.dataset.src, img.title]).toEqual(['logo x', 'img/l.png', 'L']);
  });

  it('resolves reference links', () => {
    const bs = renderBlocks('[a][r] and [r]\n\n[r]: https://r.io "Ref"\n');
    expect(bs).toHaveLength(1);
    expect([...bs[0].el.querySelectorAll('a')].map((a) => a.dataset.href)).toEqual(['https://r.io', 'https://r.io']);
  });

  it('renders lists, task lists and nesting', () => {
    const ul = one('- one\n- [ ] todo\n- [x] done\n  1. nested');
    expect(ul.tagName).toBe('UL');
    const boxes = [...ul.querySelectorAll('input')];
    expect(boxes.map((b) => [b.type, b.disabled, b.checked])).toEqual([
      ['checkbox', true, false],
      ['checkbox', true, true],
    ]);
    expect(ul.querySelector('ol li')?.textContent).toBe('nested');
    expect(ul.querySelector('p')).toBeNull(); // tight
    expect(one('3. a\n4. b').getAttribute('start')).toBe('3');
    expect(one('- a\n\n- b').querySelectorAll('li > p')).toHaveLength(2); // loose
  });

  it('renders quotes, rules, tables and code', () => {
    expect(html('> quoted\n> more')).toBe('<blockquote data-line="0"><p>quoted\nmore</p></blockquote>');
    expect(html('***')).toBe('<hr data-line="0">');
    const t = one('| a | b | c |\n|:--|--:|:-:|\n| 1 | **2** |');
    expect(t.querySelectorAll('th')).toHaveLength(3);
    expect([...t.querySelectorAll('td')].map((c) => [c.innerHTML, c.style.textAlign])).toEqual([
      ['1', 'left'],
      ['<strong>2</strong>', 'right'],
      ['', 'center'],
    ]);
    const code = one('```go\nfunc x() {}\n```');
    expect(code.outerHTML).toBe('<pre class="md-code" data-line="0"><code data-lang="go">func x() {}</code></pre>');
    expect(one('    indented\n      more').textContent).toBe('indented\n  more');
  });

  it('uses the highlighter for tagged fences', () => {
    const highlight = vi.fn((lang: string, code: string) => {
      const s = document.createElement('span');
      s.className = 'tok';
      s.textContent = `${lang}:${code}`;
      return [s];
    });
    expect(renderBlocks('```Go\nx\n```', { highlight })[0].el.innerHTML).toBe('<code data-lang="go"><span class="tok">go:x</span></code>');
    renderBlocks('```\nplain\n```', { highlight });
    expect(highlight).toHaveBeenCalledTimes(1);
  });

  it('turns mermaid fences into placeholders', () => {
    const m = one('```mermaid\ngraph TD; A-->B\n```');
    expect(m.className).toBe('md-mermaid');
    expect(m.dataset.source).toBe('graph TD; A-->B');
    expect(m.querySelector('pre code')?.textContent).toBe('graph TD; A-->B');
  });

  it('shows raw HTML as text and never runs scripts (AC4)', () => {
    const setter = vi.spyOn(Element.prototype, 'innerHTML', 'set');
    const bs = renderBlocks('<script>alert(1)</script>\n\nhi <img src=x onerror=alert(1)> <!-- c -->\n\n<div>\nraw\n</div>');
    expect(setter).not.toHaveBeenCalled();
    const root = document.createElement('div');
    for (const b of bs) root.append(b.el);
    expect(root.querySelector('script, img, div div')).toBeNull();
    expect(bs[0].el.tagName).toBe('PRE');
    expect(bs[0].el.textContent).toBe('<script>alert(1)</script>');
    expect(bs[1].el.textContent).toBe('hi <img src=x onerror=alert(1)>');
    expect(bs[2].el.textContent).toBe('<div>\nraw\n</div>');
  });

  it('records source lines and stable keys', () => {
    const a = renderBlocks('# T\n\npara one\n\n- x\n');
    expect(a.map((b) => b.line)).toEqual([0, 2, 4]);
    const b = renderBlocks('# T\n\npara one!\n\n- x\n');
    expect(b.map((x) => x.key === a[b.indexOf(x)].key)).toEqual([true, false, true]);
  });
});

describe('helpers', () => {
  it('slugify counts duplicates', () => {
    const used = new Map<string, number>();
    expect(['Hello World!', 'Hello World', 'Ünïcode & co', 'a_b-c'].map((t) => slugify(t, used))).toEqual(['hello-world', 'hello-world-1', 'ünïcode--co', 'a_b-c']);
  });

  it('decodes entities', () => {
    expect(['&amp;', '&#x1F600;', '&#0;', '&bogus;'].map(decodeEntity)).toEqual(['&', '😀', '�', '&bogus;']);
  });
});
