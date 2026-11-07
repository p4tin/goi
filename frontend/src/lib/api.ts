// Typed wrappers over the generated Wails bindings. Application code imports
// from here, never from wailsjs/ directly.

import * as AppService from '../../wailsjs/go/rpc/AppService';
import * as EditorService from '../../wailsjs/go/rpc/EditorService';
import * as FileService from '../../wailsjs/go/rpc/FileService';
import * as LangService from '../../wailsjs/go/rpc/LangService';
import * as MenuService from '../../wailsjs/go/rpc/MenuService';
import * as RecentService from '../../wailsjs/go/rpc/RecentService';
import * as SDKService from '../../wailsjs/go/rpc/SDKService';
import * as SearchService from '../../wailsjs/go/rpc/SearchService';
import * as SettingsService from '../../wailsjs/go/rpc/SettingsService';
import * as WorkspaceService from '../../wailsjs/go/rpc/WorkspaceService';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import type { gopls, gosdk, index, journal, lsp, recent, rpc, settings, vfs, workspace } from '../../wailsjs/go/models';

export type AppInfo = rpc.AppInfo;
export type Project = workspace.Project;
export type ProjectKind = 'workspace' | 'module' | 'multi-module' | 'gopath' | 'folder';
export type Module = workspace.Module;
export type ParseError = workspace.ParseError;
export type FileEntry = vfs.Entry;
export type FileMeta = vfs.FileMeta;
export type OpenedFile = rpc.OpenedFile;
export type RecentEntry = recent.Entry;
export type JournalEntry = journal.Entry;
export type SDK = gosdk.SDK;
export type SDKStatus = rpc.SDKStatus;
export type GoEnvSummary = rpc.GoEnvSummary;
export type Settings = settings.Settings;
export type SettingsLayer = 'default' | 'user' | 'project';
export type SettingsPaths = rpc.SettingsPaths;
export type FileMatch = index.Match;
export type IndexStatus = rpc.IndexStatus;
export type FileSearchResult = rpc.FileSearchResult;

export const Topics = {
  fsChanged: 'fs.changed',
  workspaceChanged: 'workspace.changed',
  settingsChanged: 'settings.changed',
  recentChanged: 'recent.changed',
  indexReady: 'index.ready',
  indexUpdated: 'index.updated',
  notify: 'notify',
  markersUpdated: 'markers.updated',
  openFile: 'app.openFile',
  beforeClose: 'app.beforeClose',
  menuCommand: 'menu.command',
  langStatus: 'lang.status',
  langDiagnostics: 'lang.diagnostics',
  langApplyEdit: 'lang.applyEdit',
} as const;

export type Topic = (typeof Topics)[keyof typeof Topics];

export interface Notification {
  id: string;
  level: 'info' | 'warn' | 'error';
  title: string;
  body?: string;
  file?: string;
  line?: number;
}

export type FsOp = 'create' | 'remove' | 'rename' | 'write';
export interface FsEvent {
  path: string;
  op: FsOp;
  isDir: boolean;
}
export interface FsBatch {
  events: FsEvent[];
}

export interface SettingsChanged {
  layer: SettingsLayer;
  effective: Settings;
}

/** Subscribes to a backend topic; returns an unsubscribe function. */
export function on<T = unknown>(topic: Topic, fn: (payload: T) => void): () => void {
  return EventsOn(topic, (payload: T) => fn(payload));
}

