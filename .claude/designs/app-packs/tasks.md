# Application Packs — Tasks

Implements `design.md`. Tasks are grouped into **waves**:

- Tasks in the same wave can run concurrently, each in its own git worktree, because they own disjoint files.
- A wave starts only after the previous wave is merged.
- "Owns" lists the files a task may change. When two tasks in a wave must touch the same file, the overlap is named and the merge order is given.

**Green gate** (every task, before merge, unless stated otherwise):

```
go vet ./... && go test ./...                                    # -race for rpc/workspace/vfs/lsp/langserver/debugger
cd frontend && npx tsc --noEmit && npm test
node tools/e2e/cdp.mjs tools/e2e/smoke.json                      # plus the task's listed E2E suites
```

No behavior changes inside move commits (design §9). One commit per task, conventional style.

---

Status (2026-11-15): implemented on `feat/app-packs`. All tasks are done; see design §10 for where the implementation differs and acceptance.md for the results.

# Part A: core/pack split, Goi unchanged

## Wave 0: spikes (3 concurrent; throwaway branches, findings appended to design.md §10)

- [x] **S1 Bindings from several main packages.** Make a second throwaway main package with one extra service. Run `wails3 generate bindings -ts -i` with both packages, and record whether one tree covers both. If it doesn't, prototype the fallback: per-app output plus a mode-chosen alias. *Done when:* §10 states which approach §4 uses.
- [x] **S2 Task include vars.** Prove that `apps/x/Taskfile.yml` can include `build/darwin/Taskfile.yml` with `APP_DIR`/`MAIN_PKG`/`APP_NAME` vars that override root vars. Prove that the root `includes: {goi: …, pyi: …}` plus bare `build` → `goi:build` works with `wails3 build` and `wails3 task pyi:build`. *Done when:* a minimal Taskfile pair builds a hello-world binary into `bin/<app>/`.
- [x] **S3 `wails3 dev` with `-config apps/x/build/config.yml`.** Check that hot reload, `dev_mode.executes` and the Vite port work when the config isn't in `build/`. *Done when:* §10 records the working invocation or the workaround.

## Wave 1: contracts (1 task)

- [x] **T1 `internal/pack` contracts.** Create `Identity`, `Pack`, all capability interfaces from design §2, `Host`, `ProjectSession`, `SDKInfo`, `TestEvent` and `SettingsSection`.
  - `rpc.NewApp(opts, p pack.Pack)` stores the pack. Nothing reads it yet.
  - Root `main.go` passes `goiPack()`, a temporary func in the root package returning today's identity values with nil capabilities.
  - *Owns:* `internal/pack/*` (new), `internal/rpc/app.go` (signature only), `main.go`, rpc test helpers that call `NewApp`.
  - *Done when:* the green gate passes and `internal/pack` has doc comments and no imports from `apps/` or Wails.

## Wave 2: foundations (4 concurrent)

- [x] **T2 Backend identity plumbing (R2.1–2.3, 2.6, 2.7, 2.9).**
  - `paths.Configure(DirName, DataVersion, HomeEnv)`; tests call it in `TestMain`.
  - rpc reads `App.id`: shell Name/Title/ProgramName, single-instance id from `BundleID`, window titles, flag set name, user-facing messages.
  - `workspace.Options.ProjectDir` and the run store's directory.
  - clientInfo in gopls/debugger/agent; mcp Implementation and server key; `<PROGRAM>_MCP_TOKEN`; `TERM_PROGRAM`.
  - `AppService.Product()` returns the identity; capabilities come later, in T13.
  - *Owns:* `internal/paths`, `internal/rpc/{shell,session,app,agent,mcp,run}.go` (identity lines only), `internal/workspace/manager.go`, one line each in `internal/gopls/server.go`, `internal/debugger/session.go`, `internal/agent/client.go`, `internal/terminal/integration.go`, `internal/mcp` bridge env name.
  - *Test:* a table test checks that a fake Identity (`Zed`, `ZED_HOME`, `.zed`) reaches every listed spot. Goi's values give byte-identical paths and ids (R3.2).
