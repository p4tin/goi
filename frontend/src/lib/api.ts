// Typed wrappers over the generated Wails bindings. Application code imports
// from here, never from bindings/ directly.

import { Browser, Events } from '@wailsio/runtime';
import {
  AppService,
  DebugService,
  EditorService,
  FileService,
  GitService,
  LangService,
  LintService,
  MenuService,
  ModService,
  RecentService,
  RunService,
  SDKService,
  SearchService,
  SettingsService,
  TerminalService,
  WorkspaceService,
} from '../../bindings/github.com/p4tin/goi/internal/rpc';
import type * as gopls from '../../bindings/github.com/p4tin/goi/internal/gopls/models';
import type * as gosdk from '../../bindings/github.com/p4tin/goi/internal/gosdk/models';
import type * as index from '../../bindings/github.com/p4tin/goi/internal/index/models';
import type * as journal from '../../bindings/github.com/p4tin/goi/internal/journal/models';
import type * as lsp from '../../bindings/github.com/p4tin/goi/internal/lsp/models';
import type * as recent from '../../bindings/github.com/p4tin/goi/internal/recent/models';
import type * as rpc from '../../bindings/github.com/p4tin/goi/internal/rpc/models';
import type * as runconfig from '../../bindings/github.com/p4tin/goi/internal/runconfig/models';
import type * as settings from '../../bindings/github.com/p4tin/goi/internal/settings/models';
import type * as vfs from '../../bindings/github.com/p4tin/goi/internal/vfs/models';
import type * as workspace from '../../bindings/github.com/p4tin/goi/internal/workspace/models';

// The generated types allow null for every Go slice and map. Bound methods
// never return nil ones (see CLAUDE.md), so the wrappers expose them as
// non-null.
type NonNullFields<T> = { [K in keyof T]: Exclude<T[K], null> };
const list = <T>(p: Promise<T[] | null>): Promise<T[]> => p.then((v) => v ?? []);
const dict = <T>(p: Promise<{ [k: string]: T | undefined } | null>): Promise<Record<string, T>> =>
  p.then((v) => (v ?? {}) as Record<string, T>);

// Wails v2 exposed the bindings as window.go.rpc.<Service>.<Method>; the E2E
// specs and devtools sessions still call them that way.
(globalThis as { go?: unknown }).go = {
  rpc: {
    AppService,
    DebugService,
    EditorService,
    FileService,
    GitService,
    LangService,
    LintService,
    MenuService,
    ModService,
    RecentService,
    RunService,
    SDKService,
    SearchService,
    SettingsService,
    TerminalService,
    WorkspaceService,
  },
};

export type AppInfo = rpc.AppInfo;
export type Project = NonNullFields<workspace.Project>;
export type ProjectKind = 'workspace' | 'module' | 'multi-module' | 'gopath' | 'folder';
export type Module = workspace.Module;
export type ParseError = workspace.ParseError;
export type FileEntry = vfs.Entry;
// Go string enums are generated as TS enums; the frontend uses their values.
export type FileMeta = Omit<vfs.FileMeta, 'mode'> & { mode: `${vfs.OpenMode}` };
export type OpenedFile = Omit<rpc.OpenedFile, 'meta'> & { meta: FileMeta };
export type RecentEntry = recent.Entry;
export type JournalEntry = journal.Entry;
export type SDK = gosdk.SDK;
export type SDKStatus = rpc.SDKStatus;
export type GoEnvSummary = rpc.GoEnvSummary;
export type Settings = Omit<settings.Settings, 'keys'> & { keys: Record<string, string[]> };
export type SettingsLayer = 'default' | 'user' | 'project';
export type SettingsPaths = rpc.SettingsPaths;
export type FileMatch = index.Match;
export type IndexStatus = rpc.IndexStatus;
export type FileSearchResult = NonNullFields<rpc.FileSearchResult>;

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
  runConfigs: 'run.configs',
  runState: 'run.state',
  runOutput: 'run.output',
  runTest: 'run.test',
  debugState: 'debug.state',
  debugBreakpoints: 'debug.breakpoints',
  debugInstall: 'debug.install',
  termOutput: 'term.output',
  termExit: 'term.exit',
  gitStatus: 'git.status',
  gitOutput: 'git.output',
  gitOp: 'git.op',
  lintDiagnostics: 'lint.diagnostics',
  lintStatus: 'lint.status',
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