export const app = {
  info: (): Promise<AppInfo> => AppService.Info(),
  openFolderDialog: (): Promise<string> => AppService.OpenFolderDialog(),
  openProject: (path: string, newWindow = false): Promise<void> => AppService.OpenProject(path, newWindow),
  closeProject: (): Promise<void> => AppService.CloseProject(),
  pendingRestore: (): Promise<JournalEntry[]> => AppService.PendingRestore(),
  journalPut: (path: string, content: string, baseModTime: string): Promise<void> =>
    AppService.JournalPut(path, content, baseModTime),
  journalDrop: (path: string): Promise<void> => AppService.JournalDrop(path),
  saveLayout: (layout: unknown): Promise<void> => AppService.SaveLayout(JSON.stringify(layout)),
  loadLayout: async <T>(): Promise<T | null> => {
    const s = await AppService.LoadLayout();
    if (!s) return null;
    try {
      return JSON.parse(s) as T;
    } catch {
      return null;
    }
  },
  quit: (): Promise<void> => AppService.Quit(),
  setAppearance: (theme: string): Promise<void> => AppService.SetAppearance(theme),
  /** Allows a close the backend prevented (after unsaved work is resolved). */
  confirmClose: (): Promise<void> => AppService.ConfirmClose(),
  /** Creates parentDir/name with go.mod + main.go; resolves to the new dir. */
  newProject: (parentDir: string, name: string, modulePath: string): Promise<string> =>
    AppService.NewProject(parentDir, name, modulePath),
};

export interface MenuItemSpec {
  label: string;
  command: string;
  /** Wails accelerator, e.g. "cmdorctrl+shift+1"; "" for none. */
  accel: string;
  separator: boolean;
}
export interface MenuSpec {
  label: string;
  items: MenuItemSpec[];
}
export interface MenuCommandEvent {
  command: string;
}

export const menuApi = {
  /** Replaces the native menu bar. */
  set: (menus: MenuSpec[]): Promise<void> => MenuService.SetMenu(menus as rpc.MenuSpec[]),
};

export const workspaceApi = {
  /** The open project, or null on the Welcome screen. */
  project: async (): Promise<Project | null> => (await WorkspaceService.Project()) ?? null,
  addContentRoot: (path: string): Promise<void> => WorkspaceService.AddContentRoot(path),
  removeContentRoot: (path: string): Promise<void> => WorkspaceService.RemoveContentRoot(path),
  setExcludes: (globs: string[]): Promise<void> => WorkspaceService.SetExcludes(globs),
  sdkStatus: (): Promise<SDKStatus> => WorkspaceService.SDKStatus(),
  goEnv: (): Promise<Record<string, string>> => WorkspaceService.GoEnv(),
  goEnvSummary: (): Promise<GoEnvSummary> => WorkspaceService.GoEnvSummary(),
};

export const files = {
  list: (dir: string): Promise<FileEntry[]> => FileService.List(dir),
  read: (path: string): Promise<OpenedFile> => FileService.Read(path),
  write: (path: string, content: string, meta: FileMeta): Promise<FileMeta> => FileService.Write(path, content, meta),
  create: (path: string, isDir: boolean, content = ''): Promise<void> => FileService.Create(path, isDir, content),
  createGoFile: (dir: string, name: string): Promise<string> => FileService.CreateGoFile(dir, name),
  rename: (from: string, to: string): Promise<void> => FileService.Rename(from, to),
  trash: (path: string): Promise<void> => FileService.Trash(path),
  reveal: (path: string): Promise<void> => FileService.Reveal(path),
  copyPath: (path: string): Promise<void> => FileService.CopyPath(path),
};

export const sdk = {
  discover: (): Promise<SDK[]> => SDKService.Discover(),
  validate: (goroot: string): Promise<SDK> => SDKService.Validate(goroot),
  select: (goroot: string): Promise<void> => SDKService.Select(goroot),
};

export const settingsApi = {
  effective: (): Promise<Settings> => SettingsService.Effective(),
  origins: (): Promise<Record<string, SettingsLayer>> =>
    SettingsService.Origins() as Promise<Record<string, SettingsLayer>>,
  set: (layer: 'user' | 'project', key: string, value: unknown): Promise<void> =>
    SettingsService.Set(layer, key, value),
  paths: (): Promise<SettingsPaths> => SettingsService.Paths(),
};

