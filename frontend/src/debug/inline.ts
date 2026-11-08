// Editor decorations while stopped (R5.4, R9.6): the execution line (or the
// selected frame's line) and inline variable values after the lines of the
// stopped function.

import { StateEffect, StateField } from '@codemirror/state';
import type { Extension, Range, Text } from '@codemirror/state';
import { Decoration, EditorView, WidgetType } from '@codemirror/view';
import type { DecorationSet } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';

export interface DebugPosition {
  path: string;
  line: number;
  /** "top" = where execution stopped; "frame" = a selected caller frame. */
  kind: 'top' | 'frame';
  /** line → "name = value" parts. */
  values: Map<number, string[]>;
}

class InlineValueWidget extends WidgetType {
  constructor(readonly text: string) {
    super();
  }

  eq(o: InlineValueWidget): boolean {
    return o.text === this.text;
  }

  toDOM(): HTMLElement {
    const s = document.createElement('span');
    s.className = 'cm-inline-value';
    s.textContent = this.text;
    return s;
  }

  ignoreEvent(): boolean {
    return true;
  }
}

const setPosition = StateEffect.define<DebugPosition | null>();

function build(doc: Text, pos: DebugPosition | null): DecorationSet {
  if (!pos) return Decoration.none;
  const decos: Range<Decoration>[] = [];
  if (pos.line >= 1 && pos.line <= doc.lines) {
    decos.push(Decoration.line({ class: pos.kind === 'top' ? 'cm-debug-line' : 'cm-debug-frame-line' }).range(doc.line(pos.line).from));
  }
  for (const [ln, parts] of [...pos.values.entries()].sort((a, b) => a[0] - b[0])) {
    if (ln < 1 || ln > doc.lines) continue;
    decos.push(Decoration.widget({ widget: new InlineValueWidget(parts.join(', ')), side: 1 }).range(doc.line(ln).to));
  }
  return Decoration.set(decos, true);
}

const field = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setPosition)) return build(tr.state.doc, e.value);
    return tr.docChanged ? value.map(tr.changes) : value;
  },
  provide: (f) => EditorView.decorations.from(f),
});

let current: DebugPosition | null = null;

export function debugDecorations(buf: Buffer): Extension {
  if (buf.binary) return [];
  return field;
}

function apply(): void {
  for (const buf of editors.buffers.values()) {
    const pos = current && current.path === buf.path ? current : null;
    for (const v of buf.views) {
      if (v.state.field(field, false) === undefined) continue;
      v.dispatch({ effects: setPosition.of(pos) });
    }
  }
}

/** Shows the execution position (null clears it everywhere). */
export function showDebugPosition(pos: DebugPosition | null): void {
  current = pos;
  apply();
}

export function currentDebugPosition(): DebugPosition | null {
  return current;
}

/** Re-applies decorations to views opened after the stop. */
export function installDebugDecorations(): void {
  editors.addExtension(debugDecorations);
  editors.hooks.viewCreated.on(({ buffer, view }) => {
    if (current && current.path === buffer.path) queueMicrotask(() => view.dispatch({ effects: setPosition.of(current) }));
  });
}
