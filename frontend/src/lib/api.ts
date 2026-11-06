// Typed wrappers over the generated Wails bindings. Application code imports
// from here, never from wailsjs/ directly.

import * as AppService from '../../wailsjs/go/rpc/AppService';
import * as EditorService from '../../wailsjs/go/rpc/EditorService';
import * as FileService from '../../wailsjs/go/rpc/FileService';
import * as RecentService from '../../wailsjs/go/rpc/RecentService';
import * as SDKService from '../../wailsjs/go/rpc/SDKService';
import * as SearchService from '../../wailsjs/go/rpc/SearchService';
import * as SettingsService from '../../wailsjs/go/rpc/SettingsService';
import * as WorkspaceService from '../../wailsjs/go/rpc/WorkspaceService';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import type { gosdk, index, journal, recent, rpc, settings, vfs, workspace } from '../../wailsjs/go/models';

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
  /** Allows a close the backend prevented (after unsaved work is resolved). */
  confirmClose: (): Promise<void> => AppService.ConfirmClose(),
  /** Creates parentDir/name with go.mod + main.go; resolves to the new dir. */
  newProject: (parentDir: string, name: string, modulePath: string): Promise<string> =>
    AppService.NewProject(parentDir, name, modulePath),
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

export const search = {
  /** Fuzzy file search; recent = absolute paths, most recent first. */
  files: (query: string, limit = 50, recent: string[] = []): Promise<FileSearchResult> =>
    SearchService.Files(query, limit, recent),
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
