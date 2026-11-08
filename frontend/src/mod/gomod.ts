// go.mod / go.work editing (R12, R13): module path and version completion,
// Mod-click links to pkg.go.dev (or a local replacement's go.mod), a module
// actions bar above go.mod/go.work editors, and the module commands, which
// run as Run window tabs.

import type { CompletionContext, CompletionResult, CompletionSource } from '@codemirror/autocomplete';
import { RangeSetBuilder } from '@codemirror/state';
import type { Text } from '@codemirror/state';
import { Decoration, EditorView, ViewPlugin, showPanel } from '@codemirror/view';
import type { DecorationSet, Panel, ViewUpdate } from '@codemirror/view';
import { promptDialog } from '../app/dialog';
import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import type { Buffer } from '../editor/buffer';
import { baseName, dirName } from '../editor/buffer';
import { editors } from '../editor/install';
import { emptyRunConfig, modApi, openExternal, run } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { extraCompletionSources } from '../lang/completion';
import { treeMenuContributors } from '../views/projectTree';
import './gomod.css';

export type Block = '' | 'require' | 'replace' | 'exclude' | 'retract' | 'use' | 'tool' | 'godebug';

/** The directive block a line is in (for lines inside "require ( … )"). */
export function blockAt(doc: Text, lineNo: number): Block {
  for (let n = lineNo - 1; n >= 1; n--) {
    const t = doc.line(n).text.trim();
    if (t === ')') return '';
    const m = /^(require|replace|exclude|retract|use|tool|godebug)\s*\($/.exec(t);
    if (m) return m[1] as Block;
  }
  return '';
}

export interface ModuleRef {
  path: string;
  version: string;
  /** Character offsets within the line. */
  pathFrom: number;
  pathTo: number;
  /** A local replacement directory (replace ... => ./dir). */
  local?: string;
}

const MOD_PATH = /[A-Za-z0-9][A-Za-z0-9._~\-/]*(?:\.[A-Za-z0-9._~\-]+)[A-Za-z0-9._~\-/]*/;

/** Module references on a go.mod line (require, replace both sides,
 * exclude). */
export function moduleRefs(line: string, block: Block): ModuleRef[] {
  const code = line.replace(/\/\/.*$/, '');
  let rest = code;
  let offset = 0;
  const kw = /^\s*(require|replace|exclude|retract|use|module|go|toolchain|tool|godebug)\b\s*/.exec(code);
  let directive: string = block;
  if (kw) {
    directive = kw[1];
    offset = kw[0].length;
    rest = code.slice(offset);
    if (rest.trim().startsWith('(')) return [];
  }
  if (!['require', 'replace', 'exclude'].includes(directive)) return [];
  const sides = directive === 'replace' ? rest.split('=>') : [rest];
  const refs: ModuleRef[] = [];
  let at = offset;
  for (const side of sides) {
    const m = /^\s*(\S+)(?:\s+(\S+))?/.exec(side);
    if (m) {
      const tokenStart = at + side.indexOf(m[1]);
      const p = m[1];
      if (p.startsWith('.') || p.startsWith('/')) {
        refs.push({ path: p, version: '', pathFrom: tokenStart, pathTo: tokenStart + p.length, local: p });
      } else if (MOD_PATH.test(p)) {
        refs.push({ path: p, version: m[2] ?? '', pathFrom: tokenStart, pathTo: tokenStart + p.length });
      }
    }
    at += side.length + 2; // skip "=>"
  }
  return refs;
}

export type CompletionKind = { kind: 'path'; from: number; prefix: string } | { kind: 'version'; from: number; prefix: string; module: string } | null;

/** What to complete at column col of a go.mod line. */
export function completionAt(line: string, col: number, block: Block): CompletionKind {
  const before = line.slice(0, col);
  if (before.includes('//')) return null;
  const kw = /^\s*(require|replace|exclude)\s+/.exec(before);
  const directive = kw ? kw[1] : block;
  if (!['require', 'replace', 'exclude'].includes(directive)) return null;
  let segment = kw ? before.slice(kw[0].length) : before;
  let base = kw ? kw[0].length : 0;
  if (directive === 'replace' && segment.includes('=>')) {
    const i = segment.indexOf('=>');
    base += i + 2;
    segment = segment.slice(i + 2);
  }
  const lead = segment.length - segment.trimStart().length;
  const tokens = segment.trimStart().split(/\s+/);
  if (tokens.length === 1) {
    const prefix = tokens[0];
    if (prefix.startsWith('.') || prefix.startsWith('/')) return null;
    return { kind: 'path', from: base + lead, prefix };
  }
  if (tokens.length === 2) {
    const prefix = tokens[1];
    return { kind: 'version', from: col - prefix.length, prefix, module: tokens[0] };
  }
  return null;
}

/** pkg.go.dev URL for a module reference. */
export function docURL(ref: ModuleRef): string {
  return `https://pkg.go.dev/${ref.path}${ref.version ? `@${ref.version}` : ''}`;
}

function isModFile(path: string): boolean {
  const b = baseName(path);
  return b === 'go.mod' || b === 'go.work';
}

const modSource: CompletionSource = async (ctx: CompletionContext): Promise<CompletionResult | null> => {
  const line = ctx.state.doc.lineAt(ctx.pos);
  const what = completionAt(line.text, ctx.pos - line.from, blockAt(ctx.state.doc, line.number));
  if (!what) return null;
  if (what.kind === 'path') {
    if (!ctx.explicit && what.prefix.length < 2) return null;
    const mods = await modApi.modules(what.prefix).catch(() => [] as string[]);
    return { from: line.from + what.from, options: mods.map((m) => ({ label: m, type: 'namespace', detail: 'module' })), validFor: /^[\w.\-~/]*$/ };
  }
  const versions = await modApi.versions(what.module).catch(() => [] as string[]);
  return {
    from: line.from + what.from,
    options: versions.map((v, i) => ({ label: v, type: 'constant', detail: i === 0 ? 'latest' : '', boost: versions.length - i })),
    validFor: /^v?[\w.\-+]*$/,
    filter: true,
  };
};

const linkMark = Decoration.mark({ class: 'cm-gomod-link', attributes: { title: 'Mod+Click to open' } });

function linkDecorations(view: EditorView): DecorationSet {
  const b = new RangeSetBuilder<Decoration>();
  for (const { from, to } of view.visibleRanges) {
    for (let pos = from; pos <= to; ) {
      const line = view.state.doc.lineAt(pos);
      for (const r of moduleRefs(line.text, blockAt(view.state.doc, line.number))) b.add(line.from + r.pathFrom, line.from + r.pathTo, linkMark);
      pos = line.to + 1;
    }
  }
  return b.finish();
}

const links = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = linkDecorations(view);
    }
    update(u: ViewUpdate): void {
      if (u.docChanged || u.viewportChanged) this.decorations = linkDecorations(u.view);
    }
  },
  { decorations: (v) => v.decorations },
);

