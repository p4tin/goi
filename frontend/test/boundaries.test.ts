// @vitest-environment node
// Frontend pack boundaries (R8.2): core code never imports a pack, packs
// don't import each other, and only lib/api.ts and packs/*/api.ts import the
// generated bindings.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { describe, expect, it } from 'vitest';

const src = join(process.cwd(), 'src');

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    return statSync(p).isDirectory() ? files(p) : /\.ts$/.test(n) ? [p] : [];
  });
}

const imports = (p: string): string[] =>
  [...readFileSync(p, 'utf8').matchAll(/(?:from|import\()\s*'([^']+)'/g)].map((m) => m[1]);

describe('frontend boundaries', () => {
  const all = files(src);
  const packOf = (p: string): string | null => {
    const r = relative(src, p).split(sep);
    return r[0] === 'packs' ? r[1] : null;
  };

  it('keeps core free of pack imports', () => {
    const bad = all.filter((p) => !packOf(p)).flatMap((p) => imports(p).filter((i) => /(^|\/)packs\//.test(i)).map((i) => `${relative(src, p)} → ${i}`));
    expect(bad).toEqual([]);
  });

  it('keeps packs apart', () => {
    const bad = all
      .filter((p) => packOf(p))
      .flatMap((p) => imports(p).filter((i) => { const m = /packs\/(\w+)/.exec(i); return !i.includes('/bindings/') && m && m[1] !== packOf(p); }).map((i) => `${relative(src, p)} → ${i}`));
    expect(bad).toEqual([]);
  });

  it("lets a pack's api.ts bind only its own backend pack's services", () => {
    const backend: Record<string, string> = { go: 'golang', python: 'python' };
    const bad = all
      .filter((p) => packOf(p))
      .flatMap((p) =>
        imports(p)
          .filter((i) => { const m = /bindings\/github\.com\/p4tin\/goi\/packs\/(\w+)/.exec(i); return m && m[1] !== backend[packOf(p)!]; })
          .map((i) => `${relative(src, p)} → ${i}`),
      );
    expect(bad).toEqual([]);
  });

  it('imports bindings only from api modules', () => {
    const bad = all
      .filter((p) => !/(^|[\\/])(lib|packs[\\/]\w+)[\\/]api\.ts$/.test(relative(src, p)))
      .flatMap((p) => imports(p).filter((i) => i.includes('/bindings/')).map((i) => `${relative(src, p)} → ${i}`));
    expect(bad).toEqual([]);
  });
});
