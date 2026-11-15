// Typed wrappers over Pyi's own bound services (the Python pack's): the
// project interpreter and new Python projects. Like lib/api.ts, the only
// place in the Python pack that imports bindings/.

import { InterpreterService, ProjectService } from '../../../bindings/github.com/p4tin/goi/packs/python/services';
import type * as services from '../../../bindings/github.com/p4tin/goi/packs/python/services/models';

const list = <T>(p: Promise<T[] | null>): Promise<T[]> => p.then((v) => v ?? []);

// The E2E specs and devtools sessions call bindings as window.go.rpc.<Service>.
const g = globalThis as { go?: { rpc?: Record<string, unknown> } };
if (g.go?.rpc) Object.assign(g.go.rpc, { InterpreterService, ProjectService });

export type Interpreter = services.Interpreter;
export type InterpreterStatus = services.InterpreterStatus;

export const interpreters = {
  status: (): Promise<InterpreterStatus> => InterpreterService.Status(),
  discover: (): Promise<Interpreter[]> => list(InterpreterService.Discover()),
  /** Stores python.interpreter ("" = automatic). */
  select: (path: string): Promise<void> => InterpreterService.Select(path),
};

/** Creates parentDir/name with pyproject.toml + main.py (and a .venv);
 * resolves to the new dir. */
export const newPythonProject = (parentDir: string, name: string, venv: boolean): Promise<string> =>
  ProjectService.NewProject(parentDir, name, venv);