function openRef(path: string, ref: ModuleRef): void {
  if (ref.local) {
    const dir = /^([A-Za-z]:)?[\\/]/.test(ref.local) ? ref.local : `${dirName(path)}/${ref.local.replace(/^\.\//, '')}`;
    void editors.open(`${dir.replace(/\/$/, '')}/go.mod`, { focus: true });
    return;
  }
  openExternal(docURL(ref));
}

const clickLinks = (path: string) =>
  EditorView.domEventHandlers({
    mousedown(e, view) {
      if (!(e.metaKey || e.ctrlKey) || e.button !== 0) return false;
      const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
      if (pos === null) return false;
      const line = view.state.doc.lineAt(pos);
      const col = pos - line.from;
      const ref = moduleRefs(line.text, blockAt(view.state.doc, line.number)).find((r) => col >= r.pathFrom && col <= r.pathTo);
      if (!ref) return false;
      e.preventDefault();
      openRef(path, ref);
      return true;
    },
  });

/** Runs a go command in dir as a Run window tab. */
export async function runGo(title: string, script: string, dir: string): Promise<void> {
  const cfg = { ...emptyRunConfig('shell', title), scriptText: script, workingDir: dir };
  try {
    await run.runConfig(cfg, '', false);
  } catch (err) {
    const { notifyError } = await import('../run/state');
    notifyError('mod', `Cannot run ${script}`, err);
  }
}

/** The module directory for the active editor (or the project's first). */
export function currentModuleDir(): string {
  const p = project.peek();
  const active = editors.activeBuffer()?.path ?? '';
  if (p) {
    const mods = [...p.modules].sort((a, b) => b.dir.length - a.dir.length);
    const m = mods.find((x) => active === x.dir || active.startsWith(x.dir + '/') || active.startsWith(x.dir + '\\'));
    if (m) return m.dir;
    if (mods.length) return mods[mods.length - 1].dir;
    return p.root;
  }
  return '';
}

