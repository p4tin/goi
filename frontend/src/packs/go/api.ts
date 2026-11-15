// Typed wrappers over Goi's own bound services (the Go pack's): Go SDKs and
// go env, go.mod completion, new Go projects. Like lib/api.ts, the only
// place in the Go pack that imports bindings/.

import { ModService, ProjectService, SDKService } from '../../../bindings/github.com/p4tin/goi/packs/golang/services';
import type * as gosdk from '../../../bindings/github.com/p4tin/goi/packs/golang/gosdk/models';
import type * as services from '../../../bindings/github.com/p4tin/goi/packs/golang/services/models';

const list = <T>(p: Promise<T[] | null>): Promise<T[]> => p.then((v) => v ?? []);
const dict = <T>(p: Promise<{ [k: string]: T | undefined } | null>): Promise<Record<string, T>> =>
  p.then((v) => (v ?? {}) as Record<string, T>);

// The E2E specs and devtools sessions call bindings as window.go.rpc.<Service>.
const g = globalThis as { go?: { rpc?: Record<string, unknown> } };
if (g.go?.rpc) Object.assign(g.go.rpc, { ModService, ProjectService, SDKService });

export type SDK = gosdk.SDK;
export type SDKStatus = services.SDKStatus;
export type GoEnvSummary = services.GoEnvSummary;

export const sdk = {
  discover: (): Promise<SDK[]> => list(SDKService.Discover()),
  validate: (goroot: string): Promise<SDK> => SDKService.Validate(goroot),
  select: (goroot: string): Promise<void> => SDKService.Select(goroot),
  status: (): Promise<SDKStatus> => SDKService.Status(),
  goEnv: (): Promise<Record<string, string>> => dict(SDKService.GoEnv()),
  goEnvSummary: (): Promise<GoEnvSummary> => SDKService.GoEnvSummary(),
};

export const modApi = {
  versions: (module: string): Promise<string[]> => list(ModService.Versions(module)),
  modules: (prefix: string): Promise<string[]> => list(ModService.Modules(prefix)),
};

/** Creates parentDir/name with go.mod + main.go; resolves to the new dir. */
export const newGoProject = (parentDir: string, name: string, modulePath: string): Promise<string> =>
  ProjectService.NewProject(parentDir, name, modulePath);
