// Annotate with Git Blame (R9): per-line hash, author and date beside the
// editor, computed from the buffer's current text. Hover shows the commit
// summary; clicking shows the commit in the Git Log. Toggle with the same
// command.

import { RangeSet, StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { gutter, GutterMarker } from '@codemirror/view';
import type { EditorView } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { gitApi } from '../lib/api';
import type { GitBlameLine } from '../lib/api';
import { h } from '../lib/dom';
import { showCommit } from './log';
import { gitError } from './state';

const UNCOMMITTED = /^0{7,}/;

/** The annotation text of a line ("abc1234 Ada 2026-10-01"). */
export function blameLabel(b: GitBlameLine): string {
  if (UNCOMMITTED.test(b.hash)) return 'Not committed yet';
  const author = b.author.length > 14 ? b.author.slice(0, 13) + '…' : b.author;
  return `${b.hash.slice(0, 7)} ${author} ${b.date.slice(0, 10)}`;
}

class BlameMarker extends GutterMarker {
  constructor(readonly b: GitBlameLine, readonly first: boolean) {
    super();
  }

  eq(o: BlameMarker): boolean {
    return o.b.hash === this.b.hash && o.first === this.first && o.b.line === this.b.line;
  }

  toDOM(): Node {
    const uncommitted = UNCOMMITTED.test(this.b.hash);
    return h(
      'span',
      {
        class: `git-blame${this.first ? '' : ' cont'}${uncommitted ? ' uncommitted' : ''}`,
        title: uncommitted ? 'Not committed yet' : `${this.b.hash.slice(0, 10)} ${this.b.author}, ${this.b.date.slice(0, 10)}\n${this.b.summary}`,
        dataset: { hash: this.b.hash },
      },
      this.first ? blameLabel(this.b) : '',
    );
  }
}

const setBlame = StateEffect.define<RangeSet<BlameMarker> | null>();
const blameField = StateField.define<RangeSet<BlameMarker> | null>({
  create: () => null,
  update(v, tr) {
    for (const e of tr.effects) if (e.is(setBlame)) return e.value;
    return v && tr.docChanged ? v.map(tr.changes) : v;
  },
});

function markers(view: EditorView, lines: GitBlameLine[]): RangeSet<BlameMarker> {
  const doc = view.state.doc;
  const out = [];
  let prev = '';
  for (const b of lines) {
    if (b.line < 1 || b.line > doc.lines) continue;
    out.push(new BlameMarker(b, b.hash !== prev).range(doc.line(b.line).from));
    prev = b.hash;
  }
  return RangeSet.of(out, true);
}

const on = new Set<string>();

export function blameExtension(buf: Buffer): Extension {
  if (buf.binary) return [];
  return [
    blameField,
    gutter({
      class: 'cm-blame-gutter',
      markers: (v) => v.state.field(blameField) ?? RangeSet.empty,
      domEventHandlers: {
        click(_view, _block, e) {
          const t = (e.target as HTMLElement).closest('.git-blame') as HTMLElement | null;
          const hash = t?.dataset.hash;
          if (!hash || UNCOMMITTED.test(hash)) return false;
          void showCommit(hash);
          return true;
        },
      },
    }),
  ];
}

async function annotate(buf: Buffer): Promise<void> {
  let lines: GitBlameLine[];
  try {
    lines = await gitApi.blame(buf.path, buf.text());
  } catch (err) {
    gitError('Cannot annotate', err);
    on.delete(buf.path);
    return;
  }
  if (!on.has(buf.path)) return;
  for (const v of buf.views) v.dispatch({ effects: setBlame.of(markers(v, lines)) });
}

export async function toggleBlame(): Promise<void> {
  const buf = editors.activeBuffer();
  if (!buf) return;
  if (on.has(buf.path)) {
    on.delete(buf.path);
    for (const v of buf.views) v.dispatch({ effects: setBlame.of(null) });
    return;
  }
  on.add(buf.path);
  await annotate(buf);
}

export function blameOn(path: string): boolean {
  return on.has(path);
}

export function installBlame(): void {
  editors.addExtension(blameExtension);
  // Refresh after saves so new lines get annotations.
  editors.hooks.saved.on((buf) => {
    if (on.has(buf.path)) void annotate(buf);
  });
  editors.hooks.closed.on((buf) => on.delete(buf.path));
}
