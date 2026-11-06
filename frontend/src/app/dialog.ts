// Modal dialogs: generic dialog, confirm, and text prompt. Escape cancels;
// Enter triggers the primary button.

import { h } from '../lib/dom';

export interface DialogButton<T> {
  label: string;
  value: T;
  primary?: boolean;
  danger?: boolean;
}

export interface DialogOptions<T> {
  title: string;
  body?: Node | string;
  buttons: DialogButton<T>[];
  /** Value resolved on Escape / backdrop click. */
  cancelValue: T;
  /** Optional element to focus initially. */
  focus?: HTMLElement;
}

export function showDialog<T>(opts: DialogOptions<T>): Promise<T> {
  return new Promise((resolve) => {
    const prevFocus = document.activeElement as HTMLElement | null;
    const done = (v: T): void => {
      backdrop.remove();
      document.removeEventListener('keydown', onKey, true);
      prevFocus?.focus?.();
      resolve(v);
    };
    const buttons = opts.buttons.map((b) =>
      h(
        'button',
        { class: `btn${b.primary ? ' btn-primary' : ''}${b.danger ? ' btn-danger' : ''}`, onClick: () => done(b.value) },
        b.label,
      ),
    );
    const dialog = h(
      'div',
      { class: 'dialog', role: 'dialog', 'aria-modal': 'true', 'aria-label': opts.title },
      h('h2', { class: 'dialog-title' }, opts.title),
      opts.body ? h('div', { class: 'dialog-body' }, opts.body) : null,
      h('div', { class: 'dialog-buttons' }, buttons),
    );
    const backdrop = h('div', { class: 'dialog-backdrop' }, dialog);
    backdrop.addEventListener('mousedown', (e) => {
      if (e.target === backdrop) done(opts.cancelValue);
    });
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        done(opts.cancelValue);
      } else if (e.key === 'Enter' && !(e.target instanceof HTMLTextAreaElement)) {
        const primary = opts.buttons.find((b) => b.primary);
        if (primary) {
          e.preventDefault();
          e.stopPropagation();
          done(primary.value);
        }
      }
    };
    document.addEventListener('keydown', onKey, true);
    document.body.appendChild(backdrop);
    (opts.focus ?? buttons.find((_, i) => opts.buttons[i].primary) ?? buttons[0])?.focus();
  });
}

export async function confirmDialog(title: string, body: string, okLabel = 'OK', danger = false): Promise<boolean> {
  return showDialog({
    title,
    body,
    cancelValue: false,
    buttons: [
      { label: 'Cancel', value: false },
      { label: okLabel, value: true, primary: true, danger },
    ],
  });
}

/** Asks for a line of text. validate returns an error message or null. */
export async function promptDialog(
  title: string,
  initial = '',
  validate?: (v: string) => string | null,
  label?: string,
): Promise<string | null> {
  const input = h('input', { class: 'input', type: 'text', value: initial, spellcheck: false });
  const error = h('div', { class: 'dialog-error', role: 'alert' });
  const body = h('div', null, label ? h('label', { class: 'dialog-label' }, label) : null, input, error);
  const check = (): boolean => {
    const msg = validate?.(input.value) ?? null;
    error.textContent = msg ?? '';
    return msg === null;
  };
  input.addEventListener('input', check);
  queueMicrotask(() => {
    input.focus();
    const dot = initial.lastIndexOf('.');
    input.setSelectionRange(0, dot > 0 ? dot : initial.length);
  });
  for (;;) {
    const ok = await showDialog({
      title,
      body,
      focus: input,
      cancelValue: false,
      buttons: [
        { label: 'Cancel', value: false },
        { label: 'OK', value: true, primary: true },
      ],
    });
    if (!ok) return null;
    if (check() && input.value.trim() !== '') return input.value.trim();
  }
}
