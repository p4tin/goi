import './theme/tokens.css';
import './style.css';
import './app/workbench.css';
import { appInfo, resolvedTheme, settings, startState } from './app/state';
import { createWorkbench, routeWorkbench } from './app/workbench';
import { features } from './features';
import { app, waitForWindow } from './lib/api';
import { effect } from './lib/store';
import { initTheme } from './theme/theme';
import type { ThemeSetting } from './theme/theme';

async function boot(): Promise<void> {
  await waitForWindow();
  const theme = initTheme('auto', (t) => void app.setAppearance(t).catch(() => {}));
  effect(() => resolvedTheme.set(theme.resolved()));
  await startState();
  // The bundle is built per app (vite --mode <app>); refuse to run one app's
  // UI against another app's backend (e.g. a stale dev server).
  const running = appInfo.peek()?.name;
  if (running && running !== __PRODUCT_NAME__) {
    throw new Error(`this window's UI was built for ${__PRODUCT_NAME__}, but the app is ${running}`);
  }
  effect(() => {
    const t = settings()?.ui.theme as ThemeSetting | undefined;
    if (t) theme.set(t);
  });

  const host = document.querySelector<HTMLDivElement>('#app')!;
  const wb = createWorkbench();
  host.replaceChildren(wb.root, wb.welcome);
  for (const install of features) install(wb);
  routeWorkbench(wb);
}

boot().catch((err: unknown) => {
  console.error('boot', err);
  document.body.textContent = `${__PRODUCT_NAME__} failed to start: ${String(err)}`;
});
