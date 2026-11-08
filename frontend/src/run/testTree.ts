// Test result tree built from `go test -json` (test2json) events (R3.1):
// package → test → subtests, with status, elapsed time and per-node output.

import type { TestEvent } from '../lib/api';

export type TestStatus = 'running' | 'pass' | 'fail' | 'skip' | 'stopped';

export interface TestNode {
  /** Unique id: package, or package + "\u0000" + test name. */
  id: string;
  pkg: string;
  /** Full test name ("TestA/sub_1"); "" for packages and the root. */
  name: string;
  /** Last name segment (or the package path). */
  label: string;
  parent: TestNode | null;
  children: TestNode[];
  status: TestStatus;
  /** Seconds. */
  elapsed: number;
  output: string[];
  /** Set on packages that failed to build. */
  buildFailed: boolean;
  /** Set on packages that have no test files. */
  noTests: boolean;
}

export interface TestSummary {
  passed: number;
  failed: number;
  skipped: number;
  running: number;
  stopped: number;
  total: number;
}

function node(id: string, pkg: string, name: string, label: string, parent: TestNode | null): TestNode {
  return { id, pkg, name, label, parent, children: [], status: 'running', elapsed: 0, output: [], buildFailed: false, noTests: false };
}

/** Strips the " [pkg.test]" suffix of build-output import paths. */
export function buildPackage(importPath: string): string {
  return importPath.replace(/\s*\[.*\]$/, '');
}

export class TestTree {
  readonly root: TestNode = node('', '', '', 'Test Results', null);
  private byId = new Map<string, TestNode>();
  /** Bumped on every change (cheap change detection for views). */
  version = 0;

  get packages(): TestNode[] {
    return this.root.children;
  }

  get(id: string): TestNode | undefined {
    return id === '' ? this.root : this.byId.get(id);
  }

  reset(): void {
    this.root.children = [];
    this.root.output = [];
    this.root.status = 'running';
    this.root.elapsed = 0;
    this.byId.clear();
    this.version++;
  }

  private pkgNode(pkg: string): TestNode {
    let n = this.byId.get(pkg);
    if (!n) {
      n = node(pkg, pkg, '', pkg, this.root);
      this.byId.set(pkg, n);
      this.root.children.push(n);
    }
    return n;
  }

  private testNode(pkg: string, name: string): TestNode {
    const id = `${pkg}\u0000${name}`;
    let n = this.byId.get(id);
    if (n) return n;
    const slash = name.lastIndexOf('/');
    const parent = slash < 0 ? this.pkgNode(pkg) : this.testNode(pkg, name.slice(0, slash));
    n = node(id, pkg, name, slash < 0 ? name : name.slice(slash + 1), parent);
    this.byId.set(id, n);
    parent.children.push(n);
    return n;
  }

  /** Applies one event; returns the node it changed. */
  apply(ev: TestEvent): TestNode {
    this.version++;
    if (ev.action === 'build-output' || ev.action === 'build-fail') {
      const pkg = buildPackage(ev.importPath || ev.package);
      const n = pkg ? this.pkgNode(pkg) : this.root;
      if (ev.output) n.output.push(ev.output);
      if (ev.action === 'build-fail') {
        n.buildFailed = true;
        n.status = 'fail';
      }
      return n;
    }
    if (!ev.package) {
      if (ev.output) this.root.output.push(ev.output);
      return this.root;
    }
    const n = ev.test ? this.testNode(ev.package, ev.test) : this.pkgNode(ev.package);
    switch (ev.action) {
      case 'start':
      case 'run':
      case 'cont':
        n.status = 'running';
        break;
      case 'output':
      case 'bench':
        if (ev.output) n.output.push(ev.output);
        break;
      case 'pass':
      case 'fail':
      case 'skip':
        n.status = ev.action;
        n.elapsed = ev.elapsed ?? 0;
        if (!ev.test && ev.failedBuild) n.buildFailed = true;
        if (!ev.test && ev.action === 'skip' && n.children.length === 0) n.noTests = n.output.some((o) => o.includes('[no test files]'));
        break;
    }
    return n;
  }

