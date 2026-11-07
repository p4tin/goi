// Diagnostics (R7): gopls problems underlined in the editor, listed in the
// Problems tool window, and counted in the status bar.

import { lintGutter, setDiagnostics } from '@codemirror/lint';
import type { Diagnostic as CmDiagnostic } from '@codemirror/lint';
import type { Extension } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';
import { ViewPlugin } from '@codemirror/view';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { baseName, dirName } from '../editor/buffer';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { on, Topics } from '../lib/api';
import type { Diagnostic, DiagnosticsEvent, Severity } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { toOffsets } from './positions';

/** Diagnostics by path (files with none are absent). */
export const problems = signal<ReadonlyMap<string, readonly Diagnostic[]>>(new Map());

const CM_SEVERITY: Record<Severity, CmDiagnostic['severity']> = { 1: 'error', 2: 'warning', 3: 'info', 4: 'hint' };

export function cmSeverity(s: Severity | undefined): CmDiagnostic['severity'] {
  return CM_SEVERITY[s ?? 1] ?? 'error';
}

export function toCmDiagnostics(view: EditorView, items: readonly Diagnostic[]): CmDiagnostic[] {
  const doc = view.state.doc;
  return items.map((d) => {
    let { from, to } = toOffsets(doc, d.range);
    if (from === to && to < doc.length) to = Math.min(doc.lineAt(from).to, from + 1) || from;
    return {
      from,
      to: Math.max(from, to),
      severity: cmSeverity(d.severity),
      message: d.message,
      source: d.source || undefined,
      markClass: d.tags?.includes(1) ? 'cm-lang-unnecessary' : d.tags?.includes(2) ? 'cm-lang-deprecated' : undefined,
    };
  });
}

export interface FileProblems {
  path: string;
  items: Diagnostic[];
  errors: number;
  warnings: number;
}

const sev = (d: Diagnostic): number => d.severity ?? 1;

/** Files with problems: files with errors first, then by path; items by
 * severity then position. */
export function groupProblems(map: ReadonlyMap<string, readonly Diagnostic[]>): FileProblems[] {
  const out: FileProblems[] = [];
  for (const [path, items] of map) {
    if (!items.length) continue;
    const sorted = [...items].sort((a, b) => sev(a) - sev(b) || a.range.start.line - b.range.start.line || a.range.start.character - b.range.start.character);
    out.push({ path, items: sorted, errors: items.filter((d) => sev(d) === 1).length, warnings: items.filter((d) => sev(d) === 2).length });
  }
  return out.sort((a, b) => Number(b.errors > 0) - Number(a.errors > 0) || a.path.localeCompare(b.path));
}

export function countProblems(map: ReadonlyMap<string, readonly Diagnostic[]>): { errors: number; warnings: number; total: number } {
  let errors = 0;
  let warnings = 0;
  let total = 0;
  for (const items of map.values()) {
    for (const d of items) {
      total++;
      if (sev(d) === 1) errors++;
      else if (sev(d) === 2) warnings++;
    }
  }
  return { errors, warnings, total };
}

function push(view: EditorView, items: readonly Diagnostic[]): void {
  view.dispatch(setDiagnostics(view.state, toCmDiagnostics(view, items)));
}

/** Per-buffer extension: gutter markers plus initial diagnostics. */
export function diagnosticsExtension(buf: Buffer): Extension {
  return [
    lintGutter(),
    ViewPlugin.define((view) => {
      const items = problems.peek().get(buf.path);
      if (items?.length) queueMicrotask(() => push(view, items));
      return {};
    }),
  ];
}

export function relPath(path: string): string {
  const root = project.peek()?.root;
  if (root && path.startsWith(root) && path.length > root.length) return path.slice(root.length + 1);
  return path;
}

export function installDiagnostics(wb: Workbench): void {
  on<DiagnosticsEvent>(Topics.langDiagnostics, (e) => {
    problems.update((m) => {
      const next = new Map(m);
      if (e.items.length) next.set(e.path, e.items);
      else next.delete(e.path);
      return next;
    });
    const buf = editors.buffers.get(e.path);
    if (buf) for (const v of buf.views) push(v, e.items);
  });

  // Clear everything when the project closes.
  effect(() => {
    if (!project()) problems.set(new Map());
  });

  wb.panels.register({
    id: 'problems',
    title: 'Problems',
    dock: 'bottom',
    order: 20,
    render: (container) => renderProblemsPanel(container),
  });

  const st = wb.status.add({ id: 'problems', side: 'left', priority: 30, title: 'Problems', onClick: () => wb.panels.show('problems') });
  effect(() => {
    const c = countProblems(problems());
    st.replaceChildren(h('span', { class: 'sev sev-1' }, '✖'), ` ${c.errors}  `, h('span', { class: 'sev sev-2' }, '⚠'), ` ${c.warnings}`);
    st.title = `${c.errors} errors, ${c.warnings} warnings, ${c.total - c.errors - c.warnings} other`;
  });
}

const SEV_ICON: Record<number, string> = { 1: '✖', 2: '⚠', 3: 'ℹ', 4: '·' };

function renderProblemsPanel(container: HTMLElement): () => void {
  const head = h('div', { class: 'results-head' });
  const body = h('div', { class: 'results-body', role: 'tree' });
  container.append(h('div', { class: 'results' }, head, body));
  const collapsed = new Set<string>();
  const dispose = effect(() => {
    const groups = groupProblems(problems());
    const c = countProblems(problems());
    head.textContent = c.total ? `${c.errors} errors, ${c.warnings} warnings, ${c.total - c.errors - c.warnings} infos in ${groups.length} files` : '';
    if (!groups.length) {
      body.replaceChildren(h('div', { class: 'results-empty' }, 'No problems found'));
      return;
    }
    const rows: Node[] = [];
    for (const g of groups) {
      const rel = relPath(g.path);
      rows.push(
        h(
          'div',
          {
            class: 'res-file',
            onClick: () => {
              if (collapsed.has(g.path)) collapsed.delete(g.path);
              else collapsed.add(g.path);
              problems.update((m) => new Map(m));
            },
          },
          h('span', null, collapsed.has(g.path) ? '▸' : '▾'),
          h('span', null, baseName(g.path)),
          h('span', { class: 'dir' }, dirName(rel) === rel ? '' : dirName(rel)),
          h('span', { class: 'count' }, String(g.items.length)),
        ),
      );
      if (collapsed.has(g.path)) continue;
      for (const d of g.items) {
        rows.push(
          h(
            'div',
            {
              class: 'res-item',
              title: d.source ? `${d.message} (${d.source})` : d.message,
              onClick: () => void editors.open(g.path, { line: d.range.start.line + 1, col: d.range.start.character + 1 }),
            },
            h('span', { class: `sev sev-${sev(d)}` }, SEV_ICON[sev(d)] ?? '·'),
            h('span', { class: 'ln' }, `${d.range.start.line + 1}:${d.range.start.character + 1}`),
            h('span', null, d.message.split('\n')[0]),
            d.source ? h('span', { class: 'tag' }, d.source) : null,
          ),
        );
      }
    }
    body.replaceChildren(...rows);
  });
  return dispose;
}
