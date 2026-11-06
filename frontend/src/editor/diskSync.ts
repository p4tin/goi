// Keeps open buffers in sync with files changed or deleted outside Goi
// (R6.8): clean buffers reload silently with a minimal diff; dirty ones get a
// conflict banner with Reload / Keep mine / Compare.

import { diff, MergeView } from '@codemirror/merge';
import { EditorState, StateEffect, StateField, Text } from '@codemirror/state';
import type { ChangeSpec } from '@codemirror/state';
import { EditorView, showPanel } from '@codemirror/view';
import type { Panel } from '@codemirror/view';
import { showDialog } from '../app/dialog';
import type { Workbench } from '../app/workbench';
import { files, on, Topics } from '../lib/api';
import type { FileMeta, FsBatch } from '../lib/api';
import { h } from '../lib/dom';
import type { Buffer } from './buffer';
import { editors } from './install';

export type ExternalDecision = 'ignore' | 'reload' | 'conflict' | 'deleted';

export interface BufferSnapshot {
  dirty: boolean;
  savedText: string;
  savedMeta: { modTime: unknown; size: number };
}

export interface DiskSnapshot {
  content: string;
  modTime: unknown;
  size: number;
}

/** Decides how to react to an external change of an open file. */
export function decideExternalChange(state: BufferSnapshot, disk: DiskSnapshot | 'deleted'): ExternalDecision {
  if (disk === 'deleted') return 'deleted';
  const sameMeta = String(disk.modTime) === String(state.savedMeta.modTime) && disk.size === state.savedMeta.size;
  if (sameMeta || disk.content === state.savedText) return 'ignore';
  return state.dirty ? 'conflict' : 'reload';
}

/** Minimal changes turning oldText into newText (keeps cursors stable). */
export function minimalChanges(oldText: string, newText: string): ChangeSpec[] {
  return diff(oldText, newText).map((c) => ({ from: c.fromA, to: c.toA, insert: newText.slice(c.fromB, c.toB) }));
}

interface Banner {
  kind: 'conflict' | 'deleted';
  disk?: { content: string; meta: FileMeta };
}

const setBanner = StateEffect.define<Banner | null>();

const bannerField = StateField.define<Banner | null>({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setBanner)) value = e.value;
    return value;
  },
});

const bannerTheme = EditorView.baseTheme({
  '.cm-goi-banner': {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    padding: '5px 10px',
    fontFamily: 'system-ui, sans-serif',
    fontSize: '12px',
    background: 'var(--goi-panel-bg)',
    color: 'var(--goi-fg)',
    borderBottom: '1px solid var(--goi-warning)',
  },
  '.cm-goi-banner-msg': { flex: '1' },
});

export function installDiskSync(wb: Workbench): void {
  const m = editors;
  const banners = new Map<string, Banner>();
  const notify = (title: string, err: unknown): void =>
    wb.notifications.push({ id: title, level: 'error', title, body: String(err) });

  const show = (buf: Buffer, b: Banner | null): void => {
    if (b) banners.set(buf.path, b);
    else banners.delete(buf.path);
    for (const v of buf.views) v.dispatch({ effects: setBanner.of(b) });
  };

  const actions: BannerActions = {
    reload: (buf, disk) => {
      applyDisk(buf, disk.content, disk.meta);
      show(buf, null);
    },
    keep: (buf) => {
      // Edits stay; the next save overwrites the disk version.
      show(buf, null);
    },
    compare: async (buf, disk) => {
      const choice = await compareDialog(buf, disk.content);
      if (choice === 'disk') actions.reload(buf, disk);
      else if (choice === 'mine') actions.keep(buf);
    },
    closeDeleted: (buf) => {
      show(buf, null);
      for (const p of m.panes.peek()) if (p.has(buf.path)) void m.close(buf.path, p, true);
    },
    keepDeleted: (buf) => {
      show(buf, null);
      buf.dirty.set(true); // saving recreates the file
    },
  };

  m.addExtension((buf) => [
    bannerTheme,
    bannerField.init(() => banners.get(buf.path) ?? null),
    showPanel.from(bannerField, (b) => (b ? (view) => bannerPanel(view, buf, b, actions) : null)),
  ]);
  m.hooks.closed.on((buf) => banners.delete(buf.path));
  m.hooks.saved.on((buf) => {
    if (banners.has(buf.path)) show(buf, null);
  });

  // Serialize checks per path so overlapping events don't race.
  const chains = new Map<string, Promise<void>>();
  const check = (path: string, removed: boolean): void => {
    const prev = chains.get(path) ?? Promise.resolve();
    const next = prev
      .then(() => checkPath(path, removed))
      .catch((err: unknown) => notify(`Cannot reload ${path}`, err))
      .finally(() => {
        if (chains.get(path) === next) chains.delete(path);
      });
    chains.set(path, next);
  };

  const checkPath = async (path: string, removed: boolean): Promise<void> => {
    const buf = m.buffers.get(path);
    if (!buf) return;
    let disk: DiskSnapshot | 'deleted';
    let meta: FileMeta | null = null;
    try {
      const f = await files.read(path);
      meta = f.meta;
      disk = { content: f.content, modTime: f.meta.modTime, size: f.meta.size };
    } catch (err) {
      if (!removed && !/no such file|not exist|cannot find/i.test(String(err))) throw err;
      disk = 'deleted';
    }
    const cur = m.buffers.get(path);
    if (cur !== buf) return; // closed meanwhile
    const decision = decideExternalChange(
      { dirty: buf.dirty.peek(), savedText: buf.saved.toString(), savedMeta: { modTime: buf.meta.modTime, size: buf.meta.size } },
      disk,
    );
    switch (decision) {
      case 'ignore':
        if (banners.get(path)?.kind === 'deleted') show(buf, null);
        return;
      case 'deleted':
        show(buf, { kind: 'deleted' });
        return;
      case 'reload':
        applyDisk(buf, (disk as DiskSnapshot).content, meta!);
        show(buf, null);
        return;
      case 'conflict':
        show(buf, { kind: 'conflict', disk: { content: (disk as DiskSnapshot).content, meta: meta! } });
    }
  };

  on<FsBatch>(Topics.fsChanged, (batch) => {
    const seen = new Map<string, boolean>();
    for (const e of batch?.events ?? []) {
      if (e.isDir || !m.buffers.has(e.path)) continue;
      seen.set(e.path, e.op === 'remove' || e.op === 'rename');
    }
    for (const [path, removed] of seen) check(path, removed);
  });
}

