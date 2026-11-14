// The Markdown preview of one tab: lives in the tab's content element beside
// the editor, with Editor / Split / Preview modes. Renders are debounced and
// keyed by block source, so unchanged blocks (images, diagrams) stay put.

import type { EditorView } from '@codemirror/view';
import type { Buffer } from '../editor/buffer';
import type { MarkdownPreviewMode } from '../lib/api';
import { h } from '../lib/dom';
import type { SchemeId, UiThemeId } from '../theme/tokens';
import { createHighlighter } from './highlight';
import { renderDiagram } from './mermaid';
import { renderBlocks } from './render';
import type { Block } from './render';

export type Mode = MarkdownPreviewMode;
export const MODES: readonly Mode[] = ['editor', 'split', 'preview'];

export const nextMode = (m: Mode): Mode => MODES[(MODES.indexOf(m) + 1) % MODES.length];

/** A setting value as a mode (unknown values: split). */
export const toMode = (v: unknown): Mode => (MODES.includes(v as Mode) ? (v as Mode) : 'split');

const DEBOUNCE_MS = 120;
const MIN_RATIO = 0.15;

export interface PreviewOptions {
  mode: Mode;
  theme: UiThemeId;
  scheme: SchemeId;
}

/** Puts blocks into body, keeping existing children whose key matches. */
export function reconcile(body: HTMLElement, blocks: Block[], prefix = ''): void {
  const pool = new Map<string, HTMLElement[]>();
  for (const c of body.children) {
    const k = (c as HTMLElement).dataset.key;
    if (k !== undefined) pool.set(k, [...(pool.get(k) ?? []), c as HTMLElement]);
  }
  const next: HTMLElement[] = [];
  for (const b of blocks) {
    const key = prefix + b.key;
    const reused = pool.get(key)?.shift();
    if (reused) {
      reused.dataset.line = String(b.line);
      next.push(reused);
    } else {
      b.el.dataset.key = key;
      next.push(b.el);
    }
  }
  let ref = body.firstChild;
  for (const el of next) {
    if (el === ref) {
      ref = ref.nextSibling;
      continue;
    }
    body.insertBefore(el, ref);
  }
  while (ref) {
    const after: ChildNode | null = ref.nextSibling;
    ref.remove();
    ref = after;
  }
}

/** Parses an SVG string without running anything (text/html parsing never
 * executes scripts) and returns the svg element. */
export function parseSvg(svg: string): SVGSVGElement | null {
  const doc = new DOMParser().parseFromString(svg, 'text/html');
  const el = doc.body.querySelector('svg');
  return el ? (document.importNode(el, true) as SVGSVGElement) : null;
}

export class Preview {
  readonly el: HTMLElement;
  readonly body: HTMLElement;
  private readonly toolbar: HTMLElement;
  private readonly divider: HTMLElement;
  private mode_: Mode;
  private ratio = 0.5;
  private stale = true;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private frame = 0;
  private generation = 0;
  private theme: UiThemeId;
  private scheme: SchemeId;
  private highlight: (lang: string, code: string) => Node[] | null;
  private disposed = false;
  /** Runs after every render (links, images, scroll sync). */
  readonly rendered = new Set<() => void>();

  constructor(
    readonly buf: Buffer,
    readonly view: EditorView,
    readonly content: HTMLElement,
    o: PreviewOptions,
  ) {
    this.theme = o.theme;
    this.scheme = o.scheme;
    this.highlight = this.makeHighlighter();
    this.body = h('div', { class: 'md-body' });
    this.el = h('div', { class: 'md-preview', tabindex: 0, 'aria-label': 'Markdown preview' }, this.body);
    this.divider = h('div', { class: 'md-divider', role: 'separator' });
    this.toolbar = h(
      'div',
      { class: 'md-toolbar', role: 'group', 'aria-label': 'Markdown view' },
      MODES.map((m) =>
        h(
          'button',
          {
            class: 'md-mode',
            dataset: { mode: m },
            title: m === 'editor' ? 'Editor only' : m === 'split' ? 'Editor and preview' : 'Preview only',
            onMousedown: (e: Event) => e.preventDefault(),
            onClick: () => this.setMode(m, true),
          },
          m[0].toUpperCase() + m.slice(1),
        ),
      ),
    );
    this.divider.addEventListener('pointerdown', (e) => this.drag(e));
    content.classList.add('md-tab');
    content.append(this.divider, this.el, this.toolbar);
    this.applyRatio();
    // Large files start in Editor mode and don't render until asked.
    this.mode_ = buf.highlight ? o.mode : 'editor';
    this.applyMode();
    if (this.mode_ !== 'editor') this.render();
  }

