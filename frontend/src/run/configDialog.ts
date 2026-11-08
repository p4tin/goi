// Run/Debug Configurations dialog (R1.4): configurations on the left, a
// type-specific form on the right. Edits are drafts until OK/Apply; each
// draft is validated by the backend as it changes.

import { showDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import { h } from '../lib/dom';
import { emptyRunConfig, run } from '../lib/api';
import type { RunConfig, RunConfigType, RunTask } from '../lib/api';
import { iconButton } from './icons';
import { configs, notifyError, selectedName, TYPE_LABEL } from './state';

interface Draft {
  cfg: RunConfig;
  /** Name it was saved under ("" for new drafts). */
  orig: string;
  dirty: boolean;
  deleted: boolean;
}

/** "K=V" lines ⇄ env map. */
export function envToText(env: Record<string, string>): string {
  return Object.entries(env)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n');
}

export function textToEnv(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const i = line.indexOf('=');
    if (i <= 0) continue;
    out[line.slice(0, i).trim()] = line.slice(i + 1);
  }
  return out;
}

/** Splits a comma/space separated list. */
export function splitList(s: string): string[] {
  return s
    .split(/[,\s]+/)
    .map((x) => x.trim())
    .filter(Boolean);
}

/** A unique name based on base among existing names (case-insensitive). */
export function uniqueName(base: string, existing: string[]): string {
  const taken = new Set(existing.map((n) => n.toLowerCase()));
  if (!taken.has(base.toLowerCase())) return base;
  for (let i = 2; ; i++) {
    const n = `${base} (${i})`;
    if (!taken.has(n.toLowerCase())) return n;
  }
}

const KINDS: Record<string, [string, string][]> = {
  app: [
    ['package', 'Package'],
    ['file', 'File'],
    ['directory', 'Directory'],
  ],
  test: [
    ['directory', 'Directory'],
    ['package', 'Package'],
    ['file', 'File'],
    ['function', 'Function (pattern)'],
  ],
};

