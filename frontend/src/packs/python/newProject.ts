// New Python project dialog: parent folder, name, and whether to create a
// virtual environment.

import { showDialog } from '../../app/dialog';
import { app } from '../../lib/api';
import { h } from '../../lib/dom';
import { newPythonProject } from './api';

export interface NewPythonProjectInput {
  parent: string;
  name: string;
}

/** Returns an error message for the form, or null when valid. */
export function validateNewPythonProject(v: NewPythonProjectInput): string | null {
  if (!v.parent.trim()) return 'Choose a parent folder.';
  const name = v.name.trim();
  if (!name) return 'Enter a project name.';
  if (/[\\/]/.test(name) || name === '.' || name === '..') return 'The name cannot contain path separators.';
  if (!/^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(name)) return 'Use letters, digits, ".", "_" and "-" (starting and ending with a letter or digit).';
  return null;
}

/** Shows the dialog; resolves to the created project directory or null. */
export async function newPythonProjectDialog(defaultParent = ''): Promise<string | null> {
  const parent = h('input', { class: 'input', type: 'text', value: defaultParent, spellcheck: false });
  const browse = h('button', { class: 'btn', type: 'button' }, 'Browse…');
  const name = h('input', { class: 'input', type: 'text', value: '', spellcheck: false, placeholder: 'myproject' });
  const venv = h('input', { type: 'checkbox', checked: true });
  const error = h('div', { class: 'dialog-error', role: 'alert' });
  browse.addEventListener('click', async () => {
    const dir = await app.openFolderDialog();
    if (dir) parent.value = dir;
  });
  const body = h(
    'div',
    { class: 'np-form' },
    h('label', { class: 'dialog-label' }, 'Location'),
    h('div', { class: 'np-row' }, parent, browse),
    h('label', { class: 'dialog-label' }, 'Name'),
    name,
    h('label', { class: 'np-row' }, venv, h('span', null, 'Create a virtual environment (.venv)')),
    error,
  );
  for (;;) {
    const ok = await showDialog({
      title: 'New Python Project',
      body,
      focus: name,
      cancelValue: false,
      buttons: [
        { label: 'Cancel', value: false },
        { label: 'Create', value: true, primary: true },
      ],
    });
    if (!ok) return null;
    const input = { parent: parent.value, name: name.value };
    const msg = validateNewPythonProject(input);
    if (msg) {
      error.textContent = msg;
      continue;
    }
    try {
      return await newPythonProject(input.parent.trim(), input.name.trim(), venv.checked);
    } catch (err) {
      error.textContent = String(err);
    }
  }
}
