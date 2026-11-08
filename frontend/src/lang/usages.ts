// Find Usages (R5.1–2) and highlight usages in file (R5.3).

import { StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view';
import type { DecorationSet } from '@codemirror/view';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { baseName, dirName } from '../editor/buffer';
import type { Buffer } from '../editor/buffer';
import { editors } from '../editor/install';
import { files, lang } from '../lib/api';
import type { Location } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { activeTarget, client, fresh, info, warn } from './context';
import { relPath } from './diagnostics';
import { openAt } from './navigation';
import { toOffsets, toPos, wordAt } from './positions';

export const HIGHLIGHT_DELAY_MS = 250;

export interface Usage {
  path: string;
  range: Location['range'];
  /** The full line text. */
  text: string;
  write: boolean;
  test: boolean;
}

const ASSIGN_AFTER = /^\s*(?:\[[^\]]*\]\s*)*(?:(?:\+|-|\*|\/|%|&\^|&|\||\^|<<|>>)?=(?!=)|:=|\+\+|--)/;
const MULTI_ASSIGN_AFTER = /^\s*(?:,\s*[\w.[\]*]+\s*)+(?::=|=(?!=))/;

/** Whether the identifier ending at endCol on line is assigned to. */
export function isWriteUsage(line: string, endCol: number): boolean {
  const rest = line.slice(endCol);
  if (/^\s*\./.test(rest)) return false; // x.f = v writes the field, not x
  return ASSIGN_AFTER.test(rest) || MULTI_ASSIGN_AFTER.test(rest);
}

export interface FileGroup {
  path: string;
  items: Usage[];
}
export interface PackageGroup {
  dir: string;
  files: FileGroup[];
  count: number;
}
export interface UsageGroup {
  kind: 'prod' | 'test';
  label: string;
  packages: PackageGroup[];
  count: number;
}

/** Production / test › package directory › file, sorted. */
export function groupUsages(items: readonly Usage[]): UsageGroup[] {
  const out: UsageGroup[] = [];
  for (const kind of ['prod', 'test'] as const) {
    const mine = items.filter((u) => (kind === 'test') === u.test);
    if (!mine.length) continue;
    const byDir = new Map<string, Map<string, Usage[]>>();
    for (const u of mine) {
      const dir = dirName(u.path);
      if (!byDir.has(dir)) byDir.set(dir, new Map());
      const files = byDir.get(dir)!;
      if (!files.has(u.path)) files.set(u.path, []);
      files.get(u.path)!.push(u);
    }
    const packages: PackageGroup[] = [...byDir.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([dir, fm]) => {
        const fileGroups = [...fm.entries()]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([path, us]) => ({ path, items: us.sort((a, b) => a.range.start.line - b.range.start.line || a.range.start.character - b.range.start.character) }));
        return { dir, files: fileGroups, count: fileGroups.reduce((n, f) => n + f.items.length, 0) };
      });
    out.push({ kind, label: kind === 'prod' ? 'Production' : 'Test', packages, count: mine.length });
  }
  return out;
}

async function linesOf(path: string): Promise<string[]> {
  const buf = editors.buffers.get(path);
  if (buf) return buf.text().split('\n');
  try {
    return (await files.read(path)).content.split('\n');
  } catch {
    return [];
  }
}

export async function toUsages(locs: readonly Location[]): Promise<Usage[]> {
  const paths = [...new Set(locs.map((l) => l.path))];
  const texts = new Map<string, string[]>();
  await Promise.all(paths.map(async (p) => texts.set(p, await linesOf(p))));
  return locs.map((l) => {
    const text = texts.get(l.path)?.[l.range.start.line] ?? '';
    return {
      path: l.path,
      range: l.range,
      text,
      write: l.range.start.line === l.range.end.line && isWriteUsage(text, l.range.end.character),
      test: l.path.endsWith('_test.go'),
    };
  });
}

interface UsageResult {
  name: string;
  items: Usage[];
}

export const usageResult = signal<UsageResult | null>(null);

export async function findUsages(wb: Workbench): Promise<void> {
  const t = await activeTarget();
  if (!t) return;
  const name = wordAt(t.view.state.doc, t.offset)?.text ?? 'symbol';
  const version = t.buf.version;
  let locs: Location[];
  try {
    locs = await lang.references(t.buf.path, t.pos);
  } catch (err) {
    warn('lang.usages', 'Cannot find usages', String(err));
    return;
  }
  if (!fresh(t.buf, version)) return;
  if (locs.length === 0) {
    info('lang.usages', `No usages of ${name} found`);
    return;
  }
  usageResult.set({ name, items: await toUsages(locs) });
  wb.panels.show('usages');
}

