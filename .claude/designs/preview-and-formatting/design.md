# Markdown Preview, Mermaid, and JSON/YAML Formatting: Design

Covers R1–R4 of `requirements.md`. R3 and R4 are designed here too, on the assumption that they're approved as written.

## 1. Overview

```
 Markdown tab (one per pane × file)
 ┌ .tab-content (new, core) ───────────────────────────────────────┐
 │  .cm-editor (EditorView)        │  .md-preview (markdown/)      │
 │                                 │   ▲ renderBlocks(text)        │
 │  ViewPlugin: docChanged ────────┼──►│  reconcile by block key   │
 │  scrollDOM scroll ──────────────┼──►│  scroll sync (data-line)  │
 │                    [Editor|Split|Preview] toolbar               │
 └─────────────────────────────────────────────────────────────────┘
          images: FileService.ReadImage (data URL)   mermaid: lazy import

 Reformat File / beforeSave (editor/saveFlow.ts)
   .go           → EditorService.Format (unchanged)
   json family   → format/json.ts  (hand-written, token-preserving)
   .yaml/.yml    → format/yaml.ts  (lazy `yaml`, failsafe schema + round-trip check)
```

Principles:
- Everything is frontend except one bound call, `FileService.ReadImage` (R1.6).
- **One small core change.** Today `EditorView.dom` is a direct child of `pane.host`, so a feature can't put anything beside an editor inside its tab. T2 wraps each text view in a `.tab-content` element and hands it to features through `viewCreated`. This is a generic extension point, not Markdown code in `editorManager.ts`.
- The Markdown parser is `@lezer/markdown` with GFM. It's already in the bundle (through `@codemirror/lang-markdown`), it implements CommonMark properly, and it gives source positions for scroll sync. The renderer builds DOM nodes and never uses `innerHTML`. `agent/markdown.ts` stays as it is: it's a streaming-tolerant subset tuned for the agent transcript.
- Pure modules (`markdown/render.ts`, `format/json.ts`, `format/yaml.ts`) have no workbench imports, so they can be unit tested and built in parallel.
- Heavy dependencies (`mermaid`, `yaml`) load with dynamic `import()`. Vite splits them into their own chunks, so boot and first paint don't change.

## 2. Backend

### 2.1 `internal/vfs`: `ReadDataURL`

`vfs/image.go`:
- `ReadDataURL(path string, maxBytes int64) (string, error)` returns `data:<mime>;base64,<…>`.
- The MIME type comes from an extension allowlist: `png, jpg, jpeg, gif, webp, avif, bmp, ico, svg` (`image/svg+xml`). Any other extension returns `ErrNotImage`. Goi never sniffs content.
- Files larger than `maxBytes` return `ErrTooLarge`. The rpc layer passes 10 MB.
- An SVG shown through `<img>` can't run scripts, so it needs no sanitizing.

### 2.2 `internal/rpc`: `FileService.ReadImage`

`files.go`:

```go
// ReadImage returns an image inside the calling window's project as a data URL.
func (s *FileService) ReadImage(ctx context.Context, path string) (string, error)
```

- It resolves `filepath.Abs` and then `filepath.EvalSymlinks`, so a symlink inside the project can't point outside it. Then it checks `sess.contains(real)`. Paths outside the project return an error. Settings files get no exception here.
- It calls `vfs.ReadDataURL(real, 10<<20)`.
- Errors are plain errors. The frontend shows the image's alt text for any failure.

### 2.3 Settings (`internal/settings`)

`settings.go`, plus three new tables in `defaults.toml`:

```toml
[markdown]
previewMode = "split"       # editor | split | preview: how Markdown files open

[json]
formatOnSave = false        # pretty-print JSON files when saving

[yaml]
formatOnSave = false        # reformat YAML files when saving
```

