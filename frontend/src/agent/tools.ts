// The IDE side of agent requests: ACP fs/read_text_file and
// fs/write_text_file (editor-aware), and the MCP IDE tools whose schemas are
// in internal/rpc/agenttools.go (TOOL_NAMES must match). Everything goes
// through ports so tests can use fakes; ports.ts has the real ones.

import type { ChangeLog } from './changes';

export interface OpenBuffer {
  path: string;
  text(): string;
  dirty: boolean;
  readOnly: boolean;
}

export interface ActiveEditor {
  path: string;
  line: number;
  column: number;
  selection: { startLine: number; startColumn: number; endLine: number; endColumn: number; text: string } | null;
}

export interface EditorPort {
  buffer(path: string): OpenBuffer | null;
  openEditors(): { path: string; dirty: boolean; active: boolean }[];
  active(): ActiveEditor | null;
  /** Opens (or focuses) path at a 1-based line/column. */
  open(path: string, line?: number, column?: number): Promise<boolean>;
  /** Replaces the open buffer's text as one undoable change. */
  replace(path: string, content: string): void;
  save(path: string): Promise<boolean>;
}

export interface DiskPort {
  /** The file's text, or null when it doesn't exist. */
  read(path: string): Promise<string | null>;
  /** Writes (creating the file and its folders when missing). */
  write(path: string, content: string): Promise<void>;
  remove(path: string): Promise<void>;
}

export interface DiagnosticItem {
  path: string;
  line: number;
  column: number;
  severity: 'error' | 'warning' | 'info' | 'hint';
  source: string;
  message: string;
}

export interface SymbolItem {
  name: string;
  kind: string;
  container: string;
  path: string;
  line: number;
  column: number;
}

export interface LocationItem {
  path: string;
  line: number;
  column: number;
  preview?: string;
}

export interface RunOutcome {
  exitCode: number | null;
  signal?: string;
  state: string;
  output: string;
  /** Test summary when the run was a test run. */
  tests?: { passed: number; failed: number; skipped: number; failures: string[] };
  timedOut?: boolean;
}

export interface IdePort {
  projectRoot(): string;
  diagnostics(): DiagnosticItem[];
  symbols(query: string): Promise<SymbolItem[]>;
  definition(path: string, line: number, column: number): Promise<LocationItem[]>;
  references(path: string, line: number, column: number): Promise<LocationItem[]>;
  search(q: { pattern: string; regex: boolean; caseSensitive: boolean; mask: string; max: number }): Promise<{ matches: LocationItem[]; truncated: boolean }>;
  runConfigs(): Promise<{ name: string; type: string }[]>;
  runConfig(name: string, timeoutMs: number): Promise<RunOutcome>;
  runTests(dir: string, pattern: string, timeoutMs: number): Promise<RunOutcome>;
  /** Formatted text, or a syntax error message. */
  format(path: string, content: string): Promise<{ content: string; error?: string }>;
  gitStatus(): Promise<unknown>;
}

export interface ToolPorts {
  editor: EditorPort;
  disk: DiskPort;
  ide: IdePort;
  changes: ChangeLog;
}

/** A tool answer: plain text (with an error flag), or any JSON value. */
export type ToolAnswer = { text: string; isError?: boolean } | Record<string, unknown> | unknown[];

export const TOOL_NAMES = [
  'get_open_editors',
  'get_active_editor',
  'open_file',
  'read_file',
  'replace_text',
  'write_file',
  'get_diagnostics',
  'find_symbol',
  'find_references',
  'go_to_definition',
  'search_in_project',
  'list_run_configurations',
  'run_configuration',
  'run_tests',
  'format_file',
  'git_status',
] as const;

export type ToolName = (typeof TOOL_NAMES)[number];

type Args = Record<string, unknown>;

class ToolError extends Error {}

const s = (a: Args, k: string, required = true): string => {
  const v = a[k];
  if (typeof v === 'string' && (v !== '' || !required)) return v;
  if (v === undefined && !required) return '';
  throw new ToolError(`"${k}" must be a string`);
};
/** A required string that may be empty (file content, replacement text). */
const text = (a: Args, k: string): string => {
  if (typeof a[k] === 'string') return a[k] as string;
  throw new ToolError(`"${k}" must be a string`);
};
const n = (a: Args, k: string, required = true): number | undefined => {
  const v = a[k];
  if (typeof v === 'number' && Number.isFinite(v)) return Math.trunc(v);
  if (v === undefined && !required) return undefined;
  throw new ToolError(`"${k}" must be a number`);
};
const b = (a: Args, k: string): boolean => a[k] === true;

