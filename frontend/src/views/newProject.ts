// New Project dialog: parent folder, name, module path.

import { showDialog } from '../app/dialog';
import { app } from '../lib/api';
import { h } from '../lib/dom';

export interface NewProjectInput {
  parent: string;
  name: string;
  modulePath: string;
}

/** Returns an error message for the form, or null when valid. */
export function validateNewProject(v: NewProjectInput): string | null {
  if (!v.parent.trim()) return 'Choose a parent folder.';
  const name = v.name.trim();
  if (!name) return 'Enter a project name.';
  if (/[\\/]/.test(name) || name === '.' || name === '..') return 'The name cannot contain path separators.';
  const mod = v.modulePath.trim();
  if (!mod) return 'Enter a module path.';
  if (/\s/.test(mod)) return 'The module path cannot contain spaces.';
  return null;
}

/** Shows the dialog; resolves to the created project directory or null. */
export async function newProjectDialog(defaultParent = ''): Promise<string | null> {
  const parent = h('input', { class: 'input', type: 'text', value: defaultParent, spellcheck: false });
  const browse = h('button', { class: 'btn', type: 'button' }, 'Browse…');
  const name = h('input', { class: 'input', type: 'text', value: '', spellcheck: false, placeholder: 'myproject' });
  const mod = h('input', { class: 'input', type: 'text', value: '', spellcheck: false, placeholder: 'example.com/myproject' });
  const error = h('div', { class: 'dialog-error', role: 'alert' });
  let modTouched = false;

  browse.addEventListener('click', async () => {
    const dir = await app.openFolderDialog();
    if (dir) parent.value = dir;
  });
  name.addEventListener('input', () => {
    if (!modTouched) mod.value = name.value.trim();
  });
  mod.addEventListener('input', () => {
    modTouched = true;
  });

  const body = h(
    'div',
    { class: 'np-form' },
    h('label', { class: 'dialog-label' }, 'Location'),
    h('div', { class: 'np-row' }, parent, browse),
    h('label', { class: 'dialog-label' }, 'Name'),
    name,
    h('label', { class: 'dialog-label' }, 'Module path'),
    mod,
    error,
  );

  for (;;) {
    const ok = await showDialog({
      title: 'New Project',
      body,
      focus: name,
      cancelValue: false,
      buttons: [
        { label: 'Cancel', value: false },
        { label: 'Create', value: true, primary: true },
      ],
    });
    if (!ok) return null;
    const input = { parent: parent.value, name: name.value, modulePath: mod.value };
    const msg = validateNewProject(input);
    if (msg) {
      error.textContent = msg;
      continue;
    }
    try {
      return await app.newProject(input.parent.trim(), input.name.trim(), input.modulePath.trim());
    } catch (err) {
      error.textContent = String(err);
    }
  }
}
