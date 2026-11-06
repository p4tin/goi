# Fix: clicking a file in the project tree doesn't open it

## Symptom
In the built app, the project tree shows the folder, but clicking or double-clicking a file never opens it in the editor. Expected: double-click (or Enter) opens the file. Clicking a folder's chevron is probably affected too.

## Root cause hypothesis
`frontend/src/views/projectTree.ts`:
- Each row has `mousedown → this.select(row.path)`.
- `select()` calls `refresh()` → `paint()` → `sizer.replaceChildren(...)`, which **builds new row elements** on every mousedown.

So the element that got `mousedown` is detached before `mouseup`. Because press and release happen on different elements, the browser sends `click` and `dblclick` to their common ancestor (the sizer), not to the row. The row's `dblclick` listener, and the chevron's `onClick`, never run.

The E2E harness missed this because CDP dispatched synthetic events straight to the element instead of real mouse input.

## Reproduction strategy
Vitest in happy-dom (`test/projectTree.test.ts`):
- Mock `lib/api`: `files.list` returns `[main.go]`.
- Stub element size so the virtualizer renders rows.
- Install the tree with a minimal workbench stub, set `project`, and register an `editor.open` spy command.
- Dispatch real-order input on the file row: `mousedown` on the row element, then `mouseup`/`click`/`dblclick` on whatever element is now at that row (re-queried by path).
- Assert `editor.open` was called with the file path.

A first assertion checks the root cause directly: the row element is still the same node after `mousedown`. It currently fails.

## Steps
- [x] Red: failing test (3 failures: row replaced on mousedown, editor.open not called, folder not expanded)
- [x] Green: minimal fix (selection change updates row classes in place instead of rebuilding rows)
- [x] Regression: 236 frontend tests pass, tsc clean, app rebuilt
