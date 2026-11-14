// Files changed by agents, per session: the content each file had before the
// session's first change to it (null: the agent created the file), so the
// user can review (diff) and revert. External MCP clients use session "".

import { signal } from '../lib/store';

export interface FileChange {
  path: string;
  /** Content before the first change; null when the file was created. */
  before: string | null;
  /** Number of agent writes to the file. */
  writes: number;
}

export class ChangeLog {
  private bySession = new Map<string, Map<string, FileChange>>();
  /** Bumped on every change, for views. */
  readonly version = signal(0);

  /** Records a write; before is only kept for the first one. */
  record(session: string, path: string, before: string | null): void {
    let m = this.bySession.get(session);
    if (!m) this.bySession.set(session, (m = new Map()));
    const c = m.get(path);
    if (c) c.writes++;
    else m.set(path, { path, before, writes: 1 });
    this.version.update((v) => v + 1);
  }

  list(session: string): FileChange[] {
    return [...(this.bySession.get(session)?.values() ?? [])];
  }

  get(session: string, path: string): FileChange | undefined {
    return this.bySession.get(session)?.get(path);
  }

  forget(session: string, path?: string): void {
    if (path === undefined) this.bySession.delete(session);
    else this.bySession.get(session)?.delete(path);
    this.version.update((v) => v + 1);
  }
}

export const changeLog = new ChangeLog();
