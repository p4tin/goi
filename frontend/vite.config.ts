// Vite builds one frontend per app. The mode picks the app (`--mode goi`,
// `--mode pyi`): its frontend pack (src/packs/<pack>, imported as
// 'virtual:pack'), its product name (from apps/<app>/build/config.yml) and
// its output directory (apps/<app>/dist, which that app's binary embeds).
// Any other mode is an error, so a build never silently picks an app.
// Vitest runs in mode "test" and gets Goi's pack.
import { defineConfig } from 'vite';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));

/** The apps and the frontend pack each one is built with. */
export const APPS: Record<string, { pack: string }> = {
  goi: { pack: 'go' },
  pyi: { pack: 'python' },
};

/** productName in the `info:` block of an app's build/config.yml. */
function productName(app: string): string {
  const cfg = readFileSync(resolve(here, '..', 'apps', app, 'build', 'config.yml'), 'utf8');
  const info = cfg.split(/\n(?=\S)/).find((block) => block.startsWith('info:')) ?? '';
  const m = /^\s+productName:\s*"?([^"#\n]+?)"?\s*(#.*)?$/m.exec(info);
  if (!m) throw new Error(`apps/${app}/build/config.yml has no info.productName`);
  return m[1];
}

export default defineConfig(({ mode }) => {
  const app = mode === 'test' ? 'goi' : mode;
  const spec = APPS[app];
  if (!spec) {
    throw new Error(`vite: unknown app mode "${mode}"; use --mode ${Object.keys(APPS).join(' or --mode ')}`);
  }
  return {
    resolve: {
      alias: { 'virtual:pack': resolve(here, 'src', 'packs', spec.pack, 'index.ts') },
    },
    define: {
      __PRODUCT_NAME__: JSON.stringify(productName(app)),
      __APP_ID__: JSON.stringify(app),
    },
    build: {
      outDir: resolve(here, '..', 'apps', app, 'dist'),
      emptyOutDir: true,
    },
  };
});
