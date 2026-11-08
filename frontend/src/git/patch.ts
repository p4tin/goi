// Patches for partial staging (R2.2, R4.2): a file diff's header plus the
// chosen hunks, applied with `git apply --recount`.

import type { GitFileDiff, GitHunk } from '../lib/api';

/** A patch with only the given hunks of fd. */
export function buildPatch(fd: GitFileDiff, hunks: number[]): string {
  if (fd.binary) throw new Error('Binary files can’t be staged partially');
  const picked = hunks.filter((i) => i >= 0 && i < fd.hunks.length);
  if (!picked.length) throw new Error('No changes selected');
  const out = [...fd.header];
  for (const i of picked) out.push(fd.hunks[i].header, ...fd.hunks[i].lines);
  return out.join('\n') + '\n';
}

/** The hunk whose new-side range contains (or is nearest after) a 1-based
 * line, or -1. Deletion-only hunks (newLines 0) match the line they follow. */
export function hunkAtLine(hunks: readonly GitHunk[], line: number): number {
  for (let i = 0; i < hunks.length; i++) {
    const h = hunks[i];
    const start = h.newStart;
    const end = h.newLines === 0 ? h.newStart + 1 : h.newStart + h.newLines - 1;
    if (line >= start && line <= end) return i;
  }
  return -1;
}

/** 1-based new-side line range a hunk's changed lines cover (for scrolling
 * and highlighting); deletions report the following line. */
export function changedRange(h: GitHunk): { from: number; to: number } {
  let line = h.newStart;
  let from = -1;
  let to = -1;
  for (const l of h.lines) {
    const c = l[0];
    if (c === '+') {
      if (from < 0) from = line;
      to = line;
      line++;
    } else if (c === '-') {
      if (from < 0) from = line;
      to = Math.max(to, line);
    } else if (c === ' ') {
      line++;
    }
  }
  if (from < 0) from = to = h.newStart;
  return { from: Math.max(1, from), to: Math.max(1, to) };
}

/** Counts added and removed lines. */
export function hunkStats(fd: GitFileDiff): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const h of fd.hunks) {
    for (const l of h.lines) {
      if (l[0] === '+') added++;
      else if (l[0] === '-') removed++;
    }
  }
  return { added, removed };
}