The new structs are `Markdown{PreviewMode string}`, `JSON{FormatOnSave bool}` and `YAML{FormatOnSave bool}`. The `Settings` fields are `Markdown`, `JSON` (`toml:"json"`) and `YAML` (`toml:"yaml"`). An unknown `previewMode` acts as `split` in the frontend, so the backend doesn't validate it. The settings UI gets entries in `views/settings/schema.ts`: `markdown.previewMode` as a select in a new "Markdown" section, and both `formatOnSave` flags under "Save Actions".

## 3. Frontend

### 3.1 Core: tab content element (`editor/editorManager.ts`, `editor/editor.css`)

- `Pane.contents: Map<string, HTMLElement>` holds one `div.tab-content` per text tab. `createView` creates it, appends it to `pane.host`, and builds the `EditorView` with `parent: content`.
- `activate` hides and shows the contents (and binary placeholders) instead of `view.dom`. The `focusin` listener moves from `view.dom` to the content, so clicking a preview activates its tab.
- `close` and `removePane` remove the content element along with the view.
- The `viewCreated` payload gains `content: HTMLElement`. Existing listeners (`debug/inline.ts`) ignore it.
- CSS: `.pane-host > .tab-content` takes the old absolute `inset: 0` rule and is `display: flex` (row). `.tab-content > .cm-editor` is `flex: 1 1 0; min-width: 0`. A Go tab looks exactly as before.
- Risk: E2E selectors such as `.pane-host.lang-tooltip` don't depend on the direct child, and `.cm-editor` stays the same element. T2 greps `tools/e2e/*.json` and `test/` for `pane-host >` to confirm.

### 3.2 Markdown renderer (`markdown/render.ts`, pure)

```ts
export interface RenderOptions {
  /** Syntax-highlighted children for a fenced block, or null to show plain text. */
  highlight?(lang: string, code: string): Node[] | null;
}
export interface Block {
  /** Stable identity for reconciliation: node type + source text. */
  key: string;
  /** 0-based source line where the block starts (scroll sync). */
  line: number;
  el: HTMLElement;
}
export function renderBlocks(text: string, o?: RenderOptions): Block[];
export function slugify(text: string, used: Map<string, number>): string; // GitHub-style, -1, -2 for duplicates
```

- Parser: `parser.configure([GFM])` from `@lezer/markdown`, which adds tables, task lists, strikethrough and autolinks. Goi parses the whole document on each render. The parser is incremental-capable, but a full parse of a sub-threshold file takes a few milliseconds.
- Each top-level node becomes one `Block`. Node → element: `ATXHeading1..6`/`SetextHeading1..2` → `h1..h6` with `id = slugify(text)`. `Paragraph` → `p`. `BulletList`/`OrderedList` (with `start`) → `ul`/`ol` → `li`. `Task` → `<input type=checkbox disabled>` plus text. `Blockquote` → `blockquote`. `HorizontalRule` → `hr`. `Table` → `table`/`thead`/`tbody` with `align` from the delimiter row. `FencedCode`/`CodeBlock` → `pre > code` with `data-lang`. Inline: `Emphasis`, `StrongEmphasis`, `Strikethrough`, `InlineCode`, `Link`, `Autolink`/`URL`, `Image`, `HardBreak`, and `Escape` (which outputs the escaped character).
- Fenced blocks tagged `mermaid` become `div.md-mermaid[data-source]` containing the plain code block. The preview hydrates them (§3.5).
- Safety (R1.8): `HTMLBlock`, `HTMLTag`, `ProcessingInstruction` and `Comment` nodes are output as text: a block becomes `pre.md-raw`, an inline tag a text node. Links become `a[href="#"][data-href]`. Images become `img[alt][data-src]` with **no `src`**. The renderer never fetches or navigates; the preview controller resolves both (§3.6). Every string goes in through `textContent` or `createTextNode`.
- Every block element gets `data-line`.

### 3.3 Code block highlighting (`markdown/highlight.ts`, plus helpers)

