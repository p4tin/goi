// Find / Replace in Path (R11). The backend searches with Go's regexp over
// the file index (honoring excludes); unsaved buffers are searched from
// their in-memory text. Replace edits open buffers in place (undoable) and
// rewrites the other files on disk.

import { confirmDialog, showDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { baseName, dirName } from '../editor/buffer';
import { editors } from '../editor/install';
import { search } from '../lib/api';
import type { FindQuery, TextMatch } from '../lib/api';
import { h } from '../lib/dom';
import { effect, signal } from '../lib/store';
import { relPath } from './diagnostics';

export interface FindState {
  query: FindQuery;
  matches: TextMatch[];
  truncated: boolean;
  replace: boolean;
}

export const findResult = signal<FindState | null>(null);

let lastQuery: FindQuery = { pattern: '', regex: false, caseSensitive: false, wholeWord: false, mask: '', replace: '', scope: '', buffers: [] };

export interface MatchFile {
  path: string;
  items: TextMatch[];
}

/** Matches grouped by file, in result order. */
export function groupMatches(matches: readonly TextMatch[]): MatchFile[] {
  const out: MatchFile[] = [];
  const idx = new Map<string, MatchFile>();
  for (const m of matches) {
    let f = idx.get(m.path);
    if (!f) {
      f = { path: m.path, items: [] };
      idx.set(m.path, f);
      out.push(f);
    }
    f.items.push(m);
  }
  return out;
}

/** The preview split around the match (offsets are UTF-16, like JS). */
export function previewParts(m: TextMatch): [string, string, string] {
  const a = Math.max(0, m.col - m.previewStart);
  const b = Math.max(a, m.end - m.previewStart);
  return [m.preview.slice(0, a).replace(/^\s+/, ''), m.preview.slice(a, b), m.preview.slice(b)];
}

/** Converts a Go replacement template ($1, ${1}, ${name}, $$) to JS. */
export function toJsReplacement(tmpl: string): string {
  return tmpl.replace(/\$(\$|\{(\w+)\}|(\d+))/g, (_m, all: string, braced?: string, num?: string) => {
    if (all === '$') return '$$';
    const name = braced ?? num ?? '';
    return /^\d+$/.test(name) ? `$${name}` : `$<${name}>`;
  });
}

/** Replacement text for one matched span. */
export function replacementFor(q: FindQuery, matched: string): string {
  if (!q.regex) return q.replace;
  const flags = q.caseSensitive ? 'u' : 'iu';
  try {
    const re = new RegExp(`^(?:${q.pattern})$`, flags);
    return matched.replace(re, toJsReplacement(q.replace));
  } catch {
    return q.replace;
  }
}

/** Applies replacements for matches (all in one file) to text. */
export function replaceInText(text: string, matches: readonly TextMatch[], q: FindQuery): string {
  const lines = text.split('\n');
  const byLine = new Map<number, TextMatch[]>();
  for (const m of matches) {
    if (!byLine.has(m.line)) byLine.set(m.line, []);
    byLine.get(m.line)!.push(m);
  }
  for (const [ln, ms] of byLine) {
    const i = ln - 1;
    if (i < 0 || i >= lines.length) continue;
    let line = lines[i];
    for (const m of [...ms].sort((a, b) => b.col - a.col)) {
      const matched = line.slice(m.col, m.end);
      line = line.slice(0, m.col) + replacementFor(q, matched) + line.slice(m.end);
    }
    lines[i] = line;
  }
  return lines.join('\n');
}

function dirtyBuffers(): { path: string; text: string }[] {
  return editors.dirtyBuffers().map((b) => ({ path: b.path, text: b.text() }));
}

async function run(wb: Workbench, q: FindQuery, replace: boolean): Promise<void> {
  lastQuery = { ...q, buffers: [] };
  try {
    const res = await search.findInPath({ ...q, buffers: dirtyBuffers() });
    findResult.set({ query: q, matches: res.matches, truncated: res.truncated, replace });
    wb.panels.show('find');
  } catch (err) {
    wb.notifications.push({ id: 'find', level: 'error', title: 'Find in Path failed', body: err instanceof Error ? err.message : String(err) });
  }
}

async function replaceAll(wb: Workbench, st: FindState): Promise<void> {
  const groups = groupMatches(st.matches);
  const ok = await confirmDialog(
    'Replace in Path',
    `Replace ${st.matches.length} occurrence${st.matches.length === 1 ? '' : 's'} in ${groups.length} file${groups.length === 1 ? '' : 's'}${st.truncated ? ' (results were truncated; only listed matches are replaced in open files, all matches on disk)' : ''}?`,
    'Replace',
    true,
  );
  if (!ok) return;
  const onDisk: string[] = [];
  for (const g of groups) {
    const buf = editors.buffers.get(g.path);
    const view = buf?.views.values().next().value;
    if (!buf || !view) {
      onDisk.push(g.path);
      continue;
    }
    const next = replaceInText(view.state.doc.toString(), g.items, st.query);
    if (next !== view.state.doc.toString()) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: next }, userEvent: 'input.replace' });
    }
  }
  try {
    if (onDisk.length) await search.replaceInPath(st.query, onDisk);
  } catch (err) {
    wb.notifications.push({ id: 'replace', level: 'error', title: 'Some files could not be changed', body: String(err) });
  }
  await run(wb, st.query, true);
}

