// Language detection and lazily loaded syntax support per buffer (R6.2).
// Must be installed after installEditor (it uses the editors manager).

import { indentUnit, Language, LanguageSupport, StreamLanguage } from '@codemirror/language';
import type { StreamParser } from '@codemirror/language';
import { Compartment, Prec } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import type { Workbench } from '../app/workbench';
import { effect } from '../lib/store';
import type { Buffer } from './buffer';
import { editors } from './install';
import { largeFileBanner } from './largeFile';

export type LangId =
  | 'go'
  | 'gomod'
  | 'gosum'
  | 'gotmpl'
  | 'dockerfile'
  | 'makefile'
  | 'shell'
  | 'yaml'
  | 'json'
  | 'toml'
  | 'markdown'
  | 'sql'
  | 'protobuf'
  | 'html'
  | 'css'
  | 'javascript'
  | 'typescript'
  | 'xml'
  | 'properties'
  | 'plaintext';

export const languageNames: Record<LangId, string> = {
  go: 'Go',
  gomod: 'go.mod',
  gosum: 'go.sum',
  gotmpl: 'Go Template',
  dockerfile: 'Dockerfile',
  makefile: 'Makefile',
  shell: 'Shell',
  yaml: 'YAML',
  json: 'JSON',
  toml: 'TOML',
  markdown: 'Markdown',
  sql: 'SQL',
  protobuf: 'Protocol Buffers',
  html: 'HTML',
  css: 'CSS',
  javascript: 'JavaScript',
  typescript: 'TypeScript',
  xml: 'XML',
  properties: 'Properties',
  plaintext: 'Plain Text',
};

const BY_NAME: Record<string, LangId> = {
  'go.mod': 'gomod',
  'go.work': 'gomod',
  'go.sum': 'gosum',
  'go.work.sum': 'gosum',
  dockerfile: 'dockerfile',
  containerfile: 'dockerfile',
  makefile: 'makefile',
  gnumakefile: 'makefile',
  '.bashrc': 'shell',
  '.zshrc': 'shell',
  '.profile': 'shell',
  '.bash_profile': 'shell',
  '.env': 'properties',
  '.editorconfig': 'properties',
};

const BY_EXT: Record<string, LangId> = {
  go: 'go',
  gotmpl: 'gotmpl',
  tmpl: 'gotmpl',
  gohtml: 'gotmpl',
  dockerfile: 'dockerfile',
  mk: 'makefile',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  yaml: 'yaml',
  yml: 'yaml',
  json: 'json',
  jsonc: 'json',
  'code-workspace': 'json',
  toml: 'toml',
  md: 'markdown',
  markdown: 'markdown',
  sql: 'sql',
  proto: 'protobuf',
  html: 'html',
  htm: 'html',
  css: 'css',
  scss: 'css',
  less: 'css',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  jsx: 'javascript',
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  xml: 'xml',
  svg: 'xml',
  plist: 'xml',
  ini: 'properties',
  properties: 'properties',
};

/** Picks the language for a path: exact file name first, then extension. */
export function languageIdFor(path: string): LangId {
  const name = path.split(/[\\/]/).pop()!.toLowerCase();
  if (BY_NAME[name]) return BY_NAME[name];
  if (name.startsWith('.env.')) return 'properties';
  const dot = name.lastIndexOf('.');
  if (dot > 0) {
    const ext = name.slice(dot + 1);
    if (BY_EXT[ext]) return BY_EXT[ext];
  }
  return 'plaintext';
}

const legacy = async (mod: Promise<Record<string, unknown>>, key: string): Promise<Extension> =>
  StreamLanguage.define((await mod)[key] as StreamParser<unknown>);