- `editor/languages.ts` gains `languageFor(id: LangId): Promise<Language | null>`. It loads through the existing `loadLanguage` cache and unwraps `LanguageSupport.language`, or returns the `StreamLanguage` itself. It also gains `langIdForFence(tag: string): LangId | null`, which maps aliases (`golang`→go, `sh|bash|zsh|console`→shell, `yml`→yaml, `js`, `ts`, `jsonc`, …) and otherwise tries `languageIdFor('x.' + tag)`.
- `theme/schemes.ts` exports `schemeHighlightStyle(id: SchemeId): HighlightStyle`, built once per scheme and reused by `schemeExtension`, so the preview and the editor use the same style object (R1.5).
- `markdown/highlight.ts`: `createHighlighter(scheme: SchemeId, onLoaded: () => void)` returns the `RenderOptions.highlight` function. It runs `highlightCode` from `@lezer/highlight` on `language.parser.parse(code)` with the scheme's style and emits `span`s. If the language isn't loaded yet, it returns `null` (plain text), starts loading, and calls `onLoaded` once the language arrives, which triggers a re-render. `StyleModule.mount(document, style.module)` makes sure the style's classes exist even before any editor uses that scheme.

### 3.4 Mermaid (`markdown/mermaid.ts`)

```ts
export type DiagramResult = { svg: string } | { error: string };
export function renderDiagram(source: string, theme: UiThemeId): Promise<DiagramResult>;
export function mermaidConfig(theme: UiThemeId): MermaidConfig; // pure, tested
```

- Loading (R3.2): `import('mermaid')` runs on the first call and is memoized. A failed load gives `{error: 'Mermaid failed to load'}`.
- Configuration (R3.3, R3.5): `initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'base', themeVariables })`. `themeVariables` are concrete colors read from `uiThemes[theme]` in `theme/tokens.ts` (the source of truth). Mermaid computes derived colors, so it can't use CSS variables. Light → `darkMode: false`; dark and hc → `darkMode: true`. The mapping covers `background`, `primaryColor`, `primaryTextColor`, `primaryBorderColor`, `lineColor`, `secondaryColor`, `tertiaryColor`, `fontFamily` and `noteBkgColor`.
- Queue: mermaid keeps global state, so renders run one at a time through a promise chain. `initialize` runs again only when the theme changes.
- Cache (R3.6): an LRU of 64 entries keyed by `theme + '\0' + source`, holding results, errors included.
- Output: mermaid's `strict` mode sanitizes the SVG with DOMPurify. The preview parses it with `DOMParser` (`image/svg+xml`), imports the root `svg` node, and still never sets `innerHTML`. Each render gets a unique id (`goi-mmd-<n>`) to avoid id clashes between diagrams.
- Mermaid renders inside its own temporary container appended to `document.body`, then removes it. Goi doesn't render into hidden preview elements, which have no layout.

### 3.5 Preview feature (`markdown/preview.ts`, `markdown/install.ts`, `markdown/preview.css`)

`installMarkdownPreview(wb)` goes in `features.ts` after `installLanguages`.

**Mounting.** `editors.hooks.viewCreated` → for buffers where `languageIdFor(path) === 'markdown'` and the buffer isn't binary, it creates a `Preview(buf, view, content)` and keeps it in a `WeakMap<EditorView, Preview>`. One module-level `ViewPlugin` (added with `editors.addExtension(buf => isMd(buf) ? previewPlugin : [])`) forwards `update.docChanged` and handles `destroy()`. A singleton plugin keeps `refreshExtensions` from recreating it.

**DOM inside `.tab-content`:**
```
.tab-content.md-tab[data-md-mode=split]
  .cm-editor
  .md-divider            (drag to resize; ratio per tab, default 0.5)
  .md-preview[tabindex=0] > .md-body
  .md-toolbar            (absolute, top-right: Editor | Split | Preview segmented buttons, aria-pressed)
```
CSS decides what's visible: `[data-md-mode=editor] .md-preview, .md-divider {display:none}` and `[data-md-mode=preview] .cm-editor {display:none}`. After a mode change the controller calls `view.requestMeasure()`.

