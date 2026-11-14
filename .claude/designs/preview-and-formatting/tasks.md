# Markdown Preview, Mermaid, and JSON/YAML Formatting: Tasks

One commit per task (`feat(preview): … (Tn)`, `feat(format): … (Tn)`). Each task leaves `go test ./internal/...`, `npx tsc --noEmit` and `npm test` green.

## Waves

Tasks in the same wave touch disjoint files and depend only on earlier waves, so they can run concurrently (one worktree per task, merged in task-number order at the end of the wave). A wave starts after the previous wave is merged and green.

```
W0  T1 deps
W1  T2 core tab-content │ T3 ReadImage │ T4 settings │ T5 JSON fmt │ T6 YAML fmt │ T7 md renderer │ T8 code highlight │ T9 mermaid
W2  T10 bindings + api.ts
W3  T11 formatting integration │ T12 preview tab
W4  T13 preview links/images/scroll sync │ T14 E2E formatting
W5  T15 E2E preview
W6  T16 docs, acceptance, retro
```

## W0: shared setup (1 task)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T1 | Add npm dependencies once, so W1 tasks don't fight over the lockfile: `@lezer/markdown` (explicit, same 1.8.x), `yaml` ^2, `mermaid` ^11 (exact versions pinned) | `frontend/package.json`, `frontend/package-lock.json` | `npm ci && npm run build` green; no source imports yet | §3.10 |

## W1: independent building blocks (8 tasks, concurrent)

Every W1 task is self-contained. Pure modules depend only on the interfaces fixed in the design (§3.2 `RenderOptions`/`Block`, §3.4 `renderDiagram`, §3.7 `FormatOutcome`), not on each other.

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T2 | Core: `.tab-content` per text tab; `Pane.contents`; activate/close/removePane handle it; `focusin` on content; `viewCreated` payload gains `content`; CSS. Grep `tools/e2e` and `test/` for `pane-host >` selectors | `editor/editorManager.ts`, `editor/editor.css`, `test/editor.test.ts` | new tests (§4 "core tab content"); existing editor tests; `smoke.json` E2E | R1.1 (enabler) |
| T3 | Backend image read: `vfs.ReadDataURL` (MIME allowlist, size cap) and `FileService.ReadImage` (EvalSymlinks + project check) | `internal/vfs/image.go` + `image_test.go`, `internal/rpc/files.go` + `files_image_test.go` | `go test -race ./internal/vfs ./internal/rpc` | R1.6 |
| T4 | Settings `[markdown] previewMode`, `[json] formatOnSave`, `[yaml] formatOnSave`; settings UI schema entries | `internal/settings/{settings.go,defaults.toml}` + store test, `frontend/src/views/settings/schema.ts` | `go test ./internal/settings`; tsc | R1.2, R2.6, R4.7 |
| T5 | JSON formatter: lexer, stack grammar, printer, comments, trailing commas, blank-line rule, error positions | `frontend/src/format/json.ts`, `frontend/test/formatJson.test.ts` | vitest: AC5, AC6, AC7 + §4 list | R2.1–R2.5 |
| T6 | YAML formatter: lazy `yaml`, failsafe parse, stringify options, error positions, round-trip guard; fixture corpus (workflow, k8s, compose, golangci) | `frontend/src/format/yaml.ts`, `frontend/test/formatYaml.test.ts`, `frontend/test/fixtures/yaml/*` | vitest: AC12, AC13, AC14 + corpus | R4.1–R4.6, R4.9 |
| T7 | Markdown renderer on `@lezer/markdown` + GFM: blocks with keys and `data-line`, slugs, safe raw HTML, `img[data-src]`, `a[data-href]`, mermaid placeholders, `highlight` option | `frontend/src/markdown/render.ts`, `frontend/test/markdownRender.test.ts` | vitest: AC4 at unit level, node coverage, no-innerHTML spy | R1.4, R1.8, R3.1 (placeholder) |
| T8 | Code highlighting: `languageFor`, `langIdForFence` in `languages.ts`; `schemeHighlightStyle` in `schemes.ts`; `createHighlighter` | `frontend/src/editor/languages.ts`, `frontend/src/theme/schemes.ts`, `frontend/src/markdown/highlight.ts`, `frontend/test/markdownHighlight.test.ts` | vitest; existing language/scheme tests | R1.5 |
| T9 | Mermaid wrapper: lazy load, `mermaidConfig` from tokens, serialized queue, LRU cache, SVG/error result | `frontend/src/markdown/mermaid.ts`, `frontend/test/mermaid.test.ts` (mocked `mermaid`) | vitest | R3.2, R3.3, R3.5, R3.6 |

