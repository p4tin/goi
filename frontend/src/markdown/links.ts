// Link and image targets in the Markdown preview. Relative paths resolve
// against the Markdown file's folder (absolute ones against the project
// root, as on GitHub). Only http(s) is external; other schemes do nothing.

import { dirName } from '../editor/buffer';

export type LinkTarget =
  | { kind: 'external'; url: string }
  | { kind: 'anchor'; id: string }
  | { kind: 'file'; path: string; anchor?: string }
  | { kind: 'none' };

export type ImageTarget = { kind: 'remote'; url: string } | { kind: 'local'; path: string } | { kind: 'none' };

const SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const DRIVE = /^[a-z]:[\\/]/i;

const decode = (s: string): string => {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
};

/** Joins rel onto dir, resolving . and .., with dir's separator. */
export function joinPath(dir: string, rel: string): string {
  const sep = dir.includes('\\') && !dir.includes('/') ? '\\' : '/';
  const abs = rel.startsWith('/') || rel.startsWith('\\') || DRIVE.test(rel);
  const base = abs ? '' : dir;
  const prefix = /^[a-z]:/i.exec(abs ? rel : base)?.[0] ?? '';
  const parts: string[] = [];
  for (const p of `${base}/${rel}`.slice(prefix.length).split(/[\\/]+/)) {
    if (!p || p === '.') continue;
    if (p === '..') parts.pop();
    else parts.push(p);
  }
  return `${prefix}${sep}${parts.join(sep)}`;
}

/** Splits "path#anchor?query" and decodes the path. */
function splitTarget(href: string): { path: string; anchor?: string } {
  const hash = href.indexOf('#');
  const anchor = hash >= 0 ? decode(href.slice(hash + 1)) : undefined;
  let path = hash >= 0 ? href.slice(0, hash) : href;
  const q = path.indexOf('?');
  if (q >= 0) path = path.slice(0, q);
  return { path: decode(path), anchor: anchor || undefined };
}

function local(path: string, mdPath: string, root?: string): string {
  if (DRIVE.test(path)) return joinPath(path, '.');
  if (path.startsWith('/')) return joinPath(root ?? '/', path.slice(1));
  return joinPath(dirName(mdPath), path);
}

export function classifyLink(href: string, mdPath: string, root?: string): LinkTarget {
  const h = href.trim();
  if (!h) return { kind: 'none' };
  if (/^https?:\/\//i.test(h)) return { kind: 'external', url: h };
  if (h.startsWith('#')) return h.length > 1 ? { kind: 'anchor', id: decode(h.slice(1)) } : { kind: 'none' };
  if (SCHEME.test(h) && !DRIVE.test(h)) return { kind: 'none' };
  const t = splitTarget(h);
  if (!t.path) return t.anchor ? { kind: 'anchor', id: t.anchor } : { kind: 'none' };
  return { kind: 'file', path: local(t.path, mdPath, root), anchor: t.anchor };
}

export function resolveImage(src: string, mdPath: string, root?: string): ImageTarget {
  const s = src.trim();
  if (!s) return { kind: 'none' };
  if (/^https?:\/\//i.test(s)) return { kind: 'remote', url: s };
  if (SCHEME.test(s) && !DRIVE.test(s)) return { kind: 'none' };
  const { path } = splitTarget(s);
  return path ? { kind: 'local', path: local(path, mdPath, root) } : { kind: 'none' };
}

/** Whether path is inside one of roots. */
export function insideRoots(path: string, roots: readonly string[]): boolean {
  const norm = (p: string): string => p.replace(/\\/g, '/').replace(/\/+$/, '');
  const p = norm(path);
  return roots.some((r) => {
    const n = norm(r);
    return p === n || p.startsWith(`${n}/`);
  });
}
