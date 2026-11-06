// Key sequence notation, compatible with CodeMirror: "Mod-Shift-s", chords
// as space-separated strokes "Mod-k Mod-s". Mod is Cmd on macOS, Ctrl
// elsewhere. Canonical strokes order modifiers Alt-Ctrl-Meta-Shift.

export interface KeyStroke {
  mod: boolean;
  alt: boolean;
  ctrl: boolean;
  meta: boolean;
  shift: boolean;
  key: string;
}

const PUNCT: Record<string, string> = {
  ',': 'Comma',
  '.': 'Period',
  '/': 'Slash',
  '\\': 'Backslash',
  '`': 'Backquote',
  '[': 'BracketLeft',
  ']': 'BracketRight',
  ';': 'Semicolon',
  "'": 'Quote',
  '=': 'Equal',
  '-': 'Minus',
  ' ': 'Space',
};

const NAMED = [
  'Enter', 'Escape', 'Tab', 'Space', 'Backspace', 'Delete', 'Insert', 'Home', 'End', 'PageUp', 'PageDown',
  'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', ...Object.values(PUNCT),
  ...Array.from({ length: 12 }, (_, i) => `F${i + 1}`),
];
const NAMED_LOWER = new Map(NAMED.map((n) => [n.toLowerCase(), n]));
const ALIASES: Record<string, string> = {
  esc: 'Escape', del: 'Delete', return: 'Enter', up: 'ArrowUp', down: 'ArrowDown', left: 'ArrowLeft', right: 'ArrowRight',
};

/** Normalizes a key name: letters/digits lower case, punctuation and named
 * keys to their canonical names. */
export function normalizeKey(k: string): string {
  if (k.length === 1) return PUNCT[k] ?? k.toLowerCase();
  const l = k.toLowerCase();
  return NAMED_LOWER.get(l) ?? ALIASES[l] ?? k.charAt(0).toUpperCase() + k.slice(1);
}

export function parseStroke(s: string): KeyStroke {
  // A trailing "-" means the Minus key ("Mod--").
  const parts = s.endsWith('--') ? [...s.slice(0, -2).split('-'), '-'] : s.split('-');
  const key = parts.pop() ?? '';
  const st: KeyStroke = { mod: false, alt: false, ctrl: false, meta: false, shift: false, key: normalizeKey(key) };
  for (const m of parts) {
    switch (m.toLowerCase()) {
      case 'mod':
        st.mod = true;
        break;
      case 'alt':
      case 'option':
      case 'opt':
        st.alt = true;
        break;
      case 'ctrl':
      case 'control':
        st.ctrl = true;
        break;
      case 'meta':
      case 'cmd':
      case 'command':
        st.meta = true;
        break;
      case 'shift':
        st.shift = true;
        break;
      default:
        throw new Error(`unknown modifier "${m}" in "${s}"`);
    }
  }
  return st;
}

export function parseSequence(seq: string): KeyStroke[] {
  return seq.trim().split(/\s+/).filter(Boolean).map(parseStroke);
}

export function stringifyStroke(st: KeyStroke): string {
  return `${st.mod ? 'Mod-' : ''}${st.alt ? 'Alt-' : ''}${st.ctrl ? 'Ctrl-' : ''}${st.meta ? 'Meta-' : ''}${st.shift ? 'Shift-' : ''}${st.key}`;
}

function resolveStroke(st: KeyStroke, isMac: boolean): KeyStroke {
  if (!st.mod) return st;
  return { ...st, mod: false, meta: st.meta || isMac, ctrl: st.ctrl || !isMac };
}

/** Replaces Mod with Meta (macOS) or Ctrl and normalizes the sequence. */
export function resolveMod(seq: string, isMac: boolean): string {
  return parseSequence(seq).map((s) => stringifyStroke(resolveStroke(s, isMac))).join(' ');
}

/** Inverse of resolveMod for storage: the platform's Mod key becomes Mod. */
export function toModForm(seq: string, isMac: boolean): string {
  return parseSequence(seq)
    .map((s) => {
      const r = resolveStroke(s, isMac);
      if (isMac && r.meta) return stringifyStroke({ ...r, meta: false, mod: true });
      if (!isMac && r.ctrl) return stringifyStroke({ ...r, ctrl: false, mod: true });
      return stringifyStroke(r);
    })
    .join(' ');
}

export interface KeyEventLike {
  key: string;
  code: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

const MODIFIER_KEYS = new Set(['Shift', 'Control', 'Alt', 'Meta', 'OS', 'AltGraph', 'CapsLock', 'Fn']);

/** The canonical (resolved, no Mod) stroke for a keyboard event, or null for
 * a bare modifier press. Letters and digits come from e.code so that
 * Alt-modified keys on macOS (which change e.key) still match. */
export function strokeFromEvent(e: KeyEventLike, _isMac: boolean): string | null {
  if (MODIFIER_KEYS.has(e.key)) return null;
  let key: string | undefined;
  if (/^Key[A-Z]$/.test(e.code)) key = e.code.slice(3).toLowerCase();
  else if (/^Digit\d$/.test(e.code)) key = e.code.slice(5);
  else if (NAMED_LOWER.has(e.code.toLowerCase())) key = NAMED_LOWER.get(e.code.toLowerCase());
  key ??= normalizeKey(e.key);
  return stringifyStroke({ mod: false, alt: e.altKey, ctrl: e.ctrlKey, meta: e.metaKey, shift: e.shiftKey, key });
}

const MAC_KEYS: Record<string, string> = {
  ArrowUp: '↑', ArrowDown: '↓', ArrowLeft: '←', ArrowRight: '→', Enter: '↩', Escape: '⎋', Backspace: '⌫',
  Delete: '⌦', Tab: '⇥', Space: 'Space', PageUp: '⇞', PageDown: '⇟', Home: '↖', End: '↘',
};
const PUNCT_DISPLAY = Object.fromEntries(Object.entries(PUNCT).map(([ch, name]) => [name, ch === ' ' ? 'Space' : ch]));

function keyLabel(key: string, isMac: boolean): string {
  if (isMac && MAC_KEYS[key]) return MAC_KEYS[key];
  if (PUNCT_DISPLAY[key]) return PUNCT_DISPLAY[key];
  return key.length === 1 ? key.toUpperCase() : key;
}

/** Human-readable form: "⌘⇧S" on macOS, "Ctrl+Shift+S" elsewhere. */
export function formatForDisplay(seq: string, isMac: boolean): string {
  return parseSequence(seq)
    .map((raw) => {
      const s = resolveStroke(raw, isMac);
      const k = keyLabel(s.key, isMac);
      if (isMac) return `${s.ctrl ? '⌃' : ''}${s.alt ? '⌥' : ''}${s.shift ? '⇧' : ''}${s.meta ? '⌘' : ''}${k}`;
      return [s.ctrl && 'Ctrl', s.alt && 'Alt', s.shift && 'Shift', s.meta && 'Meta', k].filter(Boolean).join('+');
    })
    .join(' ');
}

/** True if stroke a has Ctrl, Meta or Alt (or Mod). */
export function hasCommandModifier(stroke: string): boolean {
  const s = parseStroke(stroke);
  return s.mod || s.ctrl || s.meta || s.alt;
}
