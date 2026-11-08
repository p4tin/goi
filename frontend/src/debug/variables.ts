// Variables tree (R9): lazy children, display options (types, addresses,
// unreadable values), Set Value, View as String, Copy Value and Add to
// Watches. Used by the Variables view, Watches and Evaluate.

import { showDialog } from '../app/dialog';
import { showMenu } from '../app/menu';
import type { MenuItem } from '../app/menu';
import { debug } from '../lib/api';
import type { DapVariable } from '../lib/api';
import { h } from '../lib/dom';
import { formatValue, isByteSlice, isUnreadable } from './model';

export interface VarNode {
  /** Stable key for keeping expansion across refreshes. */
  key: string;
  name: string;
  value: string;
  type: string;
  evaluateName: string;
  /** variablesReference of this node (children when > 0). */
  ref: number;
  /** variablesReference of the container (for setVariable); 0 = none. */
  parentRef: number;
  children: VarNode[] | null;
  expanded: boolean;
  loading: boolean;
  error: string;
  /** Watch rows know their index; scopes are headers. */
  kind: 'var' | 'scope' | 'watch';
  watchIndex: number;
}

export interface VarTreeOptions {
  sessionId: () => string;
  showTypes: () => boolean;
  showAddresses: () => boolean;
  showUnreadable: () => boolean;
  canSetValue: () => boolean;
  frameId: () => number;
  onAddWatch?: (expr: string) => void;
  onRemoveWatch?: (index: number) => void;
  onEditWatch?: (index: number) => void;
  /** Called after a value changed (refresh watches, inline values). */
  onChanged?: () => void;
  emptyText?: string;
}

export function varNode(v: DapVariable, parentKey: string, parentRef: number): VarNode {
  return {
    key: `${parentKey}/${v.name}`,
    name: v.name,
    value: v.value,
    type: v.type,
    evaluateName: v.evaluateName,
    ref: v.variablesReference,
    parentRef,
    children: null,
    expanded: false,
    loading: false,
    error: '',
    kind: 'var',
    watchIndex: -1,
  };
}

export class VarTree {
  readonly el: HTMLElement;
  private roots: VarNode[] = [];
  private expandedKeys = new Set<string>();
  private selectedKey = '';
  private byKey = new Map<string, VarNode>();

  constructor(private opts: VarTreeOptions) {
    this.el = h('div', { class: 'var-tree', role: 'tree', tabindex: '0' });
    this.el.addEventListener('click', (e) => {
      const n = this.nodeAt(e);
      if (!n) return;
      this.selectedKey = n.key;
      if ((e.target as HTMLElement).classList.contains('vt-twisty')) void this.toggle(n);
      else this.render();
    });
    this.el.addEventListener('dblclick', (e) => {
      const n = this.nodeAt(e);
      if (!n) return;
      if (n.kind === 'watch' && (e.target as HTMLElement).closest('.vt-name')) this.opts.onEditWatch?.(n.watchIndex);
      else if ((e.target as HTMLElement).closest('.vt-value')) this.editValue(n);
      else if (n.ref > 0) void this.toggle(n);
    });
    this.el.addEventListener('contextmenu', (e) => {
      const n = this.nodeAt(e);
      if (!n) return;
      e.preventDefault();
      this.selectedKey = n.key;
      this.render();
      showMenu(e.clientX, e.clientY, this.menu(n));
    });
    this.el.addEventListener('keydown', (e) => {
      const n = this.byKey.get(this.selectedKey);
      const rows = [...this.el.querySelectorAll<HTMLElement>('.vt-row')].map((r) => r.dataset.key ?? '');
      const i = rows.indexOf(this.selectedKey);
      if (e.key === 'ArrowDown' && i < rows.length - 1) this.selectedKey = rows[i + 1];
      else if (e.key === 'ArrowUp' && i > 0) this.selectedKey = rows[i - 1];
      else if (e.key === 'ArrowRight' && n && n.ref > 0 && !n.expanded) void this.toggle(n);
      else if (e.key === 'ArrowLeft' && n?.expanded) void this.toggle(n);
      else if (e.key === 'F2' && n) this.editValue(n);
      else if ((e.key === 'Delete' || e.key === 'Backspace') && n?.kind === 'watch') this.opts.onRemoveWatch?.(n.watchIndex);
      else return;
      e.preventDefault();
      this.render();
    });
  }