const loaders: Record<LangId, () => Promise<Extension>> = {
  go: async () => (await import('@codemirror/lang-go')).go(),
  gomod: async () => (await import('./lang/gomod')).gomod,
  gosum: async () => (await import('./lang/gomod')).gosum,
  gotmpl: async () => (await import('./lang/gotmpl')).gotmpl,
  dockerfile: () => legacy(import('@codemirror/legacy-modes/mode/dockerfile'), 'dockerFile'),
  makefile: () => legacy(import('@codemirror/legacy-modes/mode/shell'), 'shell'),
  shell: () => legacy(import('@codemirror/legacy-modes/mode/shell'), 'shell'),
  yaml: async () => (await import('@codemirror/lang-yaml')).yaml(),
  json: async () => (await import('@codemirror/lang-json')).json(),
  toml: () => legacy(import('@codemirror/legacy-modes/mode/toml'), 'toml'),
  markdown: async () => (await import('@codemirror/lang-markdown')).markdown(),
  sql: async () => (await import('@codemirror/lang-sql')).sql(),
  protobuf: () => legacy(import('@codemirror/legacy-modes/mode/protobuf'), 'protobuf'),
  html: async () => (await import('@codemirror/lang-html')).html(),
  css: async () => (await import('@codemirror/lang-css')).css(),
  javascript: async () => (await import('@codemirror/lang-javascript')).javascript({ jsx: true }),
  typescript: async () => (await import('@codemirror/lang-javascript')).javascript({ typescript: true, jsx: true }),
  xml: async () => (await import('@codemirror/lang-xml')).xml(),
  properties: () => legacy(import('@codemirror/legacy-modes/mode/properties'), 'properties'),
  plaintext: async () => [],
};

const cache = new Map<LangId, Promise<Extension>>();

/** Loads (once) the syntax support for a language. */
export function loadLanguage(id: LangId): Promise<Extension> {
  let p = cache.get(id);
  if (!p) {
    p = loaders[id]().catch((err: unknown) => {
      console.error('load language', id, err);
      cache.delete(id);
      return [];
    });
    cache.set(id, p);
  }
  return p;
}

/** The Language (parser) for an id, or null for plain text. */
export async function languageFor(id: LangId): Promise<Language | null> {
  const ext = await loadLanguage(id);
  if (ext instanceof LanguageSupport) return ext.language;
  if (ext instanceof Language) return ext;
  return null;
}

const FENCE_ALIASES: Record<string, LangId> = {
  go: 'go',
  golang: 'go',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  shell: 'shell',
  console: 'shell',
  yml: 'yaml',
  yaml: 'yaml',
  json: 'json',
  jsonc: 'json',
  js: 'javascript',
  javascript: 'javascript',
  jsx: 'javascript',
  ts: 'typescript',
  typescript: 'typescript',
  tsx: 'typescript',
  html: 'html',
  xml: 'xml',
  css: 'css',
  sql: 'sql',
  toml: 'toml',
  proto: 'protobuf',
  protobuf: 'protobuf',
  dockerfile: 'dockerfile',
  docker: 'dockerfile',
  make: 'makefile',
  makefile: 'makefile',
  md: 'markdown',
  markdown: 'markdown',
  gomod: 'gomod',
  'go.mod': 'gomod',
  ini: 'properties',
  properties: 'properties',
  tmpl: 'gotmpl',
  gotmpl: 'gotmpl',
};

/** The language of a Markdown code fence tag ("go", "sh", "yml"…). */
export function langIdForFence(tag: string): LangId | null {
  const t = tag.trim().toLowerCase();
  if (!t) return null;
  const id = FENCE_ALIASES[t] ?? languageIdFor(`x.${t}`);
  return id === 'plaintext' ? null : id;
}

/** Languages whose files are indented with tabs. */
export const TAB_INDENTED: ReadonlySet<LangId> = new Set<LangId>(['go', 'makefile', 'gomod']);

export function installLanguages(wb: Workbench): void {
  const compartments = new WeakMap<Buffer, Compartment>();
  const loaded = new WeakMap<Buffer, Extension>();
  const started = new WeakSet<Buffer>();

  const ensureLoaded = (buf: Buffer, comp: Compartment): void => {
    if (started.has(buf) || !buf.highlight) return;
    started.add(buf);
    const id = languageIdFor(buf.path);
    void loadLanguage(id).then((lang) => {
      const ext: Extension = TAB_INDENTED.has(id) ? [lang, Prec.high(indentUnit.of('\t'))] : lang;
      loaded.set(buf, ext);
      for (const v of buf.views) v.dispatch({ effects: comp.reconfigure(ext) });
    });
  };

  editors.addExtension((buf) => {
    let comp = compartments.get(buf);
    if (!comp) {
      comp = new Compartment();
      compartments.set(buf, comp);
    }
    ensureLoaded(buf, comp);
    return comp.of(loaded.get(buf) ?? []);
  });
  editors.addExtension(largeFileBanner);

  const label = wb.status.add({ id: 'editor-language', side: 'right', priority: 55, title: 'Language' });
  effect(() => {
    const path = editors.activePath();
    label.hidden = !path;
    label.textContent = path ? languageNames[languageIdFor(path)] : '';
  });
}
