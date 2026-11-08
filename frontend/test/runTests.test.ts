// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { delta, deltaLabel, metricLabel, parseBenchLine, parseHistory } from '../src/run/bench';
import { parseExpectedActual, unquoteGo } from '../src/run/diff';
import { pkgMatchScore, rerunConfig, shortPkg } from '../src/run/testView';
import { elapsedLabel, quoteMeta, summaryText, TestTree, testRunPattern } from '../src/run/testTree';
import { emptyRunConfig } from '../src/lib/api';
import type { TestEvent } from '../src/lib/api';

const ev = (action: string, pkg: string, test = '', extra: Partial<TestEvent> = {}): TestEvent => ({
  action, package: pkg, test, elapsed: 0, output: '', importPath: '', failedBuild: '', ...extra,
});

function sampleTree(): TestTree {
  const t = new TestTree();
  const P = 'example.com/m/calc';
  for (const e of [
    ev('start', P),
    ev('run', P, 'TestAdd'),
    ev('output', P, 'TestAdd', { output: '=== RUN   TestAdd\n' }),
    ev('run', P, 'TestAdd/ok'),
    ev('pass', P, 'TestAdd/ok', { elapsed: 0.001 }),
    ev('run', P, 'TestAdd/broken_case'),
    ev('output', P, 'TestAdd/broken_case', { output: '    calc_test.go:12: expected failure\n' }),
    ev('fail', P, 'TestAdd/broken_case', { elapsed: 0.002 }),
    ev('fail', P, 'TestAdd', { elapsed: 0.003 }),
    ev('run', P, 'TestSkip'),
    ev('skip', P, 'TestSkip'),
    ev('run', P, 'TestTop'),
    ev('fail', P, 'TestTop'),
    ev('output', P, '', { output: 'FAIL\n' }),
    ev('fail', P, '', { elapsed: 0.4 }),
    ev('start', 'example.com/m/empty'),
    ev('output', 'example.com/m/empty', '', { output: '?   \texample.com/m/empty\t[no test files]\n' }),
    ev('skip', 'example.com/m/empty'),
  ]) t.apply(e);
  t.finish(false);
  return t;
}

describe('test tree', () => {
  it('builds package → test → subtest nodes with statuses', () => {
    const t = sampleTree();
    expect(t.packages.map((p) => p.pkg)).toEqual(['example.com/m/calc', 'example.com/m/empty']);
    const calc = t.packages[0];
    expect(calc.status).toBe('fail');
    expect(calc.children.map((c) => `${c.label}:${c.status}`)).toEqual(['TestAdd:fail', 'TestSkip:skip', 'TestTop:fail']);
    expect(calc.children[0].children.map((c) => c.label)).toEqual(['ok', 'broken_case']);
    expect(t.packages[1].noTests).toBe(true);
    expect(t.root.status).toBe('fail');
  });
  it('summarizes leaf tests', () => {
    const s = sampleTree().summary();
    expect(s).toMatchObject({ passed: 1, failed: 2, skipped: 1, total: 4 });
    expect(summaryText(s)).toBe('Tests failed: 2, passed: 1, skipped: 1 of 4 tests');
    expect(summaryText({ passed: 0, failed: 0, skipped: 0, running: 0, stopped: 0, total: 0 })).toBe('No tests run');
  });
  it('lists failed leaves, not parents failed by subtests', () => {
    expect(sampleTree().failedLeaves().map((n) => n.name)).toEqual(['TestAdd/broken_case', 'TestTop']);
  });
  it('collects output of a node and its descendants', () => {
    const t = sampleTree();
    const add = t.packages[0].children[0];
    expect(t.allOutput(add)).toBe('=== RUN   TestAdd\n    calc_test.go:12: expected failure\n');
  });
  it('records build failures', () => {
    const t = new TestTree();
    t.apply(ev('build-output', '', '', { importPath: 'example.com/m/bad [example.com/m/bad.test]', output: 'bad/x.go:3:1: syntax error\n' }));
    t.apply(ev('build-fail', '', '', { importPath: 'example.com/m/bad [example.com/m/bad.test]' }));
    t.apply(ev('fail', 'example.com/m/bad', '', { failedBuild: 'example.com/m/bad [example.com/m/bad.test]' }));
    const p = t.packages[0];
    expect(p.pkg).toBe('example.com/m/bad');
    expect(p.buildFailed).toBe(true);
    expect(p.status).toBe('fail');
    expect(p.output.join('')).toContain('syntax error');
  });
  it('marks running tests stopped when the run is stopped', () => {
    const t = new TestTree();
    t.apply(ev('run', 'p', 'TestLong'));
    t.finish(true);
    expect(t.packages[0].children[0].status).toBe('stopped');
  });
  it('formats elapsed times', () => {
    expect(elapsedLabel(0.0123)).toBe('12 ms');
    expect(elapsedLabel(1.234)).toBe('1.23 s');
    expect(elapsedLabel(12.34)).toBe('12.3 s');
  });
});

