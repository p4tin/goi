# Product — Goi

## Vision
Goi is an original, cross-platform desktop IDE for Go. Its feature set is comparable to commercial Go IDEs, but it is built by orchestrating the official Go toolchain (`go`, `gopls`, `dlv`, linters) rather than reimplementing Go analysis. Full product spec: `SPEC.md` (repo root).

## Target audience
- Go backend developers working with modules and multi-module workspaces (`go.work`).
- Polyglot service developers who also edit YAML, Dockerfile, SQL, protobuf, Terraform and Markdown.

## Platforms
macOS (arm64, amd64), Windows (amd64, arm64), Linux (amd64, arm64). All three are first-class: no feature may be macOS-only unless the OS forces it.

## Core use cases (by release)
- **v0.1:** open a Go project; edit with highlighting; format on save; navigate files; gopls code intelligence; run, test and debug; integrated terminal; basic git.
- **v0.2:** refactorings, generators, coverage, golangci-lint, `go.mod` tooling, HTTP client.
- **v0.3:** profiling, Docker/K8s/DB tooling, remote mode, AI/MCP.

## Current milestone
**M1: Editor + Project model** — implemented (see `.claude/designs/m1-editor-project/acceptance.md`); Windows/Linux runs and native-window checks pending.
**M2: Intelligence** — implemented (see `.claude/designs/m2-intelligence/acceptance.md`): gopls completion, navigation, usages, docs, diagnostics/Problems, quick-fixes, rename, palette symbols, Find/Replace in Path. Windows/Linux runs pending.
**M3: Run/Test/Debug** — implemented (see `.claude/designs/m3-run-debug-test/acceptance.md`): run configurations (Go Build/Test/Remote, Shell, Makefile, before-launch tasks), Run window with ANSI console, test tree with rerun failed and benchmarks, Delve debugger (breakpoints, goroutines, variables, watches, inline values), integrated terminal. Windows/Linux runs pending.
**M4: Git + Lint + Modules → v0.1** — implemented (see `.claude/designs/m4-git-lint-modules/acceptance.md`): git status/gutter/commit/diff/partial staging/branches/stash/log/blame/3-way merge, push/pull/fetch, golangci-lint diagnostics and fixes, go.mod completion and module actions, installers (dmg, NSIS, deb/rpm/tar.gz) and a release workflow. Pending: the first release-workflow run on GitHub, signing certificates.
**Wails v3 / multi-window** — one process owns every project window; macOS shows one Dock icon with a project list (see `.claude/designs/wails-v3/`).
**Next: M5 — v0.2** (generators/intentions, refactorings, coverage, fuzz, HTTP client, protobuf, inspection batch run, local history).

## Principles
- Fast and calm: no UI freeze > 100 ms; heavy work stays off the UI thread.
- Toolchain-first: if `go`/`gopls`/`dlv` can do it, call them; don't reimplement them.
- Project state lives in `.goi/`, is shareable through VCS, and is human-readable (TOML).
- Untrusted folders never auto-execute anything (no `go generate`, tasks or watchers).
- Original identity: no third-party IDE names, icons, color schemes or branding.
