import './theme/tokens.css';
import './style.css';
import './app/workbench.css';
import { settings, startState } from './app/state';
import { createWorkbench, routeWorkbench } from './app/workbench';
import { features } from './features';
import { effect } from './lib/store';
import { initTheme } from './theme/theme';
import type { ThemeSetting } from './theme/theme';

async function boot(): Promise<void> {
  const theme = initTheme('auto');
  await startState();
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
  document.body.textContent = `Goi failed to start: ${String(err)}`;
});
