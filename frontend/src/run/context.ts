// Running from context (R4): gutter markers, the function at the cursor, the
// current file, and project tree directories.

import { editors } from '../editor/install';
import { editorApi, lang, run } from '../lib/api';
import type { ContextKind, ContextRequest, DocumentSymbol, RunMarker } from '../lib/api';
import { launch } from './launch';
import { configs, notifyError, workbench } from './state';

/** Gutter marker kind → context kind. */
export function markerContext(path: string, m: Pick<RunMarker, 'kind' | 'name'>): ContextRequest {
  return { path, kind: m.kind as ContextKind, name: m.name };
}

/** Starts a temporary configuration for req. */
export async function runContextRequest(req: ContextRequest, debug: boolean): Promise<void> {
  try {
    const cfg = await run.contextConfig(req);
    configs.set(await run.configs());
    await launch(cfg.name, debug);
  } catch (err) {
    notifyError('run-context', `Cannot ${debug ? 'debug' : 'run'}`, err);
  }
}

/** The function symbol (kind 12) enclosing 0-based line, if any. */
export function enclosingFunction(syms: readonly DocumentSymbol[], line: number): string | null {
  for (const s of syms) {
    if (line < s.range.start.line || line > s.range.end.line) continue;
    if (s.kind === 12) return s.name;
    const inner = enclosingFunction(s.children ?? [], line);
    if (inner) return inner;
  }
  return null;
}

/** Picks the context to run for a cursor line (1-based): the marker of the
 * enclosing function, else (without symbols) the nearest marker above, else
 * the file's tests or the main package. */
export function pickContext(path: string, markers: RunMarker[], line: number, enclosing: string | null | undefined): ContextRequest | null {
  if (enclosing !== undefined) {
    const m = markers.find((x) => x.name === enclosing);
    if (m) return markerContext(path, m);
  } else {
    const above = markers.filter((x) => x.line <= line).sort((a, b) => b.line - a.line)[0];
    if (above && path.endsWith('_test.go')) return markerContext(path, above);
  }
  if (path.endsWith('_test.go') && markers.some((m) => m.kind !== 'main')) return { path, kind: 'file', name: '' };
  const main = markers.find((m) => m.kind === 'main');
  if (main) return markerContext(path, main);
  return null;
}

/** Runs or debugs whatever is at the cursor in the active Go editor. */
export async function runAtCursor(debug: boolean): Promise<void> {
  const buf = editors.activeBuffer();
  const view = editors.activeView();
  if (!buf || !view || !buf.path.endsWith('.go')) {
    workbench().notifications.push({ id: 'run-context', level: 'info', title: 'Nothing to run here', body: 'Open a Go file with a main function or tests.' });
    return;
  }
  const line = view.state.doc.lineAt(view.state.selection.main.head).number;
  let markers: RunMarker[] = [];
  try {
    markers = await editorApi.runMarkers(buf.path, buf.text());
  } catch {
    // no markers
  }
  let enclosing: string | null | undefined;
  try {
    enclosing = enclosingFunction(await lang.documentSymbols(buf.path), line - 1);
  } catch {
    enclosing = undefined; // gopls unavailable: fall back to marker positions
  }
  const req = pickContext(buf.path, markers, line, enclosing);
  if (!req) {
    workbench().notifications.push({ id: 'run-context', level: 'info', title: 'Nothing to run here', body: 'Put the cursor in a test, benchmark or func main.' });
    return;
  }
  await runContextRequest(req, debug);
}

/** Runs or debugs the tests in a directory. */
export function runDirTests(dir: string, debug: boolean): Promise<void> {
  return runContextRequest({ path: dir, kind: 'dir', name: '' }, debug);
}
