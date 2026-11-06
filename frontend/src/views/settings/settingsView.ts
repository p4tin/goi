// Settings view (R13.3): searchable, schema-driven controls for every M1
// setting with per-row origin badges and a target layer (user/project).

import './settings.css';
import { project, settings } from '../../app/state';
import type { Workbench } from '../../app/workbench';
import { on, settingsApi, Topics, workspaceApi } from '../../lib/api';
import type { SettingsLayer } from '../../lib/api';
import { commands } from '../../lib/commands';
import { Disposables, h } from '../../lib/dom';
import { effect, signal } from '../../lib/store';
import { renderKeymapEditor } from './keymapEditor';
import { filterSchema, getByKey, parseIntInput, sections, validate } from './schema';
import type { Section, SettingDescriptor, WritableLayer } from './schema';

const DEBOUNCE_MS = 400;

/** Origin of a key: exact entry, else the highest layer among its leaves. */
export function originOf(origins: Record<string, SettingsLayer>, key: string): SettingsLayer {
  if (origins[key]) return origins[key];
  let best: SettingsLayer = 'default';
  const rank = { default: 0, user: 1, project: 2 } as const;
  for (const [k, l] of Object.entries(origins)) {
    if (k.startsWith(`${key}.`) && rank[l] > rank[best]) best = l;
  }
  return best;
}

/** "KEY=value" lines to a map; returns an error string for bad lines. */
export function parseEnvLines(text: string): Record<string, string> | string {
  const out: Record<string, string> = {};
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line) continue;
    const i = line.indexOf('=');
    if (i <= 0) return `Expected KEY=value, got "${line}"`;
    out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return out;
}

export function listLines(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s !== '');
}

interface Row {
  el: HTMLElement;
  update(): void;
}

export function installSettings(wb: Workbench): void {
  let close: (() => void) | null = null;
  commands.register({
    id: 'settings.open',
    title: 'Settings…',
    category: 'File',
    run: (key) => {
      if (close) {
        if (typeof key === 'string') focusKey(key);
        return;
      }
      close = openView(typeof key === 'string' ? key : undefined, () => {
        close = null;
      });
    },
  });
  void wb;
}

let focusKey: (key: string) => void = () => {};

function openView(initialKey: string | undefined, onClosed: () => void): () => void {
  const d = new Disposables();
  const query = signal('');
  const origins = signal<Record<string, SettingsLayer>>({});
  const layerChoice = new Map<string, WritableLayer>();
  const refreshOrigins = (): void => {
    settingsApi
      .origins()
      .then((o) => origins.set(o))
      .catch(() => {});
  };
  refreshOrigins();
  d.add(on(Topics.settingsChanged, refreshOrigins));

  const search = h('input', {
    class: 'input settings-search',
    type: 'search',
    placeholder: 'Search settings',
    'aria-label': 'Search settings',
    spellcheck: false,
  });
  search.addEventListener('input', () => query.set(search.value));

  const content = h('div', { class: 'settings-content' });
  const nav = h(
    'nav',
    { class: 'settings-nav', 'aria-label': 'Settings sections' },
    sections.map((s) =>
      h('button', { class: 'settings-nav-item', onClick: () => content.querySelector(`[data-section="${s}"]`)?.scrollIntoView({ block: 'start' }) }, s),
    ),
  );
  const footer = h('footer', { class: 'settings-footer' });
  const closeBtn = h('button', { class: 'btn', onClick: () => doClose() }, 'Close');
  const view = h(
    'div',
    { class: 'settings-overlay', role: 'dialog', 'aria-label': 'Settings' },
    h('header', { class: 'settings-header' }, h('h1', null, 'Settings'), search, closeBtn),
    h('div', { class: 'settings-body' }, nav, content),
    footer,
  );
  document.body.appendChild(view);

  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && !document.querySelector('.dialog-backdrop')) {
      e.preventDefault();
      e.stopPropagation();
      doClose();
    }
  };
  view.addEventListener('keydown', onKey);

  function doClose(): void {
    d.dispose();
    view.remove();
    onClosed();
  }

  // Footer links to the settings files.
  settingsApi
    .paths()
    .then((p) => {
      footer.replaceChildren(
        h('button', { class: 'link-btn', onClick: () => openFile(p.user) }, 'Open user settings file'),
        h(
          'button',
          { class: 'link-btn', disabled: !p.project, title: p.project ? p.project : 'No project open', onClick: () => openFile(p.project) },
          'Open project settings file',
        ),
      );
    })
    .catch(() => {});
  const openFile = (path: string): void => {
    if (!path) return;
    doClose();
    void commands.run('editor.open', path);
  };

  // Rows are rebuilt when the query changes; values refresh in place.
  let rows: Row[] = [];
  let keymapStop: (() => void) | null = null;
  const keymapQuery = signal('');
  d.add(
    effect(() => {
      const q = query();
      keymapQuery.set(q);
      keymapStop?.();
      keymapStop = null;
      const matches = filterSchema(q);
      rows = [];
      const groups: HTMLElement[] = [];
      for (const s of sections) {
        const items = matches.filter((m) => m.section === s);
        const showKeymap = s === 'Keymap';
        if (!items.length && !showKeymap) continue;
        const group = h('section', { class: 'settings-group', dataset: { section: s } }, h('h2', null, s));
        for (const desc of items) {
          const row = buildRow(desc, origins, layerChoice);
          rows.push(row);
          group.append(row.el);
        }
        if (showKeymap) {
          const host = h('div', { class: 'keymap-host' });
          group.append(host);
          keymapStop = renderKeymapEditor(host, keymapQuery);
        }
        groups.push(group);
      }
      content.replaceChildren(...(groups.length ? groups : [h('p', { class: 'muted' }, 'No settings match your search.')]));
      for (const r of rows) r.update();
    }),
  );
  d.add(() => keymapStop?.());
  d.add(
    effect(() => {
      settings();
      origins();
      project();
      for (const r of rows) r.update();
    }),
  );

  focusKey = (key: string): void => {
    const row = content.querySelector<HTMLElement>(`[data-key="${CSS.escape(key)}"]`);
    if (!row) return;
    row.scrollIntoView({ block: 'center' });
    row.classList.add('settings-row-highlight');
    setTimeout(() => row.classList.remove('settings-row-highlight'), 2000);
    row.querySelector<HTMLElement>('input, select, textarea, button')?.focus();
  };
  if (initialKey) queueMicrotask(() => focusKey(initialKey));
  else search.focus();

  return doClose;
}