**Modes (R1.2, R1.11).** The initial mode is `settings.markdown.previewMode`, falling back to `split`. If `buf.meta.mode !== 'normal'` (above the large-file threshold), the initial mode is `editor` and the controller doesn't render until the user leaves Editor mode. The mode is per tab and in memory. The command `markdown.togglePreview` ("Markdown: Toggle Preview", category Markdown, `when`: active buffer is Markdown) cycles editor → split → preview on the active view's controller. It has no keymap entry, and it goes in the View menu (`app/menuBar.ts`). When focus would be lost (editor hidden), focus moves to `.md-preview`.

**Rendering (R1.3).** Doc changes are debounced 120 ms (trailing edge). Renders run in `requestAnimationFrame`, so typing never waits on them. With the debounce plus a ~10 ms render, updates land within 300 ms. In Editor mode, changes only mark the preview stale; it renders when it becomes visible.

**Reconciliation (R3.6, and steady images).** `reconcile(body, blocks)` keeps the existing child whose `data-key` equals the new block's key (in order, a simple keyed list diff) and replaces the rest. Reused elements only get `data-line` updated. Unchanged images don't reload, unchanged diagrams don't re-render, and scroll position is kept. The key includes a render generation that changes on theme or scheme changes, so those rebuild everything.

**Theme (R1.10, R3.5).** Preview CSS uses only `var(--goi-*)`. Code blocks use the editor scheme. An `effect` on `resolvedTheme` and `resolveScheme(settings.ui.scheme, resolvedTheme())` bumps the generation and re-renders.

**Mermaid hydration.** After reconcile, every new `div.md-mermaid` that isn't hydrated yet calls `renderDiagram(source, theme)`. On `{svg}`, it replaces the code with the SVG and adds hover actions **Copy source** / **Copy SVG** (`navigator.clipboard.writeText`) (R3.7). On `{error}`, it keeps the code and appends `div.md-error` with the message (R3.4). If the block was replaced before the promise resolved, the result is dropped (checked with `isConnected`).

### 3.6 Links, images and scroll sync (`markdown/links.ts`, `preview.ts`)

`links.ts` (pure):
```ts
export type LinkTarget =
  | { kind: 'external'; url: string }
  | { kind: 'anchor'; id: string }
  | { kind: 'file'; path: string; anchor?: string }
  | { kind: 'none' };
export function classifyLink(href: string, mdPath: string): LinkTarget;
export function resolveImage(src: string, mdPath: string): { kind: 'remote'; url: string } | { kind: 'local'; path: string } | { kind: 'none' };
```
- `http(s)://` → external. `#x` → anchor. Paths without a scheme are resolved against `dirName(mdPath)` with `.`/`..` normalized (OS separators preserved), and `%20` is decoded. Any other scheme (`javascript:`, `file:`, `data:`, `mailto:`) → none.
- Click (delegated on `.md-body`): external → `openExternal(url)`. Anchor → scroll to `#id` inside the preview, never `location.hash`. File → `editors.open(path)` (the backend rejects paths outside the project, and the editor reports the error).
- Images (R1.6): remote → set `src`. Local → `files.readImage(path)`, cached per preview as `Map<path, Promise<string>>` and cleared when the buffer is saved or `Topics.fsChanged` reports that path. None, or any load error → replace the `img` with `span.md-img-alt` holding the alt text. `img.onerror` handles remote failures the same way.

Scroll sync (R1.9, split mode only, editor → preview): a `scroll` listener on `view.scrollDOM`, throttled to one per animation frame. It finds the top line with `view.lineBlockAtHeight(scrollTop)`, then the two blocks around it (greatest `data-line` ≤ line, next block) by binary search over a cached `[line, offsetTop]` array that is rebuilt after each reconcile and on resize. It interpolates `preview.scrollTop`. At the editor's bottom it pins the preview to its bottom. A flag skips the update right after a programmatic scroll.

### 3.7 Formatting (`editor/formatters.ts`, `format/`, `editor/saveFlow.ts`)

