// Goi's frontend pack: the Go-specific features and the product identity.

import type { FrontendPack } from '../../pack';
import { installGoMod } from './gomod';
import { installGoStatus } from './goStatus';
import { keywordDoc } from './keywords';
import { newProjectDialog } from './newProject';
import { installGoSettings } from './settings';

const pack: FrontendPack = {
  features: [
    // Go, Linting and Delve settings (before the settings view renders).
    { id: 'goSettings', after: 'welcome', install: installGoSettings },
    // go.mod/go.work completion, links and module actions.
    { id: 'goMod', after: 'lint', install: installGoMod },
    // Go SDK and GOOS/GOARCH status items.
    { id: 'goStatus', after: 'agent', install: installGoStatus },
  ],
  newProject: newProjectDialog,
  langFiles: isGoLangFile,
  runFiles: (path) => path.endsWith('.go'),
  runHint: 'Open a Go file with a main function or tests.',
  markerLabel: (m) => (m.kind === 'main' ? 'go build' : m.name),
  keywordDoc,
};

/** Files gopls handles: Go sources, go.mod and go.work. */
export function isGoLangFile(path: string): boolean {
  const base = path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1);
  return base.endsWith('.go') || base === 'go.mod' || base === 'go.work';
}

export default pack;
