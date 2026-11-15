// The frontend pack vitest runs with (vite.config.ts, mode "test"): a
// product but no pack features, so core modules load without pulling in a
// pack's bindings. Pack tests import their pack's modules directly.
import type { FrontendPack } from '../../src/pack';
import { keywordDoc } from '../../src/packs/go/keywords';

const pack: FrontendPack = {
  features: [],
  // Go's rule, so the language client tests run against Go files.
  langFiles: (path) => /\.go$|(^|[\\/])go\.(mod|work)$/.test(path),
  runFiles: (path) => path.endsWith('.go'),
  markerLabel: (m) => (m.kind === 'main' ? 'go build' : m.name),
  keywordDoc,
};

export default pack;
