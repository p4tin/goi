// Pure project-tree model: path helpers, visible-row flattening and
// file-system event reconciliation. No DOM or backend access.

export interface Entry {
  name: string;
  path: string;
  isDir: boolean;
  size?: number;
}

export interface TreeRoot {
  path: string;
  label: string;
  /** True for additional content roots (not the project root). */
  extra: boolean;
}

export interface TreeState {
  /** Loaded children per directory; 'loading' while a fetch is in flight. */
  children: Map<string, Entry[] | 'loading'>;
  expanded: Set<string>;
  selected: string | null;
}

export interface Row {
  path: string;
  name: string;
  depth: number;
  isDir: boolean;
  expanded: boolean;
  /** Placeholder row shown while a directory's children load. */
  loading: boolean;
  /** Set on top-level rows. */
  root?: TreeRoot;
}

export interface FsEvent {
  path: string;
  op: 'create' | 'remove' | 'rename' | 'write';
  isDir: boolean;
}

export function emptyState(): TreeState {
  return { children: new Map(), expanded: new Set(), selected: null };
}

export function cloneState(s: TreeState): TreeState {
  return { children: new Map(s.children), expanded: new Set(s.expanded), selected: s.selected };
}

/** The separator used by path p ('\\' for Windows-style paths). */
export function sepOf(p: string): string {
  return p.includes('\\') && !p.includes('/') ? '\\' : '/';
}

function lastSep(p: string): number {
  return Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'));
}

function trimTrailing(p: string): string {
  while (p.length > 1 && (p.endsWith('/') || p.endsWith('\\')) && !/^[A-Za-z]:[\\/]$/.test(p)) p = p.slice(0, -1);
  return p;
}

export function baseName(p: string): string {
  p = trimTrailing(p);
  return p.slice(lastSep(p) + 1);
}

export function parentOf(p: string): string {
  p = trimTrailing(p);
  const i = lastSep(p);
  if (i < 0) return p;
  if (i === 0) return p[0];
  const parent = p.slice(0, i);
  return /^[A-Za-z]:$/.test(parent) ? parent + p[i] : parent;
}

export function join(dir: string, name: string): string {
  const sep = sepOf(dir);
  return dir.endsWith('/') || dir.endsWith('\\') ? dir + name : dir + sep + name;
}

/** Reports whether path is ancestor itself or lies below it. */
export function isWithin(path: string, ancestor: string): boolean {
  if (path === ancestor) return true;
  const a = trimTrailing(ancestor);
  return path.startsWith(a + '/') || path.startsWith(a + '\\');
}

/** Slash-separated path of p relative to root ('' for root itself). */
export function relativeTo(root: string, p: string): string {
  if (!isWithin(p, root)) return p;
  return p.slice(trimTrailing(root).length).replace(/^[\\/]/, '').replace(/\\/g, '/');
}

/** Flattens the expanded tree into display rows. */
export function visibleRows(roots: TreeRoot[], state: TreeState): Row[] {
  const rows: Row[] = [];
  const walk = (dir: string, depth: number): void => {
    const kids = state.children.get(dir);
    if (kids === undefined || kids === 'loading') {
      rows.push({ path: `${dir}\u0000loading`, name: 'loading…', depth, isDir: false, expanded: false, loading: true });
      return;
    }
    for (const e of kids) {
      const expanded = e.isDir && state.expanded.has(e.path);
      rows.push({ path: e.path, name: e.name, depth, isDir: e.isDir, expanded, loading: false });
      if (expanded) walk(e.path, depth + 1);
    }
  };
  for (const r of roots) {
    const expanded = state.expanded.has(r.path);
    rows.push({ path: r.path, name: r.label, depth: 0, isDir: true, expanded, loading: false, root: r });
    if (expanded) walk(r.path, 1);
  }
  return rows;
}

/**
 * Reconciles file-system events with the tree. Returns a new state and the
 * set of loaded directories whose listing must be refetched. Removed (or
 * renamed-away) directories are dropped with their descendants; a removed
 * selection moves to its parent.
 */
export function applyFsEvents(state: TreeState, events: FsEvent[]): { state: TreeState; reload: Set<string> } {
  const next = cloneState(state);
  const reload = new Set<string>();
  for (const ev of events) {
    const parent = parentOf(ev.path);
    if (Array.isArray(next.children.get(parent))) reload.add(parent);
    if (ev.op === 'remove' || ev.op === 'rename') {
      for (const p of [...next.expanded]) if (isWithin(p, ev.path)) next.expanded.delete(p);
      for (const p of [...next.children.keys()]) if (isWithin(p, ev.path)) next.children.delete(p);
      for (const p of [...reload]) if (isWithin(p, ev.path)) reload.delete(p);
      if (next.selected && isWithin(next.selected, ev.path)) next.selected = parent;
    }
  }
  return { state: next, reload };
}

/** Index of the row for path, or -1. */
export function rowIndex(rows: Row[], path: string | null): number {
  return path === null ? -1 : rows.findIndex((r) => r.path === path);
}

/** Path delta rows away from the selection (clamped), skipping placeholders. */
export function moveSelection(rows: Row[], selected: string | null, delta: number): string | null {
  const real = rows.filter((r) => !r.loading);
  if (real.length === 0) return null;
  const i = real.findIndex((r) => r.path === selected);
  const j = i < 0 ? 0 : Math.max(0, Math.min(real.length - 1, i + delta));
  return real[j].path;
}

/** Directories from the root (exclusive) down to path's parent, top first. */
export function ancestorsWithin(root: string, path: string): string[] {
  const out: string[] = [];
  let p = parentOf(path);
  while (isWithin(p, root) && p !== root) {
    out.unshift(p);
    const up = parentOf(p);
    if (up === p) break;
    p = up;
  }
  return out;
}