**Registry (`editor/formatters.ts`):**
```ts
export interface FormatOptions { indentUnit: string; tabSize: number; path: string }
export type FormatOutcome =
  | { ok: true; text: string }
  | { ok: false; line: number; col: number; message: string }; // 1-based
export interface DocFormatter {
  id: string;                        // 'json' | 'yaml', also the diagnostic source
  matches(path: string): boolean;
  onSave(s: Settings): boolean;      // json.formatOnSave / yaml.formatOnSave
  format(text: string, o: FormatOptions): Promise<FormatOutcome>;
}
export function registerFormatter(f: DocFormatter): void;
export function formatterFor(path: string): DocFormatter | null;
```

**`saveFlow.ts` changes.**
- `run(buf, mode?)` keeps the Go path. For other files it uses `formatterFor(buf.path)` with `indentUnit` from `view.state.facet(indentUnit)` and `tabSize` from `view.state.tabSize`, so per-file indent settings apply (R2.1).
- The rest is shared: a syntax error sets the diagnostic and shows the status message "Not formatted: syntax error at L:C" (R2.5, R4.6), with source `id`. Success clears the diagnostic. If the document changed while formatting, nothing is applied. Otherwise it dispatches one `changesFromDiff` transaction with `userEvent: 'format'`: one undo step, minimal changes (R2.1, R4.1, AC8). Read-only buffers return early (R2.7, R4.8).
- `editor.reformat`'s `when` becomes "`.go` or `formatterFor(path)`".
- `beforeSave` runs the Go path as before (`save.format`), and otherwise runs the formatter if `onSave(settings)` (R2.6, R4.7).

**`format/install.ts`.** `installFormatters` registers JSON and YAML. It goes in `features.ts` after `installSaveFlow`. JSON is `.json`, `.jsonc`, `.code-workspace`, plus any path where `languageIdFor` returns `json`. YAML is `.yaml` and `.yml`.

### 3.8 JSON formatter (`format/json.ts`, pure, no dependencies)

`formatJSON(text: string, indent: string): FormatOutcome` (synchronous; the registry wraps it).

- **Lexer.** It produces tokens `{ } [ ] : ,`, strings (escapes validated, text kept verbatim), numbers (grammar-checked with a regex, text kept verbatim: R2.2), `true`/`false`/`null`, `// …` and `/* … */` comments. Each token records `nlBefore`, the number of newlines between it and the previous token.
- **Grammar check.** A state machine with a stack decides whether each token is allowed: value / key / colon / comma-or-close. A single trailing comma before `}` or `]` is accepted and kept. Nesting is handled by an explicit stack, not recursion, so deep nesting can't overflow. The first violation reports the offending token's start, or end of input. Empty input → `{ ok: true, text }` unchanged. A second top-level value → error.
- **Printer.** `{`/`[` followed directly by its closer prints `{}`/`[]` (R2.4). An empty container that holds a comment prints multi-line. Otherwise, after an opener: newline plus indent. After `,`: newline plus indent. Before a closer: newline plus outer indent. `:` prints as `": "`.
- **Comments (R2.3).** A comment with `nlBefore = 0` that follows a value or comma stays at the end of that line (`␠// c`). A comment with `nlBefore > 0` goes on its own line at the current indent. Inline `/* */` between tokens on one line stays inline with single spaces. Block comments keep their inner text.
- **Blank lines.** **(decision)** A member preceded by one or more blank lines keeps exactly one blank line, so grouping survives. This is the same rule as Prettier.
- **Ending.** The output ends with `\n` when the input did. Comments and trailing commas are accepted for every JSON-family file. **(decision)** One lenient grammar is simpler, and R2.3 only requires that `.json` comments don't cause an error.

### 3.9 YAML formatter (`format/yaml.ts`)

`formatYAML(text: string, indentSize: number): Promise<FormatOutcome>`. The indent size is `indentUnit.length` when it's spaces, else `tabSize` (R4.2).