function buildRow(desc: SettingDescriptor, origins: { (): Record<string, SettingsLayer>; peek(): Record<string, SettingsLayer> }, layerChoice: Map<string, WritableLayer>): Row {
  const id = `setting-${desc.key.replace(/\./g, '-')}`;
  const error = h('div', { class: 'settings-error', role: 'alert' });
  const badge = h('span', { class: 'badge' });
  const layerSel = h(
    'select',
    { class: 'input settings-layer', 'aria-label': `Write ${desc.title} to` },
    h('option', { value: 'user' }, 'User'),
    h('option', { value: 'project' }, 'Project'),
  );
  const reset = h('button', { class: 'link-btn settings-reset', 'aria-label': `Reset ${desc.title}` }, 'Reset');

  const chosenLayer = (): WritableLayer => {
    const allowedProject = desc.layers.includes('project') && !!project.peek();
    const fromMap = layerChoice.get(desc.key);
    if (fromMap === 'project' && allowedProject) return 'project';
    if (fromMap === 'user' && desc.layers.includes('user')) return 'user';
    if (!desc.layers.includes('user')) return 'project';
    return allowedProject && originOf(origins.peek(), desc.key) === 'project' ? 'project' : 'user';
  };
  layerSel.addEventListener('change', () => {
    layerChoice.set(desc.key, layerSel.value as WritableLayer);
    update();
  });

  const write = async (value: unknown): Promise<void> => {
    const msg = validate(desc, value);
    error.textContent = msg ?? '';
    if (msg) return;
    try {
      await settingsApi.set(chosenLayer(), desc.key, value);
    } catch (err) {
      error.textContent = String(err);
    }
  };
  reset.addEventListener('click', () => {
    error.textContent = '';
    settingsApi.set(chosenLayer(), desc.key, null).catch((err: unknown) => (error.textContent = String(err)));
  });

  let timer: ReturnType<typeof setTimeout> | undefined;
  const debounced = (fn: () => void): void => {
    clearTimeout(timer);
    timer = setTimeout(fn, DEBOUNCE_MS);
  };

  const { control, setValue } = buildControl(desc, id, write, debounced, error);

  const el = h(
    'div',
    { class: 'settings-row', dataset: { key: desc.key } },
    h(
      'div',
      { class: 'settings-label' },
      h('label', { for: id }, desc.title),
      h('code', { class: 'settings-key' }, desc.key),
      h('div', { class: 'settings-desc muted' }, desc.description),
    ),
    h('div', { class: 'settings-control' }, control, error),
    h('div', { class: 'settings-meta' }, badge, desc.key === 'project.contentRoots' ? null : layerSel, reset),
  );

  function update(): void {
    const origin = originOf(origins.peek(), desc.key);
    badge.textContent = origin;
    badge.className = `badge badge-${origin}`;
    const layer = chosenLayer();
    layerSel.value = layer;
    const projOpt = layerSel.options[1];
    projOpt.disabled = !project.peek() || !desc.layers.includes('project');
    (layerSel.options[0] as HTMLOptionElement).disabled = !desc.layers.includes('user');
    reset.hidden = origin !== layer || desc.key === 'project.contentRoots';
    setValue(getByKey(settings.peek(), desc.key));
  }
  return { el, update };
}