- [x] **T3 Move Goi to `apps/goi` and rewire the build (R3.5, R4.1).**
  - `internal/boot.Main` holds the old `main.go` body. `apps/goi/main.go` embeds `dist/`, `build/config.yml` and the icon. `apps/goi/pack.go` replaces `goiPack()`.
  - `git mv` the per-app assets from `build/` to `apps/goi/build/`. The shared Taskfiles use `{{.APP_DIR}}`/`{{.MAIN_PKG}}`, following S2. `apps/goi/Taskfile.yml` sets the vars; the root Taskfile delegates to `goi:`. Output goes to `bin/goi/`.
  - Update `scripts/version.sh <app> [tag]`, `scripts/icons.sh <app>`, `.run/*` (including the `-sectcreate` plist path), the E2E `server` blocks (`./apps/goi`), and the build paths in `ci.yml`/`release.yml` (still Goi-only).
  - *Owns:* `main.go` (deleted), `apps/goi/{main.go,pack.go,Taskfile.yml,build/**}`, `internal/boot/*`, `build/**` (shared Taskfiles), `Taskfile.yml`, `scripts/*`, `.run/*`, `tools/e2e/*.json` server blocks, `.github/workflows/*`, `frontend/vite.config.ts` (outDir only, `../apps/goi/dist`).
  - *Overlap:* `apps/goi/pack.go` and T1's root func. T3 deletes the root one.
  - *E2E:* all suites (smoke, multiwindow, m2, m3, m4, agent, format, preview). `wails3 build`, `wails3 dev` and `wails3 task darwin:package:universal` still work, and the bundle is `bin/goi/Goi.app` with the same Info.plist.
- [x] **T4 Frontend pack switch (R2.1, R2.4, R6.1, R6.2, Q4).**
  - `virtual:pack` alias and `--mode goi|pyi` in Vite. `__PRODUCT_NAME__` is read from `apps/<app>/build/config.yml`. `frontend/src/packs/go/index.ts` exports `product` (name and an icon moved from `assets/goi-icon.svg`) and an empty `features`/`settings`/`languages`.
  - About, Welcome, `document.title` and the "Goi MCP Server" dialog read `product`.
  - Generic theme and keymap display names, with ids unchanged.
  - `features.ts` builds the list as core features + pack features at named anchors, and a test pins Goi's current order.
  - *Owns:* `frontend/vite.config.ts` (alias/mode/define; T3 owns outDir), `frontend/src/{pack.ts,features.ts,packs/go/index.ts}`, `app/{about,workbench}.ts`, `views/welcome.ts`, `agent/install.ts` (title), `theme/tokens.ts`, `keymap/keymaps.ts`, `views/settings/schema.ts` (theme/keymap labels), tests.
  - *Merge after T3* (vite.config.ts and the config path). *E2E:* smoke, preview.
- [x] **T5 Settings sections and toolbin tools (R5.6, R6.4 backend).**
  - `settings.Section[T](eff, name)` and `pack.SettingsSection{Name, Defaults}`. Move `Go`, `Gopls`, `Debug.DlvPath`, `Lint.GolangciPath` out of the core struct into Go-pack sections declared in `apps/goi/settings.go`. TOML keys and files are unchanged.
  - `toolbin.Tool{ID, Binary, SettingKey, Install(ctx, env) *exec.Cmd}`. The resolve order stays the same. Goi's tool list (gopls, dlv, golangci-lint) is declared in the Go pack.
  - *Owns:* `internal/settings/*`, `internal/toolbin/*`, `apps/goi/{settings.go,tools.go}`, the field accessors in `internal/rpc/{sdk,lang,debug,lint,workspace}.go` (mechanical `Section` calls only).
  - *Overlap:* `apps/goi/` is created by T3, so T5 merges after T3. T2's rpc edits are identity lines, T5's are settings accessors; merge T2 first.
  - *Test:* a settings golden test shows an existing user `settings.toml` loading to the same effective values.

## Wave 3: backend capabilities (5 concurrent)

Each task moves one capability behind its `pack` interface, declares it in `apps/goi/pack.go`, and makes rpc call the interface instead of the Go package. Everyone edits `apps/goi/pack.go` (one field each), so merge in listed order and resolve that file trivially.

