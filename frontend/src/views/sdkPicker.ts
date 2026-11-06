// SDK picker dialog: choose a discovered Go SDK, add one manually, or fall
// back to automatic selection (R3.3, R3.6).

import { showDialog } from '../app/dialog';
import { app, sdk, workspaceApi } from '../lib/api';
import type { SDK } from '../lib/api';
import { h } from '../lib/dom';
import { sdkSourceLabel } from './goStatus';
import './sdk.css';

type Choice = 'cancel' | 'auto' | 'use';

/** Opens the picker. Resolves true when the SDK selection changed. */
export async function openSdkPicker(): Promise<boolean> {
  let sdks: SDK[] = [];
  let selected: string | null = null;
  let current: string | null = null;

  const list = h('div', { class: 'sdk-list', role: 'listbox', 'aria-label': 'Go SDKs' }, h('div', { class: 'muted sdk-empty' }, 'Discovering SDKs…'));
  const error = h('div', { class: 'dialog-error', role: 'alert' });
  const add = h('button', { class: 'btn', onClick: () => void addManual() }, 'Add SDK…');
  const body = h('div', { class: 'sdk-picker' }, list, h('div', { class: 'sdk-actions' }, add), error);

  const render = (): void => {
    if (sdks.length === 0) {
      list.replaceChildren(h('div', { class: 'muted sdk-empty' }, 'No Go SDKs found. Use “Add SDK…” to choose a GOROOT.'));
      return;
    }
    list.replaceChildren(
      ...sdks.map((s) =>
        h(
          'button',
          {
            class: `sdk-row${s.goroot === selected ? ' selected' : ''}`,
            role: 'option',
            'aria-selected': String(s.goroot === selected),
            title: s.goroot,
            onClick: () => {
              selected = s.goroot;
              error.textContent = '';
              render();
            },
          },
          h('span', { class: 'sdk-version' }, s.version || 'unknown'),
          h('span', { class: 'sdk-path' }, s.goroot),
          h('span', { class: 'sdk-badge' }, sdkSourceLabel(s.source)),
          s.goroot === current ? h('span', { class: 'sdk-current' }, 'current') : null,
        ),
      ),
    );
  };

  async function addManual(): Promise<void> {
    error.textContent = '';
    const dir = await app.openFolderDialog();
    if (!dir) return;
    try {
      const found = await sdk.validate(dir);
      found.source = found.source || 'manual';
      if (!sdks.some((s) => s.goroot === found.goroot)) sdks = [...sdks, found];
      selected = found.goroot;
      render();
    } catch (err) {
      error.textContent = String(err);
    }
  }

  void (async () => {
    try {
      const [discovered, status] = await Promise.all([sdk.discover(), workspaceApi.sdkStatus()]);
      sdks = discovered;
      current = status.ok ? status.sdk.goroot : null;
      if (current && !sdks.some((s) => s.goroot === current)) sdks = [status.sdk, ...sdks];
      selected = current ?? sdks[0]?.goroot ?? null;
    } catch (err) {
      error.textContent = `Discovery failed: ${String(err)}`;
    }
    render();
  })();

  for (;;) {
    const choice = await showDialog<Choice>({
      title: 'Go SDK',
      body,
      cancelValue: 'cancel',
      buttons: [
        { label: 'Cancel', value: 'cancel' },
        { label: 'Use default (auto)', value: 'auto' },
        { label: 'Use', value: 'use', primary: true },
      ],
    });
    if (choice === 'cancel') return false;
    try {
      if (choice === 'auto') {
        await sdk.select('');
        return true;
      }
      if (!selected) {
        error.textContent = 'Select an SDK first.';
        continue;
      }
      if (selected === current) return false;
      await sdk.select(selected);
      return true;
    } catch (err) {
      error.textContent = String(err);
    }
  }
}