  /** Replaces the roots; previously expanded nodes re-expand. */
  setRoots(roots: VarNode[]): void {
    this.roots = roots;
    for (const r of roots) if (this.expandedKeys.has(r.key) || (r.kind === 'scope' && !this.expandedKeys.has(`!${r.key}`))) void this.expand(r);
    this.render();
  }

  clear(text = ''): void {
    this.roots = [];
    this.opts.emptyText = text || this.opts.emptyText;
    this.render();
  }

  private nodeAt(e: Event): VarNode | undefined {
    const row = (e.target as HTMLElement).closest('.vt-row') as HTMLElement | null;
    return row ? this.byKey.get(row.dataset.key ?? '') : undefined;
  }

  private async toggle(n: VarNode): Promise<void> {
    if (n.expanded) {
      n.expanded = false;
      this.expandedKeys.delete(n.key);
      if (n.kind === 'scope') this.expandedKeys.add(`!${n.key}`);
      this.render();
      return;
    }
    if (n.kind === 'scope') this.expandedKeys.delete(`!${n.key}`);
    await this.expand(n);
  }

  private async expand(n: VarNode): Promise<void> {
    if (n.ref <= 0) return;
    n.expanded = true;
    this.expandedKeys.add(n.key);
    if (n.children) {
      this.render();
      for (const c of n.children) if (this.expandedKeys.has(c.key)) void this.expand(c);
      return;
    }
    n.loading = true;
    this.render();
    try {
      const vars = await debug.variables(this.opts.sessionId(), n.ref);
      n.children = vars.map((v) => varNode(v, n.key, n.ref));
    } catch (err) {
      n.error = err instanceof Error ? err.message : String(err);
      n.children = [];
    }
    n.loading = false;
    this.render();
    for (const c of n.children) if (this.expandedKeys.has(c.key)) void this.expand(c);
  }

  private menu(n: VarNode): MenuItem[] {
    const items: MenuItem[] = [];
    if (n.kind !== 'scope') {
      items.push({ label: 'Copy Value', run: () => void navigator.clipboard?.writeText(n.value) });
      if (n.evaluateName || n.kind === 'watch') items.push({ label: 'Copy Name', run: () => void navigator.clipboard?.writeText(n.evaluateName || n.name) });
      if (isByteSlice(n.type)) items.push({ label: 'View as String', run: () => void this.viewAsString(n) });
      if (n.kind === 'var' && this.opts.canSetValue() && n.parentRef > 0) items.push({ label: 'Set Value…', hint: 'F2', run: () => this.editValue(n) });
      if (n.kind === 'var' && n.evaluateName && this.opts.onAddWatch) items.push({ label: 'Add to Watches', run: () => this.opts.onAddWatch!(n.evaluateName) });
      if (n.kind === 'watch') {
        items.push({ separator: true }, { label: 'Edit Watch', run: () => this.opts.onEditWatch?.(n.watchIndex) }, { label: 'Remove Watch', hint: 'Delete', run: () => this.opts.onRemoveWatch?.(n.watchIndex) });
      }
    }
    if (n.ref > 0) items.push({ separator: true }, { label: n.expanded ? 'Collapse' : 'Expand', run: () => void this.toggle(n) });
    return items;
  }

