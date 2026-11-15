# Application Packs — Requirements

Status: approved 2026-11-15 (Q1–Q4 resolved below).

## Context

Goi is a Go IDE, but about two thirds of it (windows and lifecycle, vfs, settings, git, terminal, search, LSP/DAP clients, AI agents, Markdown, JSON/YAML, editor, themes, keymaps) doesn't depend on the language. We want a second product, a **Python IDE**, built on the same core.

The products ship as **separate applications**, not as one IDE with plugins. Each one is defined by an **application pack**. A pack holds:

- **Identity:** name, icons, bundle identifier, config/cache directory names, version, file associations, release artifact names.
- **Language tooling:** language server(s), formatter, linter, SDK/interpreter resolver, run/test planner, debug adapter, project detection.
- **Language-specific UI:** frontend features such as the Go test tree's benchmarks, `go.mod` tooling and the Go SDK status.

This feature covers:

- **Part A:** the core/pack split. Go moves behind the pack interface with no change in behavior, and the build/release pipeline produces N apps.
- **Part B:** a second app (working name **Pyi**, see Q1). Its pack has its own identity and the minimum language tooling to prove the interface: Python syntax, a Python language server, and running a script.

Full Python tooling (pytest tree, debugpy, ruff lint and format, interpreter/venv management) is a follow-up spec, `python-app`. Its requirements are sketched in R9 so that Part A's interfaces can carry it.

## Terms

- **Core:** everything that isn't in a pack. It knows nothing about Go or Python.
- **Pack:** the Go code, frontend code and build assets for one product.
- **App:** the binary/bundle built from core + one pack.

## R1 Pack definition

- R1.1 A pack SHALL live in two fixed places: `apps/<app>/` (entry point, backend pack code, build assets: icons, plist/manifest templates, desktop file, NSIS/nfpm/dmg settings) and `frontend/src/packs/<app>/` (frontend pack code). The frontend half stays inside `frontend/` so that Vite, tsc and vitest resolve `node_modules` without extra configuration.
- R1.2 Each app SHALL be built from core + exactly one pack. Core SHALL NOT import any pack, and no pack SHALL import another pack.
- R1.3 Core SHALL get everything language- or product-specific through interfaces that the pack implements (backend) or registers (frontend features). Core code SHALL NOT branch on which pack is active.
- R1.4 A pack MAY leave out a capability (for example, no linter). IF a capability is missing THEN core SHALL hide its UI (commands, menus, settings, status items) and SHALL NOT fail.

## R2 Product identity

A pack SHALL define all of the following. None of them may be hard-coded in core:

- R2.1 Display name: window titles, About, Welcome, macOS app menu, Dock, dialogs, toasts and error messages.
- R2.2 Program name: binary name, `ProgramName`, log file name, CLI flag-set name.
- R2.3 Bundle/app identifier (macOS `CFBundleIdentifier`, Windows AppUserModelID/manifest, Linux desktop id) and the `SingleInstance` id.
- R2.4 Icons: app icon (svg source, png, icns, ico) and the in-app icon shown in Welcome and About.
- R2.5 Product version. Each pack has its own version (in its build config) and its own release tags (see Q3).
- R2.6 User directory names: `<UserConfigDir>/<Name>/<major.minor>`, `<UserCacheDir>/<Name>/<major.minor>`, and the home-override environment variable (`GOI_HOME` for Goi).
- R2.7 Project state directory (`.goi/` for Goi).
- R2.8 File associations.
- R2.9 Names the app announces to other programs: LSP/DAP `clientInfo`, ACP `clientInfo`, the MCP server name and its agent config key, `TERM_PROGRAM`, the `mcp-bridge` executable and its token environment variable.
- R2.10 Release artifact names: dmg, zip, NSIS installer, deb, rpm, tar.gz, and the GitHub Release title and notes.

## R3 Goi stays the same (Part A)

- R3.1 WHEN Goi is built from core + Go pack THEN every existing feature SHALL behave as it does today.
- R3.2 Goi SHALL keep its current identity values: bundle id `dev.goi.goi`, user dirs `Goi/0.1`, `GOI_HOME`, `.goi/`, binary `goi`, artifact names. Existing users' settings, recent projects, layouts, run configurations, journal, installed tools and agents, and macOS permissions SHALL carry over without migration.
- R3.3 Saved setting values SHALL keep their meaning, including theme ids (`goi-light`…) and keymap ids (`goi-default`). Only the display names change (Q4).
- R3.4 The existing Go tests, vitest suites and E2E suites (smoke, multiwindow, m2, m3, m4, agent, format, preview) SHALL pass. Test changes SHALL be limited to import paths and wiring.
- R3.5 The developer commands in `CLAUDE.md` (`wails3 dev`, `wails3 build`, `go test ./internal/...`, the server build, the E2E harness) SHALL keep working for Goi. A command with a different form after the split SHALL be documented.

## R4 Building and releasing several apps