- `const Y = await import('yaml')` is memoized (R4 lazy load).
- `parseAllDocuments(text, { schema: 'failsafe', strict: false, uniqueKeys: false, prettyErrors: true })`. **The failsafe schema reads every scalar as a string**, so `yes`, `1.10`, `0x1F` and `~` can't be re-typed or re-printed (R4.4). Custom and core tags (`!Ref`, `!!int`) stay as tags; their "unresolved tag" warnings are ignored.
- **Errors.** The first `doc.errors[0]` → `linePos[0]` → `{line, col}` (R4.6). Helm templates fail here (R4.9).
- **Stringify.** Each document goes through `doc.toString({ indent, indentSeq: true, lineWidth: 0, minContentWidth: 0, flowCollectionPadding: false })`, and the results are joined. The parsed nodes already carry the scalar `type` (PLAIN, QUOTE_SINGLE, QUOTE_DOUBLE, BLOCK_LITERAL, BLOCK_FOLDED with chomping), `flow` on collections (R4.5), `anchor`/alias nodes, tags, `commentBefore`/`comment`, `spaceBefore`, and explicit document markers (R4.3).
- **Round-trip guard.** This is the main mitigation for library edge cases. The output is parsed again the same way and must be equal on (a) `doc.toJS()` for every document (deep equality on failsafe strings) and (b) the multiset of comment texts collected with `Y.visit` plus the document-level comments. On any mismatch → `{ ok: false, line: 1, col: 1, message: 'formatting would change the document' }`, shown as "Not formatted: …". In practice the guard fires rarely, and when it does the user's file is safe.
- An empty or whitespace-only input is returned unchanged.

### 3.10 Dependencies and bundle

- `package.json` gets `@lezer/markdown` (explicit; already present at 1.8.x), `yaml` (^2) and `mermaid` (^11). The exact versions are pinned when T1 installs them.
- `mermaid` and `yaml` are only reached through `import()`. T12 confirms `vite build` emits them as separate chunks and that `index-*.js` grows by less than 40 KB gzip (renderer + preview + JSON formatter).

### 3.11 API (`lib/api.ts`)

- `files.readImage(path: string): Promise<string>` → `FileService.ReadImage`.
- `Settings` picks up `markdown`, `json` and `yaml` from regenerated bindings. `MarkdownPreviewMode = 'editor' | 'split' | 'preview'` is exported for the frontend.

## 4. Testing

| Area | Tests |
|---|---|
| `vfs.ReadDataURL` | MIME by extension, unknown extension, size limit, base64 round trip |
| `FileService.ReadImage` | inside OK; outside rejected; symlink escaping the project rejected |
| settings | defaults load; TOML round trip of the new tables |
| `format/json.ts` | AC5 exact output; big ints and `1.0` verbatim; escapes; key order; `{}`/`[]`; comments (trailing, own line, inline block) kept (AC7); trailing comma kept; blank-line rule; tabs; AC6 error position; second top-level value; empty input; 10k-deep nesting doesn't throw |
| `format/yaml.ts` | AC12 (4→2 indent, comments, anchor/alias, `|` block); AC13 (`yes`, `1.10`); quoting styles kept; `---` multi-doc; flow collections stay flow; `indentSeq`; tags; AC14 error position; Helm template fails; round-trip guard trips on a forced mismatch (injected stringify) |
| `markdown/render.ts` | each node type; heading slugs and duplicates; task lists disabled; raw HTML and `<script>` as text (AC4); no `innerHTML` (spy on the setter); images have no `src`; `data-line`; keys stable across unrelated edits |
| `markdown/links.ts` | external, anchor, relative file with `..` and `%20`, `javascript:` → none; image resolution |
| `markdown/highlight.ts` | known language highlighted after load callback; unknown → null |
| `markdown/mermaid.ts` | config is strict, `startOnLoad:false`, theme variables from tokens; cache hit; error result; serialized queue (mocked `mermaid` via `vi.mock`) |
| core tab content | text tab lives in `.tab-content`; activate toggles contents; `viewCreated` carries content; focusin on a sibling activates the tab; close removes it |
| preview | default split; setting respected; large file starts in editor and doesn't render; toggle cycles; typing updates within debounce (fake timers) (AC2); reconcile keeps unchanged nodes; theme change re-renders; mermaid success, error and actions with mocked `renderDiagram`; link clicks route; image alt fallback |
| saveFlow | JSON/YAML reformat one undo (AC8); syntax error leaves doc unchanged plus diagnostic and status; read-only no-op; formatOnSave on/off; Go path unchanged |
| E2E | `tools/e2e/format.json` (AC5–AC8, AC12–AC14 in the real app), `tools/e2e/preview.json` (AC1–AC4, AC9–AC11) with `tools/e2e/preview-sample.sh <dir>` |