File ownership in W1: only T2 edits `editorManager.ts`. Only T8 edits `languages.ts`/`schemes.ts`. Only T4 edits `schema.ts`. Nobody edits `features.ts`, `saveFlow.ts`, `api.ts` or `bindings/`.

## W2: bridge (1 task)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T10 | `npm run build`, `wails3 generate bindings -ts -i`; `files.readImage`; `MarkdownPreviewMode`; `Settings` narrowing for the new tables | `frontend/bindings/**`, `frontend/src/lib/api.ts` | tsc; vitest | R1.6, settings |

## W3: features (2 tasks, concurrent)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T11 | Formatting integration: `editor/formatters.ts` registry; generalize `saveFlow.ts` `run`, `editor.reformat` `when` and `beforeSave`; `format/install.ts` registers JSON (sync wrapper) and YAML; indent from `indentUnit`/`tabSize` facets | `frontend/src/editor/{formatters,saveFlow}.ts`, `frontend/src/format/install.ts`, `frontend/src/features.ts` (one line after `installSaveFlow`), `frontend/test/saveFlow.test.ts` | vitest: AC8 for JSON and YAML, error leaves doc and sets diagnostic/status, read-only no-op, formatOnSave on/off, Go path unchanged | R2.1, R2.5–R2.7, R4.1, R4.6–R4.8 |
| T12 | Preview tab: `Preview` controller, singleton ViewPlugin, `.tab-content` layout (editor, divider, preview, toolbar), modes + `markdown.previewMode` + large-file rule, `markdown.togglePreview` command + View menu, debounced render + keyed reconcile, highlighter wiring, theme/scheme generation, mermaid hydration with Copy source / Copy SVG and error display; CSS from tokens only. Check `vite build` chunking (§3.10) | `frontend/src/markdown/{preview,install}.ts`, `frontend/src/markdown/preview.css`, `frontend/src/features.ts` (one line after `installLanguages`), `frontend/src/app/menuBar.ts`, `frontend/test/markdownPreview.test.ts` | vitest (§4 "preview", fake timers for AC2); bundle check | R1.1–R1.5, R1.8, R1.10, R1.11, R3.1–R3.7 |

Both W3 tasks add one line to `features.ts` at different spots. Merge T11 first, then rebase T12; the conflict is trivial.

## W4: interactions and first E2E (2 tasks, concurrent)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T13 | `markdown/links.ts` (`classifyLink`, `resolveImage`); delegated click handling (external, anchor, file); image loading through `files.readImage` with per-preview cache invalidated on save/`fs.changed`; alt-text fallback; editor → preview scroll sync. Check remote images against the Wails CSP in a dev build | `frontend/src/markdown/{links,preview}.ts`, `frontend/test/markdownLinks.test.ts`, `frontend/test/markdownPreview.test.ts` | vitest; manual check of remote and local images in `wails3 dev` | R1.6, R1.7, R1.9 |
| T14 | Formatting E2E: sample project with JSON, JSONC, YAML fixtures; reformat, undo, error diagnostic, formatOnSave | `tools/e2e/format.json`, `tools/e2e/format-sample.sh` | `node tools/e2e/cdp.mjs tools/e2e/format.json` | AC5–AC8, AC12–AC14 |

## W5: preview E2E (1 task)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T15 | Preview E2E: README with heading, `<script>`, mermaid (valid and invalid), relative image and link; split default, toolbar modes, typing updates, theme switch re-renders diagrams | `tools/e2e/preview.json`, `tools/e2e/preview-sample.sh` | `node tools/e2e/cdp.mjs tools/e2e/preview.json` (server build) | AC1–AC4, AC9–AC11 |

## W6: wrap-up (1 task)

| # | Task | Files | Verify | Req |
|---|---|---|---|---|
| T16 | Docs: CLAUDE.md (commands for the new E2E, architecture notes for `markdown/`, `format/`, the `.tab-content` hook and `viewCreated.content`, gotchas found), steering (`structure.md`, `tech.md`: new deps and lazy chunks), `acceptance.md` (AC1–AC14 results), `retro.md`, design §6 | `CLAUDE.md`, `.claude/steering/*`, `.claude/designs/preview-and-formatting/{design,acceptance,retro}.md` | none | none |

## Critical path

T1 → T2/T7 → T10 → T12 → T13 → T15 → T16. The formatting track (T5/T6 → T11 → T14) is shorter and fully parallel to the preview track from W1 on.