export async function openConfigDialog(focusName?: string): Promise<void> {
  const drafts: Draft[] = configs.peek().map((c) => ({ cfg: structuredClone(c), orig: c.name, dirty: false, deleted: false }));
  let current: Draft | null = drafts.find((d) => d.cfg.name.toLowerCase() === (focusName ?? selectedName.peek()).toLowerCase()) ?? drafts[0] ?? null;

  const list = h('div', { class: 'rc-list', role: 'listbox', tabindex: '0' });
  const form = h('div', { class: 'rc-form' });
  const errors = h('div', { class: 'rc-errors', role: 'alert' });

  const names = (except?: Draft): string[] => drafts.filter((d) => !d.deleted && d !== except).map((d) => d.cfg.name);

  const renderList = (): void => {
    const groups: [string, Draft[]][] = [
      ['Project (shared)', drafts.filter((d) => !d.deleted && d.cfg.shared)],
      ['Local', drafts.filter((d) => !d.deleted && !d.cfg.shared && !d.cfg.temporary)],
      ['Temporary', drafts.filter((d) => !d.deleted && d.cfg.temporary)],
    ];
    const rows: Node[] = [];
    for (const [title, ds] of groups) {
      if (!ds.length) continue;
      rows.push(h('div', { class: 'rc-group' }, title));
      for (const d of ds) {
        rows.push(
          h(
            'div',
            {
              class: `rc-item${d === current ? ' selected' : ''}`,
              role: 'option',
              'aria-selected': String(d === current),
              onClick: () => {
                current = d;
                renderList();
                renderForm();
              },
            },
            h('span', { class: `rc-type rc-type-${d.cfg.type}` }, TYPE_LABEL[d.cfg.type].split(' ').pop()!.slice(0, 4)),
            h('span', null, d.cfg.name || '(unnamed)'),
            d.dirty ? h('span', { class: 'rc-dirty', title: 'Modified' }, '•') : null,
          ),
        );
      }
    }
    if (!rows.length) rows.push(h('div', { class: 'results-empty' }, 'No configurations. Click + to add one.'));
    list.replaceChildren(...rows);
  };

  let validateSeq = 0;
  const validate = async (): Promise<void> => {
    const d = current;
    if (!d) {
      errors.replaceChildren();
      return;
    }
    const seq = ++validateSeq;
    let errs: string[] = [];
    try {
      errs = await run.validate(d.cfg);
    } catch (err) {
      errs = [String(err)];
    }
    if (d.cfg.name && names(d).some((n) => n.toLowerCase() === d.cfg.name.toLowerCase())) errs.unshift('Another configuration has this name');
    if (seq !== validateSeq) return;
    errors.replaceChildren(...errs.map((e) => h('div', null, e)));
  };

  const changed = (): void => {
    if (!current) return;
    current.dirty = true;
    renderList();
    void validate();
  };

  // ---- Form field helpers bound to the current draft.
  const row = (label: string, field: Node, hint?: string): HTMLElement =>
    h('label', { class: 'rc-row' }, h('span', { class: 'rc-label' }, label), h('span', { class: 'rc-field' }, field, hint ? h('span', { class: 'rc-hint' }, hint) : null));

  const text = (get: () => string, set: (v: string) => void, placeholder = ''): HTMLInputElement => {
    const el = h('input', { class: 'input', type: 'text', value: get(), placeholder, spellcheck: false });
    el.addEventListener('input', () => {
      set(el.value);
      changed();
    });
    return el;
  };
  const area = (get: () => string, set: (v: string) => void, placeholder = '', rows = 3): HTMLTextAreaElement => {
    const el = h('textarea', { class: 'input rc-area', rows, placeholder, spellcheck: false });
    el.value = get();
    el.addEventListener('input', () => {
      set(el.value);
      changed();
    });
    return el;
  };
  const check = (label: string, get: () => boolean, set: (v: boolean) => void): HTMLElement => {
    const el = h('input', { type: 'checkbox', checked: get() });
    el.addEventListener('change', () => {
      set(el.checked);
      changed();
    });
    return h('label', { class: 'rc-check' }, el, ` ${label}`);
  };
  const select = (opts: [string, string][], get: () => string, set: (v: string) => void, rerender = false): HTMLSelectElement => {
    const el = h('select', { class: 'input' }, opts.map(([v, l]) => h('option', { value: v, selected: v === get() }, l)));
    el.addEventListener('change', () => {
      set(el.value);
      changed();
      if (rerender) renderForm();
    });
    return el;
  };
  const num = (get: () => number, set: (v: number) => void, placeholder = ''): HTMLInputElement => {
    const el = h('input', { class: 'input rc-num', type: 'number', min: '0', value: get() ? String(get()) : '', placeholder });
    el.addEventListener('input', () => {
      set(Math.max(0, Math.floor(Number(el.value) || 0)));
      changed();
    });
    return el;
  };

  const beforeLaunch = (c: RunConfig): HTMLElement => {
    const box = h('div', { class: 'rc-tasks' });
    const render = (): void => {
      const rows = c.beforeLaunch.map((t, i) => {
        const field =
          t.type === 'config'
            ? select(
                [['', 'Choose…'], ...names(current!).filter((n) => n.toLowerCase() !== c.name.toLowerCase()).map((n): [string, string] => [n, n])],
                () => t.config,
                (v) => (t.config = v),
              )
            : t.type === 'generate'
              ? text(() => t.package, (v) => (t.package = v), './...')
              : text(() => t.command, (v) => (t.command = v), 'command and arguments');
        const label = t.type === 'config' ? 'Run configuration' : t.type === 'generate' ? 'go generate' : 'External command';
        return h(
          'div',
          { class: 'rc-task' },
          h('span', { class: 'rc-task-label' }, label),
          field,
          iconButton('up', 'Move up', () => {
            if (i === 0) return;
            [c.beforeLaunch[i - 1], c.beforeLaunch[i]] = [c.beforeLaunch[i], c.beforeLaunch[i - 1]];
            changed();
            render();
          }),
          iconButton('down', 'Move down', () => {
            if (i === c.beforeLaunch.length - 1) return;
            [c.beforeLaunch[i + 1], c.beforeLaunch[i]] = [c.beforeLaunch[i], c.beforeLaunch[i + 1]];
            changed();
            render();
          }),
          iconButton('minus', 'Remove', () => {
            c.beforeLaunch.splice(i, 1);
            changed();
            render();
          }),
        );
      });
      const add = iconButton('plus', 'Add task', () => {
        const r = add.getBoundingClientRect();
        const push = (t: RunTask): void => {
          c.beforeLaunch.push(t);
          changed();
          render();
        };
        showMenu(r.left, r.bottom, [
          { label: 'Run another configuration', run: () => push({ type: 'config', config: '', package: '', command: '' }) },
          { label: 'Run go generate', run: () => push({ type: 'generate', config: '', package: './...', command: '' }) },
          { label: 'Run external command', run: () => push({ type: 'command', config: '', package: '', command: '' }) },
        ]);
      });
      box.replaceChildren(...rows, add);
    };
    render();
    return box;
  };

  const targetFields = (c: RunConfig): Node[] => {
    const out: Node[] = [row('Run kind', select(KINDS[c.type], () => c.kind, (v) => (c.kind = v), true))];
    if (c.kind === 'package' || c.kind === 'function') {
      out.push(row('Package', text(() => c.package, (v) => (c.package = v), c.type === 'test' ? '. or ./pkg or import path' : './cmd/app or import path')));
    }
    if (c.kind === 'file') {
      out.push(row('Files', text(() => c.files.join(' '), (v) => (c.files = splitList(v)), 'main.go (relative to the project)')));
    }
    if (c.kind === 'directory') {
      out.push(row('Directory', text(() => c.directory, (v) => (c.directory = v), '. or ./internal')));
    }
    if (c.kind === 'function') {
      out.push(row('Pattern', text(() => c.pattern, (v) => (c.pattern = v), '^TestFoo$'), 'Passed to -run'));
    }
    return out;
  };

  const processFields = (c: RunConfig, argsLabel = 'Program arguments'): Node[] => [
    row('Working directory', text(() => c.workingDir, (v) => (c.workingDir = v), 'Default: the package directory')),
    row(argsLabel, text(() => c.args, (v) => (c.args = v))),
    row('Environment', area(() => envToText(c.env), (v) => (c.env = textToEnv(v)), 'KEY=value, one per line')),
    row('Env file', text(() => c.envFile, (v) => (c.envFile = v), '.env')),
  ];

  const buildFields = (c: RunConfig): Node[] => [
    row('Go tool arguments', text(() => c.buildFlags, (v) => (c.buildFlags = v), '-ldflags=… -gcflags=…')),
    row('Build tags', text(() => c.tags.join(','), (v) => (c.tags = splitList(v)), 'Added to the project tags')),
  ];

  const renderForm = (): void => {
    const d = current;
    if (!d) {
      form.replaceChildren(h('div', { class: 'results-empty' }, 'Select or add a configuration'));
      errors.replaceChildren();
      return;
    }
    const c = d.cfg;
    const fields: Node[] = [
      h('div', { class: 'rc-title' }, TYPE_LABEL[c.type]),
      row('Name', text(() => c.name, (v) => (c.name = v))),
      h('div', { class: 'rc-row rc-flags' }, check('Store as project file (.goi/runConfigurations)', () => c.shared, (v) => {
        c.shared = v;
        if (v) c.temporary = false;
        renderList();
      })),
    ];
    switch (c.type) {
      case 'app':
        fields.push(
          ...targetFields(c),
          ...processFields(c),
          ...buildFields(c),
          row('Output path', text(() => c.output, (v) => (c.output = v), 'Default: a temporary file')),
          row('Redirect input from', text(() => c.inputFile, (v) => (c.inputFile = v), 'File (optional)')),
          h('div', { class: 'rc-row rc-flags' }, check('Run after build', () => c.runAfterBuild !== false, (v) => (c.runAfterBuild = v))),
        );
        break;
      case 'test':
        fields.push(
          ...targetFields(c),
          ...processFields(c, 'Test binary arguments'),
          ...buildFields(c),
          h(
            'div',
            { class: 'rc-row rc-flags' },
            check('-race', () => c.race, (v) => (c.race = v)),
            check('-short', () => c.short, (v) => (c.short = v)),
            check('-benchmem', () => c.benchmem, (v) => (c.benchmem = v)),
          ),
          row('-count', num(() => c.count, (v) => (c.count = v), 'default')),
          row('-timeout', text(() => c.timeout, (v) => (c.timeout = v), '10m')),
          row('-bench', text(() => c.bench, (v) => (c.bench = v), 'Benchmark pattern (optional)')),
          row('-fuzz', text(() => c.fuzz, (v) => (c.fuzz = v), 'Fuzz target pattern (optional)')),
          row('-fuzztime', text(() => c.fuzzTime, (v) => (c.fuzzTime = v), '30s')),
          row('Other test flags', text(() => c.testFlags, (v) => (c.testFlags = v), '-v -failfast')),
        );
        break;
      case 'remote':
        fields.push(
          row('Host', text(() => c.host, (v) => (c.host = v), 'localhost')),
          row('Port', num(() => c.port, (v) => (c.port = v), '2345')),
          h('div', { class: 'rc-help' }, 'Start the program with: dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient'),
        );
        break;
      case 'shell':
        fields.push(
          row('Script file', text(() => c.script, (v) => (c.script = v), 'scripts/dev.sh')),
          row('Script text', area(() => c.scriptText, (v) => (c.scriptText = v), 'Used when no file is set', 4)),
          row('Interpreter', text(() => c.interpreter, (v) => (c.interpreter = v), '/bin/sh')),
          ...processFields(c, 'Script arguments'),
        );
        break;
      case 'make':
        fields.push(
          row('Makefile', text(() => c.makefile, (v) => (c.makefile = v), 'Makefile in the working directory')),
          row('Targets', text(() => c.targets.join(' '), (v) => (c.targets = splitList(v)), 'build test')),
          ...processFields(c, 'Make arguments'),
        );
        break;
    }
    fields.push(h('div', { class: 'rc-section' }, 'Before launch'), beforeLaunch(c));
    form.replaceChildren(...fields);
    void validate();
  };

  const add = (type: RunConfigType): void => {
    const name = uniqueName(`Unnamed ${TYPE_LABEL[type]}`, names());
    const d: Draft = { cfg: emptyRunConfig(type, name), orig: '', dirty: true, deleted: false };
    drafts.push(d);
    current = d;
    renderList();
    renderForm();
  };

  const toolbar = h(
    'div',
    { class: 'rc-toolbar' },
    (() => {
      const b = iconButton('plus', 'Add configuration', () => {
        const r = b.getBoundingClientRect();
        showMenu(
          r.left,
          r.bottom,
          (Object.keys(TYPE_LABEL) as RunConfigType[]).map((t) => ({ label: TYPE_LABEL[t], run: () => add(t) })),
        );
      });
      return b;
    })(),
    iconButton('minus', 'Delete configuration', () => {
      if (!current) return;
      current.deleted = true;
      current = drafts.find((d) => !d.deleted) ?? null;
      renderList();
      renderForm();
    }),
    iconButton('copy', 'Copy configuration', () => {
      if (!current) return;
      const cfg = structuredClone(current.cfg);
      cfg.name = uniqueName(cfg.name, names());
      cfg.temporary = false;
      const d: Draft = { cfg, orig: '', dirty: true, deleted: false };
      drafts.push(d);
      current = d;
      renderList();
      renderForm();
    }),
    iconButton('save', 'Save temporary configuration', () => {
      if (!current?.cfg.temporary) return;
      current.cfg.temporary = false;
      changed();
    }),
  );

  renderList();
  renderForm();

  const body = h('div', { class: 'rc-dialog' }, h('div', { class: 'rc-left' }, toolbar, list), h('div', { class: 'rc-right' }, form, errors));

  // OK applies; the loop keeps the dialog open when saving fails.
  for (;;) {
    const action = await showDialog<'ok' | 'cancel'>({
      title: 'Run/Debug Configurations',
      body,
      cancelValue: 'cancel',
      buttons: [
        { label: 'Cancel', value: 'cancel' },
        { label: 'OK', value: 'ok', primary: true },
      ],
    });
    if (action === 'cancel') return;
    try {
      for (const d of drafts) {
        if (d.deleted && d.orig) await run.deleteConfig(d.orig);
      }
      for (const d of drafts) {
        if (d.deleted || !d.dirty) continue;
        const saved = await run.saveConfig(d.cfg, d.orig);
        if (current === d) selectedName.set(saved.name);
        d.orig = saved.name;
        d.dirty = false;
      }
      configs.set(await run.configs());
      return;
    } catch (err) {
      notifyError('run-config-save', 'Cannot save configuration', err);
    }
  }
}