function toggle(label: string, title: string, on: boolean): HTMLButtonElement {
  const b = h('button', { class: 'find-toggle', type: 'button', title, 'aria-pressed': String(on) }, label);
  b.addEventListener('click', () => b.setAttribute('aria-pressed', String(b.getAttribute('aria-pressed') !== 'true')));
  return b;
}

export async function openFindDialog(wb: Workbench, replace: boolean): Promise<void> {
  const view = editors.activeView();
  const buf = editors.activeBuffer();
  const sel = view ? view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to) : '';
  const initial = sel && !sel.includes('\n') ? sel : lastQuery.pattern;

  const find = h('input', { class: 'input', type: 'text', value: initial, spellcheck: false, 'aria-label': 'Find' });
  const repl = h('input', { class: 'input', type: 'text', value: lastQuery.replace, spellcheck: false, 'aria-label': 'Replace with' });
  const mask = h('input', { class: 'input', type: 'text', value: lastQuery.mask, placeholder: 'e.g. *.go, !*_test.go', spellcheck: false, 'aria-label': 'File mask' });
  const tCase = toggle('Aa', 'Match case', lastQuery.caseSensitive);
  const tWord = toggle('W', 'Words', lastQuery.wholeWord);
  const tRegex = toggle('.*', 'Regex', lastQuery.regex);
  const root = project.peek()?.root ?? '';
  const defaultDir = buf ? dirName(buf.path) : root;
  const dir = h('input', { class: 'input', type: 'text', value: lastQuery.scope || defaultDir, spellcheck: false, 'aria-label': 'Directory' });
  const scopeProject = h('input', { type: 'radio', name: 'find-scope', checked: !lastQuery.scope });
  const scopeDir = h('input', { type: 'radio', name: 'find-scope', checked: !!lastQuery.scope });
  const error = h('div', { class: 'dialog-error', role: 'alert' });
  const syncDir = (): void => {
    dir.disabled = !scopeDir.checked;
  };
  scopeProject.addEventListener('change', syncDir);
  scopeDir.addEventListener('change', syncDir);
  syncDir();

  const body = h(
    'div',
    { class: 'find-form' },
    h('label', null, 'Find'),
    find,
    h('div', { class: 'find-toggles' }, tCase, tWord, tRegex),
    replace ? h('label', null, 'Replace') : null,
    replace ? repl : null,
    replace ? h('span') : null,
    h('label', null, 'File mask'),
    mask,
    h('span'),
    h('label', null, 'Scope'),
    h('div', { class: 'find-scope' }, h('label', null, scopeProject, ' Project'), h('label', null, scopeDir, ' Directory')),
    h('span'),
    h('span'),
    dir,
    h('span'),
    error,
  );
  queueMicrotask(() => {
    find.focus();
    find.select();
  });
  for (;;) {
    const ok = await showDialog({
      title: replace ? 'Replace in Path' : 'Find in Path',
      body,
      focus: find,
      cancelValue: false,
      buttons: [
        { label: 'Cancel', value: false },
        { label: replace ? 'Find Matches' : 'Find', value: true, primary: true },
      ],
    });
    if (!ok) return;
    if (!find.value) {
      error.textContent = 'Enter text to find';
      continue;
    }
    const q: FindQuery = {
      pattern: find.value,
      regex: tRegex.getAttribute('aria-pressed') === 'true',
      caseSensitive: tCase.getAttribute('aria-pressed') === 'true',
      wholeWord: tWord.getAttribute('aria-pressed') === 'true',
      mask: mask.value.trim(),
      replace: replace ? repl.value : '',
      scope: scopeDir.checked ? dir.value.trim() : '',
      buffers: [],
    };
    if (q.regex) {
      try {
        new RegExp(q.pattern, 'u');
      } catch (err) {
        error.textContent = `Invalid regular expression: ${err instanceof Error ? err.message : String(err)}`;
        continue;
      }
    }
    await run(wb, q, replace);
    return;
  }
}