function highlightLine(u: Usage): Node[] {
  const text = u.text;
  const a = Math.min(u.range.start.character, text.length);
  const b = u.range.start.line === u.range.end.line ? Math.min(u.range.end.character, text.length) : text.length;
  const lead = text.slice(0, a).replace(/^\s+/, '');
  return [document.createTextNode(lead), h('mark', null, text.slice(a, b)), document.createTextNode(text.slice(b))];
}

function renderUsagesPanel(container: HTMLElement): () => void {
  const head = h('div', { class: 'results-head' });
  const body = h('div', { class: 'results-body', role: 'tree' });
  container.append(h('div', { class: 'results' }, head, body));
  const collapsed = new Set<string>();
  const rerender = signal(0);
  return effect(() => {
    rerender();
    const res = usageResult();
    if (!res) {
      head.textContent = '';
      body.replaceChildren(h('div', { class: 'results-empty' }, 'Use Find Usages (Alt+F7) on a symbol to list its usages here'));
      return;
    }
    const writes = res.items.filter((u) => u.write).length;
    head.textContent = `Usages of ${res.name}: ${res.items.length} (${writes} write${writes === 1 ? '' : 's'})`;
    const toggle = (key: string) => () => {
      if (collapsed.has(key)) collapsed.delete(key);
      else collapsed.add(key);
      rerender.update((n) => n + 1);
    };
    const rows: Node[] = [];
    const root = project.peek()?.root ?? '';
    for (const g of groupUsages(res.items)) {
      const gk = g.kind;
      rows.push(h('div', { class: 'res-group', onClick: toggle(gk) }, collapsed.has(gk) ? '▸' : '▾', g.label, h('span', { class: 'count' }, String(g.count))));
      if (collapsed.has(gk)) continue;
      for (const pkg of g.packages) {
        const pk = `${gk}:${pkg.dir}`;
        const rel = root && pkg.dir.startsWith(root) ? pkg.dir.slice(root.length + 1) || '.' : pkg.dir;
        rows.push(
          h('div', { class: 'res-file', onClick: toggle(pk) }, collapsed.has(pk) ? '▸' : '▾', h('span', null, rel), h('span', { class: 'count' }, String(pkg.count))),
        );
        if (collapsed.has(pk)) continue;
        for (const f of pkg.files) {
          for (const u of f.items) {
            rows.push(
              h(
                'div',
                { class: 'res-item', title: relPath(u.path), onClick: () => void openAt(u.path, u.range) },
                h('span', { class: 'ln' }, `${baseName(f.path)}:${u.range.start.line + 1}`),
                h('span', null, highlightLine(u)),
                u.write ? h('span', { class: 'tag' }, 'write') : null,
              ),
            );
          }
        }
      }
    }
    body.replaceChildren(...rows);
  });
}

// ---- Highlight usages in the current file

const setHighlights = StateEffect.define<DecorationSet>();
const highlightField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) if (e.is(setHighlights)) return e.value;
    return tr.docChanged ? deco.map(tr.changes) : deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

const readMark = Decoration.mark({ class: 'cm-lang-highlight' });
const writeMark = Decoration.mark({ class: 'cm-lang-highlight-write' });

export function highlightExtension(buf: Buffer): Extension {
  return [
    highlightField,
    ViewPlugin.define((view) => {
      let timer: ReturnType<typeof setTimeout> | null = null;
      let seq = 0;
      const clear = (): void => {
        if (view.state.field(highlightField).size) view.dispatch({ effects: setHighlights.of(Decoration.none) });
      };
      const schedule = (): void => {
        if (timer) clearTimeout(timer);
        timer = setTimeout(async () => {
          timer = null;
          const mine = ++seq;
          const head = view.state.selection.main.head;
          if (!client.live() || !view.state.selection.main.empty || !wordAt(view.state.doc, head)) {
            clear();
            return;
          }
          const version = await client.flush(buf);
          let hs;
          try {
            hs = await lang.documentHighlight(buf.path, toPos(view.state.doc, head));
          } catch {
            return;
          }
          if (mine !== seq || buf.version !== version || view.state.selection.main.head !== head) return;
          const marks = hs
            .map((x) => ({ ...toOffsets(view.state.doc, x.range), write: x.kind === 3 }))
            .filter((x) => x.to > x.from)
            .sort((a, b) => a.from - b.from)
            .map((x) => (x.write ? writeMark : readMark).range(x.from, x.to));
          view.dispatch({ effects: setHighlights.of(marks.length > 1 ? Decoration.set(marks) : Decoration.none) });
        }, HIGHLIGHT_DELAY_MS);
      };
      return {
        update(u) {
          if (u.selectionSet || u.docChanged) schedule();
        },
        destroy() {
          if (timer) clearTimeout(timer);
        },
      };
    }),
  ];
}

export function installUsages(wb: Workbench): void {
  wb.panels.register({ id: 'usages', title: 'Usages', dock: 'bottom', order: 25, render: (c) => renderUsagesPanel(c) });
  effect(() => {
    if (!project()) usageResult.set(null);
  });
}
