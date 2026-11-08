// Benchmark results (R3.5): parsed from `go test -bench` output lines and
// compared with the previous run of the same benchmark.

export interface BenchResult {
  /** Benchmark name without the -GOMAXPROCS suffix. */
  name: string;
  procs: number;
  iterations: number;
  /** Unit → value, e.g. "ns/op": 12.5, "B/op": 0, "allocs/op": 0, "MB/s": 80. */
  metrics: Record<string, number>;
}

export type BenchHistory = Record<string, BenchResult>;

const LINE_RE = /^(Benchmark\S*?)(?:-(\d+))?\s+(\d+)\s+(.+)$/;

/** Parses a benchmark result line, or returns null. */
export function parseBenchLine(line: string): BenchResult | null {
  const m = LINE_RE.exec(line.trim());
  if (!m) return null;
  const fields = m[4].trim().split(/\s+/);
  if (fields.length < 2 || fields.length % 2 !== 0) return null;
  const metrics: Record<string, number> = {};
  for (let i = 0; i < fields.length; i += 2) {
    const v = Number(fields[i]);
    if (!Number.isFinite(v)) return null;
    metrics[fields[i + 1]] = v;
  }
  if (!('ns/op' in metrics) && Object.keys(metrics).length === 0) return null;
  return { name: m[1], procs: m[2] ? Number(m[2]) : 1, iterations: Number(m[3]), metrics };
}

/** Relative change in percent from prev to cur (null when not comparable). */
export function delta(cur: number | undefined, prev: number | undefined): number | null {
  if (cur === undefined || prev === undefined || prev === 0) return null;
  return ((cur - prev) / prev) * 100;
}

/** "+12.3%" / "−4.0%" / "±0%". */
export function deltaLabel(d: number | null): string {
  if (d === null) return '';
  if (Math.abs(d) < 0.05) return '±0%';
  return `${d > 0 ? '+' : '−'}${Math.abs(d).toFixed(1)}%`;
}

/** Formats a metric value compactly. */
export function metricLabel(v: number | undefined): string {
  if (v === undefined) return '';
  if (Number.isInteger(v)) return v.toLocaleString('en-US');
  return v < 10 ? v.toFixed(3) : v < 1000 ? v.toFixed(1) : Math.round(v).toLocaleString('en-US');
}

export function parseHistory(json: string): BenchHistory {
  if (!json) return {};
  try {
    const v = JSON.parse(json) as unknown;
    return v && typeof v === 'object' ? (v as BenchHistory) : {};
  } catch {
    return {};
  }
}
