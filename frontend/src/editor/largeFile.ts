// Banner shown above files opened in a degraded mode (R6.5).

import type { Extension } from '@codemirror/state';
import { EditorView, showPanel } from '@codemirror/view';
import type { Buffer } from './buffer';

export function formatMB(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Banner text for a buffer, or null when it opened normally. */
export function largeFileMessage(buf: Pick<Buffer, 'meta' | 'binary'>): string | null {
  if (buf.binary) return null;
  const size = formatMB(buf.meta.size);
  if (buf.meta.mode === 'noHighlight') return `Large file (${size}): syntax highlighting disabled`;
  if (buf.meta.mode === 'readOnly') return `Very large file (${size}): opened read-only`;
  return null;
}

const bannerTheme = EditorView.baseTheme({
  '.goi-large-file': {
    padding: '4px 10px',
    fontSize: '12px',
    background: 'var(--goi-panel-bg)',
    color: 'var(--goi-warning)',
    borderBottom: '1px solid var(--goi-panel-border)',
  },
});

export function largeFileBanner(buf: Buffer): Extension {
  const msg = largeFileMessage(buf);
  if (!msg) return [];
  return [
    bannerTheme,
    showPanel.of(() => {
      const dom = document.createElement('div');
      dom.className = 'goi-large-file';
      dom.setAttribute('role', 'status');
      dom.textContent = msg;
      return { dom, top: true };
    }),
  ];
}
