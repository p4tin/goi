# Markdown Preview, Mermaid, and JSON/YAML Formatting: Retro

## What went right

- **Lezer instead of a hand-rolled parser.** `@lezer/markdown` was already in the bundle through `lang-markdown`. Rendering its GFM tree gave real CommonMark (nesting, reference links, setext headings, escapes) and source positions for free. Keyed reconciliation and scroll sync both rest on those positions.
- **Probing libraries before writing code.** Ten-line scratch scripts against `yaml` and the Lezer tree showed the actual node names, the CST shape of block scalars, and the `>` re-folding problem before any production code depended on them.
- **The failsafe schema.** Reading every YAML scalar as a string made "values never change" (`yes`, `1.10`, `0x1F`) structural instead of a pile of special cases. The round-trip guard turns any remaining library surprise into a refusal, never a damaged file.
- **Waves held up.** The interfaces fixed in the design (`Block`, `RenderOptions`, `renderDiagram`, `FormatOutcome`) let the W1 modules be built and tested in isolation, and W3 integration needed no changes to them.
- **One small core change.** The `.tab-content` element was the only edit to `editorManager.ts`. It's a generic hook (`viewCreated.content`), and smoke and M2 E2E showed no regressions.

## What went wrong

- **A CSS bug only the E2E could see.** CodeMirror's `display: flex !important` kept the editor visible in Preview mode. happy-dom doesn't apply CodeMirror's styles, so every unit test passed.
- **A commit with type errors.** T5/T6 were committed after running vitest but before `tsc` (narrowing through closures; a `yaml` CST union). They were fixed by amending T6. Run `tsc` before every commit, not just at the end of a wave.
- **Class-name collision.** The agent window's global `.md-code`/`.md-table` styles would have leaked into the preview. This was caught by reading `agent.css` before writing the preview CSS, but only by chance.
- **Test noise.** happy-dom tests that import the editor pull in `@wailsio/runtime`, which tries to reach localhost:3000. Several existing tests already print those errors, which hides real ones.
- **A comparison spoiled by shared state.** The first M2 E2E run reused the smoke run's profile, which had split panes, and looked like a breadcrumbs regression until it was re-run on a clean profile and against a build from before the branch.

## Action items

1. **Check visibility and layout with the E2E for every new mode or panel.** Assert with `getComputedStyle`/`getBoundingClientRect` in the CDP harness, since unit tests can't see third-party CSS. Each E2E run gets a fresh `GOI_HOME`.
2. **Mock `editor/install` in a shared test setup** (a vitest `setupFiles` entry for happy-dom tests), so the runtime never loads in unit tests and the localhost:3000 noise goes away.
