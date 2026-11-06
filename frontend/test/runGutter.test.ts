// @vitest-environment happy-dom
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { describe, expect, it } from 'vitest';
import { markerSet } from '../src/editor/runGutter';

describe('markerSet', () => {
  it('places markers at line starts and drops out-of-range lines', () => {
    const view = new EditorView({ state: EditorState.create({ doc: 'package x\n\nfunc main() {}\n' }) });
    const set = markerSet(view, [
      { kind: 'main', name: 'main', line: 3 },
      { kind: 'test', name: 'TestGone', line: 99 },
    ]);
    const froms: number[] = [];
    set.between(0, view.state.doc.length, (from) => void froms.push(from));
    expect(froms).toEqual([view.state.doc.line(3).from]);
    view.destroy();
  });
});
