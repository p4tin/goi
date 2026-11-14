# Markdown Preview, Mermaid, and JSON/YAML Formatting: Acceptance

Status as of 2026-11-14, branch `feat/preview-and-formatting`, macOS (arm64). Written by the implementer; the user's review is pending.

## Automated checks

| Check | Result |
|---|---|
| `go test -race ./internal/...` | pass |
| `go vet ./...`, `go vet -tags server ./internal/rpc`, gofmt | clean |
| `npx tsc --noEmit`, `npm test` | clean; 485 tests (56 files) pass |
| E2E `tools/e2e/format.json` (server build) | every step passes (log below) |
| E2E `tools/e2e/preview.json` (server build) | every step passes (log below) |
| E2E `smoke.json`, `m2.json` with a server block (T2 touches every editor tab) | pass; same results as a build from before the branch |
| Bundle | main chunk +20.6 KB gzip; `mermaid` and `yaml` in lazy chunks |

New tests:
- **`internal/vfs`:** `ReadDataURL` (MIME by extension, unknown extension, directories, size limit, round trip).
- **`internal/rpc`:** `ReadImage` inside the project; outside, `..` escape and a symlink pointing outside are all rejected.
- **`internal/settings`:** defaults and layering for `[markdown]`, `[json]`, `[yaml]`.
- **Frontend:**
  - The JSON formatter (11 tests) and the YAML formatter (14 tests, including 4 real-world fixtures, idempotence and the round-trip guard).
  - The Markdown renderer (12 tests, including a spy proving `innerHTML` is never set), highlighting, Mermaid (mocked), links.
  - The preview (modes, large files, debounce, reconciliation, theme, diagrams, links, images, scroll targets), `formatBuffer` (undo, diagnostics, read-only, format on save), and the core tab-content element.

## Requirements

| Req | Status | Evidence |
|---|---|---|
| R1.1 split view on open | pass | E2E AC1; `markdownPreview.test.ts` |
| R1.2 three modes, toolbar, Toggle Preview command, `markdown.previewMode` | pass | E2E AC3 + "Toggle Preview command cycles"; unit tests. No default shortcut; the command is in the View menu |
| R1.3 updates within 300 ms, debounced | pass | E2E: h1 after 202 ms; unit test with fake timers |
| R1.4 Markdown coverage | pass | `markdownRender.test.ts`: headings, lists, task lists (disabled checkboxes), quotes, rules, tables with alignment, fences, inline code, emphasis, strikethrough, links (inline, reference, autolinks), images, entities |
| R1.5 fenced code highlighted with the editor's parsers and scheme | pass | E2E "6 highlighted spans"; `markdownHighlight.test.ts` |
| R1.6 relative images via the backend, alt text on failure | pass | E2E "dot.png loaded; alt: missing image"; rpc and unit tests |
| R1.7 http(s) in the browser, project files in the editor, anchors scroll | pass | E2E relative link opens `other.md`, `#details` present; unit test of click routing. Links outside the project show a warning |
| R1.8 no scripts, raw HTML as text, only http(s) remote images | pass | E2E AC4 (`window.__xss` undefined); unit tests |
| R1.9 editor → preview scroll sync | pass (unit) | `scrollTarget` interpolation tested; **not checked visually in the E2E** |
| R1.10 theme tokens only, follows theme changes | pass | `preview.css` uses only `var(--goi-*)`; E2E theme switch |
| R1.11 large files open in Editor mode, no automatic render | pass | unit test |
| R2.1 one undoable change, file indent settings | pass | E2E AC5/AC8; unit test with a tab indent unit |
| R2.2 tokens as written | pass | AC5 (20-digit int), `1.0`, escapes, key order (unit) |
| R2.3 comments and trailing commas | pass | E2E AC7; unit tests |
| R2.4 `{}` / `[]`, one member per line, `": "` | pass | AC5 |
| R2.5 syntax error: unchanged + diagnostic + status | pass | E2E AC6 ("Not formatted: syntax error at 1:7") |
| R2.6 `json.formatOnSave` (default off) | pass | E2E "formatted and saved"; unit test |
| R2.7 read-only does nothing | pass | unit test |
| R3.1 mermaid fences render as SVG | pass | E2E AC9 (nodes A, B) |
| R3.2 lazy load, source shown meanwhile | pass | dynamic `import('mermaid')`; separate chunk |
| R3.3 strict, `startOnLoad: false` | pass | `mermaid.test.ts` |
| R3.4 invalid diagram: source + error, rest renders | pass | E2E AC10 |
| R3.5 theme from tokens, re-render on theme change | pass | E2E AC11; unit test |
| R3.6 only changed diagrams re-render; typing not blocked | pass | keyed reconciliation + cache (unit tests); renders are queued and async |
| R3.7 Copy source / Copy SVG | pass | E2E "Copy source \| Copy SVG" (clipboard not exercised headless) |
| R4.1 one undoable change | pass | E2E AC8 for YAML |
| R4.2 indent size, spaces, sequences indented | pass | E2E AC12; tab indent unit → tabSize spaces (unit) |
| R4.3 comments, order, documents, anchors, tags, quoting, block scalars | pass | AC12; unit tests. Folded scalars keep their lines (see design §6) |
| R4.4 values unchanged, no folding | pass | E2E AC13; long-line unit test |
| R4.5 flow collections stay flow | pass | unit tests |
| R4.6 syntax error: unchanged + diagnostic + status | pass | E2E AC14 |
| R4.7 `yaml.formatOnSave` (default off) | pass | unit test |
| R4.8 read-only does nothing | pass | shared path with JSON (unit) |
| R4.9 Helm templates fail | pass | unit test |

