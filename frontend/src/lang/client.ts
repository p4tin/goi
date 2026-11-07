// LangClient keeps gopls's view of open Go buffers current: didOpen on
// open, debounced full-text didChange on edits, didSave, didClose. Every
// intelligence request first calls flush() so it sees the current text.

import type { Buffer } from '../editor/buffer';
import { baseName } from '../editor/buffer';
import { lang } from '../lib/api';
import type { LangState, LangStatus } from '../lib/api';
import { signal } from '../lib/store';

export const SYNC_DEBOUNCE_MS = 150;

/** Files gopls understands. */
export function isLangPath(path: string): boolean {
  const base = baseName(path);
  return base.endsWith('.go') || base === 'go.mod' || base === 'go.work';
}

/** Buffers sent to gopls: Go-family files in normal (not large-file) mode. */
export function isLangBuffer(buf: Buffer): boolean {
  return isLangPath(buf.path) && buf.highlight && !buf.binary;
}

export function isGoFile(path: string): boolean {
  return path.endsWith('.go');
}

const LIVE: LangState[] = ['ready', 'indexing'];

export interface SyncApi {
  didOpen(path: string, version: number, text: string): Promise<void>;
  didChange(path: string, version: number, text: string): Promise<void>;
  didSave(path: string): Promise<void>;
  didClose(path: string): Promise<void>;
}

interface Tracked {
  buf: Buffer;
  sent: number;
  timer: ReturnType<typeof setTimeout> | null;
}

export class LangClient {
  readonly status = signal<LangStatus>({ state: 'stopped', message: '', bin: '' });
  private docs = new Map<string, Tracked>();

  constructor(private api: SyncApi = lang) {}

  /** True when gopls can answer requests. */
  live(): boolean {
    return LIVE.includes(this.status.peek().state);
  }

  setStatus(st: LangStatus): void {
    const wasLive = this.live();
    this.status.set(st);
    // A new or restarted server may not know our buffers (e.g. gopls was
    // installed after they opened): send them again.
    if (!wasLive && LIVE.includes(st.state)) this.reopenAll();
    if (st.state === 'starting') this.reopenAll();
  }

  tracked(path: string): boolean {
    return this.docs.has(path);
  }

  open(buf: Buffer): void {
    if (!isLangBuffer(buf) || this.docs.has(buf.path)) return;
    this.docs.set(buf.path, { buf, sent: buf.version, timer: null });
    void this.api.didOpen(buf.path, buf.version, buf.text()).catch(() => {});
  }

  changed(buf: Buffer): void {
    const t = this.docs.get(buf.path);
    if (!t) return;
    if (t.timer) clearTimeout(t.timer);
    t.timer = setTimeout(() => {
      t.timer = null;
      void this.send(t);
    }, SYNC_DEBOUNCE_MS);
  }

  private async send(t: Tracked): Promise<void> {
    if (t.sent === t.buf.version) return;
    t.sent = t.buf.version;
    await this.api.didChange(t.buf.path, t.buf.version, t.buf.text()).catch(() => {});
  }

  /** Sends pending changes now. Returns the version gopls has. */
  async flush(buf: Buffer): Promise<number> {
    const t = this.docs.get(buf.path);
    if (!t) return buf.version;
    if (t.timer) {
      clearTimeout(t.timer);
      t.timer = null;
    }
    await this.send(t);
    return t.sent;
  }

  saved(buf: Buffer): void {
    if (!this.docs.has(buf.path)) return;
    void this.flush(buf).then(() => this.api.didSave(buf.path).catch(() => {}));
  }

  close(buf: Buffer): void {
    const t = this.docs.get(buf.path);
    if (!t) return;
    if (t.timer) clearTimeout(t.timer);
    this.docs.delete(buf.path);
    void this.api.didClose(buf.path).catch(() => {});
  }

  private reopenAll(): void {
    for (const t of this.docs.values()) {
      if (t.timer) {
        clearTimeout(t.timer);
        t.timer = null;
      }
      t.sent = t.buf.version;
      void this.api.didOpen(t.buf.path, t.buf.version, t.buf.text()).catch(() => {});
    }
  }
}
