// Source locations in program, test and compiler output: "file.go:12",
// "./pkg/x.go:3:7", "/abs/path/main.go:42 +0x1d" (panic traces),
// "C:\src\x.go:9". Relative paths resolve against a base directory, then
// against the project's file index.

export interface LocationMatch {
  start: number;
  end: number;
  path: string;
  line: number;
  col: number;
}

const EXT = 'go|s|mod|sum|work|tmpl|gohtml|txt|json|ya?ml|toml|proto|c|h|sql|md';
const LOC_RE = new RegExp(
  `((?:[A-Za-z]:[\\\\/]|[\\\\/]|\\.{1,2}[\\\\/])?(?:[\\w.@+~-]+[\\\\/])*[\\w.@+~-]+\\.(?:${EXT})):(\\d+)(?::(\\d+))?`,
  'g',
);

/** Finds file locations in a line of text. */
export function findLocations(text: string): LocationMatch[] {
  const out: LocationMatch[] = [];
  LOC_RE.lastIndex = 0;
  for (let m = LOC_RE.exec(text); m; m = LOC_RE.exec(text)) {
    const start = m.index;
    // Skip URLs (http://host/x.go:80): the surrounding word contains "://".
    const wordStart = Math.max(text.lastIndexOf(' ', start), text.lastIndexOf('\t', start)) + 1;
    if (text.slice(wordStart, start + m[0].length).includes('://')) continue;
    const line = Number(m[2]);
    if (!line) continue;
    out.push({ start, end: start + m[0].length, path: m[1], line, col: m[3] ? Number(m[3]) : 1 });
  }
  return out;
}

export function isAbsolutePath(p: string): boolean {
  return p.startsWith('/') || p.startsWith('\\') || /^[A-Za-z]:[\\/]/.test(p);
}

/** Joins a relative path onto base, normalizing "." and ".." segments. */
export function joinPath(base: string, rel: string): string {
  if (isAbsolutePath(rel) || !base) return rel;
  const sep = base.includes('\\') && !base.includes('/') ? '\\' : '/';
  const parts = base.split(/[\\/]/);
  for (const seg of rel.split(/[\\/]/)) {
    if (seg === '' || seg === '.') continue;
    if (seg === '..') {
      if (parts.length > 1) parts.pop();
    } else parts.push(seg);
  }
  return parts.join(sep);
}

/** Picks the candidate path that best matches a relative location:
 * it must end with the relative path; candidates under base win. */
export function bestCandidate(rel: string, candidates: string[], base: string): string | null {
  const norm = (s: string): string => s.replace(/\\/g, '/');
  const want = '/' + norm(rel).replace(/^\.\//, '');
  const matching = candidates.filter((c) => norm(c).endsWith(want));
  if (!matching.length) return null;
  const b = norm(base);
  matching.sort((x, y) => {
    const ix = b && norm(x).startsWith(b + '/') ? 0 : 1;
    const iy = b && norm(y).startsWith(b + '/') ? 0 : 1;
    return ix - iy || x.length - y.length;
  });
  return matching[0];
}
