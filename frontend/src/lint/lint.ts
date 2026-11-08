// golangci-lint (R11): diagnostics merged with gopls's, linting the saved
// file's package on save and the project or package on demand, quick fixes
// from suggested edits, and Install golangci-lint.

import { showMenu } from '../app/menu';
import { confirmDialog } from '../app/dialog';
import { project, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import { dirName } from '../editor/buffer';
import { editors } from '../editor/install';
import { lintApi, on, Topics } from '../lib/api';
import type { Diagnostic, FileEdit, LintDiagnosticsEvent, LintStatus } from '../lib/api';
import { commands } from '../lib/commands';
import { effect, signal } from '../lib/store';
import { actionProviders } from '../lang/codeActions';
import type { LocalAction } from '../lang/codeActions';
import { replaceDiagnosticsSource } from '../lang/diagnostics';

export const LINT_SOURCE = 'golangci-lint';
export const LINT_DEBOUNCE_MS = 400;

const status = signal<LintStatus>({ found: false, bin: '', version: '', installing: false, running: 0, message: '' });

function norm(p: string): string {
  return p.replace(/\\/g, '/');
}

/** Whether path is covered by a run over dirs. */
export function inLintScope(path: string, dirs: readonly string[], recursive: boolean): boolean {
  const p = norm(path);
  return dirs.some((d) => {
    const dir = norm(d).replace(/\/$/, '');
    return recursive ? p.startsWith(dir + '/') : p.slice(0, p.lastIndexOf('/')) === dir;
  });
}

/** Quick fixes for lint diagnostics that carry suggested edits. */
export function lintActions(diags: readonly Diagnostic[]): LocalAction[] {
  const out: LocalAction[] = [];
  for (const d of diags) {
    const data = d.data as { goiLintFix?: FileEdit[] } | undefined;
    if (!data?.goiLintFix?.length) continue;
    out.push({ title: `Apply fix: ${d.message.split('\n')[0]} (${d.source})`, kind: 'quickfix', isPreferred: true, goiEdits: data.goiLintFix } as LocalAction);
  }
  return out;
}

let notFoundWarned = false;

async function run(wb: Workbench, scope: 'package' | 'project', path: string, explicit: boolean): Promise<void> {
  try {
    const res = await lintApi.run(scope, path);
    if (res.error && res.error !== 'cancelled') {
      wb.notifications.push({ id: 'lint-error', level: 'error', title: 'golangci-lint failed', body: res.error });
    } else if (explicit && !res.error) {
      wb.notifications.push({ id: 'lint-done', level: 'info', title: res.issues ? `golangci-lint: ${res.issues} issue${res.issues === 1 ? '' : 's'} in ${res.files} file${res.files === 1 ? '' : 's'}` : 'golangci-lint: no issues' });
      if (res.issues) wb.panels.show('problems');
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    if (/golangci-lint not found/.test(msg)) {
      if (explicit) await offerInstall(wb);
      else if (!notFoundWarned) {
        notFoundWarned = true;
        wb.notifications.push({ id: 'lint-missing', level: 'info', title: 'golangci-lint is not installed', body: 'Run "Install golangci-lint" to lint on save, or turn off lint.onSave.' });
      }
      return;
    }
    if (explicit || !/disabled/.test(msg)) wb.notifications.push({ id: 'lint-error', level: 'error', title: 'golangci-lint failed', body: msg });
  }
}

async function offerInstall(wb: Workbench): Promise<void> {
  const ok = await confirmDialog('Install golangci-lint', 'golangci-lint is not installed. Install it into Goi’s tools folder with "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"? This can take a few minutes.', 'Install');
  if (!ok) return;
  try {
    await lintApi.install();
    wb.notifications.push({ id: 'lint-install', level: 'info', title: 'Installing golangci-lint…' });
  } catch (err) {
    wb.notifications.push({ id: 'lint-install', level: 'error', title: 'Cannot install golangci-lint', body: String(err) });
  }
}

export function installLint(wb: Workbench): void {
  on<LintDiagnosticsEvent>(Topics.lintDiagnostics, (ev) => {
    replaceDiagnosticsSource(LINT_SOURCE, (p) => inLintScope(p, ev.dirs, ev.recursive), ev.files);
  });
  on<LintStatus>(Topics.lintStatus, (st) => {
    const was = status.peek();
    status.set(st);
    if (was.installing && !st.installing) {
      wb.notifications.push(st.found ? { id: 'lint-install', level: 'info', title: `golangci-lint ${st.version} installed` } : { id: 'lint-install', level: 'error', title: 'Cannot install golangci-lint', body: st.message });
    }
  });
  effect(() => {
    if (project()) void lintApi.status().then((st) => status.set(st)).catch(() => {});
  });
  actionProviders.push((_path, diags) => lintActions(diags));

  // Lint the saved file's package (debounced per directory).
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  editors.hooks.saved.on((buf) => {
    const s = settings.peek()?.lint;
    if (!s?.enabled || !s.onSave || !buf.path.endsWith('.go')) return;
    const dir = dirName(buf.path);
    clearTimeout(timers.get(dir));
    timers.set(
      dir,
      setTimeout(() => {
        timers.delete(dir);
        void run(wb, 'package', buf.path, false);
      }, LINT_DEBOUNCE_MS),
    );
  });

  // Status item: running / not installed.
  const el = wb.status.add({
    id: 'lint',
    side: 'right',
    priority: 45,
    title: 'golangci-lint',
    onClick: () => {
      const r = el.getBoundingClientRect();
      const st = status.peek();
      showMenu(r.left, r.top - 90, [
        { label: 'Run on Project', disabled: !st.found, run: () => void commands.run('lint.runProject') },
        { label: 'Run on Current Package', disabled: !st.found || !editors.activeBuffer(), run: () => void commands.run('lint.runPackage') },
        { label: st.found ? `golangci-lint ${st.version}` : 'Install golangci-lint…', disabled: st.found || st.installing, run: () => void offerInstall(wb) },
      ]);
    },
  });
  effect(() => {
    const st = status();
    const s = settings()?.lint;
    el.hidden = !project() || !s?.enabled;
    el.textContent = st.installing ? 'Lint: installing…' : st.running ? 'Lint ⟳' : st.found ? 'Lint' : 'Lint: not installed';
    el.title = st.found ? `golangci-lint ${st.version}` : st.message || 'golangci-lint is not installed';
    el.classList.toggle('lint-missing', !st.found && !st.installing);
  });

  const has = (): boolean => !!project.peek();
  commands.register({ id: 'lint.runProject', title: 'Run golangci-lint on Project', category: 'Code', when: has, run: () => run(wb, 'project', project.peek()?.root ?? '', true) });
  commands.register({
    id: 'lint.runPackage',
    title: 'Run golangci-lint on Current Package',
    category: 'Code',
    when: () => has() && !!editors.activeBuffer()?.path.endsWith('.go'),
    run: () => run(wb, 'package', editors.activeBuffer()?.path ?? '', true),
  });
  commands.register({ id: 'lint.install', title: 'Install golangci-lint', category: 'Code', when: has, run: () => offerInstall(wb) });
}
