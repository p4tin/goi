// Evaluate Expression (R9.4): evaluates in the selected frame and shows the
// result as an expandable tree; Add to Watches keeps the expression.

import { showDialog } from '../app/dialog';
import { settings } from '../app/state';
import { editors } from '../editor/install';
import { debug } from '../lib/api';
import { h } from '../lib/dom';
import { addWatch } from './debugWindow';
import type { DebugTab } from './debugWindow';
import { VarTree } from './variables';

export async function openEvaluate(t: DebugTab, initial?: string): Promise<void> {
  const view = editors.activeView();
  const sel = view ? view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to) : '';
  const input = h('input', { class: 'input eval-input', type: 'text', value: initial ?? (sel && !sel.includes('\n') ? sel : ''), placeholder: 'Expression, e.g. len(items) or user.Name', spellcheck: false });
  const frameId = (): number => t.frames[t.frameIndex]?.id ?? 0;
  const tree = new VarTree({
    sessionId: () => t.id,
    showTypes: () => settings.peek()?.debug?.showTypes ?? true,
    showAddresses: () => settings.peek()?.debug?.showAddresses ?? false,
    showUnreadable: () => true,
    canSetValue: () => false,
    frameId,
    emptyText: 'Enter an expression and press Enter',
  });
  tree.render();
  let seq = 0;
  const evaluate = async (): Promise<void> => {
    const expr = input.value.trim();
    if (!expr) return;
    const my = ++seq;
    try {
      const r = await debug.evaluate(t.id, expr, frameId(), 'repl');
      if (my !== seq) return;
      tree.setRoots([{ key: `eval:${expr}`, name: expr, value: r.result, type: r.type, evaluateName: expr, ref: r.variablesReference, parentRef: 0, children: null, expanded: r.variablesReference > 0, loading: false, error: '', kind: 'watch', watchIndex: -1 }]);
    } catch (err) {
      if (my !== seq) return;
      tree.setRoots([{ key: `eval:${expr}`, name: expr, value: '', type: '', evaluateName: expr, ref: 0, parentRef: 0, children: null, expanded: false, loading: false, error: (err instanceof Error ? err.message : String(err)).replace(/^dap: evaluate: /, ''), kind: 'watch', watchIndex: -1 }]);
    }
  };
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      e.stopPropagation();
      void evaluate();
    }
  });
  const frame = t.frames[t.frameIndex];
  const body = h('div', { class: 'eval' }, h('div', { class: 'eval-frame' }, frame ? `In ${frame.name} (goroutine ${t.threadId})` : 'No frame selected'), input, h('div', { class: 'eval-result' }, tree.el));
  queueMicrotask(() => {
    input.focus();
    input.select();
    if (input.value) void evaluate();
  });
  const res = await showDialog<'watch' | 'close'>({
    title: 'Evaluate Expression',
    body,
    focus: input,
    cancelValue: 'close',
    buttons: [
      { label: 'Add to Watches', value: 'watch' },
      { label: 'Close', value: 'close' },
    ],
  });
  if (res === 'watch' && input.value.trim()) addWatch(input.value.trim());
}
