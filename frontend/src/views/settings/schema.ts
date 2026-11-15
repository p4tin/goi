// Descriptors for every setting shown in the settings view, plus pure helpers used by the settings
// view (lookup, search, validation).

import { product } from '../../product';

export type SettingType = 'bool' | 'int' | 'string' | 'enum' | 'stringList' | 'map';
export type WritableLayer = 'user' | 'project';

/** Settings sections in display order. Packs add theirs with registerSettings. */
export const sections: string[] = ['Appearance', 'Editor', 'Run & Debug', 'Terminal', 'Version Control', 'AI', 'Project', 'Save Actions', 'Keymap', 'Startup'];
export type Section = string;

export interface SettingDescriptor {
  key: string;
  title: string;
  section: Section;
  description: string;
  type: SettingType;
  options?: { value: string; label: string }[];
  min?: number;
  max?: number;
  layers: WritableLayer[];
}

export const both: WritableLayer[] = ['user', 'project'];
export const userOnly: WritableLayer[] = ['user'];
export const opts = (...vs: [string, string][]): { value: string; label: string }[] => vs.map(([value, label]) => ({ value, label }));

export const schema: SettingDescriptor[] = [
  {
    key: 'ui.theme',
    title: 'Theme',
    section: 'Appearance',
    description: 'UI theme. Auto follows the operating system light/dark setting.',
    type: 'enum',
    options: opts(['auto', 'Auto'], ['light', 'Light'], ['dark', 'Dark'], ['hc', 'High Contrast']),
    layers: userOnly,
  },
  {
    key: 'ui.scheme',
    title: 'Editor Color Scheme',
    section: 'Appearance',
    description: 'Syntax colors. Auto matches the UI theme.',
    type: 'enum',
    options: opts(['auto', 'Auto'], ['goi-light', 'Light'], ['goi-dark', 'Dark'], ['goi-ember', 'Ember'], ['goi-paper', 'Paper']),
    layers: userOnly,
  },
  { key: 'editor.fontFamily', title: 'Font Family', section: 'Editor', description: 'CSS font-family list for the editor.', type: 'string', layers: both },
  { key: 'editor.fontSize', title: 'Font Size', section: 'Editor', description: 'Editor font size in pixels.', type: 'int', min: 8, max: 40, layers: both },
  { key: 'editor.tabSize', title: 'Tab Size', section: 'Editor', description: 'Width of a tab, in columns.', type: 'int', min: 1, max: 16, layers: both },
  { key: 'editor.softWrap', title: 'Soft Wrap', section: 'Editor', description: 'Wrap long lines to the window width.', type: 'bool', layers: both },
  {
    key: 'editor.maxHighlightSizeKB',
    title: 'Highlighting Size Limit (KB)',
    section: 'Editor',
    description: 'Files larger than this open without syntax highlighting.',
    type: 'int',
    min: 1,
    max: 1_000_000,
    layers: both,
  },
  {
    key: 'editor.maxOpenSizeKB',
    title: 'Read-only Size Limit (KB)',
    section: 'Editor',
    description: 'Files larger than this open read-only.',
    type: 'int',
    min: 1,
    max: 10_000_000,
    layers: both,
  },
  { key: 'run.outputLimitLines', title: 'Console Line Limit', section: 'Run & Debug', description: 'Lines kept per Run/Debug console tab; older lines are dropped.', type: 'int', min: 1000, max: 1000000, layers: userOnly },
  { key: 'debug.showTypes', title: 'Show Variable Types', section: 'Run & Debug', description: 'Show types next to variables in the debugger.', type: 'bool', layers: userOnly },
  { key: 'debug.showAddresses', title: 'Show Pointer Addresses', section: 'Run & Debug', description: 'Keep 0x… memory addresses in variable values.', type: 'bool', layers: userOnly },
  { key: 'debug.showUnreadable', title: 'Show Unreadable Variables', section: 'Run & Debug', description: 'List variables Delve cannot read (optimized away or invalid memory).', type: 'bool', layers: userOnly },
  { key: 'debug.inlineValues', title: 'Inline Values', section: 'Run & Debug', description: 'Show variable values in the editor while the program is suspended.', type: 'bool', layers: userOnly },
  { key: 'terminal.shell', title: 'Shell', section: 'Terminal', description: 'Shell executable. Empty uses $SHELL (Windows: pwsh, powershell, cmd).', type: 'string', layers: userOnly },
  { key: 'terminal.shellIntegration', title: 'Shell Integration', section: 'Terminal', description: 'For zsh and bash: track the working directory and complete go commands.', type: 'bool', layers: userOnly },
  { key: 'terminal.fontSize', title: 'Font Size', section: 'Terminal', description: 'Terminal font size in pixels. 0 uses the editor font size.', type: 'int', min: 0, max: 40, layers: userOnly },
  { key: 'terminal.scrollback', title: 'Scrollback Lines', section: 'Terminal', description: 'Lines kept in each terminal tab.', type: 'int', min: 100, max: 1000000, layers: userOnly },
  { key: 'git.path', title: 'Git Executable', section: 'Version Control', description: 'Path to git. Empty uses git on $PATH.', type: 'string', layers: userOnly },
  { key: 'git.pullRebase', title: 'Pull with Rebase', section: 'Version Control', description: 'Pull rebases local commits instead of merging.', type: 'bool', layers: both },
  { key: 'ai.defaultAgent', title: 'Default Agent', section: 'AI', description: 'Agent preselected for new sessions: claude, gemini, codex or a custom id.', type: 'string', layers: userOnly },
  {
    key: 'ai.agents',
    title: 'Agent Commands',
    section: 'AI',
    description: 'id = command line. Adds a custom ACP agent, or overrides the command of claude, gemini or codex.',
    type: 'map',
    layers: userOnly,
  },
  { key: 'ai.mcpServer', title: 'IDE Tools for Agents', section: 'AI', description: `Give agents started by ${product.name} tools to open files, read diagnostics, run configurations and more (MCP).`, type: 'bool', layers: userOnly },
  { key: 'ai.externalMcp', title: 'External MCP Clients', section: 'AI', description: 'Also serve the IDE tools to other MCP clients (Claude Code in a terminal, …) on the port below. Takes effect after a restart.', type: 'bool', layers: userOnly },
  { key: 'ai.mcpPort', title: 'MCP Port', section: 'AI', description: 'Local port for external MCP clients.', type: 'int', min: 1024, max: 65535, layers: userOnly },
  {
    key: 'markdown.previewMode',
    title: 'Markdown Preview',
    section: 'Editor',
    description: 'How Markdown files open: source only, source and preview side by side, or preview only.',
    type: 'enum',
    options: opts(['split', 'Split'], ['editor', 'Editor'], ['preview', 'Preview']),
    layers: both,
  },
  { key: 'project.excludes', title: 'Excluded Folders', section: 'Project', description: 'Glob patterns hidden from the tree, search and watcher.', type: 'stringList', layers: both },
  { key: 'project.contentRoots', title: 'Additional Content Roots', section: 'Project', description: 'Extra folders shown alongside the project.', type: 'stringList', layers: ['project'] },
  { key: 'json.formatOnSave', title: 'Format JSON on Save', section: 'Save Actions', description: 'Pretty-print JSON files when saving them.', type: 'bool', layers: both },
  { key: 'yaml.formatOnSave', title: 'Format YAML on Save', section: 'Save Actions', description: 'Reformat YAML files when saving them.', type: 'bool', layers: both },
  {
    key: 'ui.keymap',
    title: 'Keymap',
    section: 'Keymap',
    description: 'Base keyboard shortcuts.',
    type: 'enum',
    options: opts(['goi-default', 'Default'], ['vscode', 'VS Code']),
    layers: userOnly,
  },
  {
    key: 'startup.reopenLastProject',
    title: 'Reopen Last Project',
    section: 'Startup',
    description: `Open the most recent project when ${product.name} starts.`,
    type: 'bool',
    layers: userOnly,
  },
];

