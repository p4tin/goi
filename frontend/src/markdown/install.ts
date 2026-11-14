// Markdown preview feature: a Preview beside every Markdown editor view, the
// Markdown: Toggle Preview command, and theme/scheme tracking.

import { ViewPlugin } from '@codemirror/view';
import type { EditorView, ViewUpdate } from '@codemirror/view';
import { resolvedTheme, settings } from '../app/state';
import type { Workbench } from '../app/workbench';
import type { Buffer } from '../editor/buffer';
import { editors, resolveScheme } from '../editor/install';
import { languageIdFor } from '../editor/languages';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { nextMode, Preview, toMode } from './preview';
import './preview.css';

const previews = new WeakMap<EditorView, Preview>();
const live = new Set<Preview>();

export const isMarkdown = (buf: Buffer): boolean => !buf.binary && languageIdFor(buf.path) === 'markdown';

/** The preview of the active editor view, if it shows Markdown. */
export function activePreview(): Preview | null {
  const v = editors.activeView();
  return (v && previews.get(v)) ?? null;
}

// One plugin instance per view (a shared ViewPlugin survives extension
// refreshes): forwards document changes and disposes the preview.
const previewPlugin = ViewPlugin.fromClass(
  class {
    constructor(readonly view: EditorView) {}
    update(u: ViewUpdate): void {
      if (u.docChanged) previews.get(u.view)?.docChanged();
    }
    destroy(): void {
      const p = previews.get(this.view);
      if (!p) return;
      p.dispose();
      live.delete(p);
      previews.delete(this.view);
    }
  },
);

export function installMarkdownPreview(_wb: Workbench): void {
  const currentScheme = () => resolveScheme(settings.peek()?.ui.scheme ?? 'auto', resolvedTheme.peek());

  editors.addExtension((buf) => (isMarkdown(buf) ? previewPlugin : []));
  editors.hooks.viewCreated.on(({ buffer, view, content }) => {
    if (!isMarkdown(buffer)) return;
    const p = new Preview(buffer, view, content, {
      mode: toMode(settings.peek()?.markdown?.previewMode),
      theme: resolvedTheme.peek(),
      scheme: currentScheme(),
    });
    previews.set(view, p);
    live.add(p);
  });

  effect(() => {
    const theme = resolvedTheme();
    const scheme = resolveScheme(settings()?.ui.scheme ?? 'auto', theme);
    for (const p of live) p.setTheme(theme, scheme);
  });

  commands.register({
    id: 'markdown.togglePreview',
    title: 'Toggle Preview',
    category: 'Markdown',
    when: () => !!activePreview(),
    run: () => {
      const p = activePreview();
      p?.setMode(nextMode(p.mode), true);
    },
  });
}
