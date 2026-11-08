// ANSI escape handling for console output: SGR styles (16, 256 and true
// colors, bold, dim, italic, underline, inverse, strike) become styled
// segments; other control sequences (cursor movement, erase, OSC titles and
// hyperlinks) are dropped. The parser keeps its style and any escape
// sequence split across chunks between feed() calls.

export interface Style {
  /** Palette index 0–15 ("p3") or a CSS rgb() color. */
  fg?: string;
  bg?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
  inverse?: boolean;
  strike?: boolean;
}

export interface Segment {
  text: string;
  style: Style;
}

const ESC = '\x1b';

/** The 256-color palette entry n (16–255) as rgb(); 0–15 map to "pN". */
export function color256(n: number): string {
  if (n < 16) return `p${n}`;
  if (n < 232) {
    const i = n - 16;
    const v = (x: number): number => (x === 0 ? 0 : 55 + x * 40);
    return `rgb(${v(Math.floor(i / 36))},${v(Math.floor(i / 6) % 6)},${v(i % 6)})`;
  }
  const g = 8 + (n - 232) * 10;
  return `rgb(${g},${g},${g})`;
}

export function sameStyle(a: Style, b: Style): boolean {
  return (
    a.fg === b.fg &&
    a.bg === b.bg &&
    !!a.bold === !!b.bold &&
    !!a.dim === !!b.dim &&
    !!a.italic === !!b.italic &&
    !!a.underline === !!b.underline &&
    !!a.inverse === !!b.inverse &&
    !!a.strike === !!b.strike
  );
}

/** Applies SGR parameters to style, returning the new style. */
export function applySgr(style: Style, params: number[]): Style {
  const s: Style = { ...style };
  if (params.length === 0) params = [0];
  for (let i = 0; i < params.length; i++) {
    const p = params[i];
    if (p === 0) {
      for (const k of Object.keys(s) as (keyof Style)[]) delete s[k];
    } else if (p === 1) s.bold = true;
    else if (p === 2) s.dim = true;
    else if (p === 3) s.italic = true;
    else if (p === 4) s.underline = true;
    else if (p === 7) s.inverse = true;
    else if (p === 9) s.strike = true;
    else if (p === 21 || p === 22) {
      delete s.bold;
      delete s.dim;
    } else if (p === 23) delete s.italic;
    else if (p === 24) delete s.underline;
    else if (p === 27) delete s.inverse;
    else if (p === 29) delete s.strike;
    else if (p >= 30 && p <= 37) s.fg = `p${p - 30}`;
    else if (p >= 90 && p <= 97) s.fg = `p${p - 90 + 8}`;
    else if (p >= 40 && p <= 47) s.bg = `p${p - 40}`;
    else if (p >= 100 && p <= 107) s.bg = `p${p - 100 + 8}`;
    else if (p === 39) delete s.fg;
    else if (p === 49) delete s.bg;
    else if (p === 38 || p === 48) {
      const key = p === 38 ? 'fg' : 'bg';
      if (params[i + 1] === 5 && i + 2 < params.length) {
        s[key] = color256(Math.max(0, Math.min(255, params[i + 2])));
        i += 2;
      } else if (params[i + 1] === 2 && i + 4 < params.length) {
        const [r, g, b] = params.slice(i + 2, i + 5).map((x) => Math.max(0, Math.min(255, x)));
        s[key] = `rgb(${r},${g},${b})`;
        i += 4;
      }
    }
  }
  return s;
}

export class AnsiParser {
  private style: Style = {};
  private pending = '';

  /** Parses a chunk of output into styled segments. */
  feed(chunk: string): Segment[] {
    const text = this.pending + chunk;
    this.pending = '';
    const out: Segment[] = [];
    let buf = '';
    const flush = (): void => {
      if (!buf) return;
      const last = out[out.length - 1];
      if (last && sameStyle(last.style, this.style)) last.text += buf;
      else out.push({ text: buf, style: { ...this.style } });
      buf = '';
    };
    let i = 0;
    while (i < text.length) {
      const c = text[i];
      if (c !== ESC) {
        // Drop other C0 controls except \n, \r and \t.
        const code = c.charCodeAt(0);
        if (code >= 0x20 || c === '\n' || c === '\r' || c === '\t') buf += c;
        else if (c === '\b') buf = buf.slice(0, -1);
        i++;
        continue;
      }
      // Escape sequence.
      if (i + 1 >= text.length) {
        this.pending = text.slice(i);
        break;
      }
      const kind = text[i + 1];
      if (kind === '[') {
        let j = i + 2;
        while (j < text.length && !(text.charCodeAt(j) >= 0x40 && text.charCodeAt(j) <= 0x7e)) j++;
        if (j >= text.length) {
          this.pending = text.slice(i);
          break;
        }
        if (text[j] === 'm') {
          const raw = text.slice(i + 2, j);
          const params = raw === '' ? [] : raw.split(/[;:]/).map((x) => (x === '' ? 0 : Number(x)));
          if (params.every((n) => Number.isFinite(n))) {
            flush();
            this.style = applySgr(this.style, params);
          }
        }
        i = j + 1;
      } else if (kind === ']') {
        // OSC: terminated by BEL or ESC \.
        let j = i + 2;
        let end = -1;
        while (j < text.length) {
          if (text[j] === '\x07') {
            end = j + 1;
            break;
          }
          if (text[j] === ESC && text[j + 1] === '\\') {
            end = j + 2;
            break;
          }
          j++;
        }
        if (end < 0) {
          this.pending = text.slice(i);
          break;
        }
        i = end;
      } else {
        // Two-character escape (charset selection etc.): drop it.
        i += kind === '(' || kind === ')' ? 3 : 2;
      }
    }
    flush();
    return out;
  }

  reset(): void {
    this.style = {};
    this.pending = '';
  }
}

/** Strips all escape sequences. */
export function stripAnsi(s: string): string {
  return new AnsiParser()
    .feed(s)
    .map((x) => x.text)
    .join('');
}
