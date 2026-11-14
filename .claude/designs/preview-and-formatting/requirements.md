# Markdown Preview, Mermaid, and JSON/YAML Formatting: Requirements

Status: R1 (Markdown preview) and R2 (JSON) approved. R3 (Mermaid) and R4 (YAML) were added after review and are waiting for approval.

## Context

Goi already highlights Markdown, JSON and YAML (`editor/languages.ts`) and formats Go files on save (`editor/saveFlow.ts`). Projects usually also contain READMEs and design notes (often with Mermaid diagrams), plus JSON and YAML config, CI workflows, Kubernetes manifests and fixtures. This feature adds:

1. A live rendered preview for Markdown files, including Mermaid diagrams.
2. Pretty-printing for JSON and YAML files.

All of it is frontend work except one backend call: a bound call that reads a local image inside the project and returns it as a data URL (R1.6). Goi has no binary-read binding today. Goi already has a safe Markdown renderer for the AI Agent window (`agent/markdown.ts`, which builds DOM nodes and never uses innerHTML).

New npm dependencies (both **lazily loaded**, so boot time and the first editor paint don't change):
- `mermaid`: renders diagrams (R3). It's large, so it loads only when a preview first contains a diagram.
- `yaml` (eemeli/yaml): a YAML parser that keeps comments (R4). It loads on the first YAML format.

## R1 Markdown preview

- R1.1 WHEN the user opens a file whose language is Markdown (`.md`, `.markdown`) THEN Goi SHALL show the editor and a rendered preview side by side in the same tab (split mode).
- R1.2 Each Markdown tab SHALL have three view modes: **Editor**, **Split** and **Preview**. A small toolbar at the top right of the tab SHALL switch between them. The command **Markdown: Toggle Preview** cycles through the modes, and it has no default shortcut. **(decision)** The default mode for a newly opened file is Split. The `markdown.previewMode` setting (`editor` | `split` | `preview`, default `split`) can change it.
- R1.3 WHEN the document changes (typing, undo, agent edit, reload from disk) THEN the preview SHALL update within 300 ms. Updates are debounced so typing never blocks on rendering.
- R1.4 The preview SHALL render at least what `agent/markdown.ts` supports: headings, paragraphs, ordered and unordered lists, task lists (`- [ ]` / `- [x]`, shown as read-only checkboxes), block quotes, horizontal rules, tables, fenced code blocks, inline code, bold, italic, links and images.
- R1.5 Fenced code blocks with a known language tag (go, json, yaml, sh, …) SHOULD be syntax-highlighted with the same CodeMirror highlighter and theme as the editor.
- R1.6 Images with relative paths SHALL resolve against the Markdown file's folder. The frontend never touches the filesystem, so local images load through a backend call. IF an image cannot load THEN the preview SHALL show its alt text.
- R1.7 WHEN the user clicks an `http(s)` link in the preview THEN Goi SHALL open it in the system browser. WHEN the user clicks a relative link to a project file THEN Goi SHALL open that file in the editor. A `#anchor` link SHALL scroll the preview to the matching heading.
- R1.8 The preview SHALL NOT run scripts, render raw HTML as markup, or load remote content other than `http(s)` images. Raw HTML blocks are shown as plain text.
- R1.9 In split mode, scrolling the editor SHALL scroll the preview to the matching block (editor to preview only). **(decision)** Two-way scroll sync is out of scope.
- R1.10 Preview colors and fonts SHALL come only from theme tokens (`var(--goi-*)`) and SHALL follow theme changes.
- R1.11 IF a Markdown file is larger than the large-file threshold THEN Goi SHALL open it in Editor mode and SHALL NOT render it automatically. The user can still switch to the preview.

## R2 JSON formatting

- R2.1 WHEN the user runs **Reformat File** (`editor.reformat`) on a JSON buffer THEN Goi SHALL pretty-print it as one undoable change. Indentation comes from the editor's indent settings (tab size and tabs or spaces) for that file.
- R2.2 The formatter SHALL keep tokens as written: numbers stay as typed (no precision loss on large integers, `1.0` stays `1.0`), strings keep their escapes, and key order is unchanged.
- R2.3 For `.jsonc` and `.code-workspace` files, the formatter SHALL keep `//` and `/* */` comments and allow trailing commas. For `.json`, comments are kept if present, but they don't cause an error.
- R2.4 Empty objects and arrays SHALL be printed as `{}` and `[]`. Each other member or element goes on its own line, with `": "` after keys.
- R2.5 IF the JSON has a syntax error THEN Goi SHALL leave the document unchanged, show a diagnostic at the error position, and show a status message ("Not formatted: syntax error at line:col"), the same way the Go flow does.
- R2.6 WHEN a JSON file is saved and `json.formatOnSave` is true THEN Goi SHALL format it before writing. **(decision)** The default is `false`, so saving never rewrites JSON fixtures unless the user opts in.
- R2.7 IF the buffer is read-only THEN Reformat File SHALL do nothing.

## R3 Mermaid diagrams in the preview

- R3.1 WHEN the preview contains a fenced code block tagged `mermaid` THEN Goi SHALL render it as a diagram (SVG) in place of the code block.
- R3.2 Mermaid SHALL load only the first time a preview contains a `mermaid` block. While it loads, the block shows its source as code.
- R3.3 Mermaid SHALL run with `securityLevel: 'strict'`: no click handlers, no scripts, and HTML labels sanitized. `startOnLoad` SHALL be off; Goi calls the renderer itself.
- R3.4 IF a diagram has a syntax error THEN the preview SHALL show the source as code with the error message under it. The rest of the document still renders.
- R3.5 Diagrams SHALL use a Mermaid theme that matches Goi's current light or dark theme, with colors taken from theme tokens. WHEN the theme changes THEN the diagrams SHALL re-render.
- R3.6 Only diagrams whose source changed SHALL re-render on a document change (cache by source text + theme), so typing in prose doesn't redraw every diagram. Rendering SHALL NOT block typing in the editor.
- R3.7 Each diagram SHALL offer **Copy source** and **Copy SVG** actions.

## R4 YAML formatting

- R4.1 WHEN the user runs **Reformat File** on a YAML buffer (`.yaml`, `.yml`) THEN Goi SHALL reformat it as one undoable change.
- R4.2 Indentation SHALL use the file's indent size with spaces (YAML forbids tabs). Block sequences under a mapping key SHALL be indented by one level (`key:\n  - item`). **(decision)** This is the style most linters and formatters produce.
- R4.3 The formatter SHALL keep comments (full-line and end-of-line), key order, multiple documents (`---`), anchors and aliases, tags, and the quoting style of scalars (plain, single, double). Block scalars (`|`, `>`) keep their content and chomping indicators.
- R4.4 The formatter SHALL NOT change values: no `yes`→`true` conversion, numbers stay as typed, and long lines are not folded (line width unlimited).
- R4.5 Flow collections (`[a, b]`, `{a: 1}`) SHALL stay in flow style.
- R4.6 IF the YAML has a syntax error THEN Goi SHALL leave the document unchanged, show a diagnostic at the error position, and show the status message ("Not formatted: syntax error at line:col").
- R4.7 WHEN a YAML file is saved and `yaml.formatOnSave` is true THEN Goi SHALL format it before writing. Default is `false`.
- R4.8 IF the buffer is read-only THEN Reformat File SHALL do nothing.
- R4.9 Go templates in YAML (Helm charts: `{{ … }}` outside quotes) are usually not valid YAML. **(decision)** Such files fail per R4.6, and Goi never guesses.

## Out of scope

- Math (KaTeX) and footnotes in the preview.
- Exporting the preview or diagrams to HTML, PDF or image files.
- A Mermaid editor or live diagram toolbar beyond R3.7.
- Minify or compact JSON, sorting keys, and JSON Schema validation.
- Converting between JSON and YAML.
- Formatting TOML or other languages.

## Acceptance criteria

- AC1 WHEN `README.md` is opened THEN the tab shows the editor on the left and the rendered preview on the right.
- AC2 WHEN the user types `# Hello` in the editor THEN an `<h1>Hello</h1>` appears in the preview within 300 ms.
- AC3 WHEN the user picks Preview in the toolbar THEN only the rendered document is visible. Picking Editor shows only the source.
- AC4 IF the Markdown contains `<script>alert(1)</script>` THEN no script runs and the text is shown literally.
- AC5 WHEN Reformat File runs on `{"a":1,"b":[1,2,{"c":12345678901234567890}],"d":{}}` with 2-space indentation THEN the buffer becomes the expected multi-line form, with `12345678901234567890` unchanged and `"d": {}`.
- AC6 WHEN Reformat File runs on `{"a": }` THEN the document is unchanged and a diagnostic marks the error.
- AC7 WHEN a `.jsonc` file with comments is reformatted THEN every comment is still present.
- AC8 One undo after Reformat File restores the original text (JSON and YAML).
- AC9 WHEN a Markdown file contains a ```` ```mermaid ```` block with `graph TD; A-->B` THEN the preview shows an SVG diagram with nodes A and B.
- AC10 WHEN a mermaid block contains invalid syntax THEN the preview shows the source and an error message, and the rest of the document still renders.
- AC11 WHEN the theme switches between light and dark THEN the diagrams re-render in the new theme.
- AC12 WHEN Reformat File runs on YAML with 4-space indentation, comments, an anchor/alias and a `|` block scalar, using a 2-space indent setting THEN the result is indented by 2, with every comment, the anchor/alias and the block scalar content unchanged.
- AC13 WHEN Reformat File runs on YAML containing `enabled: yes` and `version: 1.10` THEN they stay `yes` and `1.10`.
- AC14 WHEN Reformat File runs on invalid YAML (`a: [1, 2`) THEN the document is unchanged and a diagnostic marks the error.
