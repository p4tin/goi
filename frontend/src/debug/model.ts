// Pure debugger helpers: goroutine parsing/grouping/filtering (R8), value
// formatting options (R9.1), byte-slice detection (R9.2) and inline values
// (R9.6).

import type { DapThread } from '../lib/api';

export interface Goroutine {
  id: number;
  /** Delve marks the goroutine that triggered the stop with "*". */
  current: boolean;
  /** Current function (or location) as reported. */
  func: string;
  /** OS thread id when the goroutine is running on one. */
  thread: string;
  /** Anything else Delve reported (labels, state). */
  extra: string;
  name: string;
}

const THREAD_RE = /^(\*\s*)?\[Go (\d+)\]\s*(.*)$/;

/** Parses a Delve thread name: "* [Go 36] main.work (Thread 8509839)". */
export function parseGoroutine(t: DapThread): Goroutine {
  const m = THREAD_RE.exec(t.name.trim());
  if (!m) return { id: t.id, current: false, func: t.name, thread: '', extra: '', name: t.name };
  let rest = m[3];
  let thread = '';
  const tm = /\s*\(Thread (\d+)\)\s*$/.exec(rest);
  if (tm) {
    thread = tm[1];
    rest = rest.slice(0, tm.index);
  }
  const sp = rest.indexOf(' ');
  const func = sp < 0 ? rest : rest.slice(0, sp);
  const extra = sp < 0 ? '' : rest.slice(sp + 1).trim();
  return { id: Number(m[2]) || t.id, current: !!m[1], func, thread, extra, name: t.name };
}

export interface GoroutineGroup {
  func: string;
  items: Goroutine[];
}

/** Groups goroutines by function, largest groups first; the group holding
 * the current goroutine sorts first. */
export function groupByFunction(list: Goroutine[]): GoroutineGroup[] {
  const map = new Map<string, Goroutine[]>();
  for (const g of list) {
    if (!map.has(g.func)) map.set(g.func, []);
    map.get(g.func)!.push(g);
  }
  return [...map.entries()]
    .map(([func, items]) => ({ func, items: items.sort((a, b) => a.id - b.id) }))
    .sort((a, b) => {
      const ca = a.items.some((g) => g.current) ? 0 : 1;
      const cb = b.items.some((g) => g.current) ? 0 : 1;
      return ca - cb || b.items.length - a.items.length || a.func.localeCompare(b.func);
    });
}

/** Keeps goroutines matching text (id, function, extra, thread) and not
 * hidden. The current goroutine is never filtered out by hiding. */
export function filterGoroutines(list: Goroutine[], text: string, hidden: ReadonlySet<number>): Goroutine[] {
  const q = text.trim().toLowerCase();
  return list.filter((g) => {
    if (hidden.has(g.id) && !g.current) return false;
    if (!q) return true;
    return String(g.id) === q.replace(/^#/, '') || g.func.toLowerCase().includes(q) || g.extra.toLowerCase().includes(q) || g.thread.includes(q);
  });
}

/** Label of a goroutine row. */
export function goroutineLabel(g: Goroutine, showThreads: boolean): string {
  let s = `Goroutine ${g.id} ${g.func}`;
  if (g.extra) s += ` ${g.extra}`;
  if (showThreads && g.thread) s += ` (thread ${g.thread})`;
  return s;
}

// ---- Values

/** Removes memory addresses (0x… with ≥ 6 hex digits) from a value. */
export function stripAddresses(value: string): string {
  return value
    .replace(/\s*\(0x[0-9a-fA-F]{6,}\)/g, '')
    .replace(/\b0x[0-9a-fA-F]{6,}\b/g, '…');
}

export interface ValueOptions {
  showAddresses: boolean;
}

export function formatValue(value: string, opts: ValueOptions): string {
  return opts.showAddresses ? value : stripAddresses(value);
}

export function isUnreadable(value: string): boolean {
  return value.startsWith('(unreadable');
}

/** []byte, []uint8 or [N]byte/[N]uint8 (incl. named via the type text). */
export function isByteSlice(type: string): boolean {
  return /^\[\d*\](?:uint8|byte)$/.test(type.trim());
}

/** Truncates a value for one-line display. */
export function truncate(s: string, max = 60): string {
  const one = s.replace(/\s+/g, ' ');
  return one.length > max ? `${one.slice(0, max - 1)}…` : one;
}

// ---- Inline values

/** First line (1-based) of the top-level func containing line. */
export function functionStart(lines: string[], line: number): number {
  for (let i = Math.min(line, lines.length) - 1; i >= 0; i--) {
    if (/^func\b/.test(lines[i])) return i + 1;
  }
  return 1;
}

/** Strips string/rune literals and // comments so identifiers inside them
 * don't count. */
function codeOnly(line: string): string {
  return line
    .replace(/`[^`]*`/g, '``')
    .replace(/"(?:[^"\\]|\\.)*"/g, '""')
    .replace(/'(?:[^'\\]|\\.)*'/g, "''")
    .replace(/\/\/.*$/, '');
}

const IDENT_RE = /[\p{L}_][\p{L}\p{N}_]*/gu;

/** For each line from the function start to stopLine (1-based, inclusive),
 * the "name = value" pairs of variables mentioned on it, in first-mention
 * order. Fields (after ".") are ignored. */
export function inlineValues(lines: string[], stopLine: number, values: ReadonlyMap<string, string>, max = 60): Map<number, string[]> {
  const out = new Map<number, string[]>();
  if (!values.size) return out;
  const start = functionStart(lines, stopLine);
  for (let ln = start; ln <= Math.min(stopLine, lines.length); ln++) {
    const code = codeOnly(lines[ln - 1]);
    const seen = new Set<string>();
    const parts: string[] = [];
    IDENT_RE.lastIndex = 0;
    for (let m = IDENT_RE.exec(code); m; m = IDENT_RE.exec(code)) {
      const name = m[0];
      if (m.index > 0 && code[m.index - 1] === '.') continue;
      if (seen.has(name) || !values.has(name)) continue;
      seen.add(name);
      parts.push(`${name} = ${truncate(values.get(name)!, max)}`);
    }
    if (parts.length) out.set(ln, parts);
  }
  return out;
}

/** Whether path is under root (frames outside the project are muted). */
export function inProject(path: string | undefined, root: string): boolean {
  if (!path || !root) return false;
  const p = path.replace(/\\/g, '/');
  const r = root.replace(/\\/g, '/').replace(/\/$/, '');
  return p.startsWith(r + '/');
}
