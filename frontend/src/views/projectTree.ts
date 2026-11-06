// Project tree panel: virtualized, lazily loaded view of the content roots
// with file operations and live updates from fs.changed.

import { elementScroll, observeElementOffset, observeElementRect, Virtualizer } from '@tanstack/virtual-core';
import type { VirtualizerOptions } from '@tanstack/virtual-core';
import { confirmDialog, promptDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { appInfo, isMac, project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { app, files, on, Topics, workspaceApi } from '../lib/api';
import type { FsBatch, Project } from '../lib/api';
import { commands } from '../lib/commands';
import { h } from '../lib/dom';
import { effect } from '../lib/store';
import {
  ancestorsWithin,
  applyFsEvents,
  baseName,
  emptyState,
  isWithin,
  join,
  moveSelection,
  parentOf,
  relativeTo,
  rowIndex,
  visibleRows,
} from './treeModel';
import type { Entry, Row, TreeRoot, TreeState } from './treeModel';
import './projectTree.css';

const ROW_HEIGHT = 22;
const FS_DEBOUNCE_MS = 50;

interface SavedTree {
  root: string;
  expanded: string[];
  selected: string | null;
}

const glyphs: Record<string, string> = {
  go: 'go',
  mod: 'md',
  sum: 'sm',
  work: 'wk',
  md: '¶',
  json: '{}',
  yaml: '≡',
  yml: '≡',
  toml: '≡',
  ts: 'ts',
  js: 'js',
  html: '<>',
  css: '#',
  sql: 'db',
  sh: '$',
  proto: 'pb',
  tmpl: '{{',
  gotmpl: '{{',
};

function glyphFor(name: string): string {
  if (name === 'Dockerfile' || name.startsWith('Dockerfile.')) return 'dk';
  if (name === 'Makefile') return 'mk';
  const ext = name.includes('.') ? name.slice(name.lastIndexOf('.') + 1).toLowerCase() : '';
  return glyphs[ext] ?? '·';
}

export function installProjectTree(wb: Workbench): void {
  const tree = new ProjectTree(wb);
  wb.panels.register({ id: 'project', title: 'Project', dock: 'left', order: 0, render: (c) => tree.mount(c) });
  commands.register({
    id: 'tree.focus',
    title: 'Focus Project Tree',
    category: 'View',
    run: () => {
      wb.panels.show('project');
      tree.focus();
    },
  });
  commands.register({
    id: 'tree.revealActive',
    title: 'Select in Project Tree',
    category: 'View',
    run: (path?: unknown) => {
      if (typeof path === 'string') {
        wb.panels.show('project');
        void tree.reveal(path);
      }
    },
  });
}

class ProjectTree {
  private state: TreeState = emptyState();
  private roots: TreeRoot[] = [];
  private rows: Row[] = [];
  private projectRoot: string | null = null;
  private pendingReload = new Set<string>();
  private reloadTimer: ReturnType<typeof setTimeout> | null = null;

  private scrollEl = h('div', { class: 'tree-scroll', tabindex: '0', role: 'tree', 'aria-label': 'Project files' });
  private sizer = h('div', { class: 'tree-sizer' });
  private virtualizer: Virtualizer<HTMLElement, HTMLElement> | null = null;
  private vopts: VirtualizerOptions<HTMLElement, HTMLElement> | null = null;

  constructor(private wb: Workbench) {
    this.scrollEl.append(this.sizer);
    this.scrollEl.addEventListener('keydown', (e) => this.onKey(e));
    effect(() => this.setProject(project()));
    on<FsBatch>(Topics.fsChanged, (b) => this.onFs(b));
    wb.layout.register<SavedTree>('projectTree', {
      save: () => ({
        root: this.projectRoot ?? '',
        expanded: [...this.state.expanded],
        selected: this.state.selected,
      }),
      restore: (v) => this.restore(v),
    });
  }

  mount(container: HTMLElement): () => void {
    const toolbar = h(
      'div',
      { class: 'panel-toolbar tree-toolbar' },
      h('button', { class: 'link-btn', title: 'Add a folder outside the project', onClick: () => void this.addContentRoot() }, 'Add Content Root…'),
      h('button', { class: 'link-btn', onClick: () => this.collapseAll() }, 'Collapse All'),
    );
    container.classList.add('tree-panel');
    container.append(toolbar, this.scrollEl);
    this.vopts = {
      count: this.rows.length,
      getScrollElement: () => this.scrollEl,
      estimateSize: () => ROW_HEIGHT,
      observeElementRect,
      observeElementOffset,
      scrollToFn: elementScroll,
      overscan: 12,
      onChange: () => this.paint(),
    };
    this.virtualizer = new Virtualizer(this.vopts);
    const cleanup = this.virtualizer._didMount();
    this.virtualizer._willUpdate();
    this.paint();
    return cleanup;
  }

  focus(): void {
    this.scrollEl.focus();
  }

  // --- state transitions -------------------------------------------------

  private setProject(p: Project | null): void {
    if (!p) {
      this.projectRoot = null;
      this.roots = [];
      this.state = emptyState();
      this.refresh();
      return;
    }
    this.roots = p.contentRoots.map((r, i) => ({ path: r, label: i === 0 ? p.name : baseName(r), extra: i > 0 }));
    if (p.root !== this.projectRoot) {
      this.projectRoot = p.root;
      this.state = emptyState();
      this.state.selected = p.root;
      void this.expand(p.root);
    }
    // Drop state for roots that went away.
    for (const dir of [...this.state.expanded]) {
      if (!this.roots.some((r) => isWithin(dir, r.path))) this.state.expanded.delete(dir);
    }
    this.refresh();
  }

  private restore(v: SavedTree): void {
    if (!v || !this.projectRoot || v.root !== this.projectRoot) return;
    const inRoots = (p: string): boolean => this.roots.some((r) => isWithin(p, r.path));
    for (const dir of v.expanded ?? []) if (inRoots(dir)) void this.expand(dir, false);
    if (v.selected && inRoots(v.selected)) this.state.selected = v.selected;
    this.refresh();
  }

  private async load(dir: string): Promise<void> {
    if (!Array.isArray(this.state.children.get(dir))) this.state.children.set(dir, 'loading');
    this.refresh();
    try {
      const entries: Entry[] = await files.list(dir);
      if (this.state.children.has(dir) || this.state.expanded.has(dir)) this.state.children.set(dir, entries);
    } catch (err) {
      this.state.children.set(dir, []);
      this.error('Cannot list folder', err);
    }
    this.refresh();
  }

  private async expand(dir: string, save = true): Promise<void> {
    this.state.expanded.add(dir);
    if (save) this.wb.layout.scheduleSave();
    if (!Array.isArray(this.state.children.get(dir))) await this.load(dir);
    else this.refresh();
  }

  private collapse(dir: string): void {
    this.state.expanded.delete(dir);
    this.wb.layout.scheduleSave();
    this.refresh();
  }

  private toggle(row: Row): void {
    if (row.expanded) this.collapse(row.path);
    else void this.expand(row.path);
  }

  private collapseAll(): void {
    this.state.expanded = new Set(this.roots.map((r) => r.path));
    this.wb.layout.scheduleSave();
    this.refresh();
  }

  private select(path: string | null, scroll = true): void {
    this.state.selected = path;
    this.wb.layout.scheduleSave();
    this.refresh();
    if (scroll && this.virtualizer) {
      const i = rowIndex(this.rows, path);
      if (i >= 0) this.virtualizer.scrollToIndex(i, { align: 'auto' });
    }
  }

  async reveal(path: string): Promise<void> {
    const root = this.roots.find((r) => isWithin(path, r.path));
    if (!root) return;
    await this.expand(root.path, false);
    for (const dir of ancestorsWithin(root.path, path)) await this.expand(dir, false);
    this.select(path);
    this.focus();
  }

  private onFs(batch: FsBatch): void {
    const { state, reload } = applyFsEvents(this.state, batch.events);
    this.state = state;
    for (const d of reload) this.pendingReload.add(d);
    if (this.reloadTimer) clearTimeout(this.reloadTimer);
    this.reloadTimer = setTimeout(() => {
      this.reloadTimer = null;
      const dirs = [...this.pendingReload];
      this.pendingReload.clear();
      for (const d of dirs) if (Array.isArray(this.state.children.get(d))) void this.load(d);
    }, FS_DEBOUNCE_MS);
    this.refresh();
  }

  // --- rendering ---------------------------------------------------------

  private refresh(): void {
    this.rows = visibleRows(this.roots, this.state);
    if (this.virtualizer && this.vopts) {
      this.vopts = { ...this.vopts, count: this.rows.length };
      this.virtualizer.setOptions(this.vopts);
      this.virtualizer._willUpdate();
    }
    this.paint();
  }

  private paint(): void {
    if (!this.virtualizer) return;
    this.sizer.style.height = `${this.virtualizer.getTotalSize()}px`;
    const items = this.virtualizer.getVirtualItems();
    this.sizer.replaceChildren(
      ...items.map((it) => {
        const row = this.rows[it.index];
        return row ? this.renderRow(row, it.start) : h('div');
      }),
    );
  }

  private renderRow(row: Row, top: number): HTMLElement {
    const selected = row.path === this.state.selected;
    const el = h(
      'div',
      {
        class: `tree-row${selected ? ' selected' : ''}${row.loading ? ' loading' : ''}${row.root ? ' root' : ''}`,
        role: 'treeitem',
        'aria-level': String(row.depth + 1),
        'aria-selected': String(selected),
        'aria-expanded': row.isDir ? String(row.expanded) : undefined,
        style: `transform: translateY(${top}px); padding-left: ${6 + row.depth * 14}px`,
        title: row.loading ? undefined : row.path,
      },
      h(
        'span',
        {
          class: 'tree-chevron',
          onClick: (e: MouseEvent) => {
            e.stopPropagation();
            this.toggle(row);
          },
        },
        row.isDir ? (row.expanded ? '▾' : '▸') : '',
      ),
      row.loading ? null : h('span', { class: `tree-glyph${row.isDir ? ' dir' : ''}` }, row.isDir ? '' : glyphFor(row.name)),
      h('span', { class: 'tree-name' }, row.name),
      row.root?.extra ? h('span', { class: 'tree-badge' }, 'content root') : null,
    );
    if (row.loading) return el;
    el.addEventListener('mousedown', () => this.select(row.path, false));
    el.addEventListener('dblclick', () => this.activate(row));
    el.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      this.select(row.path, false);
      showMenu(e.clientX, e.clientY, this.menuFor(row));
    });
    return el;
  }

  // --- interaction -------------------------------------------------------

  private selectedRow(): Row | undefined {
    return this.rows.find((r) => r.path === this.state.selected && !r.loading);
  }

  private activate(row: Row): void {
    if (row.isDir) this.toggle(row);
    else this.open(row.path);
  }

  private open(path: string): void {
    if (commands.get('editor.open')) void commands.run('editor.open', path);
  }

  private onKey(e: KeyboardEvent): void {
    const row = this.selectedRow();
    switch (e.key) {
      case 'ArrowDown':
        this.select(moveSelection(this.rows, this.state.selected, 1));
        break;
      case 'ArrowUp':
        this.select(moveSelection(this.rows, this.state.selected, -1));
        break;
      case 'ArrowRight':
        if (row?.isDir && !row.expanded) void this.expand(row.path);
        else this.select(moveSelection(this.rows, this.state.selected, 1));
        break;
      case 'ArrowLeft':
        if (row?.isDir && row.expanded) this.collapse(row.path);
        else if (row && !row.root) this.select(parentOf(row.path));
        break;
      case 'Enter':
        if (row) this.activate(row);
        break;
      case 'F2':
        if (row && !row.root) void this.rename(row);
        break;
      case 'Delete':
        if (row && !row.root) void this.trash(row);
        break;
      case 'Backspace':
        if (row && !row.root && (!isMac() || e.metaKey)) void this.trash(row);
        break;
      default:
        return;
    }
    e.preventDefault();
    e.stopPropagation();
  }

  private menuFor(row: Row): MenuItem[] {
    const dir = row.isDir ? row.path : parentOf(row.path);
    const os = appInfo.peek()?.os ?? '';
    const revealLabel = os === 'darwin' ? 'Reveal in Finder' : os === 'windows' ? 'Show in Explorer' : 'Open in File Manager';
    const items: MenuItem[] = [
      { label: 'New File…', run: () => void this.create(dir, 'file') },
      { label: 'New Directory…', run: () => void this.create(dir, 'dir') },
      { label: 'New Go File…', run: () => void this.create(dir, 'go') },
      { separator: true },
      { label: 'Rename…', hint: 'F2', disabled: !!row.root, run: () => void this.rename(row) },
      { label: 'Move to Trash', danger: true, disabled: !!row.root, run: () => void this.trash(row) },
      { separator: true },
      { label: 'Copy Path', run: () => void files.copyPath(row.path).catch((err) => this.error('Copy failed', err)) },
      { label: revealLabel, run: () => void files.reveal(row.path).catch((err) => this.error('Reveal failed', err)) },
    ];
    if (row.isDir && !row.root) {
      items.push({ separator: true }, { label: 'Exclude from Project', run: () => void this.exclude(row.path) });
    }
    if (row.root?.extra) {
      items.push(
        { separator: true },
        {
          label: 'Remove Content Root',
          run: () => void workspaceApi.removeContentRoot(row.path).catch((err) => this.error('Cannot remove content root', err)),
        },
      );
    }
    return items;
  }

  private async create(dir: string, kind: 'file' | 'dir' | 'go'): Promise<void> {
    const title = kind === 'dir' ? 'New Directory' : kind === 'go' ? 'New Go File' : 'New File';
    const name = await promptDialog(title, '', validName, kind === 'go' ? 'File name (".go" is added)' : 'Name');
    if (!name) return;
    try {
      let path: string;
      if (kind === 'go') {
        path = await files.createGoFile(dir, name);
      } else {
        path = join(dir, name);
        await files.create(path, kind === 'dir', '');
      }
      await this.expand(dir);
      await this.load(dir);
      this.select(path);
      if (kind !== 'dir') this.open(path);
    } catch (err) {
      this.error(`Cannot create ${kind === 'dir' ? 'directory' : 'file'}`, err);
    }
  }

  private async rename(row: Row): Promise<void> {
    const name = await promptDialog('Rename', row.name, validName);
    if (!name || name === row.name) return;
    const to = join(parentOf(row.path), name);
    try {
      await files.rename(row.path, to);
      this.select(to);
    } catch (err) {
      this.error('Cannot rename', err);
    }
  }

  private async trash(row: Row): Promise<void> {
    const ok = await confirmDialog('Move to Trash', `Move "${row.name}" to the trash?`, 'Move to Trash', true);
    if (!ok) return;
    try {
      await files.trash(row.path);
      if (this.state.selected === row.path) this.select(parentOf(row.path));
    } catch (err) {
      const unavailable = /trash unavailable/i.test(String(err));
      this.error(unavailable ? 'Trash is not available for this location' : 'Cannot move to trash', err);
    }
  }

  private async exclude(path: string): Promise<void> {
    const p = project.peek();
    const root = this.roots.find((r) => isWithin(path, r.path));
    if (!p || !root) return;
    const rel = relativeTo(root.path, path);
    if (!rel || p.excludes.includes(rel)) return;
    try {
      await workspaceApi.setExcludes([...p.excludes, rel]);
    } catch (err) {
      this.error('Cannot exclude folder', err);
    }
  }

  private async addContentRoot(): Promise<void> {
    try {
      const dir = await app.openFolderDialog();
      if (dir) await workspaceApi.addContentRoot(dir);
    } catch (err) {
      this.error('Cannot add content root', err);
    }
  }

  private error(title: string, err: unknown): void {
    this.wb.notifications.push({ id: `tree:${title}`, level: 'error', title, body: String(err) });
  }
}

function validName(v: string): string | null {
  const s = v.trim();
  if (!s) return 'Name is required';
  if (/[\\/]/.test(s)) return 'Name cannot contain path separators';
  if (s === '.' || s === '..') return 'Invalid name';
  return null;
}
