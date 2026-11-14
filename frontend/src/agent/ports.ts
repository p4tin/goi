// Real ports for AgentTools: the editor manager, the backend file service,
// gopls, Find in Path, run configurations, the formatter and git.

import { EditorSelection } from '@codemirror/state';
import { project } from '../app/state';
import { editors } from '../editor/install';
import { editorApi, emptyRunConfig, files, gitApi, lang, on, run, search, Topics } from '../lib/api';
import type { Diagnostic, Location, OutputChunk, RunInfo, RunOutputEvent, RunTestEvent } from '../lib/api';
import { problems } from '../lang/diagnostics';
import type { DiagnosticItem, DiskPort, EditorPort, IdePort, LocationItem, RunOutcome, SymbolItem } from './tools';

const SYMBOL_KINDS: Record<number, string> = {
  1: 'file', 2: 'module', 3: 'namespace', 4: 'package', 5: 'class', 6: 'method', 7: 'property', 8: 'field', 9: 'constructor',
  10: 'enum', 11: 'interface', 12: 'function', 13: 'variable', 14: 'constant', 15: 'string', 16: 'number', 17: 'boolean',
  18: 'array', 19: 'object', 20: 'key', 21: 'null', 22: 'enum member', 23: 'struct', 24: 'event', 25: 'operator', 26: 'type parameter',
};

const SEVERITIES: DiagnosticItem['severity'][] = ['error', 'error', 'warning', 'info', 'hint'];

export function editorPort(): EditorPort {
  return {
    buffer(path) {
      const buf = editors.buffers.get(path);
      if (!buf) return null;
      return { path, text: () => buf.text(), dirty: buf.dirty.peek(), readOnly: buf.readOnly };
    },
    openEditors() {
      const active = editors.activePath.peek();
      const seen = new Set<string>();
      const out: { path: string; dirty: boolean; active: boolean }[] = [];
      for (const pane of editors.panes.peek()) {
        for (const t of pane.tabs.peek()) {
          if (seen.has(t.path)) continue;
          seen.add(t.path);
          out.push({ path: t.path, dirty: editors.buffers.get(t.path)?.dirty.peek() ?? false, active: t.path === active });
        }
      }
      return out;
    },
    active() {
      const path = editors.activePath.peek();
      const view = editors.activeView();
      if (!path || !view) return null;
      const sel = view.state.selection.main;
      const pos = (p: number): { line: number; column: number } => {
        const l = view.state.doc.lineAt(p);
        return { line: l.number, column: p - l.from + 1 };
      };
      const head = pos(sel.head);
      const from = pos(sel.from);
      const to = pos(sel.to);
      return {
        path,
        line: head.line,
        column: head.column,
        selection: sel.empty ? null : { startLine: from.line, startColumn: from.column, endLine: to.line, endColumn: to.column, text: view.state.sliceDoc(sel.from, sel.to) },
      };
    },
    async open(path, line, column) {
      return !!(await editors.open(path, { line, col: column }));
    },
    replace(path, content) {
      const buf = editors.buffers.get(path);
      const view = buf?.views.values().next().value;
      if (!buf || !view) return;
      // Keep the cursor near where it was.
      const head = Math.min(view.state.selection.main.head, content.length);
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: content }, selection: EditorSelection.cursor(head), userEvent: 'input.agent' });
    },
    save: (path) => editors.save(path),
  };
}

const missing = (err: unknown): boolean => /no such file|cannot find|not exist/i.test(String(err));

export function diskPort(): DiskPort {
  return {
    async read(path) {
      try {
        const f = await files.read(path);
        if (f.meta.binary) throw new Error(`${path} is a binary file`);
        return f.content;
      } catch (err) {
        if (missing(err)) return null;
        throw err;
      }
    },
    async write(path, content) {
      let meta;
      try {
        meta = (await files.read(path)).meta;
      } catch (err) {
        if (!missing(err)) throw err;
        await files.create(path, false, content);
        return;
      }
      await files.write(path, content, meta);
    },
    remove: (path) => files.trash(path),
  };
}

const locItem = (l: Location): LocationItem => ({ path: l.path, line: l.range.start.line + 1, column: l.range.start.character + 1 });

