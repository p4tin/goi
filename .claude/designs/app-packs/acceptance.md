# Application Packs — Acceptance

Run on macOS (arm64), 2026-11-15, branch `feat/app-packs`. The baseline is `main` (655acc6) built the same way.

## Part A: Goi unchanged (R3, R8)

| Check | Result |
|---|---|
| `go vet ./...`, `go test ./...` (core, both packs, apps) | pass |
| `GOI_DLV=… go test -race ./internal/debugger ./internal/rpc` | pass |
| `GOI_GOLANGCI=… go test ./internal/rpc ./packs/golang/golangci` | pass (`TestLintRunEmitsDiagnostics`, `TestRealRun`) |
| `npx tsc --noEmit`, `npm test` | pass (58 files, 495 tests) |
| E2E smoke | same output as the baseline |
| E2E m2 (code intelligence) | identical to the baseline (completion p50/p95 1/2 ms on both) |
| E2E m3 (run/test/debug/terminal) | identical apart from timings, run-cache hashes and stdout/stderr interleaving |
| E2E m4 (git/lint/go.mod) | identical apart from commit hashes |
| E2E format, preview, multiwindow, agent | identical to the baseline |
| Boundary tests (Go `TestPackBoundaries`, `frontend/test/boundaries.test.ts`) | pass |
| `wails3 build`, `wails3 task darwin:package` | `bin/goi/Goi.app`, `CFBundleIdentifier` dev.goi.goi, same Info.plist keys |
| User data | Goi still uses `Goi/0.1`, `GOI_HOME`, `.goi/`, theme and keymap ids (display names are now generic) |
| Binary size (server build) | 29.06 MB → 29.29 MB (+0.8%); Pyi 24.95 MB |
| GoLand-style build with `-sectcreate` (absolute and repo-relative plist paths) | `__info_plist` section present |

Not run here: the gopls completion bench (`GOI_GOPLS_BENCH`; the m2 latency check matched), a startup-time comparison, opening a copy of a real profile, a Dock launch smoke, and Windows/Linux builds (CI covers them once pushed). The release workflow is unchanged in shape but untested until a tag or `workflow_dispatch` run.

## Part B: Pyi (R7)

`tools/e2e/pyi-smoke.json` against `bin/pyi-server` on `tools/e2e/pyi-sample.sh`, with basedpyright on PATH:

| Check | Result |
|---|---|
| Identity | window title "proj — Pyi"; `AppInfo`: name Pyi, project dir `.pyi`; bundle `bin/pyi/Pyi.app`, `CFBundleIdentifier` dev.goi.pyi |
| Capabilities | languageServer basedpyright, no linter, no debugger, run types `[python]` |
| Python highlighting | yes (lang-python) |
| basedpyright | status ready; go to definition `double` → `pkg/util.py:0`; completion after `os.pa` lists `path`, `pardir`, `pathsep`… |
| Interpreter status | "Python 3.9.6" (system python3; a `.venv` is preferred when present) |
| Run marker on `if __name__ == "__main__":` | yes; running main.py prints "hello from pyi 42" and "Process finished with exit code 0" |
| No Go UI | no Go SDK/target/gopls status items; Debug button hidden; no Debug or lint panels |
| `go test ./packs/python/...` | pass (`TestRealBasedpyright` passes with `GOI_PYRIGHT` set); `Server.Install` (venv + pip into a temp tools dir) works, and `Command` then resolves the linked server |

Not run here: Goi and Pyi as desktop apps side by side (they have separate bundle ids, single-instance ids, user dirs and MCP token files by construction), and a `pyi-v0.1.0` release dry run.
