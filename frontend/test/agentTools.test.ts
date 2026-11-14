import { describe, expect, it } from 'vitest';
import { ChangeLog } from '../src/agent/changes';
import { AgentTools, sliceLines, TOOL_NAMES } from '../src/agent/tools';
import type { DiagnosticItem, EditorPort, IdePort, RunOutcome } from '../src/agent/tools';

class FakeWorld {
  disk = new Map<string, string>();
  buffers = new Map<string, { text: string; readOnly: boolean; saves: number }>();
  opened: string[] = [];
  removed: string[] = [];
  diags: DiagnosticItem[] = [];
  runs: string[] = [];

  editor(): EditorPort {
    return {
      buffer: (path) => {
        const b = this.buffers.get(path);
        return b ? { path, text: () => b.text, dirty: false, readOnly: b.readOnly } : null;
      },
      openEditors: () => [...this.buffers.keys()].map((path, i) => ({ path, dirty: false, active: i === 0 })),
      active: () => ({ path: '/p/a.go', line: 2, column: 3, selection: null }),
      open: async (path, line) => {
        this.opened.push(`${path}:${line ?? ''}`);
        return this.disk.has(path) || this.buffers.has(path);
      },
      replace: (path, content) => {
        this.buffers.get(path)!.text = content;
      },
      save: async (path) => {
        const b = this.buffers.get(path)!;
        b.saves++;
        this.disk.set(path, b.text);
        return true;
      },
    };
  }

  ports() {
    const ide: IdePort = {
      projectRoot: () => '/p',
      diagnostics: () => this.diags,
      symbols: async (q) => [{ name: q, kind: 'function', container: 'main', path: '/p/a.go', line: 1, column: 6 }],
      definition: async (path, line, column) => [{ path, line: line + 1, column }],
      references: async () => [{ path: '/p/b.go', line: 3, column: 1 }],
      search: async (q) => ({ matches: [{ path: '/p/a.go', line: 1, column: 1, preview: q.pattern }], truncated: false }),
      runConfigs: async () => [{ name: 'App', type: 'build' }],
      runConfig: async (name): Promise<RunOutcome> => {
        this.runs.push(name);
        return { exitCode: 0, state: 'exited', output: 'ok' };
      },
      runTests: async (dir, pattern): Promise<RunOutcome> => {
        this.runs.push(`test ${dir} ${pattern}`);
        return { exitCode: 1, state: 'exited', output: '', tests: { passed: 1, failed: 1, skipped: 0, failures: ['m TestX'] } };
      },
      format: async (_path, content) => (content.includes('syntax') ? { content, error: '1:1: bad' } : { content: content.replace(/  +/g, ' ') }),
      gitStatus: async () => ({ branch: 'main' }),
    };
    return {
      editor: this.editor(),
      disk: {
        read: async (path: string) => this.disk.get(path) ?? null,
        write: async (path: string, content: string) => void this.disk.set(path, content),
        remove: async (path: string) => {
          this.removed.push(path);
          this.disk.delete(path);
        },
      },
      ide,
      changes: new ChangeLog(),
    };
  }
}

function setup() {
  const w = new FakeWorld();
  w.disk.set('/p/a.go', 'package a\n\nfunc A() {}\n');
  w.disk.set('/p/b.go', 'package b // saved');
  w.buffers.set('/p/b.go', { text: 'package b // unsaved', readOnly: false, saves: 0 });
  const ports = w.ports();
  return { w, ports, tools: new AgentTools(ports) };
}

describe('agent fs requests', () => {
  it('reads open buffers (unsaved text) and files on disk, with line ranges', async () => {
    const { tools } = setup();
    expect(await tools.read({ path: '/p/b.go' })).toEqual({ content: 'package b // unsaved' });
    expect(await tools.read({ path: '/p/a.go', line: 3, limit: 1 })).toEqual({ content: 'func A() {}' });
    await expect(tools.read({ path: '/p/none.go' })).rejects.toThrow('does not exist');
    expect(sliceLines('a\nb\nc', 2)).toBe('b\nc');
  });

  it('writes through the buffer (saved) or disk and records changes once per file', async () => {
    const { w, ports, tools } = setup();
    expect(await tools.write('s1', '/p/b.go', 'package b // agent')).toEqual({ changed: true, created: false });
    expect(w.buffers.get('/p/b.go')!.saves).toBe(1);
    expect(w.disk.get('/p/b.go')).toBe('package b // agent');
    await tools.write('s1', '/p/b.go', 'package b // agent 2');
    expect(await tools.write('s1', '/p/new.go', 'package n')).toEqual({ changed: true, created: true });
    expect(await tools.write('s1', '/p/new.go', 'package n')).toEqual({ changed: false, created: false });
    expect(ports.changes.list('s1')).toEqual([
      { path: '/p/b.go', before: 'package b // unsaved', writes: 2 },
      { path: '/p/new.go', before: null, writes: 1 },
    ]);
    w.buffers.get('/p/b.go')!.readOnly = true;
    await expect(tools.write('s1', '/p/b.go', 'x')).rejects.toThrow('read-only');
  });

  it('reverts changes: restores content or removes created files', async () => {
    const { w, ports, tools } = setup();
    await tools.write('s1', '/p/a.go', 'changed');
    await tools.write('s1', '/p/b.go', 'changed');
    await tools.write('s1', '/p/c.go', 'created');
    await tools.revert('s1', '/p/a.go');
    await tools.revert('s1', '/p/b.go');
    await tools.revert('s1', '/p/c.go');
    expect(w.disk.get('/p/a.go')).toBe('package a\n\nfunc A() {}\n');
    expect(w.buffers.get('/p/b.go')!.text).toBe('package b // unsaved');
    expect(w.removed).toEqual(['/p/c.go']);
    expect(ports.changes.list('s1')).toEqual([]);
  });
});

