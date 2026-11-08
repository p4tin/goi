// Test runner UI (R3): the test tree beside a test run's console, with a
// summary, Hide passed, rerun failed / single tests, jump to source, Show
// Difference for testify failures, and a benchmarks table compared with the
// previous run.

import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { editors } from '../editor/install';
import { run, search } from '../lib/api';
import type { OutputChunk, RunConfig, RunInfo, TestEvent, TextMatch } from '../lib/api';
import { h } from '../lib/dom';
import { delta, deltaLabel, metricLabel, parseBenchLine, parseHistory } from './bench';
import type { BenchHistory, BenchResult } from './bench';
import { parseExpectedActual, showDifference } from './diff';
import { icon, iconButton } from './icons';
import { launch } from './launch';
import type { RunTab, TestView } from './runWindow';
import { configs, notifyError } from './state';
import { elapsedLabel, quoteMeta, summaryText, TestTree, testRunPattern } from './testTree';
import type { TestNode } from './testTree';

const STATUS_ICON = { running: 'running', pass: 'passed', fail: 'failed', skip: 'skipped', stopped: 'skipped' } as const;

/** Configuration that reruns the given nodes with base's flags. */
export function rerunConfig(base: RunConfig, nodes: TestNode[], name: string): RunConfig {
  const pkgs = [...new Set(nodes.map((n) => n.pkg))];
  const tests = nodes.filter((n) => n.name).map((n) => n.name);
  const cfg: RunConfig = {
    ...structuredClone(base),
    name,
    shared: false,
    temporary: false,
    created: 0,
    files: [],
    directory: '',
    package: '',
    packages: pkgs,
    kind: 'function',
    pattern: '',
    bench: '',
  };
  if (tests.length === 0) {
    // Whole packages.
    cfg.kind = 'package';
    if (pkgs.length === 1) {
      cfg.package = pkgs[0];
      cfg.packages = [];
    }
    cfg.pattern = '';
    return cfg;
  }
  const benches = tests.filter((t) => t.startsWith('Benchmark'));
  if (benches.length === tests.length) {
    cfg.pattern = '^$';
    cfg.bench = testRunPattern(benches);
  } else {
    cfg.pattern = testRunPattern(tests.filter((t) => !t.startsWith('Benchmark')));
  }
  return cfg;
}

/** Short package label: the last two path elements. */
export function shortPkg(pkg: string): string {
  return pkg.split('/').slice(-2).join('/');
}

/** Scores how well a file path matches a package import path (trailing
 * directory segments in common). */
export function pkgMatchScore(path: string, pkg: string): number {
  const dirs = path.replace(/\\/g, '/').split('/').slice(0, -1);
  const segs = pkg.split('/');
  let n = 0;
  while (n < dirs.length && n < segs.length && dirs[dirs.length - 1 - n] === segs[segs.length - 1 - n]) n++;
  return n;
}

async function findTest(n: TestNode): Promise<TextMatch | null> {
  const top = n.name.split('/')[0];
  const q = {
    pattern: `^func (\\([^)]*\\) )?${quoteMeta(top)}\\(`,
    regex: true,
    caseSensitive: true,
    wholeWord: false,
    mask: '*_test.go',
    replace: '',
    scope: '',
    buffers: [],
  };
  const res = await search.findInPath(q);
  if (!res.matches.length) return null;
  const best = [...res.matches].sort((a, b) => pkgMatchScore(b.path, n.pkg) - pkgMatchScore(a.path, n.pkg))[0];
  if (!n.name.includes('/')) return best;
  // Subtest: find its t.Run name literal in the same file (test2json turns
  // spaces into underscores).
  const sub = n.label;
  const pat = `"${quoteMeta(sub).replace(/_/g, '[ _]')}"`;
  const dir = best.path.replace(/[\\/][^\\/]*$/, '');
  const base = best.path.slice(dir.length + 1);
  const subRes = await search.findInPath({ ...q, pattern: pat, mask: base, scope: dir });
  return subRes.matches.find((m) => m.path === best.path && m.line >= best.line) ?? best;
}

