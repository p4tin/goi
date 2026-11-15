// The frontend pack of the app this bundle was built for. vite.config.ts
// resolves 'virtual:pack' to src/packs/<pack>/index.ts from the build mode
// (`--mode goi` → packs/go). Core code reads the pack's features and hooks
// from here and never imports a pack directly. The product identity is in
// product.ts. Read these exports inside functions, not at module load: the
// pack imports core modules, so they load before this one finishes.

import type { Workbench } from './app/workbench';
import pack from 'virtual:pack';

/** A pack feature and where it goes in the feature order: right after the
 * core feature (or earlier pack feature) with id `after`. Order matters for
 * dock defaults (the first panel registered in a dock is its default tab). */
export interface PackFeature {
  id: string;
  after: string;
  install: (wb: Workbench) => void;
}

/** What a frontend pack exports (src/packs/<pack>/index.ts). */
export interface FrontendPack {
  features: PackFeature[];
  /** Asks for a new project and creates it; resolves to its folder, or null
   * when cancelled. Without it there is no New Project command. */
  newProject?: () => Promise<string | null>;
  /** Files the pack's language server handles (mirrors its LanguageID). */
  langFiles?: (path: string) => boolean;
  /** Files the pack can run (gutter markers, Run at cursor). */
  runFiles?: (path: string) => boolean;
  /** What "Run at cursor" asks for when nothing runnable is found. */
  runHint?: string;
  /** The gutter menu's name for a run marker ("Run 'go build'"); default
   * the marker name. */
  markerLabel?: (m: { kind: string; name: string }) => string;
  /** Markdown docs for a language keyword, shown by Quick Documentation
   * when the language server has none. */
  keywordDoc?: (word: string) => string | null;
}

export const packFeatures: PackFeature[] = pack.features;
export const newProjectDialog: (() => Promise<string | null>) | undefined = pack.newProject;
export const langFiles: (path: string) => boolean = pack.langFiles ?? (() => false);
export const runFiles: (path: string) => boolean = pack.runFiles ?? (() => false);
export const runHint: string = pack.runHint ?? 'Open a file you can run.';
export const markerLabel = (m: { kind: string; name: string }): string => pack.markerLabel?.(m) ?? m.name;
export const keywordDoc = (word: string): string | null => pack.keywordDoc?.(word) ?? null;

/** Inserts each pack feature right after the feature named by its `after`
 * (core or an earlier pack feature). An unknown anchor is a build mistake. */
export function composeFeatures<F>(core: Array<[string, F]>, extra: Array<{ id: string; after: string; install: F }>): Array<[string, F]> {
  const out = [...core];
  for (const f of extra) {
    const i = out.findIndex(([id]) => id === f.after);
    if (i < 0) throw new Error(`pack feature ${f.id}: no feature "${f.after}" to follow`);
    out.splice(i + 1, 0, [f.id, f.install]);
  }
  return out;
}

