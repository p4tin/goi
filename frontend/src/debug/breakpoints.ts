// Breakpoints (R6): line breakpoints in a gutter of Go editors (and by
// clicking line numbers), mapped through edits and persisted per project;
// conditions, hit counts, logpoints and enable/disable; function breakpoints
// and Break on panic. The backend gets every change; running sessions report
// verification results back.

import { RangeSet, StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { EditorView, gutter, GutterMarker, ViewPlugin } from '@codemirror/view';
import { showDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { debug, on, Topics } from '../lib/api';
import type { DebugBreakpointsEvent, DebugStateEvent, FunctionBreakpoint, LineBreakpoint } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { iconButton } from '../run/icons';

export interface Verification {
  verified: boolean;
  message: string;
}

/** Line breakpoints by file path, sorted by line. */
export const lineBreakpoints = signal<Record<string, LineBreakpoint[]>>({});
export const functionBreakpoints = signal<FunctionBreakpoint[]>([]);
export const breakOnPanic = signal(false);
/** Latest verification by path → line (from the active session). */
export const verification = signal<Record<string, Record<number, Verification>>>({});
export const functionVerification = signal<Verification[]>([]);

export function newBreakpoint(line: number): LineBreakpoint {
  return { line, condition: '', hitCondition: '', logMessage: '', enabled: true };
}

/** "== 3" for "3", otherwise the expression as given. */
export function hitConditionLabel(s: string): string {
  const t = s.trim();
  return /^\d+$/.test(t) ? `== ${t}` : t;
}

/** Tooltip/description of a breakpoint. */
export function describeBreakpoint(b: LineBreakpoint, v?: Verification): string {
  const parts: string[] = [`Line ${b.line}`];
  if (!b.enabled) parts.push('disabled');
  if (b.condition) parts.push(`when ${b.condition}`);
  if (b.hitCondition) parts.push(`hit count ${hitConditionLabel(b.hitCondition)}`);
  if (b.logMessage) parts.push(`logs "${b.logMessage}"`);
  if (v && !v.verified) parts.push(v.message || 'not verified');
  return parts.join(', ');
}

/** Marker class for a breakpoint's look. */
export function breakpointClass(b: LineBreakpoint, v?: Verification): string {
  const cls = ['bp'];
  if (b.logMessage) cls.push('bp-log');
  else if (b.condition || b.hitCondition) cls.push('bp-cond');
  if (!b.enabled) cls.push('bp-disabled');
  else if (v && !v.verified) cls.push('bp-unverified');
  return cls.join(' ');
}

/** Sorts by line and keeps one breakpoint per line (the first). */
export function normalize(bps: LineBreakpoint[]): LineBreakpoint[] {
  const seen = new Set<number>();
  return [...bps]
    .sort((a, b) => a.line - b.line)
    .filter((b) => {
      if (b.line < 1 || seen.has(b.line)) return false;
      seen.add(b.line);
      return true;
    });
}

/** Adds or removes a breakpoint on line; returns the new list. */
export function toggleIn(bps: LineBreakpoint[], line: number): LineBreakpoint[] {
  return bps.some((b) => b.line === line) ? bps.filter((b) => b.line !== line) : normalize([...bps, newBreakpoint(line)]);
}

function setFile(path: string, bps: LineBreakpoint[]): void {
  const next = { ...lineBreakpoints.peek() };
  const list = normalize(bps);
  if (list.length) next[path] = list;
  else delete next[path];
  lineBreakpoints.set(next);
}

export function toggleBreakpoint(path: string, line: number): void {
  setFile(path, toggleIn(lineBreakpoints.peek()[path] ?? [], line));
}

export function updateBreakpoint(path: string, line: number, change: Partial<LineBreakpoint> | null): void {
  const bps = lineBreakpoints.peek()[path] ?? [];
  setFile(path, change === null ? bps.filter((b) => b.line !== line) : bps.map((b) => (b.line === line ? { ...b, ...change } : b)));
}

// ---- Editor integration

class BreakpointMarker extends GutterMarker {
  constructor(
    readonly bp: LineBreakpoint,
    readonly v: Verification | undefined,
  ) {
    super();
  }

  eq(o: BreakpointMarker): boolean {
    return breakpointClass(o.bp, o.v) === breakpointClass(this.bp, this.v) && describeBreakpoint(o.bp, o.v) === describeBreakpoint(this.bp, this.v);
  }

  toDOM(): Node {
    return h('span', { class: breakpointClass(this.bp, this.v), title: describeBreakpoint(this.bp, this.v) });
  }
}

const setMarkers = StateEffect.define<RangeSet<BreakpointMarker>>();

const markerField = StateField.define<RangeSet<BreakpointMarker>>({
  create: () => RangeSet.empty,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setMarkers)) return e.value;
    return tr.docChanged ? value.map(tr.changes) : value;
  },
});