/** Runs until the run ends (or timeout), collecting output and test results. */
function awaitRun(start: () => Promise<string>, timeoutMs: number): Promise<RunOutcome> {
  return new Promise((resolve, reject) => {
    let id = '';
    let output = '';
    const tests = { passed: 0, failed: 0, skipped: 0, failures: [] as string[] };
    let isTest = false;
    const early: { kind: 'out' | 'state' | 'test'; payload: unknown }[] = [];
    let timer: ReturnType<typeof setTimeout> | undefined;
    const offs: (() => void)[] = [];
    const finish = (o: RunOutcome): void => {
      offs.forEach((f) => f());
      if (timer) clearTimeout(timer);
      resolve(o);
    };
    const handle = (kind: 'out' | 'state' | 'test', payload: unknown): void => {
      if (!id) {
        early.push({ kind, payload });
        return;
      }
      if (kind === 'out') {
        const e = payload as RunOutputEvent;
        if (e.id !== id) return;
        output += e.chunks.map((c: OutputChunk) => c.text).join('');
        if (output.length > 20000) output = output.slice(-16000);
      } else if (kind === 'test') {
        const e = payload as RunTestEvent;
        if (e.id !== id) return;
        isTest = true;
        for (const t of e.events) {
          if (!t.test) continue;
          if (t.action === 'pass') tests.passed++;
          else if (t.action === 'skip') tests.skipped++;
          else if (t.action === 'fail') {
            tests.failed++;
            if (tests.failures.length < 50) tests.failures.push(`${t.package} ${t.test}`);
          }
        }
      } else {
        const r = payload as RunInfo;
        if (r.id !== id || (r.state !== 'exited' && r.state !== 'failed')) return;
        finish({
          exitCode: r.exit ? r.exit.code : null,
          signal: r.exit?.signal || undefined,
          state: r.state + (r.message ? `: ${r.message}` : ''),
          output: output.slice(-8000),
          tests: isTest || r.test ? tests : undefined,
        });
      }
    };
    offs.push(on<RunOutputEvent>(Topics.runOutput, (e) => handle('out', e)));
    offs.push(on<RunTestEvent>(Topics.runTest, (e) => handle('test', e)));
    offs.push(on<RunInfo>(Topics.runState, (e) => handle('state', e)));
    timer = setTimeout(() => {
      if (id) void run.stop(id).catch(() => {});
      finish({ exitCode: null, state: 'timed out', output: output.slice(-8000), timedOut: true, tests: isTest ? tests : undefined });
    }, timeoutMs);
    start().then(
      (runId) => {
        id = runId;
        for (const e of early.splice(0)) handle(e.kind, e.payload);
      },
      (err: unknown) => {
        offs.forEach((f) => f());
        if (timer) clearTimeout(timer);
        reject(err instanceof Error ? err : new Error(String(err)));
      },
    );
  });
}

export function idePort(): IdePort {
  return {
    projectRoot: () => project.peek()?.root ?? '',
    diagnostics() {
      const out: DiagnosticItem[] = [];
      for (const [path, items] of problems.peek()) {
        for (const d of items as readonly Diagnostic[]) {
          out.push({
            path,
            line: d.range.start.line + 1,
            column: d.range.start.character + 1,
            severity: SEVERITIES[d.severity ?? 1] ?? 'error',
            source: d.source ?? '',
            message: d.message,
          });
        }
      }
      return out;
    },
    async symbols(query) {
      const res = await lang.workspaceSymbols(query);
      return res.map(
        (s): SymbolItem => ({ name: s.name, kind: SYMBOL_KINDS[s.kind] ?? String(s.kind), container: s.containerName, ...locItem(s.location) }),
      );
    },
    async definition(path, line, column) {
      return (await lang.definition(path, { line: line - 1, character: column - 1 })).map(locItem);
    },
    async references(path, line, column) {
      return (await lang.references(path, { line: line - 1, character: column - 1 })).map(locItem);
    },
    async search(q) {
      const buffers = [...editors.buffers.values()].filter((b) => b.dirty.peek()).map((b) => ({ path: b.path, text: b.text() }));
      const r = await search.findInPath({ pattern: q.pattern, regex: q.regex, caseSensitive: q.caseSensitive, wholeWord: false, mask: q.mask, replace: '', scope: '', buffers });
      return {
        matches: r.matches.slice(0, q.max).map((m) => ({ path: m.path, line: m.line, column: m.col + 1, preview: m.preview.trim().slice(0, 200) })),
        truncated: r.truncated || r.matches.length > q.max,
      };
    },
    async runConfigs() {
      return (await run.configs()).map((c) => ({ name: c.name, type: c.type }));
    },
    runConfig: (name, timeoutMs) => awaitRun(() => run.run(name, false), timeoutMs),
    runTests(dir, pattern, timeoutMs) {
      const c = emptyRunConfig('test', 'Agent: go test');
      c.kind = 'directory';
      c.directory = dir;
      c.pattern = pattern;
      c.temporary = true;
      return awaitRun(() => run.runConfig(c, '', false), timeoutMs);
    },
    async format(path, content) {
      const r = await editorApi.format(path, content);
      if (r.syntaxError) return { content, error: `${r.syntaxError.line}:${r.syntaxError.col}: ${r.syntaxError.msg}` };
      return { content: r.content };
    },
    async gitStatus() {
      const st = await gitApi.state();
      if (!st.repo) return { repo: false };
      const g = st.status;
      return {
        repo: true,
        branch: g.branch,
        upstream: g.upstream,
        ahead: g.ahead,
        behind: g.behind,
        operation: g.op,
        files: g.files.filter((f) => f.index !== '!' && f.work !== '!').map((f) => ({ path: f.rel, index: f.index, worktree: f.work, conflict: f.conflict })),
      };
    },
  };
}