## Not verified

- **The native app.** Every E2E ran in the server build in headless Chrome, not in the macOS WebKit webview. Remote `http(s)` images depend on the webview. Goi sets no CSP, so nothing should block them, but this wasn't checked in `wails3 dev`.
- **Scroll sync by eye** (R1.9), and the clipboard contents of Copy source / Copy SVG.
- Windows and Linux.

## E2E logs

`node tools/e2e/cdp.mjs tools/e2e/format.json`
```
EVAL AC5 Reformat File pretty-prints JSON, big int kept "formatted as expected"
EVAL AC8 one undo restores the JSON "restored"
EVAL AC6 syntax error: unchanged, diagnostic, status "Not formatted: syntax error at 1:7"
EVAL AC7 JSONC comments kept "// editor settings\n{\n  \"tabs\": true, // trailing note\n  /* block */ \"size\": 4,\n  \"list\": [\n    1,\n    2,\n  ] // after list\n}\n"
EVAL AC12 YAML reindented to 2, comments/anchor/block scalar kept "formatted as expected"
EVAL AC8 one undo restores the YAML "restored"
EVAL AC13 yes and 1.10 unchanged "unchanged"
EVAL AC14 invalid YAML: unchanged, diagnostic "Not formatted: syntax error at 2:1"
EVAL R2.6 json.formatOnSave formats before writing "formatted and saved"
```

`node tools/e2e/cdp.mjs tools/e2e/preview.json`
```
EVAL AC1 editor left, preview right (split) "editor 508px | preview 508px"
EVAL AC4 script shown as text, never run "literal, not executed"
EVAL AC2 typing '# Hello' renders <h1>Hello</h1> within 300 ms "h1 after 202 ms (typed in 8 ms)"
EVAL R1.5 Go fence highlighted "6 highlighted spans"
EVAL R1.6 local image loaded, missing one shows alt "dot.png loaded; alt: missing image"
EVAL AC9 mermaid renders an SVG with nodes A and B "svg nodes: A,B"
EVAL R3.7 diagram actions "Copy source | Copy SVG"
EVAL AC10 invalid diagram: source + error, rest renders "error: Parse error on line 2: ..."
EVAL AC11 theme switch re-renders the diagram "theme dark, new svg"
EVAL AC3 Preview only "only the preview"
EVAL AC3 Editor only "only the source"
EVAL R1.2 Toggle Preview command cycles to split "split"
EVAL R1.7 relative link opens the file "opened other.md×"
EVAL R1.7 anchor heading exists in the opened preview "h2#details"
```
