// Global key dispatcher: maps keydown events (including multi-stroke chords)
// to commands. Runs in the capture phase so Goi bindings win over editor
// defaults for the same keys.

import { formatForDisplay, hasCommandModifier, parseStroke, strokeFromEvent } from './chords';
import type { KeyEventLike } from './chords';

export const CHORD_TIMEOUT_MS = 1500;

export interface DispatchEvent extends KeyEventLike {
  target: EventTarget | null;
  repeat?: boolean;
  preventDefault(): void;
  stopPropagation(): void;
}

export interface DispatcherOptions {
  /** Resolved sequence → command id. */
  bindings: () => Map<string, string>;
  canRun: (id: string) => boolean;
  run: (id: string) => void;
  isMac: () => boolean;
  /** Called with a hint while a chord is pending, null when it ends. */
  onPending?: (hint: string | null) => void;
  timeoutMs?: number;
}

/** True when target is a text field outside CodeMirror. */
function inTextField(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || typeof el.closest !== 'function') return false;
  if (el.closest('.cm-editor')) return false;
  return el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el.isContentEditable === true;
}

/** True when target is inside the integrated terminal. */
function inTerminal(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  return !!el && typeof el.closest === 'function' && !!el.closest('.xterm');
}

/** Commands that still run while the terminal has focus; other keys go to
 * the shell (Ctrl-R, Ctrl-D, … belong to it). */
export const TERMINAL_PASSTHROUGH = new Set([
  'terminal.toggle',
  'terminal.new',
  'terminal.close',
  'view.toggleBottom',
  'view.toggleLeft',
  'nav.gotoFile',
  'nav.gotoAction',
  'nav.recentFiles',
  'settings.open',
]);

/** Whether a binding may run while the terminal is focused: allowlisted
 * commands, function keys, and on macOS Cmd shortcuts that aren't editor
 * commands (shells don't use Cmd). */
export function allowedInTerminal(id: string, stroke: string, isMac: boolean): boolean {
  if (TERMINAL_PASSTHROUGH.has(id)) return true;
  const s = parseStroke(stroke);
  if (/^F\d{1,2}$/.test(s.key)) return true;
  return isMac && s.meta && !id.startsWith('editor.');
}

function inDialog(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  return !!el && typeof el.closest === 'function' && !!el.closest('.dialog, .dialog-backdrop');
}

export class KeyDispatcher {
  private pending: string[] = [];
  private pendingAt = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private prefixCache: { map: Map<string, string>; prefixes: Set<string> } | null = null;

  constructor(private opts: DispatcherOptions) {}

  /** Handles one keydown. Returns true if it was consumed. */
  handle(e: DispatchEvent): boolean {
    const stroke = strokeFromEvent(e, this.opts.isMac());
    if (!stroke) return false;
    if (inDialog(e.target)) {
      this.reset();
      return false;
    }
    if (inTextField(e.target) && !hasCommandModifier(stroke)) {
      this.reset();
      return false;
    }
    const timeout = this.opts.timeoutMs ?? CHORD_TIMEOUT_MS;
    if (this.pending.length && Date.now() - this.pendingAt > timeout) this.reset();

    const map = this.opts.bindings();
    const hadPending = this.pending.length > 0;
    const seq = [...this.pending, stroke].join(' ');
    const id = map.get(seq);
    if (id && inTerminal(e.target) && !allowedInTerminal(id, stroke, this.opts.isMac())) {
      this.reset();
      return false;
    }
    if (id) {
      this.reset();
      if (!this.opts.canRun(id)) return hadPending ? this.consume(e) : false;
      this.opts.run(id);
      return this.consume(e);
    }
    if (this.prefixes(map).has(seq)) {
      this.pending.push(stroke);
      this.pendingAt = Date.now();
      this.opts.onPending?.(`(${formatForDisplay(seq, this.opts.isMac())}) was pressed, waiting for second key…`);
      if (this.timer) clearTimeout(this.timer);
      this.timer = setTimeout(() => this.reset(), timeout);
      return this.consume(e);
    }
    if (hadPending) {
      // An unbound chord continuation is swallowed rather than typed.
      this.reset();
      return this.consume(e);
    }
    return false;
  }

  get pendingSequence(): string {
    return this.pending.join(' ');
  }

  reset(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    if (this.pending.length) {
      this.pending = [];
      this.opts.onPending?.(null);
    }
  }

  private consume(e: DispatchEvent): boolean {
    e.preventDefault();
    e.stopPropagation();
    return true;
  }

  private prefixes(map: Map<string, string>): Set<string> {
    if (this.prefixCache?.map !== map) {
      const prefixes = new Set<string>();
      for (const seq of map.keys()) {
        const parts = seq.split(' ');
        for (let i = 1; i < parts.length; i++) prefixes.add(parts.slice(0, i).join(' '));
      }
      this.prefixCache = { map, prefixes };
    }
    return this.prefixCache.prefixes;
  }
}
