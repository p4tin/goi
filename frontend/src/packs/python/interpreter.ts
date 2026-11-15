// Python interpreter status bar item and picker: shows the project's
// interpreter ("Python 3.12.1 (.venv)") and lets the user choose another.

import { showDialog } from '../../app/dialog';
import { project } from '../../app/state';
import type { Workbench } from '../../app/workbench';
import { on, Topics } from '../../lib/api';
import { commands } from '../../lib/commands';
import { h } from '../../lib/dom';
import { effect } from '../../lib/store';
import { interpreters } from './api';
import type { Interpreter, InterpreterStatus } from './api';

/** The last path segment of a venv folder, else "". */
function venvName(i: Pick<Interpreter, 'venv'>): string {
  return i.venv ? i.venv.split(/[\\/]/).pop() ?? '' : '';
}

/** "Python 3.12.1 (.venv)", or a warning when none is usable. */
export function interpreterLabel(st: Pick<InterpreterStatus, 'ok' | 'problem' | 'interpreter'>): { text: string; title: string; warn: boolean } {
  if (!st.ok) return { text: '⚠ No Python', title: st.problem || 'No Python interpreter', warn: true };
  const v = st.interpreter.version ? `Python ${st.interpreter.version}` : 'Python';
  const venv = venvName(st.interpreter);
  return { text: venv ? `${v} (${venv})` : v, title: st.interpreter.path, warn: false };
}

/** Lets the user pick an interpreter; resolves true when the choice changed. */
export async function openInterpreterPicker(): Promise<boolean> {
  const [found, current] = await Promise.all([interpreters.discover().catch(() => [] as Interpreter[]), interpreters.status()]);
  let chosen = current.fromSetting ? current.interpreter.path : '';
  const custom = h('input', { class: 'input', type: 'text', value: '', spellcheck: false, placeholder: '/path/to/python' });
  const rows = h('div', { class: 'np-form' });
  const option = (label: string, detail: string, value: string): HTMLElement => {
    const radio = h('input', { type: 'radio', name: 'py-interp', checked: value === chosen });
    radio.addEventListener('change', () => (chosen = value));
    return h('label', { class: 'np-row', title: value }, radio, h('span', null, label), h('span', { class: 'muted' }, detail));
  };
  rows.append(option('Automatic', "the project's .venv, then python3 on PATH", ''));
  for (const i of found) {
    const venv = venvName(i);
    rows.append(option(`Python ${i.version || '?'}${venv ? ` (${venv})` : ''}`, i.path, i.path));
  }
  rows.append(h('label', { class: 'dialog-label' }, 'Other interpreter'), custom);
  const ok = await showDialog({
    title: 'Python Interpreter',
    body: rows,
    cancelValue: false,
    buttons: [
      { label: 'Cancel', value: false },
      { label: 'Use', value: true, primary: true },
    ],
  });
  if (!ok) return false;
  const pick = custom.value.trim() || chosen;
  await interpreters.select(pick);
  return true;
}

export function installInterpreterStatus(wb: Workbench): void {
  let el: HTMLElement | null = null;
  let seq = 0;
  const pick = async (): Promise<void> => {
    try {
      if (await openInterpreterPicker()) await refresh();
    } catch (err) {
      wb.notifications.push({ id: 'python.interpreter', level: 'error', title: 'Cannot set the interpreter', body: String(err) });
    }
  };
  async function refresh(): Promise<void> {
    const mine = ++seq;
    if (!project.peek()) {
      wb.status.remove('python-interpreter');
      el = null;
      return;
    }
    el ??= wb.status.add({ id: 'python-interpreter', side: 'right', priority: 10, onClick: () => void pick() });
    let st: InterpreterStatus;
    try {
      st = await interpreters.status();
    } catch (err) {
      st = { ok: false, problem: String(err) } as InterpreterStatus;
    }
    if (mine !== seq || !el) return;
    const l = interpreterLabel(st);
    el.textContent = l.text;
    el.title = l.title;
    el.classList.toggle('status-warn', l.warn);
  }
  effect(() => {
    project();
    void refresh();
  });
  on(Topics.settingsChanged, () => void refresh());
  commands.register({ id: 'python.selectInterpreter', title: 'Select Python Interpreter…', category: 'Python', run: () => pick() });
}