const SEVERITY_RANK: Record<DiagnosticItem['severity'], number> = { error: 0, warning: 1, info: 2, hint: 3 };

/** Lines [line, line+limit) of text; line is 1-based. */
export function sliceLines(text: string, line?: number | null, limit?: number | null): string {
  if (!line && !limit) return text;
  const lines = text.split('\n');
  const start = Math.max(0, (line ?? 1) - 1);
  const end = limit ? start + limit : lines.length;
  return lines.slice(start, end).join('\n');
}

function countOccurrences(text: string, needle: string): number {
  let count = 0;
  for (let i = text.indexOf(needle); i >= 0; i = text.indexOf(needle, i + needle.length)) count++;
  return count;
}

export class AgentTools {
  constructor(private p: ToolPorts) {}

  /** Current text of path: the open buffer, else the file on disk. */
  async text(path: string): Promise<string | null> {
    const buf = this.p.editor.buffer(path);
    return buf ? buf.text() : this.p.disk.read(path);
  }

  /** fs/read_text_file. */
  async read(params: { path: string; line?: number | null; limit?: number | null }): Promise<{ content: string }> {
    const t = await this.text(params.path);
    if (t === null) throw new ToolError(`${params.path} does not exist`);
    return { content: sliceLines(t, params.line, params.limit) };
  }

  /**
   * Writes content to path for session: through the open buffer (one
   * undoable change, then saved) or on disk. Records the first change of
   * the session for review.
   */
  async write(session: string, path: string, content: string): Promise<{ changed: boolean; created: boolean }> {
    const buf = this.p.editor.buffer(path);
    if (buf) {
      if (buf.readOnly) throw new ToolError(`${path} is read-only`);
      const before = buf.text();
      if (before === content) return { changed: false, created: false };
      this.p.changes.record(session, path, before);
      this.p.editor.replace(path, content);
      await this.p.editor.save(path);
      return { changed: true, created: false };
    }
    const before = await this.p.disk.read(path);
    if (before === content) return { changed: false, created: false };
    this.p.changes.record(session, path, before);
    await this.p.disk.write(path, content);
    return { changed: true, created: before === null };
  }

  /** Restores a file to its content before the session's changes. */
  async revert(session: string, path: string): Promise<void> {
    const c = this.p.changes.get(session, path);
    if (!c) return;
    if (c.before === null) {
      await this.p.disk.remove(path);
    } else if (this.p.editor.buffer(path)) {
      this.p.editor.replace(path, c.before);
      await this.p.editor.save(path);
    } else {
      await this.p.disk.write(path, c.before);
    }
    this.p.changes.forget(session, path);
  }

  /** Runs an MCP tool. Errors become {text, isError}. */
  async call(session: string, name: string, args: Args): Promise<ToolAnswer> {
    try {
      return await this.run(session, name as ToolName, args ?? {});
    } catch (err) {
      return { text: err instanceof Error ? err.message : String(err), isError: true };
    }
  }

