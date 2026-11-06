// Client-side fuzzy ranking for small sets (actions, settings). Mirrors the
// backend file scorer: subsequence match with word-boundary, camel-case and
// consecutive-run bonuses, and gap penalties.

export interface FuzzyMatch {
  score: number;
  /** Indices into text of the matched query characters. */
  positions: number[];
}

const SCORE_MATCH = 16;
const BONUS_START = 10;
const BONUS_SEPARATOR = 8;
const BONUS_CAMEL = 10;
const BONUS_CONSECUTIVE = 8;
const BONUS_PREFIX = 20;
const GAP_START = 3;
const GAP_EXTEND = 1;
const NEG = -1e9;

function boundaryBonus(text: string, i: number): number {
  if (i === 0) return BONUS_START;
  const prev = text[i - 1];
  const cur = text[i];
  if (prev === '/' || prev === ' ' || prev === ':') return BONUS_START;
  if (prev === '_' || prev === '-' || prev === '.') return BONUS_SEPARATOR;
  if (prev >= 'a' && prev <= 'z' && cur >= 'A' && cur <= 'Z') return BONUS_CAMEL;
  return 0;
}

/** Scores query against text, or null if query isn't a subsequence of text
 * (case-insensitive). Whitespace in query is ignored. */
export function fuzzyScore(query: string, text: string): FuzzyMatch | null {
  const q = query.replace(/\s+/g, '').toLowerCase();
  if (q === '') return { score: 0, positions: [] };
  const t = text.toLowerCase();
  const n = t.length;
  const m = q.length;
  if (m > n) return null;
  // Quick subsequence check.
  for (let i = 0, j = 0; ; j++) {
    if (i === m) break;
    if (j === n) return null;
    if (t[j] === q[i]) i++;
  }

  const S = new Float64Array(m * n).fill(NEG);
  const F = new Int32Array(m * n).fill(-1);
  for (let i = 0; i < m; i++) {
    let run = NEG;
    let runFrom = -1;
    for (let j = 0; j < n; j++) {
      if (i > 0 && j >= 2) {
        if (run !== NEG) run -= GAP_EXTEND;
        const prev = S[(i - 1) * n + j - 2];
        if (prev !== NEG && prev - GAP_START > run) {
          run = prev - GAP_START;
          runFrom = j - 2;
        }
      }
      if (t[j] !== q[i]) continue;
      const cs = SCORE_MATCH + boundaryBonus(text, j);
      const idx = i * n + j;
      if (i === 0) {
        S[idx] = cs - Math.min(j, 10) * 0.1;
        continue;
      }
      let best = NEG;
      let from = -1;
      if (j >= 1 && S[(i - 1) * n + j - 1] !== NEG) {
        best = S[(i - 1) * n + j - 1] + BONUS_CONSECUTIVE;
        from = j - 1;
      }
      if (run !== NEG && run > best) {
        best = run;
        from = runFrom;
      }
      if (best === NEG) continue;
      S[idx] = best + cs;
      F[idx] = from;
    }
  }
  let best = NEG;
  let bj = -1;
  for (let j = 0; j < n; j++) {
    const v = S[(m - 1) * n + j];
    if (v > best) {
      best = v;
      bj = j;
    }
  }
  if (bj < 0) return null;
  const positions = new Array<number>(m);
  for (let i = m - 1, j = bj; i >= 0; i--) {
    positions[i] = j;
    j = F[i * n + j];
  }
  if (t.startsWith(q)) best += BONUS_PREFIX;
  return { score: Math.round(best - n / 32), positions };
}

export interface Ranked<T> {
  item: T;
  score: number;
  positions: number[];
}

/** Filters and sorts items by fuzzy score of getText(item). An empty query
 * keeps the original order. Ties sort by shorter text, then alphabetically. */
export function rankItems<T>(query: string, items: T[], getText: (item: T) => string): Ranked<T>[] {
  const out: Ranked<T>[] = [];
  for (const item of items) {
    const m = fuzzyScore(query, getText(item));
    if (m) out.push({ item, score: m.score, positions: m.positions });
  }
  if (query.trim() === '') return out;
  return out.sort((a, b) => {
    if (a.score !== b.score) return b.score - a.score;
    const ta = getText(a.item);
    const tb = getText(b.item);
    return ta.length - tb.length || ta.localeCompare(tb);
  });
}
