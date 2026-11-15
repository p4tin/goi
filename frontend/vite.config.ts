// Vite builds one frontend per app. The mode picks the app (`--mode goi`,
// `--mode pyi`): its frontend pack (src/packs/<pack>, imported as
// 'virtual:pack'), its product name (from apps/<app>/build/config.yml) and
// its output directory (apps/<app>/dist, which that app's binary embeds).
// Any other mode is an error, so a build never silently picks an app.
// Vitest runs in mode "test" with a featureless test pack
// (test/fixtures/testPack.ts) and Goi's product name.
import { defineConfig } from 'vite';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));

/** The apps and the frontend pack each one is built with. */
export const APPS: Record<string, { pack: string; site: string }> = {
  goi: { pack: 'go', site: 'https://github.com/p4tin/goi' },
  pyi: { pack: 'python', site: 'https://github.com/p4tin/goi' },
};

/** A key of the `info:` block of an app's build/config.yml. */
function info(app: string, key: string): string {
  const cfg = readFileSync(resolve(here, '..', 'apps', app, 'build', 'config.yml'), 'utf8');
  const block = cfg.split(/\n(?=\S)/).find((b) => b.startsWith('info:')) ?? '';
  const m = new RegExp(`^\\s+${key}:\\s*"?([^"#\\n]+?)"?\\s*(#.*)?$`, 'm').exec(block);
  if (!m) throw new Error(`apps/${app}/build/config.yml has no info.${key}`);
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
      alias: {
        'virtual:pack':
          mode === 'test' ? resolve(here, 'test', 'fixtures', 'testPack.ts') : resolve(here, 'src', 'packs', spec.pack, 'index.ts'),
        'virtual:pack-icon': resolve(here, 'src', 'packs', spec.pack, 'icon.svg'),
      },
    },
    define: {
      __PRODUCT_NAME__: JSON.stringify(info(app, 'productName')),
      __PRODUCT_TAGLINE__: JSON.stringify(info(app, 'description')),
      __PRODUCT_SITE__: JSON.stringify(spec.site),
      __APP_ID__: JSON.stringify(app),
    },
    build: {
      outDir: resolve(here, '..', 'apps', app, 'dist'),
      emptyOutDir: true,
    },
    test: {
      setupFiles: ['test/setup.ts'],
    },
  };
});