  private async viewAsString(n: VarNode): Promise<void> {
    const expr = `string(${n.evaluateName || n.name})`;
    let text: string;
    try {
      const r = await debug.evaluate(this.opts.sessionId(), expr, this.opts.frameId(), 'clipboard');
      text = r.result.startsWith('"') ? (JSON.parse(r.result.replace(/\\x([0-9a-fA-F]{2})/g, '\\u00$1')) as string) : r.result;
    } catch (err) {
      text = `Cannot evaluate ${expr}: ${err instanceof Error ? err.message : String(err)}`;
    }
    const area = h('textarea', { class: 'input vt-string', readonly: true, rows: 12, spellcheck: false });
    area.value = text;
    await showDialog({ title: `${n.name} as string`, body: area, cancelValue: undefined, buttons: [{ label: 'Close', value: undefined, primary: true }] });
  }

  private editValue(n: VarNode): void {
    if (n.kind !== 'var' || n.parentRef <= 0 || !this.opts.canSetValue()) return;
    const row = this.el.querySelector<HTMLElement>(`.vt-row[data-key="${CSS.escape(n.key)}"] .vt-value`);
    if (!row) return;
    const input = h('input', { class: 'input vt-edit', type: 'text', value: n.value, spellcheck: false });
    row.replaceChildren(input);
    input.focus();
    input.select();
    let done = false;
    const finish = async (commit: boolean): Promise<void> => {
      if (done) return;
      done = true;
      if (commit && input.value !== n.value) {
        try {
          const r = await debug.setVariable(this.opts.sessionId(), n.parentRef, n.name, input.value);
          n.value = r.value;
          n.ref = r.variablesReference;
          n.children = null;
          n.error = '';
          this.opts.onChanged?.();
        } catch (err) {
          n.error = err instanceof Error ? err.message : String(err);
        }
      }
      this.render();
      this.el.focus();
    };
    input.addEventListener('keydown', (e) => {
      e.stopPropagation();
      if (e.key === 'Enter') void finish(true);
      else if (e.key === 'Escape') void finish(false);
    });
    input.addEventListener('blur', () => void finish(false));
  }

  render(): void {
    this.byKey.clear();
    const rows: HTMLElement[] = [];
    const types = this.opts.showTypes();
    const addrs = this.opts.showAddresses();
    const unreadable = this.opts.showUnreadable();
    const add = (n: VarNode, depth: number): void => {
      if (!unreadable && n.kind === 'var' && isUnreadable(n.value)) return;
      this.byKey.set(n.key, n);
      const hasKids = n.ref > 0;
      const value = n.kind === 'scope' ? '' : formatValue(n.value, { showAddresses: addrs });
      rows.push(
        h(
          'div',
          {
            class: `vt-row vt-${n.kind}${n.key === this.selectedKey ? ' selected' : ''}`,
            role: 'treeitem',
            dataset: { key: n.key },
            'aria-expanded': hasKids ? String(n.expanded) : undefined,
            style: `padding-left:${4 + depth * 14}px`,
            title: n.error || (n.kind === 'scope' ? n.name : `${n.name}${n.type ? ` (${n.type})` : ''} = ${n.value}`),
          },
          h('span', { class: `vt-twisty${hasKids ? '' : ' leaf'}` }, hasKids ? (n.expanded ? '▾' : '▸') : ''),
          h('span', { class: 'vt-name' }, n.name),
          types && n.type && n.kind !== 'scope' ? h('span', { class: 'vt-type' }, n.type) : null,
          n.kind !== 'scope' ? h('span', { class: 'vt-eq' }, '=') : null,
          h('span', { class: `vt-value${n.error ? ' vt-error' : ''}` }, n.error && n.kind === 'watch' ? n.error : value),
          n.loading ? h('span', { class: 'vt-loading' }, '…') : null,
        ),
      );
      if (n.expanded && n.children) {
        for (const c of n.children) add(c, depth + 1);
        if (n.error && n.kind !== 'watch') rows.push(h('div', { class: 'vt-row vt-error', style: `padding-left:${18 + depth * 14}px` }, n.error));
      }
    };
    for (const r of this.roots) add(r, 0);
    if (!rows.length) rows.push(h('div', { class: 'results-empty' }, this.opts.emptyText ?? ''));
    this.el.replaceChildren(...rows);
  }
}