- [x] **T6 Project model + SDK (R5.1, R5.2).**
  - `workspace.Project` becomes generic: `Kind string`, `Units`, and the JSON names the frontend uses today (or a mapping in `packs/go`).
  - Goi's detector moves to `apps/goi/internal/godetect`.
  - `internal/gosdk` → `apps/goi/internal/gosdk`, behind `SDKProvider`.
  - `rpc/sdk.go` → `apps/goi/internal/services/sdk.go` (SdkService through `Host`).
  - Every consumer of the Go environment (run plans, terminal, agents, the language server env) asks `SDK.Env`.
  - *Owns:* `internal/workspace/detect*.go`, `internal/rpc/{workspace,sdk}.go`, `apps/goi/internal/{godetect,gosdk,services}`, env call sites in `rpc/{run,terminal,agent}.go`, and the rpc `Host` implementation (`internal/rpc/host.go`, new).
  - *Test:* detect tests move with the code. An rpc test opens a module, a go.work project and a folder, and gets the same `Project` JSON as before.
- [x] **T7 Language servers (R5.3).**
  - `internal/gopls` → `internal/langserver` (supervisor, sync, versions, edits, uri, requests; `ServerSpec`) + `apps/goi/internal/gopls` (resolve/install, settings, gopls commands) implementing `LanguageServerSpec`.
  - `rpc/lang.go` runs one server per spec and routes by language id. `GOI_GOPLS_BENCH` moves with the gopls package.
  - *Owns:* `internal/gopls` (removed), `internal/langserver/*`, `apps/goi/internal/gopls/*`, `internal/rpc/lang.go`, the import line in `internal/rpc/lint.go`.
  - *E2E:* m2. *Bench:* completion p95 is within 5% of `main` (R8.1).
- [x] **T8 Formatter (R5.4).** `internal/format` → `apps/goi/internal/gofmt` behind `Formatter`; `rpc/editor.go` calls the pack. *Owns:* those files. *E2E:* format, smoke (format on save).
- [x] **T9 Run types, tests, markers (R5.7, R5.8).**
  - `runconfig` keeps the model, store, validation, type registry, Shell/Makefile and before-launch tasks.
  - The Go Build/Test/Remote types, args and test2json move to `apps/goi/internal/gorun`. `internal/gosyntax` → `apps/goi/internal/gosyntax`, used by the Go types' `Markers`.
  - `rpc/markers.go` and `rpc/run.go` go through the registry and `TestParser`.
  - `runconfig/debug.go` (dlv args) stays put; T11 moves it.
  - *Owns:* `internal/runconfig/{config,plan,args,validate,store}.go`, `internal/gosyntax` (removed), `apps/goi/internal/{gorun,gosyntax}`, `internal/rpc/{run,markers}.go`, the runconfig line in `rpc/app.go`.
  - *Test:* existing `.goi/runConfigurations/*.toml` fixtures load and plan identically. *E2E:* m3.
- [x] **T10 Linter + mod service + agent env (R5.5, R5.10, pack services).**
  - `internal/lint` keeps `Issue`/`Fix`; the golangci runner → `apps/goi/internal/golangci` behind `Linter`; `rpc/lint.go` becomes generic.
  - `internal/gomod` → `apps/goi/internal/gomod`; `rpc/mod.go` → `apps/goi/internal/services/mod.go`.
  - `AgentContext` for the Go env vars agents get today.
  - *Owns:* `internal/lint/*`, `internal/gomod` (removed), `apps/goi/internal/{golangci,gomod,services/mod.go}`, `internal/rpc/{lint,mod}.go`, the agent env line in `rpc/agent.go`.
  - *Overlap:* the `rpc/lint.go` import with T7 (merge T7 first); `services/` with T6 (separate files).
  - *E2E:* m4, agent.

Merge order: T6 → T7 → T8 → T9 → T10.

## Wave 4: debug, capabilities, bindings (2 concurrent)

- [x] **T11 Debug adapter (R5.9).**
  - `internal/debugger` keeps the generic DAP session driver.
  - `apps/goi/internal/dlv` gets resolve, `dlv dap` start, launch/attach args (from `runconfig/debug.go`, with the `output` temp path and symlink resolution) and `AdapterID`.
  - `rpc/debug.go` calls `pack.Debug`.
  - *Owns:* `internal/debugger/*`, `internal/runconfig/debug.go` (removed), `apps/goi/internal/dlv/*`, `internal/rpc/debug.go`.
  - *Test:* `GOI_DLV=… go test -race ./internal/debugger ./internal/rpc ./apps/goi/...`. *E2E:* m3.
