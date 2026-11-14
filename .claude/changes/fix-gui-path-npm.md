# Fix: Goi launched from the Dock/Finder can't find npm (Node.js "not installed")

## Symptom

- **Now:** in the installed app (`/Applications/Goi.app`, 0.2.0), clicking **Install** for Claude Code in the AI Agent window fails with "npm not found: install Node.js…". Node 26 is installed with Homebrew (`/opt/homebrew/bin/npm`).
- **Should:** Goi finds npm and installs the agent, exactly as when it runs from a terminal (`wails3 dev`, the E2E server build). That's why dev runs and the E2E never showed the bug.

## Root cause

- macOS starts GUI apps (Dock, Finder, Spotlight, Launchpad) through launchd with `PATH=/usr/bin:/bin:/usr/sbin:/sbin`. It doesn't read `~/.zprofile`/`~/.zshrc`, where Homebrew, nvm, gvm etc. add their directories. Verified: Goi launched like that reports exactly that `PATH` (`ps eww`), and `command -v npm` fails with it.
- `internal/rpc/agent.go:625` calls `agent.FindNpm(nil)` → `exec.LookPath("npm")` on that `PATH` → `ErrNoNpm`.
- The same `PATH` would break the next steps too, even if npm were found by absolute path. `npm` and the agent adapters (`claude-agent-acp`, `codex-acp`) are `#!/usr/bin/env node` scripts, so `node` must be on the `PATH` passed to them (`os.Environ()` in `agent.Install`, `r.Environ()`/`os.Environ()` in `agentManager.env`). Agents installed with `npm -g` (`gemini`) are also looked up on `PATH`.
- Nothing in Goi imports the user's login-shell environment. The terminal works because it starts `$SHELL` as a login shell, and Go tools work because SDKs and `toolbin` search known folders.

## Fix (proposed, minimal)

New package `internal/shellenv` (no Wails):
- `LoginPath(ctx, shell string) (string, error)` runs `$SHELL -l -i -c` with a command that prints `PATH` between unique markers (shell startup files may print banners). It has a 5 s timeout and returns the `PATH` found between the markers.
- `Merge(current, login string) string` returns the login `PATH` followed by any entries of the current one it lacks.

`main.go`, on darwin and linux, before the app starts: if Goi wasn't started from a terminal (stdin is not a TTY), replace `PATH` with `Merge(os.Getenv("PATH"), LoginPath(…))`. On error or timeout, keep the current `PATH` and log a warning. Only `PATH` is imported; other variables (GOROOT, GOPATH…) are untouched to keep Go SDK selection as it is.

Windows is unaffected: GUI apps there get the full user/system `PATH` from the registry.

## Reproduction strategy (tests)

1. `internal/shellenv` tests with a **fake shell script** as `$SHELL`. It ignores its flags, prints a banner, then runs the command. The tests check that:
   - `LoginPath` returns the `PATH` the shell's startup would produce, ignoring the banner.
   - A shell that hangs times out, and a shell that fails returns an error.
   - `Merge` puts the login entries first, keeps entries missing from the login `PATH`, and removes duplicates.
2. Red phase: an `internal/agent` test that sets `PATH=/usr/bin:/bin:/usr/sbin:/sbin` while `npm` lives only in another folder. It shows `FindNpm(nil)` fails (today's behaviour), and that after applying `shellenv` (with a fake login shell that adds that folder) `FindNpm` succeeds. It fails today because `shellenv` doesn't exist and nothing repairs `PATH`.
3. Manual check after a release build: launch `/Applications/Goi.app` from the Dock and install Claude Code.

## Steps

- [x] Phase 1: failing test (`TestFixFindsToolsFromALoginShell`: "npm still not found; PATH=/usr/bin:/bin:/usr/sbin:/sbin")
- [x] Phase 2: minimal fix (`internal/shellenv`, wired in `main.go`)
- [x] Phase 3: regression check (`go test -race ./internal/...`, vet, gofmt). Also checked:
  - the real zsh, `GOI_REAL_SHELL=1` under `env -i`: 343 ms, npm and node found
  - server build with launchd's `PATH` and no `TERM`, fix skipped (`TERM` set): "npm not found" (the reported error)
  - same, with the fix: "Agent installed. Claude Code is ready", and Start reached "Claude Code is ready"
- [ ] CI on all OSes, release 0.2.1