  private async run(session: string, name: ToolName, a: Args): Promise<ToolAnswer> {
    const { editor, ide } = this.p;
    switch (name) {
      case 'get_open_editors':
        return { editors: editor.openEditors() };
      case 'get_active_editor':
        return { ...(editor.active() ?? { text: 'No editor is active.' }) };
      case 'open_file': {
        const path = s(a, 'path');
        const ok = await editor.open(path, n(a, 'line', false), n(a, 'column', false));
        if (!ok) throw new ToolError(`Cannot open ${path}`);
        return { text: `Opened ${path}` };
      }
      case 'read_file': {
        const path = s(a, 'path');
        const t = await this.text(path);
        if (t === null) throw new ToolError(`${path} does not exist`);
        const start = n(a, 'startLine', false);
        const end = n(a, 'endLine', false);
        if (!start && !end) return { text: t };
        const from = start ?? 1;
        return { text: sliceLines(t, from, end ? end - from + 1 : undefined) };
      }
      case 'replace_text': {
        const path = s(a, 'path');
        const oldText = s(a, 'oldText');
        const newText = text(a, 'newText');
        const t = await this.text(path);
        if (t === null) throw new ToolError(`${path} does not exist`);
        const count = countOccurrences(t, oldText);
        if (count === 0) throw new ToolError('oldText was not found in the file (it must match exactly, including whitespace)');
        if (count > 1 && !b(a, 'replaceAll')) throw new ToolError(`oldText occurs ${count} times; add surrounding lines to make it unique, or set replaceAll`);
        const next = b(a, 'replaceAll') ? t.split(oldText).join(newText) : t.replace(oldText, () => newText);
        await this.write(session, path, next);
        return { text: `Replaced ${b(a, 'replaceAll') ? count : 1} occurrence${count > 1 && b(a, 'replaceAll') ? 's' : ''} in ${path}` };
      }
      case 'write_file': {
        const path = s(a, 'path');
        const r = await this.write(session, path, text(a, 'content'));
        return { text: r.created ? `Created ${path}` : r.changed ? `Wrote ${path}` : `${path} already had this content` };
      }
      case 'get_diagnostics': {
        const path = s(a, 'path', false);
        const min = (s(a, 'severity', false) || 'warning') as 'error' | 'warning' | 'all';
        const limit = min === 'error' ? 0 : min === 'warning' ? 1 : 3;
        const items = ide
          .diagnostics()
          .filter((d) => (!path || d.path === path) && SEVERITY_RANK[d.severity] <= limit)
          .sort((x, y) => SEVERITY_RANK[x.severity] - SEVERITY_RANK[y.severity] || x.path.localeCompare(y.path) || x.line - y.line);
        return { count: items.length, diagnostics: items.slice(0, 200), truncated: items.length > 200 };
      }
      case 'find_symbol':
        return { symbols: (await ide.symbols(s(a, 'query'))).slice(0, 50) };
      case 'find_references': {
        const refs = await ide.references(s(a, 'path'), n(a, 'line')!, n(a, 'column')!);
        return { count: refs.length, references: refs.slice(0, 200) };
      }
      case 'go_to_definition':
        return { definitions: await ide.definition(s(a, 'path'), n(a, 'line')!, n(a, 'column')!) };
      case 'search_in_project': {
        const r = await ide.search({
          pattern: s(a, 'pattern'),
          regex: b(a, 'regex'),
          caseSensitive: b(a, 'caseSensitive'),
          mask: s(a, 'fileMask', false),
          max: Math.min(Math.max(n(a, 'maxResults', false) ?? 100, 1), 1000),
        });
        return { count: r.matches.length, truncated: r.truncated, matches: r.matches };
      }
      case 'list_run_configurations':
        return { configurations: await ide.runConfigs() };
      case 'run_configuration': {
        const timeout = (n(a, 'timeoutSeconds', false) ?? 300) * 1000;
        return { ...(await ide.runConfig(s(a, 'name'), timeout)) };
      }
      case 'run_tests': {
        const root = ide.projectRoot();
        let pkg = (s(a, 'package', false) || './...').replace(/\\/g, '/');
        pkg = pkg.replace(/\/\.\.\.$/, '').replace(/^\.\.\.$/, '.') || '.';
        if (pkg.startsWith('/') || /^[A-Za-z]:/.test(pkg)) throw new ToolError('package must be relative to the project root, like ./internal/foo');
        const dir = pkg === '.' || pkg === './' ? root : `${root}/${pkg.replace(/^\.\//, '')}`;
        const timeout = (n(a, 'timeoutSeconds', false) ?? 300) * 1000;
        return { ...(await ide.runTests(dir, s(a, 'test', false), timeout)) };
      }
      case 'format_file': {
        const path = s(a, 'path');
        const t = await this.text(path);
        if (t === null) throw new ToolError(`${path} does not exist`);
        const r = await ide.format(path, t);
        if (r.error) throw new ToolError(`Not formatted: ${r.error}`);
        const w = await this.write(session, path, r.content);
        return { text: w.changed ? `Formatted ${path}` : `${path} was already formatted` };
      }
      case 'git_status':
        return { status: await ide.gitStatus() };
    }
    throw new ToolError(`Unknown tool ${String(name)}`);
  }
}