function actionsBar(path: string): Panel {
  const dir = dirName(path);
  const isWork = baseName(path) === 'go.work';
  const btn = (label: string, fn: () => void): HTMLElement => h('button', { class: 'gomod-btn', type: 'button', onClick: fn }, label);
  const dom = h(
    'div',
    { class: 'gomod-bar' },
    ...(isWork
      ? [btn('go work sync', () => void runGo('go work sync', 'go work sync', dir)), btn('go work use…', () => void commands.run('mod.workUse'))]
      : [
          btn('go mod tidy', () => void runGo('go mod tidy', 'go mod tidy', dir)),
          btn('Sync (go mod download)', () => void runGo('go mod download', 'go mod download', dir)),
          btn('go mod vendor', () => void runGo('go mod vendor', 'go mod vendor', dir)),
        ]),
  );
  return { dom, top: true };
}

export function gomodExtension(buf: Buffer) {
  if (!isModFile(buf.path)) return [];
  return [links, clickLinks(buf.path), showPanel.of(() => actionsBar(buf.path))];
}

export function installGoMod(_wb: Workbench): void {
  extraCompletionSources.push((buf) => (isModFile(buf.path) ? modSource : null));
  editors.addExtension(gomodExtension);

  const has = (): boolean => !!project.peek();
  const mod = (id: string, title: string, script: string): void => {
    commands.register({ id, title, category: 'Tools', when: has, run: () => runGo(script, script, currentModuleDir()) });
  };
  mod('mod.tidy', 'go mod tidy', 'go mod tidy');
  mod('mod.download', 'Sync Dependencies (go mod download)', 'go mod download');
  mod('mod.vendor', 'go mod vendor', 'go mod vendor');
  commands.register({ id: 'mod.workSync', title: 'go work sync', category: 'Tools', when: () => !!project.peek()?.modules.length, run: () => runGo('go work sync', 'go work sync', project.peek()?.root ?? '') });
  commands.register({
    id: 'mod.workUse',
    title: 'go work use…',
    category: 'Tools',
    when: has,
    run: async () => {
      const root = project.peek()?.root ?? '';
      const dir = await promptDialog('go work use', './', (v) => (v.trim() ? null : 'Enter a module directory'), 'Module directory (relative to the project root)');
      if (dir) await runGo(`go work use ${dir}`, `go work use ${quote(dir)}`, root);
    },
  });
  commands.register({
    id: 'mod.init',
    title: 'New go.mod (go mod init)…',
    category: 'Tools',
    when: has,
    run: async () => {
      const dir = editors.activeBuffer() ? dirName(editors.activeBuffer()!.path) : project.peek()?.root ?? '';
      const path = await promptDialog('New go.mod', '', (v) => (/^[\w.\-~/]+$/.test(v.trim()) ? null : 'Enter a module path, e.g. example.com/hello'), `Module path (in ${dir})`);
      if (path) await runGo(`go mod init ${path}`, `go mod init ${quote(path)}`, dir);
    },
  });
  commands.register({
    id: 'mod.workInit',
    title: 'New go.work (go work init)',
    category: 'Tools',
    when: has,
    run: () => {
      const p = project.peek();
      if (!p) return;
      const dirs = p.modules.map((m) => relTo(p.root, m.dir)).map(quote).join(' ');
      return runGo('go work init', `go work init ${dirs}`.trim(), p.root);
    },
  });
  treeMenuContributors.push((row) => {
    if (!has()) return [];
    const name = baseName(row.path);
    if (name === 'go.mod') {
      const dir = dirName(row.path);
      return [
        { label: 'go mod tidy', run: () => void runGo('go mod tidy', 'go mod tidy', dir) },
        { label: 'go mod download', run: () => void runGo('go mod download', 'go mod download', dir) },
        { label: 'go mod vendor', run: () => void runGo('go mod vendor', 'go mod vendor', dir) },
      ];
    }
    if (name === 'go.work') return [{ label: 'go work sync', run: () => void runGo('go work sync', 'go work sync', dirName(row.path)) }];
    return [];
  });
}

function quote(s: string): string {
  return /^[\w.\-~/@:+=]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`;
}

function relTo(root: string, dir: string): string {
  if (dir === root) return '.';
  return dir.startsWith(root + '/') || dir.startsWith(root + '\\') ? './' + dir.slice(root.length + 1).replace(/\\/g, '/') : dir;
}