  get mode(): Mode {
    return this.mode_;
  }

  setMode(m: Mode, focus = false): void {
    if (m === this.mode_) return;
    const hadFocus = this.view.hasFocus || this.el.contains(document.activeElement);
    this.mode_ = m;
    this.applyMode();
    if (m !== 'editor' && this.stale) this.render();
    this.view.requestMeasure();
    if (focus || hadFocus) {
      if (m === 'preview') this.el.focus({ preventScroll: true });
      else this.view.focus();
    }
  }

  /** The document changed: render soon (or when the preview is shown). */
  docChanged(): void {
    this.stale = true;
    if (this.mode_ === 'editor') return;
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = null;
      this.schedule();
    }, DEBOUNCE_MS);
  }

  /** The UI theme or editor scheme changed: rebuild everything. */
  setTheme(theme: UiThemeId, scheme: SchemeId): void {
    if (theme === this.theme && scheme === this.scheme) return;
    if (scheme !== this.scheme) {
      this.scheme = scheme;
      this.highlight = this.makeHighlighter();
    }
    this.theme = theme;
    this.invalidate();
  }

  /** Renders now (synchronously). */
  render(): void {
    if (this.disposed) return;
    this.stale = false;
    const blocks = renderBlocks(this.view.state.doc.toString(), { highlight: this.highlight });
    reconcile(this.body, blocks, `${this.generation}\u0001`);
    this.hydrateDiagrams();
    for (const fn of this.rendered) fn();
  }

  dispose(): void {
    this.disposed = true;
    if (this.timer) clearTimeout(this.timer);
    cancelAnimationFrame(this.frame);
    this.divider.remove();
    this.el.remove();
    this.toolbar.remove();
    this.content.classList.remove('md-tab');
    delete this.content.dataset.mdMode;
  }

  private makeHighlighter(): (lang: string, code: string) => Node[] | null {
    // A language that finishes loading re-renders with highlighting.
    return createHighlighter(this.scheme, () => this.invalidate());
  }

  private invalidate(): void {
    this.generation++;
    this.stale = true;
    if (this.mode_ !== 'editor') this.schedule();
  }

  private schedule(): void {
    if (this.frame || this.disposed) return;
    this.frame = requestAnimationFrame(() => {
      this.frame = 0;
      this.render();
    });
  }

  private applyMode(): void {
    this.content.dataset.mdMode = this.mode_;
    for (const b of this.toolbar.querySelectorAll<HTMLElement>('.md-mode')) b.setAttribute('aria-pressed', String(b.dataset.mode === this.mode_));
  }

  private applyRatio(): void {
    this.content.style.setProperty('--md-ratio', String(this.ratio));
  }

  private drag(down: PointerEvent): void {
    down.preventDefault();
    this.divider.setPointerCapture?.(down.pointerId);
    const rect = this.content.getBoundingClientRect();
    const move = (e: PointerEvent): void => {
      if (!rect.width) return;
      this.ratio = Math.min(1 - MIN_RATIO, Math.max(MIN_RATIO, (e.clientX - rect.left) / rect.width));
      this.applyRatio();
    };
    const up = (): void => {
      this.divider.removeEventListener('pointermove', move);
      this.divider.removeEventListener('pointerup', up);
      this.view.requestMeasure();
    };
    this.divider.addEventListener('pointermove', move);
    this.divider.addEventListener('pointerup', up);
  }

  private hydrateDiagrams(): void {
    const theme = this.theme;
    for (const box of this.body.querySelectorAll<HTMLElement>('.mdp-mermaid:not([data-state])')) {
      box.dataset.state = 'pending';
      const source = box.dataset.source ?? '';
      void renderDiagram(source, theme).then((r) => {
        if (!box.isConnected || this.disposed) return;
        const svg = 'svg' in r ? parseSvg(r.svg) : null;
        if ('svg' in r && svg) {
          box.dataset.state = 'ok';
          box.replaceChildren(svg, this.diagramActions(source, r.svg));
        } else {
          box.dataset.state = 'error';
          box.append(h('div', { class: 'md-error', role: 'alert' }, 'error' in r ? r.error : 'Invalid diagram output'));
        }
      });
    }
  }

  private diagramActions(source: string, svg: string): HTMLElement {
    const copy = (text: string): void => void navigator.clipboard?.writeText(text);
    return h(
      'div',
      { class: 'md-diagram-actions' },
      h('button', { class: 'md-diagram-btn', onClick: () => copy(source) }, 'Copy source'),
      h('button', { class: 'md-diagram-btn', onClick: () => copy(svg) }, 'Copy SVG'),
    );
  }
}