describe('MCP IDE tools', () => {
  it('implements every tool the backend lists', () => {
    expect(new Set(TOOL_NAMES).size).toBe(TOOL_NAMES.length);
  });

  it('edits with replace_text and write_file', async () => {
    const { w, ports, tools } = setup();
    expect(await tools.call('s2', 'replace_text', { path: '/p/a.go', oldText: 'func A() {}', newText: 'func B() {}' })).toEqual({ text: 'Replaced 1 occurrence in /p/a.go' });
    expect(w.disk.get('/p/a.go')).toContain('func B() {}');
    expect(await tools.call('s2', 'replace_text', { path: '/p/a.go', oldText: 'nope', newText: 'x' })).toMatchObject({ isError: true });
    w.disk.set('/p/d.go', 'x x x');
    expect(await tools.call('s2', 'replace_text', { path: '/p/d.go', oldText: 'x', newText: 'y' })).toMatchObject({ isError: true, text: expect.stringContaining('3 times') });
    expect(await tools.call('s2', 'replace_text', { path: '/p/d.go', oldText: 'x', newText: '$&y', replaceAll: true })).toEqual({ text: 'Replaced 3 occurrences in /p/d.go' });
    expect(w.disk.get('/p/d.go')).toBe('$&y $&y $&y');
    expect(await tools.call('s2', 'write_file', { path: '/p/e.go', content: 'package e' })).toEqual({ text: 'Created /p/e.go' });
    expect(ports.changes.list('s2').map((c) => c.path)).toEqual(['/p/a.go', '/p/d.go', '/p/e.go']);
    expect(await tools.call('s2', 'write_file', { path: '/p/e.go' })).toMatchObject({ isError: true });
  });

  it('reads, navigates and reports diagnostics', async () => {
    const { w, tools } = setup();
    expect(await tools.call('', 'read_file', { path: '/p/a.go', startLine: 3, endLine: 3 })).toEqual({ text: 'func A() {}' });
    expect(await tools.call('', 'open_file', { path: '/p/a.go', line: 3 })).toEqual({ text: 'Opened /p/a.go' });
    expect(await tools.call('', 'open_file', { path: '/p/zz.go' })).toMatchObject({ isError: true });
    expect(w.opened).toEqual(['/p/a.go:3', '/p/zz.go:']);
    expect(await tools.call('', 'get_active_editor', {})).toMatchObject({ path: '/p/a.go', line: 2 });
    expect(await tools.call('', 'get_open_editors', {})).toEqual({ editors: [{ path: '/p/b.go', dirty: false, active: true }] });
    w.diags = [
      { path: '/p/a.go', line: 2, column: 1, severity: 'warning', source: 'lint', message: 'w' },
      { path: '/p/b.go', line: 1, column: 1, severity: 'error', source: 'compiler', message: 'e' },
      { path: '/p/a.go', line: 1, column: 1, severity: 'hint', source: 'gopls', message: 'h' },
    ];
    expect(await tools.call('', 'get_diagnostics', {})).toMatchObject({ count: 2, diagnostics: [{ message: 'e' }, { message: 'w' }] });
    expect(await tools.call('', 'get_diagnostics', { severity: 'error' })).toMatchObject({ count: 1 });
    expect(await tools.call('', 'get_diagnostics', { path: '/p/a.go', severity: 'all' })).toMatchObject({ count: 2 });
    expect(await tools.call('', 'find_symbol', { query: 'Main' })).toMatchObject({ symbols: [{ name: 'Main' }] });
    expect(await tools.call('', 'go_to_definition', { path: '/p/a.go', line: 1, column: 2 })).toEqual({ definitions: [{ path: '/p/a.go', line: 2, column: 2 }] });
    expect(await tools.call('', 'find_references', { path: '/p/a.go', line: 'x', column: 2 })).toMatchObject({ isError: true, text: '"line" must be a number' });
    expect(await tools.call('', 'search_in_project', { pattern: 'func' })).toMatchObject({ count: 1, matches: [{ preview: 'func' }] });
    expect(await tools.call('', 'git_status', {})).toEqual({ status: { branch: 'main' } });
    expect(await tools.call('', 'no_such_tool', {})).toMatchObject({ isError: true });
  });

  it('runs configurations and tests, and formats', async () => {
    const { w, tools } = setup();
    expect(await tools.call('', 'list_run_configurations', {})).toEqual({ configurations: [{ name: 'App', type: 'build' }] });
    expect(await tools.call('', 'run_configuration', { name: 'App' })).toMatchObject({ exitCode: 0, output: 'ok' });
    expect(await tools.call('', 'run_tests', { package: './internal/x/...', test: 'TestX' })).toMatchObject({ tests: { failed: 1 } });
    expect(await tools.call('', 'run_tests', {})).toMatchObject({ exitCode: 1 });
    expect(await tools.call('', 'run_tests', { package: '/abs' })).toMatchObject({ isError: true });
    expect(w.runs).toEqual(['App', 'test /p/internal/x TestX', 'test /p ']);
    w.disk.set('/p/f.go', 'package  f');
    expect(await tools.call('s3', 'format_file', { path: '/p/f.go' })).toEqual({ text: 'Formatted /p/f.go' });
    expect(w.disk.get('/p/f.go')).toBe('package f');
    expect(await tools.call('s3', 'format_file', { path: '/p/f.go' })).toEqual({ text: '/p/f.go was already formatted' });
    w.disk.set('/p/g.go', 'syntax  error');
    expect(await tools.call('s3', 'format_file', { path: '/p/g.go' })).toMatchObject({ isError: true });
  });
});
