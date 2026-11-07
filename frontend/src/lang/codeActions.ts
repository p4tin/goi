// Show Context Actions (R8): gopls quick-fixes, refactorings and source
// actions at the selection, applied as edits or server commands.

import { lang } from '../lib/api';
import type { CodeAction, Diagnostic, Range } from '../lib/api';
import { h } from '../lib/dom';
import { activeTarget, fresh, info, warn } from './context';
import { problems } from './diagnostics';
import { applyFileEdits } from './edits';
import { showListPopup } from './popup';
import { toRange } from './positions';

function before(a: { line: number; character: number }, b: { line: number; character: number }): boolean {
  return a.line < b.line || (a.line === b.line && a.character < b.character);
}

/** Diagnostics whose range overlaps sel (or touches the cursor line when
 * the selection is empty). */
export function overlapping(diags: readonly Diagnostic[], sel: Range): Diagnostic[] {
  const empty = sel.start.line === sel.end.line && sel.start.character === sel.end.character;
  return diags.filter((d) => {
    if (empty) return d.range.start.line <= sel.start.line && d.range.end.line >= sel.start.line;
    return !before(d.range.end, sel.start) && !before(sel.end, d.range.start);
  });
}

function rank(a: CodeAction): number {
  const k = a.kind ?? '';
  if (k.startsWith('quickfix')) return a.isPreferred ? 0 : 1;
  if (k.startsWith('refactor')) return 2;
  if (k.startsWith('source')) return 3;
  return 4;
}

/** Enabled actions, quick-fixes (preferred first) before refactorings and
 * source actions; stable within a rank. */
export function sortActions(actions: readonly CodeAction[]): CodeAction[] {
  return actions
    .filter((a) => !a.disabled)
    .map((a, i) => ({ a, i }))
    .sort((x, y) => rank(x.a) - rank(y.a) || x.i - y.i)
    .map((x) => x.a);
}

export function kindBadge(a: CodeAction): string {
  const k = a.kind ?? '';
  if (k.startsWith('quickfix')) return 'fix';
  if (k.startsWith('refactor')) return 'ref';
  if (k.startsWith('source')) return 'src';
  return '·';
}

async function run(a: CodeAction): Promise<void> {
  try {
    const edits = await lang.runCodeAction(a);
    if (edits.length) await applyFileEdits(edits);
  } catch (err) {
    warn('lang.codeAction', `Cannot apply “${a.title}”`, String(err));
  }
}

export async function showContextActions(): Promise<void> {
  const t = await activeTarget();
  if (!t) return;
  const sel = t.view.state.selection.main;
  const range = toRange(t.view.state.doc, sel.from, sel.to);
  const diags = overlapping(problems.peek().get(t.buf.path) ?? [], range);
  const version = t.buf.version;
  let actions: CodeAction[];
  try {
    actions = sortActions(await lang.codeActions(t.buf.path, range, diags));
  } catch (err) {
    warn('lang.codeAction', 'Cannot list context actions', String(err));
    return;
  }
  if (!fresh(t.buf, version)) return;
  if (!actions.length) {
    info('lang.codeAction', 'No context actions here');
    return;
  }
  const at = t.view.coordsAtPos(sel.head);
  showListPopup({
    items: actions,
    title: 'Context Actions',
    filter: actions.length > 8,
    text: (a) => a.title,
    at: at ? { x: at.left, y: at.bottom + 4 } : undefined,
    render: (a) => [h('span', { class: 'lp-kind' }, kindBadge(a)), h('span', null, a.title)],
    onPick: (a) => void run(a),
  });
}