describe('run patterns', () => {
  it('matches the Go implementation', () => {
    expect(testRunPattern(['TestA'])).toBe('^TestA$');
    expect(testRunPattern(['TestA', 'TestB'])).toBe('^(TestA|TestB)$');
    expect(testRunPattern(['TestA/case_(1)', 'TestA/x.y'])).toBe('^TestA$/^(case_\\(1\\)|x\\.y)$');
    expect(testRunPattern(['TestA', 'TestB/sub'])).toBe('^(TestA|TestB)$');
    expect(quoteMeta('a.b*c[d]')).toBe('a\\.b\\*c\\[d\\]');
  });
  it('builds rerun configurations from nodes', () => {
    const t = sampleTree();
    const base = { ...emptyRunConfig('test', 'go test ./...'), kind: 'directory', directory: '.', race: true };
    const failed = rerunConfig(base, t.failedLeaves(), 'go test ./...');
    expect(failed).toMatchObject({ kind: 'function', packages: ['example.com/m/calc'], pattern: '^(TestAdd|TestTop)$', race: true, directory: '' });
    const one = rerunConfig(base, [t.packages[0].children[0].children[1]], 'x');
    expect(one.pattern).toBe('^TestAdd$/^broken_case$');
    const pkg = rerunConfig(base, [t.packages[0]], 'p');
    expect(pkg).toMatchObject({ kind: 'package', package: 'example.com/m/calc', packages: [] });
  });
  it('turns benchmarks into -bench runs', () => {
    const t = new TestTree();
    const n = t.apply(ev('run', 'p', 'BenchmarkAdd'));
    expect(rerunConfig(emptyRunConfig('test'), [n], 'b')).toMatchObject({ pattern: '^$', bench: '^BenchmarkAdd$' });
  });
  it('scores package matches for jump to source', () => {
    expect(pkgMatchScore('/r/internal/calc/calc_test.go', 'example.com/m/internal/calc')).toBe(2);
    expect(pkgMatchScore('/r/other/calc_test.go', 'example.com/m/internal/calc')).toBe(0);
    expect(shortPkg('example.com/m/internal/calc')).toBe('internal/calc');
  });
});

describe('benchmarks', () => {
  it('parses result lines with standard and custom metrics', () => {
    expect(parseBenchLine('BenchmarkAdd-8   \t1000000000\t         0.2500 ns/op\t       0 B/op\t       0 allocs/op')).toEqual({
      name: 'BenchmarkAdd', procs: 8, iterations: 1000000000, metrics: { 'ns/op': 0.25, 'B/op': 0, 'allocs/op': 0 },
    });
    expect(parseBenchLine('BenchmarkCopy/size=1k-4  500  2400 ns/op  426.67 MB/s')).toMatchObject({ name: 'BenchmarkCopy/size=1k', procs: 4, metrics: { 'MB/s': 426.67 } });
    expect(parseBenchLine('BenchmarkX')).toBeNull();
    expect(parseBenchLine('=== RUN   BenchmarkX')).toBeNull();
  });
  it('computes and labels deltas', () => {
    expect(delta(110, 100)).toBeCloseTo(10);
    expect(delta(1, 0)).toBeNull();
    expect(deltaLabel(10)).toBe('+10.0%');
    expect(deltaLabel(-4)).toBe('−4.0%');
    expect(deltaLabel(0.01)).toBe('±0%');
    expect(metricLabel(1234567)).toBe('1,234,567');
    expect(metricLabel(0.25)).toBe('0.250');
    expect(parseHistory('{bad')).toEqual({});
  });
});

describe('testify difference', () => {
  it('extracts expected and actual values', () => {
    const out = [
      '    calc_test.go:20: ',
      '        \tError Trace:\t/r/calc/calc_test.go:20',
      '        \tError:      \tNot equal: ',
      '        \t            \texpected: "hello\\nworld"',
      '        \t            \tactual  : "hello\\nthere"',
      '        \t            \t',
      '        \t            \tDiff:',
      '        \tTest:       \tTestGreet',
    ].join('\n');
    expect(parseExpectedActual(out)).toEqual({ expected: 'hello\nworld', actual: 'hello\nthere' });
  });
  it('keeps multi-line struct values', () => {
    const out = ['        \t            \texpected: main.T{', '        \t            \t  A: 1,', '        \t            \t}', '        \t            \tactual  : main.T{A: 2}', ''].join('\n');
    expect(parseExpectedActual(out)).toEqual({ expected: 'main.T{\n  A: 1,\n}', actual: 'main.T{A: 2}' });
  });
  it('returns null without a pair and unquotes Go strings', () => {
    expect(parseExpectedActual('    x_test.go:3: boom')).toBeNull();
    expect(unquoteGo('"a\\tb"')).toBe('a\tb');
    expect(unquoteGo('`raw`')).toBe('raw');
    expect(unquoteGo('42')).toBe('42');
  });
});