- [x] **T12 Capabilities + bindings (R1.4, design §4).**
  - `AppService.Product()` adds `Capabilities`, derived from the pack's non-nil fields.
  - Pack services are registered from `Pack.Services(host)`.
  - Regenerate bindings per S1. `lib/api.ts` exposes `product()`/`capabilities()`. Create `packs/go/api.ts` for the SdkService and ModService wrappers, moved out of `lib/api.ts`.
  - *Owns:* `internal/rpc/app.go` (services list, Product), `frontend/bindings/**` (generated), `frontend/src/lib/api.ts`, `frontend/src/packs/go/api.ts`, call sites of the moved wrappers (import path only).
  - *Merge after T11.*

## Wave 5: frontend Go moves (3 concurrent)

- [x] **T13 Go UI into `packs/go`.**
  - Move `mod/`, `views/goStatus.ts`, `views/sdkPicker.ts` and `views/newProject.ts` (Go templates) into `packs/go/`, listed in `packs/go/index.ts` features at their current anchors.
  - Go settings sections use `registerSettingsSection`.
  - *Owns:* those files, `views/settings/schema.ts` (Go sections out), `packs/go/index.ts` (features/settings), their tests → `frontend/test/packs/go/`.
- [x] **T14 Run, editor and lang Go bits into `packs/go`.**
  - `run/bench.ts`, the Go test-name parsing in `run/testTree.ts`, and Go context runs move behind a run-contribution hook.
  - `editor/languages.ts` gets `registerLanguage`, and Go's entries move to the pack.
  - The Go-only code actions and keywords in `lang/` move behind the `actionProviders` hook and a keywords registry.
  - *Owns:* `run/{bench,testTree,context}.ts`, `editor/languages.ts`, `lang/{codeActions,keywords}.ts`, `packs/go/{run,languages,lang}.ts`, `packs/go/index.ts` (languages/run fields), tests.
  - *Overlap:* `packs/go/index.ts` with T13 (different fields; merge T13 first).
- [x] **T15 Hide UI for missing capabilities (R1.4).**
  - Core features check `capabilities()`: lint, debug, format-on-save options, language-server status, SDK status slot, test tree. Commands, menus (`MENUS` entries filtered), settings and status items disappear.
  - *Owns:* `lint/lint.ts`, `debug/install.ts`, `lang/status.ts`, `app/menuBar.ts`, `editor/saveFlow.ts` (option visibility), `run/install.ts`, a test that boots with every capability off.

## Wave 6: guardrails and pipeline (2 concurrent)

- [x] **T16 Boundary tests (R8.2).**
  - Go: `internal/pack/boundary_test.go` walks `go list -deps`. Packages under `internal/` must not import `apps/...`, and `apps/X` must not import `apps/Y`.
  - TS: a vitest test checks that no file in `frontend/src` outside `packs/` imports `packs/*`, and that only `lib/api.ts` and `packs/*/api.ts` import `bindings/`.
  - *Owns:* the two test files.
- [x] **T17 CI matrix + per-app release (R4.4, R4.5, Q3).**
  - `ci.yml` runs an OS × app build matrix. Go tests run with `./...`.
  - `release.yml` derives APP/PRODUCT from the tag (`v*`|`goi-v*` → goi, `pyi-v*` → pyi), takes the version from `scripts/version.sh <app> <tag>`, and builds artifact names and release title from `${PRODUCT}`. Packaging config comes from `apps/<app>/build/package/`.
  - *Owns:* `.github/workflows/*`, `scripts/version.sh`, `build/package/macos/make-dmg.sh` (if it has a name in it).
  - *Test:* `act` or a dry-run tag on a fork. At minimum, a `workflow_dispatch` run builds Goi's artifacts with today's names.

## Wave 7: Part A gate (1 task)

- [x] **T18 Goi regression gate (R3, R8.1).**
  - Run every E2E suite, `go test -race` with Delve and golangci, the gopls bench, and a startup-time comparison.
  - Check that the binary size is within 5% of `main`.
  - Check that an existing real profile (copied into `GOI_HOME`) opens with the same settings, recents, layouts and run configurations.
  - Do a manual smoke of the Dock launch with `env -u TERM PATH=/usr/bin:…`.
  - *Done when:* the results are recorded in `.claude/designs/app-packs/acceptance.md` (Part A section).

---

# Part B: Pyi skeleton

## Wave 8: Python pack pieces (5 concurrent)