## 5. Risks

- **`yaml` stringify fidelity**: comment placement in odd spots, flow collections spanning lines, `---`/`...` markers. Mitigations: failsafe schema, the round-trip guard (no silent damage), and the AC12–AC14 fixtures plus a corpus of real files (GitHub workflow, k8s manifest, docker-compose, golangci config) in tests.
- **Mermaid bundle size and DOM needs**: about 1–3 MB gzip across lazy chunks, which only matters once a diagram is shown. Mermaid needs real layout, which the temporary attached container provides. It can't run under happy-dom, so unit tests mock it and only E2E exercises it.
- **Wails webview CSP**: remote `http(s)` images must be allowed by the page CSP. T13 checks this in the real app. If the CSP blocks them, the alt text shows, which is allowed by R1.6 but would be a regression worth noting.
- **Core change (T2)**: touches every editor tab. It's covered by existing editor tests, the new tab-content tests, and smoke/m2/m3 E2E.
- **Scroll sync precision**: top-level blocks only, so long lists and tables sync coarsely. That's acceptable for R1.9.

## 6. Changes during implementation

- **`FormatOutcome` gained `kind: 'syntax' | 'unsafe'`.** The YAML round-trip guard returns `unsafe`. `saveFlow` shows "Not formatted: formatting would change the document" for it, without a diagnostic, since there's no error position.
- **Folded block scalars (`>`, `>-`, `>+`).** The `yaml` library re-folds their content ("long\ntext" → "long text"): same value, different text, which R4.3 doesn't allow. `format/yaml.ts` swaps each folded scalar for a placeholder before stringifying, then puts the original lines back, re-indented, keeping the chomping indicator. Scalars with an explicit indentation indicator (`>2`) keep the library's output.
- **Known YAML limitation.** Multi-line *flow* scalars (a plain or quoted string continued on the next line) are joined onto one line. The value is unchanged and the round-trip guard accepts it.
- **`saveFlow.formatBuffer`, `formatsOnSave` and `canReformat` are exported** for tests. The Go path behaves as before.
- **Preview CSS classes are `mdp-*`** (`mdp-code`, `mdp-table`, `mdp-mermaid`, `mdp-raw`, `mdp-tight`, `mdp-task`). `agent/agent.css` already styles `.md-code` and `.md-table` globally.
- **Preview mode needs `display: none !important`** on `.cm-editor`, because CodeMirror's base theme sets `display: flex !important`. The preview E2E caught this; happy-dom unit tests can't.
- **SVG parsing uses `text/html`, not `image/svg+xml`.** Mermaid's sanitized output can contain HTML labels that aren't well-formed XML. HTML parsing never runs scripts either.
- **Links to files outside the project** show a warning notification and don't open. `FileService.Read` has no project guard, so the frontend checks the project roots (`links.insideRoots`).
- **HTML comments** (`<!-- … -->`, block or inline) are hidden instead of shown as text. Other raw HTML still shows as text (R1.8).
- **Heading ids are part of the block key**, because a heading's id depends on earlier duplicates.
- **The highlight style's classes** come from the Markdown buffer's own editor view, which uses the same scheme, so the preview doesn't mount the style module itself.
- **`markdown.previewMode` is in the settings UI's Editor section**, not a new Markdown section. The two `formatOnSave` flags are under Save Actions.
- **Bundle:** the main chunk grew 20.6 KB gzip (382.8 → 403.4 KB). Mermaid (and its diagram chunks) and `yaml` are separate lazy chunks.