/** Adds a pack's settings: new sections (each after an existing one) and
 * their descriptors. Call it from a pack feature, before the settings view
 * renders. */
export function registerSettings(add: { sections?: Array<{ name: string; after: string }>; settings: SettingDescriptor[] }): void {
  for (const sec of add.sections ?? []) {
    if (sections.includes(sec.name)) continue;
    const i = sections.indexOf(sec.after);
    sections.splice(i < 0 ? sections.length : i + 1, 0, sec.name);
  }
  schema.push(...add.settings);
}

export function descriptor(key: string): SettingDescriptor | undefined {
  return schema.find((d) => d.key === key);
}

/** Reads a dotted key from a settings object. */
export function getByKey(obj: unknown, key: string): unknown {
  let cur: unknown = obj;
  for (const part of key.split('.')) {
    if (cur === null || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}

/** A setting's effective value: a core key, or a pack key (pack sections
 * arrive under `extra`, e.g. extra.python.interpreter). */
export function settingValue(settings: unknown, key: string): unknown {
  const v = getByKey(settings, key);
  if (v !== undefined) return v;
  return getByKey((settings as { extra?: unknown } | null)?.extra, key);
}

/** Descriptors whose title, key or description contain every query word. */
export function filterSchema(query: string, list: SettingDescriptor[] = schema): SettingDescriptor[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return list;
  return list.filter((d) => {
    const hay = `${d.title} ${d.key} ${d.description} ${d.section}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

/** Returns an error message for an invalid value, or null. */
export function validate(d: SettingDescriptor, value: unknown): string | null {
  switch (d.type) {
    case 'bool':
      return typeof value === 'boolean' ? null : 'Must be true or false';
    case 'int': {
      if (typeof value !== 'number' || !Number.isInteger(value)) return 'Must be a whole number';
      if (d.min !== undefined && value < d.min) return `Must be at least ${d.min}`;
      if (d.max !== undefined && value > d.max) return `Must be at most ${d.max}`;
      return null;
    }
    case 'string':
      return typeof value === 'string' ? null : 'Must be text';
    case 'enum':
      return d.options?.some((o) => o.value === value) ? null : 'Choose one of the listed values';
    case 'stringList':
      if (!Array.isArray(value) || value.some((v) => typeof v !== 'string')) return 'Must be a list of text values';
      return value.some((v) => (v as string).trim() === '') ? 'Entries cannot be empty' : null;
    case 'map': {
      if (value === null || typeof value !== 'object' || Array.isArray(value)) return 'Must be key/value pairs';
      for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(k)) return `Invalid variable name "${k}"`;
        if (typeof v !== 'string') return `Value of ${k} must be text`;
      }
      return null;
    }
  }
}

/** Parses an int input; returns NaN for non-integers. */
export function parseIntInput(s: string): number {
  return /^\s*-?\d+\s*$/.test(s) ? Number(s) : NaN;
}