- [x] **P1 Pyi app shell + identity (R7.1).**
  - `apps/pyi/{main.go,pack.go,Taskfile.yml}`. `build/config.yml` sets Pyi, `dev.goi.pyi`, 0.1.0, and `.py`/`.pyi` associations.
  - Darwin/Windows/Linux/package assets come from Goi's with names swapped.
  - An original `appicon.svg`, then `scripts/icons.sh pyi`.
  - The root Taskfile includes `pyi:`. The pack has nil capabilities.
  - *Owns:* `apps/pyi/**` except `apps/pyi/internal/**`, plus `Taskfile.yml` (one include).
  - *Done when:* `wails3 task pyi:build` produces `bin/pyi/Pyi.app`, which opens a folder with core features only.
- [x] **P2 Detection + interpreter (R5.1, R5.2 for Python).**
  - `apps/pyi/internal/pyenv`: detector (markers, excludes) and `SDKProvider` (`python.interpreter` setting, then `.venv`, then `python3`/`python`). `Env` sets `VIRTUAL_ENV` and `PATH`; Windows uses `Scripts\python.exe`.
  - *Owns:* `apps/pyi/internal/pyenv/*`, `apps/pyi/settings.go` (python section).
  - *Test:* table tests with temp dirs on all OSes.
- [x] **P3 basedpyright (R7.3).**
  - `apps/pyi/internal/pyright`: `LanguageServerSpec` (resolve: setting, then `PATH`, then `<config>/tools/bin`; settings with `python.pythonPath`) and a `toolbin.Tool` that creates `<config>/tools/venvs/basedpyright`, runs `pip install basedpyright` and links the server into `tools/bin`.
  - *Owns:* `apps/pyi/internal/pyright/*`, `apps/pyi/tools.go`.
  - *Test:* an integration test (skips without basedpyright, or with `GOI_PYRIGHT=…`) checks completion and definition on a sample.
- [x] **P4 Python File run type (R7.4).**
  - `apps/pyi/internal/pyrun`: a `runconfig.Type` that runs `python <file> <args>` with `SDK.Env`, and `Markers` on `if __name__ == "__main__":`.
  - *Owns:* `apps/pyi/internal/pyrun/*`.
- [x] **P5 Python frontend pack (R7.2, R7.5).**
  - Add `@codemirror/lang-python`.
  - `frontend/src/packs/python/index.ts` provides the product (Pyi name and icon), `registerLanguage` for Python, settings sections (`python.interpreter`, `python.lsp.path`), and an interpreter status item.
  - `vite --mode pyi` outputs to `apps/pyi/dist`.
  - *Owns:* `frontend/src/packs/python/**`, `frontend/package.json` (+ lock), `frontend/test/packs/python/*`.
  - *Overlap:* the icon file with P1 (P1 owns the svg, and P5 copies it after P1 merges).

## Wave 9: wire-up and acceptance (1 task)

- [x] **P6 Wire Pyi + E2E + side-by-side check.**
  - Fill `apps/pyi/pack.go` from P2–P4.
  - Add `tools/e2e/pyi-sample.sh` and `tools/e2e/pyi-smoke.json`: open a sample, check highlighting, completion (skip without basedpyright), run the file and see its output, and confirm no Go UI.
  - Run Goi and Pyi at once (R4.6): separate config dirs, Dock icons and MCP servers.
  - Do a release dry run for `pyi-v0.1.0`.
  - Record the results in `acceptance.md` (Part B).
  - *Owns:* `apps/pyi/pack.go`, `tools/e2e/pyi-*`, `acceptance.md`.

---

## Wave summary

| Wave | Tasks | Concurrency |
|---|---|---|
| 0 | S1, S2, S3 | 3 |
| 1 | T1 | 1 |
| 2 | T2, T3, T4, T5 | 4 (merge T2 → T3 → T4 → T5) |
| 3 | T6, T7, T8, T9, T10 | 5 (merge T6 → T7 → T8 → T9 → T10) |
| 4 | T11, T12 | 2 (merge T11 → T12) |
| 5 | T13, T14, T15 | 3 (merge T13 → T14 → T15) |
| 6 | T16, T17 | 2 |
| 7 | T18 | 1 (Part A gate) |
| 8 | P1, P2, P3, P4, P5 | 5 |
| 9 | P6 | 1 (Part B gate) |

Phase 5 (CLAUDE.md and steering updates) and Phase 6 (retro) follow wave 9. CLAUDE.md command changes that a task causes (paths, `-config`, server build) are made in that task, so the docs never point at broken commands in between.
