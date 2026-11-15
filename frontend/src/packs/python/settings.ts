// Pyi's settings: the project interpreter and basedpyright. The settings
// view shows them through registerSettings; their values live in the
// backend's [python] section.

import { both, registerSettings, userOnly } from '../../views/settings/schema';
import type { SettingDescriptor } from '../../views/settings/schema';

export const pythonSettings: SettingDescriptor[] = [
  {
    key: 'python.interpreter',
    title: 'Interpreter',
    section: 'Python',
    description: "Python used to run and analyse the project. Empty picks the project's .venv (or venv), then python3 on PATH. Relative paths are relative to the project.",
    type: 'string',
    layers: both,
  },
  { key: 'python.lsp.enabled', title: 'Enable basedpyright', section: 'Python', description: 'Code completion, navigation and diagnostics.', type: 'bool', layers: both },
  {
    key: 'python.lsp.path',
    title: 'basedpyright Path',
    section: 'Python',
    description: 'basedpyright-langserver executable. Empty searches $PATH and the tools folder.',
    type: 'string',
    layers: userOnly,
  },
];

export function installPythonSettings(): void {
  registerSettings({ sections: [{ name: 'Python', after: 'Editor' }], settings: pythonSettings });
}