function markersFor(view: EditorView, path: string): RangeSet<BreakpointMarker> {
  const doc = view.state.doc;
  const v = verification.peek()[path] ?? {};
  const ranges = (lineBreakpoints.peek()[path] ?? []).filter((b) => b.line <= doc.lines).map((b) => new BreakpointMarker(b, v[b.line]).range(doc.line(b.line).from));
  return RangeSet.of(ranges, true);
}

/** The breakpoints as currently positioned in a view (after edits). */
function linesFromView(view: EditorView): LineBreakpoint[] {
  const out: LineBreakpoint[] = [];
  const iter = view.state.field(markerField).iter();
  for (; iter.value; iter.next()) out.push({ ...iter.value.bp, line: view.state.doc.lineAt(iter.from).number });
  return normalize(out);
}

function lineAtEvent(view: EditorView, e: MouseEvent): number {
  const block = view.lineBlockAtHeight(e.clientY - view.documentTop);
  return view.state.doc.lineAt(block.from).number;
}

function onGutterClick(path: string, line: number, e: MouseEvent): boolean {
  const existing = (lineBreakpoints.peek()[path] ?? []).find((b) => b.line === line);
  if (existing && (e.button === 2 || e.shiftKey)) {
    e.preventDefault();
    void editBreakpoint(path, line);
    return true;
  }
  if (e.button !== 0) return false;
  if (e.altKey && existing) {
    updateBreakpoint(path, line, { enabled: !existing.enabled });
    return true;
  }
  toggleBreakpoint(path, line);
  return true;
}

export function breakpointsExtension(buf: Buffer): Extension {
  if (!buf.path.endsWith('.go') || buf.binary) return [];
  const path = buf.path;
  let timer: ReturnType<typeof setTimeout> | null = null;
  return [
    markerField,
    gutter({
      class: 'cm-bp-gutter',
      markers: (v) => v.state.field(markerField),
      initialSpacer: () => new BreakpointMarker(newBreakpoint(1), undefined),
      domEventHandlers: {
        mousedown: (view, block, e) => onGutterClick(path, view.state.doc.lineAt(block.from).number, e as MouseEvent),
        contextmenu: (view, block, e) => {
          const line = view.state.doc.lineAt(block.from).number;
          if (!(lineBreakpoints.peek()[path] ?? []).some((b) => b.line === line)) return false;
          e.preventDefault();
          void editBreakpoint(path, line);
          return true;
        },
      },
    }),
    // Clicking a line number also toggles (as in most IDEs).
    EditorView.domEventHandlers({
      mousedown(e, view) {
        const t = e.target as HTMLElement;
        if (!t.closest('.cm-lineNumbers .cm-gutterElement')) return false;
        return onGutterClick(path, lineAtEvent(view, e), e);
      },
    }),
    ViewPlugin.define((view) => {
      queueMicrotask(() => view.dispatch({ effects: setMarkers.of(markersFor(view, path)) }));
      return {
        update(u) {
          if (!u.docChanged) return;
          if (timer) clearTimeout(timer);
          timer = setTimeout(() => {
            timer = null;
            const moved = linesFromView(view);
            const cur = lineBreakpoints.peek()[path] ?? [];
            if (JSON.stringify(moved) !== JSON.stringify(cur)) setFile(path, moved);
          }, 300);
        },
        destroy() {
          if (timer) clearTimeout(timer);
        },
      };
    }),
  ];
}