export async function jumpToTest(n: TestNode): Promise<void> {
  if (!n.name) return;
  try {
    const m = await findTest(n);
    if (!m) {
      notifyError('jump-test', `Cannot find ${n.name}`, 'No matching function in the project');
      return;
    }
    await editors.open(m.path, { line: m.line, col: m.col + 1, focus: true });
  } catch (err) {
    notifyError('jump-test', `Cannot find ${n.name}`, err);
  }
}

export function createTestView(tab: RunTab): TestView {
  const tree = new TestTree();
  let selected = '';
  let hidePassed = false;
  const collapsed = new Set<string>();
  let rootLog: OutputChunk[] = [];
  let history: BenchHistory = {};
  const benches = new Map<string, BenchResult>();
  let rendered = -1;
  let frame = 0;

  const summary = h('div', { class: 'tv-summary' });
  const diffLink = h('button', { class: 'tv-diff-link', type: 'button', hidden: true }, 'Show Difference');
  const rows = h('div', { class: 'tv-rows', role: 'tree', tabindex: '0' });
  const benchBox = h('div', { class: 'tv-bench', hidden: true });
  const el = h('div', { class: 'tv' }, h('div', { class: 'tv-head' }, summary, diffLink), rows, benchBox);

  void run.loadBench().then((j) => (history = parseHistory(j)));

  const visibleChildren = (n: TestNode): TestNode[] =>
    n.children.filter((c) => !c.noTests && !(hidePassed && (c.status === 'pass' || c.status === 'skip')));

  const showOutput = (): void => {
    const con = tab.console;
    con.clear();
    if (selected === '') {
      for (const c of rootLog) con.append(c.stream, c.text);
      return;
    }
    const n = tree.get(selected);
    if (n) con.append('stdout', tree.allOutput(n));
    con.scrollToEnd();
  };

  const select = (id: string): void => {
    if (selected === id) return;
    selected = id;
    showOutput();
    schedule();
  };

  const isUnder = (n: TestNode | null, id: string): boolean => {
    for (let x = n; x; x = x.parent) if (x.id === id) return true;
    return false;
  };

  const render = (): void => {
    frame = 0;
    if (rendered === tree.version) return;
    rendered = tree.version;
    const s = tree.summary();
    const rootSt = s.running ? 'running' : s.failed ? 'fail' : tree.root.status;
    summary.replaceChildren(
      icon(STATUS_ICON[rootSt], `st-${rootSt === 'running' ? 'running' : rootSt === 'fail' ? 'failed' : 'passed'}`),
      h('span', null, summaryText(s)),
      h('span', { class: 'tv-time' }, tree.root.elapsed ? ` – ${elapsedLabel(tree.root.elapsed)}` : ''),
    );
    const out: HTMLElement[] = [];
    const add = (n: TestNode, depth: number): void => {
      const kids = visibleChildren(n);
      const open = !collapsed.has(n.id);
      const st = n.status;
      out.push(
        h(
          'div',
          {
            class: `tv-row${n.id === selected ? ' selected' : ''}`,
            role: 'treeitem',
            'aria-selected': String(n.id === selected),
            'aria-expanded': kids.length ? String(open) : undefined,
            dataset: { id: n.id },
            style: `padding-left:${6 + depth * 14}px`,
          },
          h('span', { class: `tv-twisty${kids.length ? '' : ' leaf'}` }, kids.length ? (open ? '▾' : '▸') : ''),
          icon(STATUS_ICON[st], `st-${st === 'pass' ? 'passed' : st === 'fail' ? 'failed' : st === 'running' ? 'running' : 'skipped'}`),
          h('span', { class: 'tv-label' }, n.name ? n.label : shortPkg(n.pkg) || n.label),
          n.buildFailed ? h('span', { class: 'tv-note' }, 'build failed') : null,
          n.elapsed && st !== 'running' ? h('span', { class: 'tv-time' }, elapsedLabel(n.elapsed)) : null,
        ),
      );
      if (open) for (const c of kids) add(c, depth + 1);
    };
    add(tree.root, 0);
    rows.replaceChildren(...out);
    const sel = tree.get(selected);
    diffLink.hidden = !(sel && sel.status === 'fail' && parseExpectedActual(tree.allOutput(sel)));
    renderBench();
  };

  const schedule = (): void => {
    rendered = -1;
    if (!frame) frame = requestAnimationFrame(render);
  };

  const renderBench = (): void => {
    benchBox.hidden = benches.size === 0;
    if (!benches.size) return;
    const units = new Set<string>();
    for (const b of benches.values()) for (const u of Object.keys(b.metrics)) units.add(u);
    const order = ['ns/op', 'B/op', 'allocs/op'];
    const cols = [...order.filter((u) => units.has(u)), ...[...units].filter((u) => !order.includes(u)).sort()];
    const head = h('tr', null, h('th', null, 'Benchmark'), h('th', null, 'Iterations'), cols.map((u) => h('th', null, u)));
    const body = [...benches.values()].map((b) => {
      const prev = history[b.name];
      return h(
        'tr',
        null,
        h('td', { class: 'tv-bench-name' }, b.name + (b.procs > 1 ? `-${b.procs}` : '')),
        h('td', null, metricLabel(b.iterations)),
        cols.map((u) => {
          const d = delta(b.metrics[u], prev?.metrics[u]);
          // Lower is better except for throughput units.
          const better = d === null ? '' : (u.endsWith('/s') ? d > 0 : d < 0) ? 'better' : Math.abs(d) < 0.05 ? '' : 'worse';
          return h('td', null, metricLabel(b.metrics[u]), d !== null ? h('span', { class: `tv-delta ${better}`, title: prev ? `Previous: ${metricLabel(prev.metrics[u])}` : '' }, ` ${deltaLabel(d)}`) : null);
        }),
      );
    });
    benchBox.replaceChildren(h('div', { class: 'tv-bench-title' }, 'Benchmarks'), h('table', null, h('thead', null, head), h('tbody', null, body)));
  };

  const baseName = (): string => tab.info.config.name;

  const runNodes = async (nodes: TestNode[], debug: boolean): Promise<void> => {
    if (!nodes.length) return;
    const n = nodes[0];
    const name = nodes.length === 1 ? (n.name ? `${n.name} in ${shortPkg(n.pkg)}` : `go test ${n.pkg}`) : `${baseName()} (selected)`;
    try {
      const saved = await run.addTemporary(rerunConfig(tab.info.config, nodes, name));
      configs.set(await run.configs());
      await launch(saved.name, debug);
    } catch (err) {
      notifyError('run-test', `Cannot ${debug ? 'debug' : 'run'} ${name}`, err);
    }
  };

  const rerunFailed = async (): Promise<void> => {
    const failed = tree.failedLeaves();
    if (!failed.length) return;
    try {
      await run.runConfig(rerunConfig(tab.info.config, failed, baseName()), tab.id, false);
    } catch (err) {
      notifyError('rerun-failed', 'Cannot rerun failed tests', err);
    }
  };

  const diffFor = async (n: TestNode): Promise<void> => {
    const d = parseExpectedActual(tree.allOutput(n));
    if (d) await showDifference(`Comparison Failure: ${n.name}`, d);
  };

  diffLink.addEventListener('click', () => {
    const n = tree.get(selected);
    if (n) void diffFor(n);
  });

  const nodeAt = (e: Event): TestNode | undefined => {
    const row = (e.target as HTMLElement).closest('.tv-row') as HTMLElement | null;
    return row ? tree.get(row.dataset.id ?? '') : undefined;
  };

  rows.addEventListener('click', (e) => {
    const n = nodeAt(e);
    if (!n) return;
    if ((e.target as HTMLElement).classList.contains('tv-twisty')) {
      if (collapsed.has(n.id)) collapsed.delete(n.id);
      else collapsed.add(n.id);
      schedule();
      return;
    }
    select(n.id);
  });
  rows.addEventListener('dblclick', (e) => {
    const n = nodeAt(e);
    if (n?.name) void jumpToTest(n);
  });
  rows.addEventListener('contextmenu', (e) => {
    const n = nodeAt(e);
    if (!n) return;
    e.preventDefault();
    select(n.id);
    const label = n.name || shortPkg(n.pkg) || 'all tests';
    const items: MenuItem[] = [];
    if (n === tree.root) {
      items.push({ label: 'Rerun', run: () => void run.rerun(tab.id) }, { label: 'Rerun Failed Tests', disabled: !tree.failedLeaves().length, run: () => void rerunFailed() });
    } else {
      items.push(
        { label: `Run '${label}'`, run: () => void runNodes([n], false) },
        { label: `Debug '${label}'`, run: () => void runNodes([n], true) },
      );
      if (n.name) items.push({ separator: true }, { label: 'Jump to Source', run: () => void jumpToTest(n) });
      if (n.status === 'fail' && parseExpectedActual(tree.allOutput(n))) items.push({ label: 'Show Difference', run: () => void diffFor(n) });
      items.push({ separator: true }, { label: 'Copy Name', run: () => void navigator.clipboard?.writeText(n.name || n.pkg) });
    }
    showMenu(e.clientX, e.clientY, items);
  });
  rows.addEventListener('keydown', (e) => {
    const ids = [...rows.querySelectorAll<HTMLElement>('.tv-row')].map((r) => r.dataset.id ?? '');
    const i = ids.indexOf(selected);
    const n = tree.get(selected);
    if (e.key === 'ArrowDown' && i < ids.length - 1) select(ids[i + 1]);
    else if (e.key === 'ArrowUp' && i > 0) select(ids[i - 1]);
    else if (e.key === 'ArrowLeft' && n && !collapsed.has(n.id) && n.children.length) {
      collapsed.add(n.id);
      schedule();
    } else if (e.key === 'ArrowRight' && n && collapsed.has(n.id)) {
      collapsed.delete(n.id);
      schedule();
    } else if (e.key === 'Enter' && n?.name) void jumpToTest(n);
    else return;
    e.preventDefault();
  });

  const hideBtn = iconButton('filter', 'Hide passed tests', () => {
    hidePassed = !hidePassed;
    hideBtn.classList.toggle('on', hidePassed);
    schedule();
  });
  const failedBtn = iconButton('rerunFailed', 'Rerun Failed Tests', () => void rerunFailed(), 'tb-rerun-failed');
  failedBtn.disabled = true;
  const tools = [
    failedBtn,
    hideBtn,
    iconButton('expandAll', 'Expand all', () => {
      collapsed.clear();
      schedule();
    }),
    iconButton('collapseAll', 'Collapse all', () => {
      for (const p of tree.packages) collapsed.add(p.id);
      schedule();
    }),
  ];

  schedule();

  return {
    el,
    tools,
    reset(_info: RunInfo) {
      void run.loadBench().then((j) => (history = parseHistory(j)));
      tree.reset();
      rootLog = [];
      benches.clear();
      selected = '';
      failedBtn.disabled = true;
      schedule();
    },
    output(c: OutputChunk) {
      rootLog.push(c);
      if (selected === '') tab.console.append(c.stream, c.text);
    },
    events(evs: TestEvent[]) {
      for (const ev of evs) {
        const n = tree.apply(ev);
        if (ev.output) {
          rootLog.push({ stream: 'stdout', text: ev.output });
          if (selected === '' || isUnder(n, selected)) tab.console.append('stdout', ev.output);
          const b = parseBenchLine(ev.output);
          if (b) benches.set(b.name, b);
        }
      }
      schedule();
    },
    finished(info: RunInfo) {
      tree.finish(info.state === 'exited' && !!info.exit?.signal);
      failedBtn.disabled = tree.failedLeaves().length === 0;
      // Expand failures, collapse fully passed packages.
      for (const p of tree.packages) if (p.status === 'pass') collapsed.add(p.id);
      if (benches.size) {
        const next: BenchHistory = { ...history };
        for (const b of benches.values()) next[b.name] = b;
        void run.saveBench(JSON.stringify(next)).catch(() => {});
      }
      schedule();
    },
    dispose() {
      if (frame) cancelAnimationFrame(frame);
    },
  };
}
