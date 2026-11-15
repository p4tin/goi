// Pyi's frontend pack: the Python-specific features.

import type { FrontendPack } from '../../pack';
import { installInterpreterStatus } from './interpreter';
import { newPythonProjectDialog } from './newProject';
import { installPythonSettings } from './settings';

const pack: FrontendPack = {
  features: [
    // Python settings (before the settings view renders).
    { id: 'pythonSettings', after: 'welcome', install: installPythonSettings },
    // Interpreter status item and picker.
    { id: 'pythonInterpreter', after: 'agent', install: installInterpreterStatus },
  ],
  newProject: newPythonProjectDialog,
  langFiles: isPythonFile,
  runFiles: (path) => path.endsWith('.py'),
  runHint: 'Open a Python file to run it.',
};

/** Files basedpyright handles: Python sources and stubs. */
export function isPythonFile(path: string): boolean {
  return /\.(py|pyi|pyw)$/i.test(path);
}

export default pack;
