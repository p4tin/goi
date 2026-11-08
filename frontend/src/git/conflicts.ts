// Conflict markers (mirror of internal/git conflicts.go): blocks of
// <<<<<<< ours / ||||||| base / ======= / >>>>>>> theirs, and resolving one.

export interface ConflictBlock {
  /** 0-based line indices of the marker lines; base is -1 without diff3. */
  start: number;
  base: number;
  mid: number;
  end: number;
  oursLabel: string;
  theirsLabel: string;
}

export type Side = 'ours' | 'theirs' | 'both';

export function conflictBlocks(text: string): ConflictBlock[] {
  const lines = text.split('\n');
  const out: ConflictBlock[] = [];
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].startsWith('<<<<<<<')) continue;
    const b: ConflictBlock = { start: i, base: -1, mid: -1, end: -1, oursLabel: lines[i].slice(7).trim(), theirsLabel: '' };
    for (let j = i + 1; j < lines.length; j++) {
      const l = lines[j];
      if (l.startsWith('<<<<<<<')) break;
      if (l.startsWith('|||||||') && b.mid < 0 && b.base < 0) b.base = j;
      else if (l.startsWith('=======') && b.mid < 0) b.mid = j;
      else if (l.startsWith('>>>>>>>') && b.mid >= 0) {
        b.end = j;
        b.theirsLabel = l.slice(7).trim();
        break;
      }
    }
    if (b.end >= 0) {
      out.push(b);
      i = b.end;
    }
  }
  return out;
}

/** The text of each side of a block. */
export function blockSides(text: string, b: ConflictBlock): { ours: string[]; theirs: string[]; base: string[] } {
  const lines = text.split('\n');
  const oursEnd = b.base >= 0 ? b.base : b.mid;
  return {
    ours: lines.slice(b.start + 1, oursEnd),
    base: b.base >= 0 ? lines.slice(b.base + 1, b.mid) : [],
    theirs: lines.slice(b.mid + 1, b.end),
  };
}

/** Replaces block b with one side (or both: ours then theirs). */
export function resolveBlock(text: string, b: ConflictBlock, side: Side): string {
  const lines = text.split('\n');
  const { ours, theirs } = blockSides(text, b);
  const repl = side === 'ours' ? ours : side === 'theirs' ? theirs : [...ours, ...theirs];
  return [...lines.slice(0, b.start), ...repl, ...lines.slice(b.end + 1)].join('\n');
}

/** Resolves every block with the same side. */
export function resolveAll(text: string, side: Side): string {
  let t = text;
  for (let blocks = conflictBlocks(t); blocks.length; blocks = conflictBlocks(t)) t = resolveBlock(t, blocks[0], side);
  return t;
}
