// Line diff for compact inline previews of agent edits (LCS on the lines
// between the common prefix and suffix; very large changes fall back to
// "all removed, all added").

export interface DiffLine {
  op: ' ' | '-' | '+';
  text: string;
}

const MAX_CELLS = 1_000_000;

export function lineDiff(oldText: string, newText: string): DiffLine[] {
  const a = oldText === '' ? [] : oldText.split('\n');
  const b = newText === '' ? [] : newText.split('\n');
  let pre = 0;
  while (pre < a.length && pre < b.length && a[pre] === b[pre]) pre++;
  let suf = 0;
  while (suf < a.length - pre && suf < b.length - pre && a[a.length - 1 - suf] === b[b.length - 1 - suf]) suf++;
  const out: DiffLine[] = a.slice(0, pre).map((text) => ({ op: ' ', text }));
  const am = a.slice(pre, a.length - suf);
  const bm = b.slice(pre, b.length - suf);
  if (am.length * bm.length > MAX_CELLS) {
    out.push(...am.map((text) => ({ op: '-' as const, text })), ...bm.map((text) => ({ op: '+' as const, text })));
  } else {
    // lcs[i][j] = LCS length of am[i:], bm[j:].
    const n = am.length;
    const m = bm.length;
    const lcs: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        lcs[i][j] = am[i] === bm[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
      }
    }
    let i = 0;
    let j = 0;
    while (i < n || j < m) {
      if (i < n && j < m && am[i] === bm[j]) {
        out.push({ op: ' ', text: am[i] });
        i++;
        j++;
      } else if (i < n && (j >= m || lcs[i + 1][j] >= lcs[i][j + 1])) {
        out.push({ op: '-', text: am[i++] });
      } else {
        out.push({ op: '+', text: bm[j++] });
      }
    }
  }
  out.push(...a.slice(a.length - suf).map((text) => ({ op: ' ' as const, text })));
  return out;
}

export type DiffRow = DiffLine | { op: '…'; text: string };

/** Keeps changed lines with `context` lines around them; gaps become "…". */
export function compact(lines: readonly DiffLine[], context = 2): DiffRow[] {
  const keep = new Array<boolean>(lines.length).fill(false);
  lines.forEach((l, i) => {
    if (l.op === ' ') return;
    for (let k = Math.max(0, i - context); k <= Math.min(lines.length - 1, i + context); k++) keep[k] = true;
  });
  const out: DiffRow[] = [];
  let gap = 0;
  lines.forEach((l, i) => {
    if (keep[i]) {
      if (gap) out.push({ op: '…', text: `${gap} unchanged line${gap === 1 ? '' : 's'}` });
      gap = 0;
      out.push(l);
    } else gap++;
  });
  if (gap && out.length) out.push({ op: '…', text: `${gap} unchanged line${gap === 1 ? '' : 's'}` });
  return out;
}

export function diffStats(lines: readonly DiffLine[]): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const l of lines) {
    if (l.op === '+') added++;
    else if (l.op === '-') removed++;
  }
  return { added, removed };
}
