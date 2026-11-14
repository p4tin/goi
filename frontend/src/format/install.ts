// JSON and YAML formatters for Reformat File and format on save.

import type { Workbench } from '../app/workbench';
import { registerFormatter } from '../editor/formatters';
import type { DocFormatter } from '../editor/formatters';
import { formatJSON } from './json';
import { formatYAML } from './yaml';

export const jsonFormatter: DocFormatter = {
  id: 'json',
  matches: (path) => /\.(json|jsonc|code-workspace)$/i.test(path),
  onSave: (s) => !!s.json?.formatOnSave,
  format: async (text, o) => formatJSON(text, o.indentUnit),
};

export const yamlFormatter: DocFormatter = {
  id: 'yaml',
  matches: (path) => /\.ya?ml$/i.test(path),
  onSave: (s) => !!s.yaml?.formatOnSave,
  // YAML forbids tabs: a tab-indented file uses tabSize spaces.
  format: (text, o) => formatYAML(text, o.indentUnit === '\t' ? o.tabSize : o.indentUnit.length),
};

export function installFormatters(_wb: Workbench): void {
  registerFormatter(jsonFormatter);
  registerFormatter(yamlFormatter);
}