/** Opens an http(s) URL in the system browser; other schemes are ignored. */
export function openExternal(url: string): void {
  if (/^https?:\/\//i.test(url)) void Browser.OpenURL(url);
}

// This window's backend id (0 until waitForWindow resolves).
let windowId = 0;

/**
 * Waits until the backend has registered this window and opened its startup
 * project, and returns the window's id. Call before anything else at boot.
 */
export async function waitForWindow(timeoutMs = 5000): Promise<number> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const id = await AppService.WindowID();
    if (id) return (windowId = id);
    if (Date.now() > deadline) throw new Error('backend did not register this window');
    await new Promise((r) => setTimeout(r, 50));
  }
}

interface Envelope {
  _w: number;
  d: unknown;
}

function isEnvelope(v: unknown): v is Envelope {
  return typeof v === 'object' && v !== null && '_w' in v && 'd' in v;
}

/**
 * Extracts an event's payload. Server builds (E2E) broadcast every event to
 * all windows wrapped in an envelope naming the window; returns undefined
 * with skip set for another window's event.
 */
export function eventPayload(data: unknown, myWindow = windowId): { skip: boolean; payload: unknown } {
  if (!isEnvelope(data)) return { skip: false, payload: data };
  return { skip: myWindow !== 0 && data._w !== myWindow, payload: data.d };
}

/** Subscribes to a backend topic; returns an unsubscribe function. */
export function on<T = unknown>(topic: Topic, fn: (payload: T) => void): () => void {
  return Events.On(topic, (ev) => {
    const { skip, payload } = eventPayload(ev.data);
    if (!skip) fn(payload as T);
  });
}

export const app = {
  info: (): Promise<AppInfo> => AppService.Info(),
  openFolderDialog: (): Promise<string> => AppService.OpenFolderDialog(),
  openProject: (path: string, newWindow = false): Promise<void> => AppService.OpenProject(path, newWindow),
  closeProject: (): Promise<void> => AppService.CloseProject(),
  pendingRestore: (): Promise<JournalEntry[]> => list(AppService.PendingRestore()),
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
  cancelClose: (): Promise<void> => AppService.CancelClose(),
  newWindow: (): Promise<void> => AppService.NewWindow(),
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
  project: async (): Promise<Project | null> => ((await WorkspaceService.Project()) as Project | null) ?? null,
  addContentRoot: (path: string): Promise<void> => WorkspaceService.AddContentRoot(path),
  removeContentRoot: (path: string): Promise<void> => WorkspaceService.RemoveContentRoot(path),
  setExcludes: (globs: string[]): Promise<void> => WorkspaceService.SetExcludes(globs),
  sdkStatus: (): Promise<SDKStatus> => WorkspaceService.SDKStatus(),
  goEnv: (): Promise<Record<string, string>> => dict(WorkspaceService.GoEnv()),
  goEnvSummary: (): Promise<GoEnvSummary> => WorkspaceService.GoEnvSummary(),
};

export const files = {
  list: (dir: string): Promise<FileEntry[]> => list(FileService.List(dir)),
  read: (path: string): Promise<OpenedFile> => FileService.Read(path) as Promise<OpenedFile>,
  write: (path: string, content: string, meta: FileMeta): Promise<FileMeta> =>
    FileService.Write(path, content, meta as vfs.FileMeta) as Promise<FileMeta>,
  create: (path: string, isDir: boolean, content = ''): Promise<void> => FileService.Create(path, isDir, content),
  createGoFile: (dir: string, name: string): Promise<string> => FileService.CreateGoFile(dir, name),
  rename: (from: string, to: string): Promise<void> => FileService.Rename(from, to),
  trash: (path: string): Promise<void> => FileService.Trash(path),
  reveal: (path: string): Promise<void> => FileService.Reveal(path),
  copyPath: (path: string): Promise<void> => FileService.CopyPath(path),
};

export const sdk = {
  discover: (): Promise<SDK[]> => list(SDKService.Discover()),
  validate: (goroot: string): Promise<SDK> => SDKService.Validate(goroot),
  select: (goroot: string): Promise<void> => SDKService.Select(goroot),
};