function renderFindPanel(wb: Workbench, container: HTMLElement): () => void {
  const head = h('div', { class: 'results-head' });
  const body = h('div', { class: 'results-body', role: 'tree' });
  container.append(h('div', { class: 'results' }, head, body));
  return effect(() => {
    const st = findResult();
    if (!st) {
      head.replaceChildren();
      body.replaceChildren(h('div', { class: 'results-empty' }, 'Use Find in Path (Mod+Shift+F) to search the project'));
      return;
    }
    const groups = groupMatches(st.matches);
    const headNodes: (HTMLElement | null)[] = [
      h('span', null, `“${st.query.pattern}”: ${st.matches.length} match${st.matches.length === 1 ? '' : 'es'} in ${groups.length} file${groups.length === 1 ? '' : 's'}`),
      st.truncated ? h('span', { class: 'sev sev-2' }, `(first ${st.matches.length} shown)`) : null,
      st.replace && st.matches.length ? h('button', { class: 'btn btn-primary', onClick: () => void replaceAll(wb, st) }, `Replace All with “${st.query.replace}”`) : null,
      h('button', { class: 'btn', onClick: () => void openFindDialog(wb, st.replace) }, 'Edit Search…'),
    ];
    head.replaceChildren(...headNodes.filter((n): n is HTMLElement => n !== null));
    if (!groups.length) {
      body.replaceChildren(h('div', { class: 'results-empty' }, 'No matches'));
      return;
    }
    const rows: Node[] = [];
    for (const g of groups) {
      const rel = relPath(g.path);
      rows.push(
        h(
          'div',
          { class: 'res-file' },
          h('span', null, baseName(g.path)),
          h('span', { class: 'dir' }, dirName(rel) === rel ? '' : dirName(rel)),
          h('span', { class: 'count' }, String(g.items.length)),
        ),
      );
      for (const m of g.items) {
        const [a, b, c] = previewParts(m);
        rows.push(
          h(
            'div',
            {
              class: 'res-item',
              onClick: async () => {
                await editors.open(m.path, { line: m.line, col: m.col + 1 });
                const v = editors.activeView();
                if (v && editors.activePath.peek() === m.path) {
                  const line = v.state.doc.line(Math.min(m.line, v.state.doc.lines));
                  v.dispatch({ selection: { anchor: line.from + m.col, head: Math.min(line.to, line.from + m.end) }, scrollIntoView: true });
                  v.focus();
                }
              },
            },
            h('span', { class: 'ln' }, String(m.line)),
            h('span', null, a, h('mark', null, b), c),
          ),
        );
      }
    }
    body.replaceChildren(...rows);
  });
}

export function installFindInPath(wb: Workbench): void {
  wb.panels.register({ id: 'find', title: 'Find', dock: 'bottom', order: 15, render: (c) => renderFindPanel(wb, c) });
  effect(() => {
    if (!project()) findResult.set(null);
  });
}