/** Pushes store changes into open editors. */
function refreshViews(): void {
  for (const buf of editors.buffers.values()) {
    if (!buf.path.endsWith('.go')) continue;
    for (const v of buf.views) {
      if (!v.state.field(markerField, false)) continue;
      v.dispatch({ effects: setMarkers.of(markersFor(v, buf.path)) });
    }
  }
}

// ---- Dialogs

export async function editBreakpoint(path: string, line: number): Promise<void> {
  const b = (lineBreakpoints.peek()[path] ?? []).find((x) => x.line === line);
  if (!b) return;
  const enabled = h('input', { type: 'checkbox', checked: b.enabled });
  const cond = h('input', { class: 'input', type: 'text', value: b.condition, placeholder: 'e.g. i == 2 && name != ""', spellcheck: false });
  const hit = h('input', { class: 'input', type: 'text', value: b.hitCondition, placeholder: 'e.g. 3, >= 5, % 2', spellcheck: false });
  const log = h('input', { class: 'input', type: 'text', value: b.logMessage, placeholder: 'x = {x} (logs instead of stopping)', spellcheck: false });
  const body = h(
    'div',
    { class: 'bp-form' },
    h('label', { class: 'rc-check' }, enabled, ' Enabled'),
    h('label', null, 'Condition'),
    cond,
    h('label', null, 'Hit count'),
    hit,
    h('label', null, 'Log message'),
    log,
    h('div', { class: 'rc-hint' }, 'A log message turns the breakpoint into a logpoint: it prints and does not suspend.'),
  );
  queueMicrotask(() => cond.focus());
  const res = await showDialog<'ok' | 'remove' | 'cancel'>({
    title: `Breakpoint at ${path.split(/[\\/]/).pop()}:${line}`,
    body,
    focus: cond,
    cancelValue: 'cancel',
    buttons: [
      { label: 'Remove', value: 'remove', danger: true },
      { label: 'Cancel', value: 'cancel' },
      { label: 'Done', value: 'ok', primary: true },
    ],
  });
  if (res === 'remove') updateBreakpoint(path, line, null);
  else if (res === 'ok') updateBreakpoint(path, line, { enabled: enabled.checked, condition: cond.value.trim(), hitCondition: hit.value.trim(), logMessage: log.value });
}