/** Loads disk content into buf with minimal edits and marks it saved. */
export function applyDisk(buf: Buffer, content: string, meta: FileMeta): void {
  const view = buf.views.values().next().value as EditorView | undefined;
  if (view) {
    const changes = minimalChanges(view.state.doc.toString(), content);
    if (changes.length) view.dispatch({ changes });
    buf.markSaved(view.state.doc, meta);
  } else {
    buf.markSaved(Text.of(content.split('\n')), meta);
  }
}

interface BannerActions {
  reload(buf: Buffer, disk: { content: string; meta: FileMeta }): void;
  keep(buf: Buffer): void;
  compare(buf: Buffer, disk: { content: string; meta: FileMeta }): Promise<void>;
  closeDeleted(buf: Buffer): void;
  keepDeleted(buf: Buffer): void;
}

function bannerPanel(_view: EditorView, buf: Buffer, b: Banner, a: BannerActions): Panel {
  const btn = (label: string, run: () => void): HTMLElement => h('button', { class: 'link-btn', onClick: run }, label);
  const dom =
    b.kind === 'deleted'
      ? h(
          'div',
          { class: 'cm-goi-banner', role: 'alert' },
          h('span', { class: 'cm-goi-banner-msg' }, 'File was deleted on disk'),
          btn('Close', () => a.closeDeleted(buf)),
          btn('Keep', () => a.keepDeleted(buf)),
        )
      : h(
          'div',
          { class: 'cm-goi-banner', role: 'alert' },
          h('span', { class: 'cm-goi-banner-msg' }, 'File changed on disk and has unsaved edits'),
          btn('Reload from disk', () => a.reload(buf, b.disk!)),
          btn('Keep mine', () => a.keep(buf)),
          btn('Compare', () => void a.compare(buf, b.disk!)),
        );
  return { dom, top: true };
}

async function compareDialog(buf: Buffer, disk: string): Promise<'disk' | 'mine' | 'cancel'> {
  const host = h('div', { class: 'goi-compare', style: 'width: min(900px, 80vw); height: 60vh; overflow: auto;' });
  const ro = [EditorState.readOnly.of(true), EditorView.editable.of(false)];
  const header = h(
    'div',
    { style: 'display:flex; font-size:12px; margin-bottom:4px;' },
    h('span', { style: 'flex:1' }, 'Mine (unsaved)'),
    h('span', { style: 'flex:1' }, 'On disk'),
  );
  const mv = new MergeView({ a: { doc: buf.text(), extensions: ro }, b: { doc: disk, extensions: ro }, parent: host });
  try {
    return await showDialog<'disk' | 'mine' | 'cancel'>({
      title: `Compare ${buf.name}`,
      body: h('div', null, header, host),
      cancelValue: 'cancel',
      buttons: [
        { label: 'Cancel', value: 'cancel' },
        { label: 'Use Disk', value: 'disk', danger: true },
        { label: 'Keep Mine', value: 'mine', primary: true },
      ],
    });
  } finally {
    mv.destroy();
  }
}
