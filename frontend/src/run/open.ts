// Opens source locations found in console output.

import { project } from '../app/state';
import { editors } from '../editor/install';
import { search } from '../lib/api';
import { bestCandidate, isAbsolutePath, joinPath } from './links';
import type { LocationMatch } from './links';
import { notifyError } from './state';

/** Resolves a relative path: against base, then by searching the project
 * index for files ending with it (preferring ones under base). */
export async function resolveLocationPath(path: string, base: string): Promise<string | null> {
  if (isAbsolutePath(path)) return path;
  const name = path.split(/[\\/]/).pop() ?? path;
  try {
    const res = await search.files(name, 200);
    const best = bestCandidate(
      path,
      res.matches.map((m) => m.path),
      base,
    );
    if (best) return best;
  } catch {
    // fall through to the joined path
  }
  const root = project.peek()?.root ?? '';
  return base || root ? joinPath(base || root, path) : null;
}

export async function openLocation(loc: LocationMatch, base: string): Promise<void> {
  const path = await resolveLocationPath(loc.path, base);
  if (!path) return;
  try {
    await editors.open(path, { line: loc.line, col: loc.col, focus: true });
  } catch (err) {
    notifyError('open-location', `Cannot open ${loc.path}`, err);
  }
}