export async function viewBreakpoints(): Promise<void> {
  const list = h('div', { class: 'bpv-list' });
  const root = project.peek()?.root ?? '';
  const rel = (p: string): string => (root && p.startsWith(root) ? p.slice(root.length + 1) : p);
  const render = (): void => {
    const rows: Node[] = [];
    const panic = h('input', { type: 'checkbox', checked: breakOnPanic.peek() });
    panic.addEventListener('change', () => breakOnPanic.set(panic.checked));
    rows.push(h('div', { class: 'bpv-group' }, 'Go'), h('label', { class: 'bpv-row' }, panic, ' Break on panic (runtime.gopanic, includes recovered panics)'));
    rows.push(h('div', { class: 'bpv-help' }, 'Unrecovered panics and fatal errors always stop the debugger.'));

    rows.push(h('div', { class: 'bpv-group' }, 'Function breakpoints'));
    const fv = functionVerification.peek();
    functionBreakpoints.peek().forEach((f, i) => {
      const en = h('input', { type: 'checkbox', checked: f.enabled });
      en.addEventListener('change', () => functionBreakpoints.set(functionBreakpoints.peek().map((x, j) => (j === i ? { ...x, enabled: en.checked } : x))));
      const name = h('input', { class: 'input', type: 'text', value: f.name, placeholder: 'pkg.Func or (*T).Method', spellcheck: false });
      name.addEventListener('change', () => functionBreakpoints.set(functionBreakpoints.peek().map((x, j) => (j === i ? { ...x, name: name.value.trim() } : x))));
      const cond = h('input', { class: 'input', type: 'text', value: f.condition, placeholder: 'condition (optional)', spellcheck: false });
      cond.addEventListener('change', () => functionBreakpoints.set(functionBreakpoints.peek().map((x, j) => (j === i ? { ...x, condition: cond.value.trim() } : x))));
      const v = fv[functionBreakpoints.peek().filter((x, j) => j < i && x.enabled && x.name).length];
      rows.push(
        h(
          'div',
          { class: 'bpv-row' },
          en,
          name,
          cond,
          v && !v.verified && f.enabled ? h('span', { class: 'bpv-err', title: v.message }, '⚠') : null,
          iconButton('minus', 'Remove', () => {
            functionBreakpoints.set(functionBreakpoints.peek().filter((_, j) => j !== i));
            render();
          }),
        ),
      );
    });
    rows.push(
      h(
        'div',
        { class: 'bpv-row' },
        iconButton('plus', 'Add function breakpoint', () => {
          functionBreakpoints.set([...functionBreakpoints.peek(), { name: '', condition: '', enabled: true }]);
          render();
          (list.querySelectorAll<HTMLInputElement>('.bpv-row input[type=text]')[(functionBreakpoints.peek().length - 1) * 2] ?? null)?.focus();
        }),
        h('span', { class: 'rc-hint' }, 'Add function breakpoint'),
      ),
    );

    rows.push(h('div', { class: 'bpv-group' }, 'Line breakpoints'));
    const files = Object.entries(lineBreakpoints.peek()).sort(([a], [b]) => a.localeCompare(b));
    if (!files.length) rows.push(h('div', { class: 'bpv-help' }, 'Click the gutter of a Go file to add one (Mod+F8 at the cursor).'));
    for (const [path, bps] of files) {
      rows.push(h('div', { class: 'bpv-file' }, rel(path)));
      for (const b of bps) {
        const en = h('input', { type: 'checkbox', checked: b.enabled });
        en.addEventListener('change', () => updateBreakpoint(path, b.line, { enabled: en.checked }));
        const v = verification.peek()[path]?.[b.line];
        rows.push(
          h(
            'div',
            { class: 'bpv-row bpv-line' },
            en,
            h('span', { class: breakpointClass(b, v) }),
            h(
              'button',
              {
                class: 'bpv-link',
                type: 'button',
                onClick: () => void editors.open(path, { line: b.line, focus: true }),
              },
              describeBreakpoint(b, v),
            ),
            iconButton('settings', 'Edit', async () => {
              await editBreakpoint(path, b.line);
              render();
            }),
            iconButton('minus', 'Remove', () => {
              updateBreakpoint(path, b.line, null);
              render();
            }),
          ),
        );
      }
    }
    list.replaceChildren(...rows);
  };
  render();
  const action = await showDialog<'clear' | 'close'>({
    title: 'Breakpoints',
    body: h('div', { class: 'bpv' }, list),
    cancelValue: 'close',
    buttons: [
      { label: 'Remove All Line Breakpoints', value: 'clear', danger: true },
      { label: 'Close', value: 'close', primary: true },
    ],
  });
  if (action === 'clear') lineBreakpoints.set({});
}

// ---- Install

interface SavedBreakpoints {
  lines?: Record<string, LineBreakpoint[]>;
  functions?: FunctionBreakpoint[];
  breakOnPanic?: boolean;
}