export const settingsApi = {
  effective: (): Promise<Settings> => SettingsService.Effective() as Promise<Settings>,
  origins: (): Promise<Record<string, SettingsLayer>> =>
    SettingsService.Origins() as Promise<Record<string, SettingsLayer>>,
  set: (layer: 'user' | 'project', key: string, value: unknown): Promise<void> =>
    SettingsService.Set(layer, key, value),
  paths: (): Promise<SettingsPaths> => SettingsService.Paths(),
};

export const recentApi = {
  list: (): Promise<RecentEntry[]> => list(RecentService.List()),
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
    SearchService.Files(query, limit, recent) as Promise<FileSearchResult>,
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

// ---- Run configurations and runs (M3)

export type RunConfigType = 'app' | 'test' | 'remote' | 'shell' | 'make';
export type RunTaskType = 'config' | 'generate' | 'command';
export interface RunTask {
  type: RunTaskType;
  config: string;
  package: string;
  command: string;
}

export interface RunConfig {
  name: string;
  type: RunConfigType;
  shared: boolean;
  temporary: boolean;
  created: number;
  kind: string;
  package: string;
  directory: string;
  files: string[];
  packages: string[];
  pattern: string;
  workingDir: string;
  args: string;
  env: Record<string, string>;
  envFile: string;
  buildFlags: string;
  tags: string[];
  output: string;
  runAfterBuild?: boolean | null;
  inputFile: string;
  race: boolean;
  short: boolean;
  count: number;
  timeout: string;
  testFlags: string;
  bench: string;
  benchmem: boolean;
  fuzz: string;
  fuzzTime: string;
  host: string;
  port: number;
  script: string;
  scriptText: string;
  interpreter: string;
  makefile: string;
  targets: string[];
  beforeLaunch: RunTask[];
}

export type RunState = 'starting' | 'building' | 'running' | 'exited' | 'failed';
export interface ExitInfo {
  code: number;
  signal: string;
  err: string;
}
export interface RunInfo {
  id: string;
  config: RunConfig;
  state: RunState;
  debug: boolean;
  exit: ExitInfo | null;
  elapsedMs: number;
  message: string;
  command: string;
  test: boolean;
}
/** Output streams; "console" carries debugger messages. */
export type OutputStream = 'stdout' | 'stderr' | 'system' | 'console';
export interface OutputChunk {
  stream: OutputStream;
  text: string;
}
export interface RunOutputEvent {
  id: string;
  chunks: OutputChunk[];
}
export interface TestEvent {
  action: string;
  package: string;
  test: string;
  elapsed: number;
  output: string;
  importPath: string;
  failedBuild: string;
}
export interface RunTestEvent {
  id: string;
  events: TestEvent[];
}
export type ContextKind = 'main' | 'test' | 'bench' | 'fuzz' | 'example' | 'file' | 'dir';
export interface ContextRequest {
  path: string;
  kind: ContextKind;
  name: string;
}

/** A blank configuration of the given type with every field present. */
export function emptyRunConfig(type: RunConfigType, name = ''): RunConfig {
  return {
    name, type, shared: false, temporary: false, created: 0,
    kind: type === 'test' ? 'directory' : 'package', package: '', directory: '', files: [], packages: [], pattern: '',
    workingDir: '', args: '', env: {}, envFile: '', buildFlags: '', tags: [], output: '', runAfterBuild: null, inputFile: '',
    race: false, short: false, count: 0, timeout: '', testFlags: '', bench: '', benchmem: false, fuzz: '', fuzzTime: '',
    host: type === 'remote' ? 'localhost' : '', port: type === 'remote' ? 2345 : 0,
    script: '', scriptText: '', interpreter: '', makefile: '', targets: [], beforeLaunch: [],
  };
}

const toGoConfig = (c: RunConfig): runconfig.Config => c as unknown as runconfig.Config;

export const run = {
  configs: (): Promise<RunConfig[]> => cast(RunService.Configs()),
  saveConfig: (c: RunConfig, oldName: string): Promise<RunConfig> => cast(RunService.SaveConfig(toGoConfig(c), oldName)),
  deleteConfig: (name: string): Promise<void> => RunService.DeleteConfig(name),
  validate: (c: RunConfig): Promise<string[]> => list(RunService.Validate(toGoConfig(c))),
  contextConfig: (req: ContextRequest): Promise<RunConfig> => cast(RunService.ContextConfig(req as rpc.ContextRequest)),
  addTemporary: (c: RunConfig): Promise<RunConfig> => cast(RunService.AddTemporary(toGoConfig(c))),
  run: (name: string, debug: boolean): Promise<string> => RunService.Run(name, debug),
  runConfig: (c: RunConfig, reuseId: string, debug: boolean): Promise<string> => RunService.RunConfig(toGoConfig(c), reuseId, debug),
  rerun: (id: string): Promise<void> => RunService.Rerun(id),
  stop: (id: string): Promise<void> => RunService.Stop(id),
  close: (id: string): Promise<void> => RunService.Close(id),
  input: (id: string, text: string): Promise<void> => RunService.Input(id, text),
  runs: (): Promise<RunInfo[]> => cast(RunService.Runs()),
  active: (): Promise<number> => RunService.Active(),
  loadBench: (): Promise<string> => RunService.LoadBench(),
  saveBench: (data: string): Promise<void> => RunService.SaveBench(data),
};

// ---- Debugger (M3). Lines and columns are 1-based (DAP).

export interface DapSource {
  name?: string;
  path?: string;
  sourceReference?: number;
}
export interface DapBreakpoint {
  id: number;
  verified: boolean;
  message: string;
  source?: DapSource | null;
  line: number;
}
export interface DapThread {
  id: number;
  name: string;
}
export interface DapStackFrame {
  id: number;
  name: string;
  source?: DapSource | null;
  line: number;
  column: number;
  presentationHint?: string;
}
export interface DapStackTrace {
  stackFrames: DapStackFrame[];
  totalFrames: number;
}
export interface DapScope {
  name: string;
  variablesReference: number;
  expensive: boolean;
}
export interface DapVariable {
  name: string;
  value: string;
  type: string;
  evaluateName: string;
  variablesReference: number;
  namedVariables: number;
  indexedVariables: number;
}
export interface DapEvaluate {
  result: string;
  type: string;
  variablesReference: number;
  namedVariables: number;
  indexedVariables: number;
}
export interface DapExceptionInfo {
  exceptionId: string;
  description: string;
  breakMode: string;
  details?: { message: string; typeName: string; stackTrace: string } | null;
}
export interface LineBreakpoint {
  line: number;
  condition: string;
  hitCondition: string;
  logMessage: string;
  enabled: boolean;
}
export interface FunctionBreakpoint {
  name: string;
  condition: string;
  enabled: boolean;
}
export type DebugStateName = 'starting' | 'running' | 'stopped' | 'exited';
export interface DebugStateEvent {
  id: string;
  state: DebugStateName;
  threadId: number;
  reason: string;
  description: string;
  text: string;
  exitCode: number | null;
  remote: boolean;
  caps: { supportsSetVariable: boolean; supportsGotoTargets: boolean; supportsRestart: boolean };
}
export interface DebugBreakpointsEvent {
  id: string;
  path: string;
  function: boolean;
  breakpoints: DapBreakpoint[];
}
export interface DlvStatus {
  path: string;
  found: boolean;
  installing: boolean;
  message: string;
}

export const debug = {
  dlvStatus: (): Promise<DlvStatus> => cast(DebugService.DlvStatus()),
  installDlv: (): Promise<void> => DebugService.InstallDlv(),
  setFileBreakpoints: (path: string, bps: LineBreakpoint[]): Promise<void> =>
    DebugService.SetFileBreakpoints(path, bps as unknown as rpc.LineBreakpoint[]),
  setFunctionBreakpoints: (fns: FunctionBreakpoint[], breakOnPanic: boolean): Promise<void> =>
    DebugService.SetFunctionBreakpoints(fns as unknown as rpc.FunctionBreakpoint[], breakOnPanic),
  threads: (id: string): Promise<DapThread[]> => cast(DebugService.Threads(id)),
  stackTrace: (id: string, threadId: number, start = 0, levels = 0): Promise<DapStackTrace> =>
    cast(DebugService.StackTrace(id, threadId, start, levels)),
  scopes: (id: string, frameId: number): Promise<DapScope[]> => cast(DebugService.Scopes(id, frameId)),
  variables: (id: string, ref: number): Promise<DapVariable[]> => cast(DebugService.Variables(id, ref)),
  evaluate: (id: string, expr: string, frameId: number, context: 'watch' | 'repl' | 'hover' | 'clipboard'): Promise<DapEvaluate> =>
    cast(DebugService.Evaluate(id, expr, frameId, context)),
  setVariable: (id: string, ref: number, name: string, value: string): Promise<{ value: string; type: string; variablesReference: number }> =>
    cast(DebugService.SetVariable(id, ref, name, value)),
  exceptionInfo: (id: string, threadId: number): Promise<DapExceptionInfo> => cast(DebugService.ExceptionInfo(id, threadId)),
  continue: (id: string, threadId: number): Promise<void> => DebugService.Continue(id, threadId),
  next: (id: string, threadId: number): Promise<void> => DebugService.Next(id, threadId),
  stepIn: (id: string, threadId: number): Promise<void> => DebugService.StepIn(id, threadId),
  stepOut: (id: string, threadId: number): Promise<void> => DebugService.StepOut(id, threadId),
  runToCursor: (id: string, threadId: number, path: string, line: number): Promise<void> => DebugService.RunToCursor(id, threadId, path, line),
  pause: (id: string, threadId: number): Promise<void> => DebugService.Pause(id, threadId),
  stop: (id: string): Promise<void> => DebugService.Stop(id),
  sessions: (): Promise<string[]> => list(DebugService.Sessions()),
};

// ---- Terminal (M3)

export interface TermInfo {
  id: string;
  shell: string;
  cwd: string;
}
export interface TermOutputEvent {
  id: string;
  data: string;
}
export interface TermExitEvent {
  id: string;
  code: number;
}

export const terminalApi = {
  open: (cols: number, rows: number): Promise<TermInfo> => cast(TerminalService.Open(cols, rows)),
  write: (id: string, data: string): Promise<void> => TerminalService.Write(id, data),
  resize: (id: string, cols: number, rows: number): Promise<void> => TerminalService.Resize(id, cols, rows),
  close: (id: string): Promise<void> => TerminalService.Close(id),
  count: (): Promise<number> => TerminalService.Count(),
};

// ---- Git (M4). Paths are absolute OS paths; rel fields are repo-relative.

export interface GitFileStatus {
  path: string;
  rel: string;
  orig: string;
  /** Porcelain status letters: '.', M, A, D, R, C, T, U, '?' (untracked), '!' (ignored). */
  index: string;
  work: string;
  conflict: boolean;
  untracked: boolean;
  ignored: boolean;
}
export type GitOp = '' | 'merge' | 'rebase' | 'cherry-pick' | 'revert';
export interface GitStatus {
  root: string;
  branch: string;
  head: string;
  upstream: string;
  ahead: number;
  behind: number;
  detached: boolean;
  initial: boolean;
  op: GitOp;
  files: GitFileStatus[];
}
export interface GitState {
  repo: boolean;
  status: GitStatus;
}
export interface GitHunk {
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  header: string;
  lines: string[];
}
export interface GitFileDiff {
  oldPath: string;
  newPath: string;
  header: string[];
  hunks: GitHunk[];
  binary: boolean;
}
export interface GitBranch {
  name: string;
  remote: boolean;
  current: boolean;
  upstream: string;
  hash: string;
  subject: string;
  date: string;
}
export interface GitCommit {
  hash: string;
  short: string;
  parents: string[];
  author: string;
  email: string;
  date: string;
  subject: string;
  body: string;
  refs: string;
}
export interface GitChangedFile {
  path: string;
  rel: string;
  status: string;
  orig: string;
}
export interface GitBlameLine {
  line: number;
  hash: string;
  author: string;
  date: string;
  summary: string;
}
export interface GitStash {
  index: number;
  message: string;
  date: string;
}
export interface GitLogQuery {
  path: string;
  skip: number;
  max: number;
  grep: string;
  ref: string;
}
export interface GitOutputEvent {
  opId: string;
  chunks: OutputChunk[];
}
export interface GitOpEvent {
  opId: string;
  title: string;
  state: 'running' | 'done' | 'failed';
  error: string;
  rejected: boolean;
}

export const gitApi = {
  state: (): Promise<GitState> => cast(GitService.State()),
  refresh: (): Promise<void> => GitService.Refresh(),
  init: (): Promise<void> => GitService.Init(),
  show: (rev: string, path: string): Promise<{ text: string; found: boolean }> => cast(GitService.Show(rev, path)),
  diff: (path: string, staged: boolean, rev = '', commit = ''): Promise<GitFileDiff[]> => cast(GitService.Diff(path, staged, rev, commit)),
  applyPatch: (patch: string, cached: boolean, reverse: boolean): Promise<void> => GitService.ApplyPatch(patch, cached, reverse),
  stage: (paths: string[]): Promise<void> => GitService.Stage(paths),
  unstage: (paths: string[]): Promise<void> => GitService.Unstage(paths),
  rollback: (paths: string[]): Promise<void> => GitService.Rollback(paths),
  commit: (message: string, amend: boolean): Promise<void> => GitService.Commit(message, amend),
  lastMessage: (): Promise<string> => GitService.LastMessage(),
  branches: (): Promise<GitBranch[]> => cast(GitService.Branches()),
  checkout: (ref: string, newBranch = ''): Promise<void> => GitService.Checkout(ref, newBranch),
  renameBranch: (from: string, to: string): Promise<void> => GitService.RenameBranch(from, to),
  deleteBranch: (name: string, force: boolean): Promise<void> => GitService.DeleteBranch(name, force),
  merge: (ref: string): Promise<void> => GitService.Merge(ref),
  rebase: (onto: string): Promise<void> => GitService.Rebase(onto),
  cherryPick: (hash: string): Promise<void> => GitService.CherryPick(hash),
  revert: (hash: string): Promise<void> => GitService.Revert(hash),
  reset: (rev: string, mode: 'soft' | 'mixed' | 'hard'): Promise<void> => GitService.Reset(rev, mode),
  continueOp: (): Promise<void> => GitService.Continue(),
  skipOp: (): Promise<void> => GitService.Skip(),
  abortOp: (): Promise<void> => GitService.Abort(),
  acceptSide: (path: string, side: 'ours' | 'theirs'): Promise<void> => GitService.AcceptSide(path, side),
  stash: (message: string, untracked: boolean): Promise<void> => GitService.Stash(message, untracked),
  stashes: (): Promise<GitStash[]> => cast(GitService.Stashes()),
  stashApply: (i: number, pop: boolean): Promise<void> => GitService.StashApply(i, pop),
  stashDrop: (i: number): Promise<void> => GitService.StashDrop(i),
  stashDiff: (i: number): Promise<GitFileDiff[]> => cast(GitService.StashDiff(i)),
  log: (q: Partial<GitLogQuery>): Promise<GitCommit[]> => cast(GitService.Log({ path: '', skip: 0, max: 200, grep: '', ref: '', ...q } as never)),
  commitFiles: (hash: string): Promise<GitChangedFile[]> => cast(GitService.CommitFiles(hash)),
  blame: (path: string, contents: string): Promise<GitBlameLine[]> => cast(GitService.Blame(path, contents)),
  addToGitignore: (paths: string[]): Promise<void> => GitService.AddToGitignore(paths),
  remotes: (): Promise<string[]> => list(GitService.Remotes()),
  fetch: (): Promise<string> => GitService.Fetch(),
  pull: (): Promise<string> => GitService.Pull(),
  push: (force = false): Promise<string> => GitService.Push(force),
  stopOp: (id: string): Promise<void> => GitService.StopOp(id),
};

// ---- golangci-lint (M4)

export interface LintStatus {
  found: boolean;
  bin: string;
  version: string;
  installing: boolean;
  running: number;
  message: string;
}
export interface LintDiagnosticsEvent {
  dirs: string[];
  recursive: boolean;
  files: Record<string, Diagnostic[]>;
}
export interface LintResult {
  issues: number;
  files: number;
  error: string;
}

export const lintApi = {
  status: (): Promise<LintStatus> => cast(LintService.Status()),
  install: (): Promise<void> => LintService.Install(),
  run: (scope: 'package' | 'project', path: string): Promise<LintResult> => cast(LintService.Run(scope, path)),
};

// ---- go.mod support (M4)

export const modApi = {
  versions: (module: string): Promise<string[]> => list(ModService.Versions(module)),
  modules: (prefix: string): Promise<string[]> => list(ModService.Modules(prefix)),
};