export const recentApi = {
  list: (): Promise<RecentEntry[]> => RecentService.List(),
  remove: (path: string): Promise<void> => RecentService.Remove(path),
};

export interface FindQuery {
  pattern: string;
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
  /** Comma-separated base-name globs; "!" excludes. */
  mask: string;
  replace: string;
  /** Directory to search ("" = whole project). */
  scope: string;
  /** Unsaved open buffers, searched instead of the file on disk. */
  buffers: { path: string; text: string }[];
}

/** A Find in Path match. col/end/previewStart are UTF-16 offsets in the line. */
export interface TextMatch {
  path: string;
  line: number;
  col: number;
  end: number;
  preview: string;
  previewStart: number;
}

export interface FindResult {
  matches: TextMatch[];
  truncated: boolean;
}

export const search = {
  /** Fuzzy file search; recent = absolute paths, most recent first. */
  files: (query: string, limit = 50, recent: string[] = []): Promise<FileSearchResult> =>
    SearchService.Files(query, limit, recent),
  findInPath: (q: FindQuery): Promise<FindResult> =>
    SearchService.FindInPath(q as unknown as rpc.FindQuery) as unknown as Promise<FindResult>,
  /** Replaces in files that are not open; returns the replacement count. */
  replaceInPath: (q: FindQuery, paths: string[]): Promise<number> =>
    SearchService.ReplaceInPath(q as unknown as rpc.FindQuery, paths),
};

// ---- Language intelligence (gopls). Positions are LSP: 0-based line and
// UTF-16 character, which matches CodeMirror offsets within a line.

export interface Pos {
  line: number;
  character: number;
}
export interface Range {
  start: Pos;
  end: Pos;
}
export interface Location {
  path: string;
  range: Range;
}
export interface TextEdit {
  range: Range;
  newText: string;
}
export interface FileEdit {
  path: string;
  edits: TextEdit[];
}
export type Severity = 1 | 2 | 3 | 4;
export interface Diagnostic {
  range: Range;
  severity?: Severity;
  code?: unknown;
  source?: string;
  message: string;
  tags?: number[];
  data?: unknown;
}
export interface Command {
  title: string;
  command: string;
  arguments?: unknown[];
}
export interface CompletionItem {
  label: string;
  kind?: number;
  detail?: string;
  documentation?: string;
  deprecated?: boolean;
  tags?: number[];
  preselect?: boolean;
  sortText?: string;
  filterText?: string;
  insertText?: string;
  insertTextFormat?: number;
  textEdit?: TextEdit;
  additionalTextEdits?: TextEdit[];
  command?: Command;
}
export interface CompletionList {
  isIncomplete: boolean;
  items: CompletionItem[];
}
export interface ParameterInfo {
  label: string;
  start: number;
  end: number;
  documentation?: string;
}
export interface SignatureInfo {
  label: string;
  documentation?: string;
  parameters: ParameterInfo[];
  activeParameter?: number;
}
export interface SignatureHelp {
  signatures: SignatureInfo[];
  activeSignature: number;
  activeParameter: number;
}
export interface DocumentSymbol {
  name: string;
  detail?: string;
  kind: number;
  range: Range;
  selectionRange: Range;
  children?: DocumentSymbol[];
}
export interface WorkspaceSymbol {
  name: string;
  kind: number;
  containerName: string;
  location: Location;
}
export interface DocumentHighlight {
  range: Range;
  kind?: number;
}
export interface CodeAction {
  title: string;
  kind?: string;
  diagnostics?: Diagnostic[];
  isPreferred?: boolean;
  disabled?: { reason: string };
  edit?: unknown;
  command?: Command;
  data?: unknown;
}
export interface PrepareRename {
  ok: boolean;
  range: Range;
  placeholder: string;
}
export interface HoverResult {
  markdown: string;
  range?: Range;
}
export type LangState = 'starting' | 'indexing' | 'ready' | 'error' | 'stopped' | 'missing' | 'disabled' | 'installing';
export interface LangStatus {
  state: LangState;
  message: string;
  bin: string;
}
export interface DiagnosticsEvent {
  path: string;
  items: Diagnostic[];
}
export interface ApplyEditEvent {
  label: string;
  edits: FileEdit[];
}