export function installBreakpoints(wb: Workbench): void {
  editors.addExtension(breakpointsExtension);

  wb.layout.register<SavedBreakpoints>('breakpoints', {
    save: () => ({ lines: lineBreakpoints.peek(), functions: functionBreakpoints.peek(), breakOnPanic: breakOnPanic.peek() }),
    restore: (v) => {
      lineBreakpoints.set(v?.lines && typeof v.lines === 'object' ? v.lines : {});
      functionBreakpoints.set(Array.isArray(v?.functions) ? v.functions : []);
      breakOnPanic.set(!!v?.breakOnPanic);
    },
  });

  // Keep the backend and the editors in sync with the store.
  let sent: Record<string, string> = {};
  effect(() => {
    const p = project();
    const all = lineBreakpoints();
    if (!p) {
      sent = {};
      return;
    }
    const paths = new Set([...Object.keys(all), ...Object.keys(sent)]);
    for (const path of paths) {
      const json = JSON.stringify(all[path] ?? []);
      if (sent[path] === json) continue;
      sent[path] = json;
      void debug.setFileBreakpoints(path, all[path] ?? []).catch(() => {});
    }
    refreshViews();
    wb.layout.scheduleSave();
  });
  effect(() => {
    const p = project();
    const fns = functionBreakpoints();
    const panic = breakOnPanic();
    if (p) void debug.setFunctionBreakpoints(fns, panic).catch(() => {});
    wb.layout.scheduleSave();
  });
  effect(() => {
    verification();
    refreshViews();
  });

  on<DebugBreakpointsEvent>(Topics.debugBreakpoints, (ev) => {
    const toV = (b: { verified: boolean; message: string }): Verification => ({ verified: b.verified, message: b.message });
    if (ev.function) {
      functionVerification.set(ev.breakpoints.map(toV));
      return;
    }
    const enabled = (lineBreakpoints.peek()[ev.path] ?? []).filter((b) => b.enabled);
    const byLine: Record<number, Verification> = {};
    if (ev.breakpoints.length === enabled.length) enabled.forEach((b, i) => (byLine[b.line] = toV(ev.breakpoints[i])));
    else for (const b of ev.breakpoints) if (b.line) byLine[b.line] = toV(b);
    verification.set({ ...verification.peek(), [ev.path]: byLine });
  });
  on<DebugStateEvent>(Topics.debugState, (ev) => {
    if (ev.state === 'exited') {
      verification.set({});
      functionVerification.set([]);
    }
  });

  commands.register({
    id: 'debug.toggleBreakpoint',
    title: 'Toggle Line Breakpoint',
    category: 'Run',
    when: () => !!editors.activeBuffer()?.path.endsWith('.go'),
    run: () => {
      const buf = editors.activeBuffer();
      const view = editors.activeView();
      if (!buf || !view) return;
      toggleBreakpoint(buf.path, view.state.doc.lineAt(view.state.selection.main.head).number);
    },
  });
  commands.register({
    id: 'debug.editBreakpoint',
    title: 'Edit Breakpoint at Cursor',
    category: 'Run',
    when: () => !!editors.activeBuffer()?.path.endsWith('.go'),
    run: () => {
      const buf = editors.activeBuffer();
      const view = editors.activeView();
      if (!buf || !view) return;
      const line = view.state.doc.lineAt(view.state.selection.main.head).number;
      if (!(lineBreakpoints.peek()[buf.path] ?? []).some((b) => b.line === line)) toggleBreakpoint(buf.path, line);
      return editBreakpoint(buf.path, line);
    },
  });
  commands.register({ id: 'debug.viewBreakpoints', title: 'View Breakpoints…', category: 'Run', when: () => !!project.peek(), run: () => viewBreakpoints() });
  commands.register({
    id: 'debug.muteBreakpoints',
    title: 'Disable All Breakpoints',
    category: 'Run',
    when: () => !!project.peek(),
    run: () => {
      const all = lineBreakpoints.peek();
      const anyOn = Object.values(all).some((l) => l.some((b) => b.enabled));
      const next: Record<string, LineBreakpoint[]> = {};
      for (const [p, l] of Object.entries(all)) next[p] = l.map((b) => ({ ...b, enabled: !anyOn }));
      lineBreakpoints.set(next);
    },
  });
}
