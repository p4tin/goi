// Run markers gutter (R7): ▶ icons on main/Test/Benchmark/Fuzz/Example
// functions in Go files, computed by the backend with go/parser and refreshed
// 300 ms after edits stop. Clicking offers Run / Debug / Modify Run
// Configuration through the run.marker commands (M3 R4.1).

import { RangeSet, StateEffect, StateField } from '@codemirror/state';
import type { Extension } from '@codemirror/state';
import { gutter, GutterMarker, ViewPlugin } from '@codemirror/view';
import type { EditorView } from '@codemirror/view';
import { showMenu } from '../app/menu';
import type { Workbench } from '../app/workbench';
import { editorApi } from '../lib/api';
import { commands } from '../lib/commands';
import type { RunMarker } from '../lib/api';
import type { Buffer } from './buffer';
import { editors } from './install';

export const MARKER_DEBOUNCE_MS = 300;

const setMarkers = StateEffect.define<RunMarker[]>();

const markersField = StateField.define<RunMarker[]>({
  create: () => [],
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setMarkers)) return e.value;
    return value;
  },
});

const KIND_TITLE: Record<RunMarker['kind'], string> = {
  main: 'Run program',
  test: 'Run test',
  bench: 'Run benchmark',
  fuzz: 'Run fuzz test',
  example: 'Run example',
};

class RunGutterMarker extends GutterMarker {
  constructor(readonly marker: RunMarker) {
    super();
  }

  eq(other: RunGutterMarker): boolean {
    return other.marker.name === this.marker.name && other.marker.kind === this.marker.kind;
  }

  toDOM(): Node {
    const el = document.createElement('span');
    el.className = `run-marker run-${this.marker.kind}`;
    el.textContent = '▶';
    el.title = `${KIND_TITLE[this.marker.kind]} ${this.marker.name}`;
    return el;
  }
}

/** Builds the marker range set for the current document. Markers whose line
 * no longer exists are dropped. */
export function markerSet(view: EditorView, markers: RunMarker[]): RangeSet<GutterMarker> {
  const doc = view.state.doc;
  const ranges = markers
    .filter((m) => m.line >= 1 && m.line <= doc.lines)
    .sort((a, b) => a.line - b.line)
    .map((m) => new RunGutterMarker(m).range(doc.line(m.line).from));
  return RangeSet.of(ranges, true);
}

function runGutterExtension(onRefresh: (buf: Buffer) => void, buf: Buffer): Extension {
  return [
    markersField,
    gutter({
      class: 'cm-run-gutter',
      markers: (view) => markerSet(view, view.state.field(markersField)),
      domEventHandlers: {
        mousedown(view, block, event) {
          const line = view.state.doc.lineAt(block.from).number;
          const marker = view.state.field(markersField).find((m) => m.line === line);
          if (!marker) return false;
          const e = event as MouseEvent;
          const target = { path: buf.path, kind: marker.kind, name: marker.name };
          const label = marker.kind === 'main' ? `'go build'` : `'${marker.name}'`;
          showMenu(e.clientX, e.clientY, [
            { label: `Run ${label}`, run: () => void commands.run('run.marker', target, false) },
            { label: `Debug ${label}`, run: () => void commands.run('run.marker', target, true) },
            { separator: true },
            { label: 'Modify Run Configuration…', run: () => void commands.run('run.editMarker', target) },
          ]);
          return true;
        },
      },
    }),
    ViewPlugin.define(() => {
      onRefresh(buf);
      return {
        update(u) {
          if (u.docChanged) onRefresh(buf);
        },
      };
    }),
  ];
}

export function installRunGutter(_wb: Workbench): void {
  const m = editors;
  const timers = new Map<string, ReturnType<typeof setTimeout>>();

  const refresh = (buf: Buffer): void => {
    clearTimeout(timers.get(buf.path));
    timers.set(
      buf.path,
      setTimeout(async () => {
        timers.delete(buf.path);
        const version = buf.version;
        let markers: RunMarker[];
        try {
          markers = await editorApi.runMarkers(buf.path, buf.text());
        } catch {
          return;
        }
        if (buf.version !== version || !m.buffers.has(buf.path)) return;
        for (const v of buf.views) v.dispatch({ effects: setMarkers.of(markers) });
      }, MARKER_DEBOUNCE_MS),
    );
  };

  m.addExtension((buf) => (buf.path.endsWith('.go') && !buf.binary ? runGutterExtension(refresh, buf) : []));
  m.hooks.closed.on((buf) => {
    clearTimeout(timers.get(buf.path));
    timers.delete(buf.path);
  });
}