type P = Promise<unknown>;
const cast = <T>(p: P): Promise<T> => p as Promise<T>;

export const lang = {
  status: (): Promise<LangStatus> => cast(LangService.Status()),
  restart: (): Promise<void> => LangService.Restart(),
  install: (): Promise<void> => LangService.InstallGopls(),
  didOpen: (path: string, version: number, text: string): Promise<void> => LangService.DidOpen(path, version, text),
  didChange: (path: string, version: number, text: string): Promise<void> => LangService.DidChange(path, version, text),
  didSave: (path: string): Promise<void> => LangService.DidSave(path),
  didClose: (path: string): Promise<void> => LangService.DidClose(path),
  completion: (path: string, p: Pos): Promise<CompletionList> => cast(LangService.Completion(path, p.line, p.character)),
  hover: (path: string, p: Pos): Promise<HoverResult> => cast(LangService.Hover(path, p.line, p.character)),
  signatureHelp: (path: string, p: Pos): Promise<SignatureHelp | null> => cast(LangService.SignatureHelp(path, p.line, p.character)),
  definition: (path: string, p: Pos): Promise<Location[]> => cast(LangService.Definition(path, p.line, p.character)),
  typeDefinition: (path: string, p: Pos): Promise<Location[]> => cast(LangService.TypeDefinition(path, p.line, p.character)),
  implementation: (path: string, p: Pos): Promise<Location[]> => cast(LangService.Implementation(path, p.line, p.character)),
  references: (path: string, p: Pos): Promise<Location[]> => cast(LangService.References(path, p.line, p.character)),
  documentHighlight: (path: string, p: Pos): Promise<DocumentHighlight[]> =>
    cast(LangService.DocumentHighlight(path, p.line, p.character)),
  documentSymbols: (path: string): Promise<DocumentSymbol[]> => cast(LangService.DocumentSymbols(path)),
  workspaceSymbols: (query: string): Promise<WorkspaceSymbol[]> => cast(LangService.WorkspaceSymbols(query)),
  prepareRename: (path: string, p: Pos): Promise<PrepareRename> => cast(LangService.PrepareRename(path, p.line, p.character)),
  rename: (path: string, p: Pos, newName: string): Promise<FileEdit[]> =>
    cast(LangService.Rename(path, p.line, p.character, newName)),
  codeActions: (path: string, range: Range, diags: Diagnostic[]): Promise<CodeAction[]> =>
    cast(LangService.CodeActions(path, range as unknown as lsp.Range, diags as unknown as lsp.Diagnostic[])),
  runCodeAction: (a: CodeAction): Promise<FileEdit[]> => cast(LangService.RunCodeAction(a as unknown as lsp.CodeAction)),
  applyFileEdits: (edits: FileEdit[]): Promise<void> => LangService.ApplyFileEdits(edits as unknown as gopls.FileEdit[]),
};

export type FormatResult = rpc.FormatResult;
export type FormatMode = 'goimports' | 'gofmt' | 'none';

export const editorApi = {
  /** Formats with the save.format setting; syntax errors are in the result. */
  format: (path: string, content: string): Promise<FormatResult> => EditorService.Format(path, content),
  formatWith: (path: string, content: string, mode: FormatMode): Promise<FormatResult> =>
    EditorService.FormatWith(path, content, mode),
  runMarkers: (path: string, content: string): Promise<RunMarker[]> =>
    EditorService.RunMarkers(path, content) as Promise<RunMarker[]>,
};

export type RunMarkerKind = 'main' | 'test' | 'bench' | 'fuzz' | 'example';
export interface RunMarker {
  kind: RunMarkerKind;
  name: string;
  line: number;
}
