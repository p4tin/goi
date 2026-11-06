// A Buffer is one open file. Several views (split panes) may show it; their
// documents are kept identical by the editor manager.

import { Text } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';
import type { FileMeta } from '../lib/api';
import { signal } from '../lib/store';

export function baseName(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  return i >= 0 ? path.slice(i + 1) : path;
}

export function dirName(path: string): string {
  const i = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  return i > 0 ? path.slice(0, i) : path;
}

export class Buffer {
  readonly path: string;
  meta: FileMeta;
  /** Document as last loaded or saved. */
  saved: Text;
  readonly dirty = signal(false);
  readonly views = new Set<EditorView>();
  /** Increments on every document change. */
  version = 0;

  constructor(path: string, meta: FileMeta, content: string) {
    this.path = path;
    this.meta = meta;
    this.saved = Text.of(content.split('\n'));
  }

  get name(): string {
    return baseName(this.path);
  }

  get binary(): boolean {
    return !!this.meta.binary;
  }

  get readOnly(): boolean {
    return this.meta.mode === 'readOnly' || this.binary;
  }

  get highlight(): boolean {
    return this.meta.mode === 'normal';
  }

  /** The live document (from any view), or the saved one. */
  doc(): Text {
    const v = this.views.values().next().value as EditorView | undefined;
    return v ? v.state.doc : this.saved;
  }

  text(): string {
    return this.doc().toString();
  }

  /** Called after the document changed. */
  changed(doc: Text): void {
    this.version++;
    this.dirty.set(!doc.eq(this.saved));
  }

  markSaved(doc: Text, meta: FileMeta): void {
    this.saved = doc;
    this.meta = meta;
    this.dirty.set(false);
  }
}