  /** Marks still-running nodes stopped and computes root status. */
  finish(stopped: boolean): void {
    this.version++;
    const walk = (n: TestNode): void => {
      for (const c of n.children) walk(c);
      if (n.status === 'running') n.status = stopped ? 'stopped' : n.children.some((c) => c.status === 'fail') ? 'fail' : 'stopped';
    };
    for (const p of this.root.children) walk(p);
    this.root.status = this.rootStatus();
    this.root.elapsed = this.root.children.reduce((s, p) => s + p.elapsed, 0);
  }

  private rootStatus(): TestStatus {
    const ps = this.root.children.filter((p) => !p.noTests);
    if (ps.some((p) => p.status === 'running')) return 'running';
    if (ps.some((p) => p.status === 'fail')) return 'fail';
    if (ps.some((p) => p.status === 'stopped')) return 'stopped';
    if (ps.length && ps.every((p) => p.status === 'skip')) return 'skip';
    return 'pass';
  }

  /** Counts leaf tests (tests without subtests). */
  summary(): TestSummary {
    const s: TestSummary = { passed: 0, failed: 0, skipped: 0, running: 0, stopped: 0, total: 0 };
    const walk = (n: TestNode): void => {
      if (n.name && n.children.length === 0) {
        s.total++;
        if (n.status === 'pass') s.passed++;
        else if (n.status === 'fail') s.failed++;
        else if (n.status === 'skip') s.skipped++;
        else if (n.status === 'running') s.running++;
        else s.stopped++;
      }
      for (const c of n.children) walk(c);
    };
    walk(this.root);
    return s;
  }

  /** Failed tests whose failure isn't explained by a failed subtest. */
  failedLeaves(): TestNode[] {
    const out: TestNode[] = [];
    const walk = (n: TestNode): void => {
      const failedKids = n.children.filter((c) => c.status === 'fail');
      if (n.name && n.status === 'fail' && failedKids.length === 0) out.push(n);
      for (const c of failedKids) walk(c);
      if (!n.name) for (const c of n.children) if (c.status !== 'fail') walk(c);
    };
    for (const p of this.root.children) walk(p);
    return out;
  }

  /** The node's output followed by its descendants', in order. */
  allOutput(n: TestNode): string {
    const parts: string[] = [];
    const walk = (x: TestNode): void => {
      parts.push(...x.output);
      for (const c of x.children) walk(c);
    };
    walk(n);
    return parts.join('');
  }
}

/** "Tests failed: 1, passed: 3, skipped: 1 of 5 tests". */
export function summaryText(s: TestSummary): string {
  const parts: string[] = [];
  if (s.running) parts.push(`running: ${s.running}`);
  if (s.failed) parts.push(`failed: ${s.failed}`);
  if (s.passed) parts.push(`passed: ${s.passed}`);
  if (s.skipped) parts.push(`skipped: ${s.skipped}`);
  if (s.stopped) parts.push(`stopped: ${s.stopped}`);
  if (!parts.length) return 'No tests run';
  return `Tests ${parts.join(', ')} of ${s.total} test${s.total === 1 ? '' : 's'}`;
}

/** Elapsed time label: "12 ms", "1.3 s". */
export function elapsedLabel(seconds: number): string {
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
  return `${seconds.toFixed(seconds < 10 ? 2 : 1)} s`;
}

/** Regexp-quotes s for Go's RE2 (same as regexp.QuoteMeta). */
export function quoteMeta(s: string): string {
  return s.replace(/[\\.+*?()|[\]{}^$]/g, '\\$&');
}

/** The `go test -run` pattern selecting the given tests (mirror of Go's
 * runconfig.TestRunPattern): anchored alternations per level. */
export function testRunPattern(names: string[]): string {
  const levels: string[][] = [];
  const seen: Set<string>[] = [];
  let minDepth = Infinity;
  for (const n of names) {
    const parts = n.split('/');
    minDepth = Math.min(minDepth, parts.length);
    parts.forEach((p, i) => {
      if (!levels[i]) {
        levels[i] = [];
        seen[i] = new Set();
      }
      if (!seen[i].has(p)) {
        seen[i].add(p);
        levels[i].push(quoteMeta(p));
      }
    });
  }
  return levels
    .slice(0, Number.isFinite(minDepth) ? minDepth : 0)
    .map((l) => (l.length === 1 ? `^${l[0]}$` : `^(${l.join('|')})$`))
    .join('/');
}
