// Go SDK and build-target status bar items (R3.5, R4.3, R4.4).

import { project } from '../app/state';
import type { Workbench } from '../app/workbench';
import { on, Topics, workspaceApi } from '../lib/api';
import type { GoEnvSummary, SDKStatus } from '../lib/api';
import { commands } from '../lib/commands';
import { effect } from '../lib/store';
import { openSdkPicker } from './sdkPicker';

export interface StatusLabel {
  text: string;
  title: string;
  warn: boolean;
}

/** "Go 1.26.0", or a warning when no SDK is usable. */
export function formatSdkLabel(status: Pick<SDKStatus, 'ok' | 'problem'> & { sdk?: { version?: string; goroot?: string } }): StatusLabel {
  if (!status.ok) {
    return { text: '⚠ No Go SDK', title: status.problem || 'No Go SDK configured', warn: true };
  }
  const v = (status.sdk?.version ?? '').replace(/^go/, '');
  return { text: v ? `Go ${v}` : 'Go', title: status.sdk?.goroot ?? '', warn: false };
}

/** "darwin/arm64", with " (cross)" when targeting another platform. */
export function formatTarget(s: Pick<GoEnvSummary, 'goos' | 'goarch' | 'goversion' | 'cross'>): StatusLabel {
  const target = `${s.goos}/${s.goarch}`;
  return {
    text: s.cross ? `${target} (cross)` : target,
    title: `GOOS/GOARCH: ${target}${s.goversion ? ` · ${s.goversion}` : ''}`,
    warn: s.cross,
  };
}

const sourceLabels: Record<string, string> = {
  path: 'PATH',
  standard: 'System',
  toolchain: 'Toolchain',
  sdk: '~/sdk',
  manual: 'Manual',
};

export function sdkSourceLabel(source: string): string {
  return sourceLabels[source] ?? (source || 'Unknown');
}

function apply(el: HTMLElement, label: StatusLabel): void {
  el.textContent = label.text;
  el.title = label.title;
  el.classList.toggle('status-warn', label.warn);
}

export function installGoStatus(wb: Workbench): void {
  let sdkEl: HTMLElement | null = null;
  let targetEl: HTMLElement | null = null;
  let warnedFor: string | null = null;
  let seq = 0;

  const pick = async (): Promise<void> => {
    if (await openSdkPicker()) await refresh();
  };

  const clear = (): void => {
    wb.status.remove('go-sdk');
    wb.status.remove('go-target');
    sdkEl = targetEl = null;
  };

  async function refresh(): Promise<void> {
    const p = project.peek();
    const mine = ++seq;
    if (!p) {
      warnedFor = null;
      clear();
      return;
    }
    sdkEl ??= wb.status.add({ id: 'go-sdk', side: 'right', priority: 10, onClick: () => void pick() });
    let status: SDKStatus;
    try {
      status = await workspaceApi.sdkStatus();
    } catch (err) {
      status = { ok: false, problem: String(err) } as SDKStatus;
    }
    if (mine !== seq || !sdkEl) return;
    apply(sdkEl, formatSdkLabel(status));
    if (!status.ok && warnedFor !== p.root) {
      warnedFor = p.root;
      wb.notifications.push({
        id: 'go.noSdk',
        level: 'warn',
        title: 'No Go SDK found',
        body: status.problem || 'Choose a Go SDK from the status bar.',
      });
    }
    if (!status.ok) {
      wb.status.remove('go-target');
      targetEl = null;
      return;
    }
    try {
      const summary = await workspaceApi.goEnvSummary();
      if (mine !== seq) return;
      targetEl ??= wb.status.add({ id: 'go-target', side: 'right', priority: 20, onClick: () => void pick() });
      apply(targetEl, formatTarget(summary));
    } catch {
      if (mine !== seq) return;
      wb.status.remove('go-target');
      targetEl = null;
    }
  }

  effect(() => {
    project();
    void refresh();
  });

  let timer: ReturnType<typeof setTimeout> | null = null;
  on(Topics.settingsChanged, () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      void refresh();
    }, 200);
  });

  commands.register({ id: 'go.selectSdk', title: 'Go: Select SDK…', category: 'Go', run: pick });
}