- R4.1 A developer SHALL be able to build, run in dev mode, and package each app from the same repository with one command per app (for example, `wails3 task goi:build`, `wails3 task pyi:dev`).
- R4.2 One Go module and one `frontend/` package SHALL serve all apps. Each app's frontend bundle SHALL include only core features plus its own pack's features.
- R4.3 The server build (`-tags server`) and the E2E harness SHALL work for each app.
- R4.4 CI SHALL test core and every pack, and SHALL build every app on macOS, Windows and Linux.
- R4.5 WHEN a release tag for an app is pushed THEN the release workflow SHALL build and draft a release for that app only, with that app's artifact names, and SHALL check the tag against that pack's version.
- R4.6 Two different apps SHALL be able to run at the same time on one machine without sharing config, cache, single-instance lock, MCP token or Dock menu.

## R5 Backend pack interface (capabilities)

Core SHALL define pack interfaces for these capabilities. The Go pack implements all of them today:

- R5.1 **Project detection:** recognise a project root and its structure (Go: `go.mod`/`go.work` modules; Python: `pyproject.toml`, `setup.py`, `requirements.txt`, `.venv`). Excludes are part of detection.
- R5.2 **SDK/interpreter:** discover, validate and select the toolchain for a project, and provide the environment that processes started for the project run with (Go: `gosdk`; Python: interpreter/venv).
- R5.3 **Language servers:** zero or more supervised LSP servers per project, each with the languages/file patterns it handles, its command and settings, and its restart policy. Today `internal/gopls`'s supervision is generic; its gopls-specific parts move to the pack.
- R5.4 **Formatter(s):** format a buffer on Reformat and on save (Go: gofmt/goimports).
- R5.5 **Linter:** run, normalise issues and fixes (Go: golangci-lint).
- R5.6 **Tool installs:** resolve and install the pack's tools (Go: `go install` of gopls, dlv, golangci-lint; Python: `uv tool`/pip into `<config>/tools`).
- R5.7 **Run configuration types:** the types the pack contributes (Go Build/Test/Remote), how they validate and become command plans, plus run markers in the gutter (Go: `gosyntax`). Shell and Makefile types stay in core.
- R5.8 **Test runner:** turn the runner's output stream into core's test events (Go: `go test -json`).
- R5.9 **Debug adapter:** start the DAP server and build launch/attach args (Go: `dlv dap`). Core keeps the DAP client and the debug UI.
- R5.10 **Agent context:** the pack MAY add environment variables and MCP tools for agents. The existing MCP tools SHALL stay in core.

## R6 Frontend pack interface

- R6.1 Each pack SHALL provide a frontend entry: its feature installers and its identity (name, icon), which About, Welcome and the window title read.
- R6.2 The frontend feature list SHALL be core features + pack features. Feature order rules (dock defaults) SHALL still hold.
- R6.3 Language-specific frontend code SHALL move into the pack: `mod/` (go.mod), the Go SDK status and picker, Go-specific parts of `run/` (benchmarks, Go test-name parsing, run gutter rules), and Go language registration.
- R6.4 Core settings SHALL stay in core. Each pack SHALL contribute its own settings sections (Go: `gopls.*`, `debug.dlvPath`, `lint.*`, `go.*`).

## R7 Second app (Part B: Pyi skeleton)

- R7.1 Pyi SHALL have its own identity for everything in R2 (name, icons, bundle id, user dirs, home override variable, project dir, artifacts).
- R7.2 WHEN Pyi opens a folder THEN it SHALL show the project tree, edit files with Python syntax highlighting, and provide git, terminal, search, Markdown preview, JSON/YAML formatting and AI agents, exactly like core features in Goi.
- R7.3 WHEN a Python language server is available THEN Pyi SHALL provide completion, hover, go to definition, usages, rename and diagnostics through it (see Q2 for the server).
- R7.4 Pyi SHALL run the current Python file as a run configuration, using the interpreter selected for the project.
- R7.5 Pyi SHALL NOT show any Go UI (Go SDK, gopls, go.mod, Delve, golangci-lint).

## R8 Non-functional

- R8.1 The split SHALL NOT measurably change Goi's startup time, completion latency (`GOI_GOPLS_BENCH`) or binary size (within 5%).
- R8.2 Boundaries SHALL be enforced by a test: core packages SHALL NOT import pack packages, and packs SHALL NOT import each other.
- R8.3 Windows, macOS and Linux stay first-class for both apps.

## R9 Out of scope (follow-up `python-app` spec)

R5 has to be able to carry these:

- pytest/unittest test tree, through a bundled pytest plugin that streams events.
- debugpy debugging.
- ruff lint, fixes and format; Black as an option.
- Interpreter discovery and selection: `.venv`, uv, poetry, conda, pyenv, system.
- `pyproject.toml` tooling.
- Django/Flask/Jupyter: not planned.

## Decisions (were open questions)

- **Q1** The Python app is **Pyi**: bundle id `dev.goi.pyi`, binary `pyi`, home override `PYI_HOME`, project dir `.pyi/`, user dirs `Pyi/<major.minor>`.
- **Q2** Python language server: **basedpyright**, installed from PyPI into `<config>/tools`; no system node needed. The ruff server is not part of Part B.
- **Q3** Each app has its own version and tags: `goi-v<version>` and `pyi-v<version>`. The existing plain `v*` tags keep releasing Goi.
- **Q4** Theme and keymap display names become generic: Light, Dark, Ember, Paper, and "Default" for the keymap. Ids are unchanged. This applies to both apps.
