// Descriptors for every M1 setting, plus pure helpers used by the settings
// view (lookup, search, validation).

export type SettingType = 'bool' | 'int' | 'string' | 'enum' | 'stringList' | 'map';
export type WritableLayer = 'user' | 'project';

export const sections = ['Appearance', 'Editor', 'Go', 'Project', 'Save Actions', 'Keymap', 'Startup'] as const;
export type Section = (typeof sections)[number];

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

const both: WritableLayer[] = ['user', 'project'];
const userOnly: WritableLayer[] = ['user'];
const opts = (...vs: [string, string][]): { value: string; label: string }[] => vs.map(([value, label]) => ({ value, label }));

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
    options: opts(['auto', 'Auto'], ['goi-light', 'Goi Light'], ['goi-dark', 'Goi Dark'], ['goi-ember', 'Goi Ember'], ['goi-paper', 'Goi Paper']),
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
  { key: 'go.goroot', title: 'GOROOT', section: 'Go', description: 'Go SDK used for this project. Empty picks one automatically.', type: 'string', layers: both },
  { key: 'go.env', title: 'Environment Variables', section: 'Go', description: 'Extra environment for go commands (GOPROXY, GOPRIVATE, GOFLAGS…).', type: 'map', layers: both },
  { key: 'go.tags', title: 'Build Tags', section: 'Go', description: 'Build tags passed as -tags to go commands.', type: 'stringList', layers: both },
  { key: 'go.goos', title: 'GOOS', section: 'Go', description: 'Target operating system. Empty uses the host.', type: 'string', layers: both },
  { key: 'go.goarch', title: 'GOARCH', section: 'Go', description: 'Target architecture. Empty uses the host.', type: 'string', layers: both },
  {
    key: 'go.cgo',
    title: 'CGO_ENABLED',
    section: 'Go',
    description: 'Enable or disable cgo. Default uses the toolchain default.',
    type: 'enum',
    options: opts(['', 'Default'], ['1', 'Enabled'], ['0', 'Disabled']),
    layers: both,
  },
  {
    key: 'go.vendor',
    title: 'Vendoring',
    section: 'Go',
    description: 'Auto uses vendor/ when present; On forces -mod=vendor; Off forces -mod=mod.',
    type: 'enum',
    options: opts(['auto', 'Auto'], ['on', 'On'], ['off', 'Off']),
    layers: both,
  },
  { key: 'project.excludes', title: 'Excluded Folders', section: 'Project', description: 'Glob patterns hidden from the tree, search and watcher.', type: 'stringList', layers: both },
  { key: 'project.contentRoots', title: 'Additional Content Roots', section: 'Project', description: 'Extra folders shown alongside the project.', type: 'stringList', layers: ['project'] },
  {
    key: 'save.format',
    title: 'Format on Save',
    section: 'Save Actions',
    description: 'Formatter run when saving Go files.',
    type: 'enum',
    options: opts(['goimports', 'goimports (format + imports)'], ['gofmt', 'gofmt'], ['none', 'None']),
    layers: both,
  },
  {
    key: 'ui.keymap',
    title: 'Keymap',
    section: 'Keymap',
    description: 'Base keyboard shortcuts.',
    type: 'enum',
    options: opts(['goi-default', 'Goi Default'], ['vscode', 'VS Code']),
    layers: userOnly,
  },
  {
    key: 'startup.reopenLastProject',
    title: 'Reopen Last Project',
    section: 'Startup',
    description: 'Open the most recent project when Goi starts.',
    type: 'bool',
    layers: userOnly,
  },
];

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