function buildControl(
  desc: SettingDescriptor,
  id: string,
  write: (v: unknown) => Promise<void>,
  debounced: (fn: () => void) => void,
  error: HTMLElement,
): { control: HTMLElement; setValue(v: unknown): void } {
  const notFocused = (el: HTMLElement): boolean => document.activeElement !== el;
  switch (desc.type) {
    case 'bool': {
      const input = h('input', { id, type: 'checkbox' });
      input.addEventListener('change', () => void write(input.checked));
      return { control: input, setValue: (v) => (input.checked = v === true) };
    }
    case 'enum': {
      const sel = h('select', { id, class: 'input' }, (desc.options ?? []).map((o) => h('option', { value: o.value }, o.label)));
      sel.addEventListener('change', () => void write(sel.value));
      return { control: sel, setValue: (v) => (sel.value = String(v ?? '')) };
    }
    case 'int': {
      const input = h('input', { id, class: 'input settings-int', type: 'number', min: desc.min, max: desc.max, step: 1 });
      input.addEventListener('input', () => debounced(() => void write(parseIntInput(input.value))));
      return { control: input, setValue: (v) => notFocused(input) && (input.value = v === undefined ? '' : String(v)) };
    }
    case 'string': {
      const input = h('input', { id, class: 'input', type: 'text', spellcheck: false });
      input.addEventListener('input', () => debounced(() => void write(input.value)));
      const control =
        desc.key === 'go.goroot'
          ? h('div', { class: 'settings-inline' }, input, h('button', { class: 'btn', onClick: () => void commands.run('go.selectSdk') }, 'Select…'))
          : input;
      return { control, setValue: (v) => notFocused(input) && (input.value = typeof v === 'string' ? v : '') };
    }
    case 'stringList': {
      if (desc.key === 'project.contentRoots') return contentRootsControl(id, error);
      const ta = h('textarea', { id, class: 'input settings-list', rows: 3, spellcheck: false, placeholder: 'One per line' });
      ta.addEventListener('input', () => debounced(() => void write(listLines(ta.value))));
      return { control: ta, setValue: (v) => notFocused(ta) && (ta.value = Array.isArray(v) ? v.join('\n') : '') };
    }
    case 'map': {
      const ta = h('textarea', { id, class: 'input settings-list', rows: 3, spellcheck: false, placeholder: 'KEY=value, one per line' });
      ta.addEventListener('input', () =>
        debounced(() => {
          const parsed = parseEnvLines(ta.value);
          if (typeof parsed === 'string') error.textContent = parsed;
          else void write(parsed);
        }),
      );
      return {
        control: ta,
        setValue: (v) =>
          notFocused(ta) &&
          (ta.value =
            v && typeof v === 'object'
              ? Object.entries(v as Record<string, string>)
                  .map(([k, val]) => `${k}=${val}`)
                  .join('\n')
              : ''),
      };
    }
  }
}

function contentRootsControl(id: string, error: HTMLElement): { control: HTMLElement; setValue(v: unknown): void } {
  const list = h('ul', { id, class: 'settings-roots' });
  const add = h(
    'button',
    {
      class: 'btn',
      onClick: async () => {
        const { app } = await import('../../lib/api');
        const dir = await app.openFolderDialog();
        if (dir) workspaceApi.addContentRoot(dir).catch((err: unknown) => (error.textContent = String(err)));
      },
    },
    'Add…',
  );
  const control = h('div', null, list, add);
  return {
    control,
    setValue: (v) => {
      const roots = Array.isArray(v) ? (v as string[]) : [];
      add.disabled = !project.peek();
      list.replaceChildren(
        ...(roots.length
          ? roots.map((r) =>
              h(
                'li',
                null,
                h('span', { class: 'settings-root-path' }, r),
                h(
                  'button',
                  {
                    class: 'link-btn',
                    'aria-label': `Remove content root ${r}`,
                    onClick: () => workspaceApi.removeContentRoot(r).catch((err: unknown) => (error.textContent = String(err))),
                  },
                  'Remove',
                ),
              ),
            )
          : [h('li', { class: 'muted' }, 'None')]),
      );
    },
  };
}

export type { Section };
