# Wails v3: one process, many windows — Requirements

## Why
Each project window is its own process today (Wails v2 supports one window per process; `internal/instance` starts a new process per project with `open -n`). macOS gives every process its own Dock icon, so two open projects show two Goi icons, with no single place to list or switch between them. Moving to Wails v3 (stable since v3.1.0) lets one Goi process own every project window.

Windows and Linux already group per-process windows under one taskbar/dock entry, so on those platforms the user-visible change is small. There, the goal is the same process model on every OS (SPEC §2.3: "one `goi-core` per user session; one *workspace* per open project window").

## Scope
In scope:
- Upgrade the desktop shell from Wails v2.17 to Wails v3 (latest stable 3.x at implementation time).
- One Goi process per user (per `GOI_HOME`). Each project, and the Welcome screen, runs in its own window inside that process.
- Per-window project sessions. Everything that is per-project today (workspace, watcher, journal, gopls, runs, debug, terminals, git, lint, mod, search index, project settings layer) belongs to the window that owns the project.
- macOS Dock icon menu and Window menu that list open projects.
- A single-instance handoff: launching Goi again (Dock, Finder, CLI, file association) routes into the running process.
- Build, dev, packaging, CI and release tooling updated for v3.

Out of scope:
- Moving editor tabs or tool windows between project windows.
- Several projects in one window (tabs of projects).
- Sharing one gopls between projects. Each project keeps its own gopls.
- Remote/headless `goi-core` (SPEC M6). The `events.Bus` boundary must stay intact so that work stays possible.
- New features unrelated to the window model.

## R1. Single process
1. WHEN Goi starts and no Goi process is running for the same config directory THEN Goi SHALL start as the primary process and open a window.
2. WHEN Goi starts while a primary process is running for the same config directory THEN the new process SHALL forward its request (project path, files to open, or "new window") to the primary and exit within 1 s without showing a window.
3. IF the primary process does not answer within 1 s (crashed, hung, stale lock) THEN the new process SHALL take over as primary.
4. Separate `GOI_HOME` values SHALL be independent: each has its own primary process.

## R2. Project windows
1. WHEN a project is opened (Open Folder, Recent Projects, New Project, CLI `--project`, folder dropped or passed by the OS) and no window has it open THEN Goi SHALL open it in a new window in the same process, unless the current window shows the Welcome screen, in which case that window SHALL open it.
2. IF a window already has that project open THEN Goi SHALL bring that window to the front and not open a second copy.
3. Each window SHALL have its own frontend state (editors, layout, tool windows, keymap dispatch) and its own backend session (R3).
4. Window titles SHALL stay as today (project name, active file, dirty marker).
5. A new window SHALL open at the default size (1280×800), offset (cascaded) from the focused window so it doesn't cover it exactly. Saving and restoring window geometry per project is out of scope (Goi doesn't do it today).
6. Opening a file through the OS (Finder "Open With", double-click, file association, CLI positional file) SHALL route it to the window whose project contains it, or open its project per R2.1.

## R3. Per-window sessions
1. A backend call from a window SHALL act on that window's project session only. A call from a window with no project SHALL fail with "no project is open", as today.
2. Backend events for a project (file changes, diagnostics, run output, debug state, terminal output, git status, lint results, notifications for that project) SHALL reach only the window that owns the project.
3. Application-wide events (user settings changed, theme/appearance changed, recent projects changed, tool installs) SHALL reach every window.
4. Project settings (`.goi/settings.toml`) SHALL apply to their own window only. User settings SHALL be shared by all windows and hot-reload in all of them.
5. Recent projects, the crash journal, the instance/window registry and installed tools SHALL work correctly with several projects open in one process (no lost writes, no cross-project mix-ups).
6. Closing a window SHALL run the existing close flow for that window only (unsaved-changes check, running processes/debug sessions prompt, journal flush) and then release its session: gopls, watchers, processes and PTYs for that project SHALL be stopped.
7. A failure in one project's session (gopls crash, panic in a handler, watcher error) SHALL NOT close or break other windows.

## R4. macOS Dock and app menus
1. Goi SHALL show exactly one Dock icon on macOS no matter how many projects are open.
2. Right-clicking the Dock icon SHALL list every open window by project name (and "Welcome" for a window without a project), marking the frontmost one. Choosing an entry SHALL bring that window to the front.
3. The Dock menu SHALL include **New Window** (opens a Welcome window) and **Open Recent ▸** (recent projects).
4. The menu bar SHALL include a standard **Window** menu (Minimize, Zoom, Bring All to Front, and the list of open windows).
5. Clicking the Dock icon with no visible windows SHALL open a Welcome window.
6. Cmd-Tab SHALL show one Goi entry. Mission Control's App Exposé SHALL show all project windows.

## R5. Menus and commands
1. The native menu bar SHALL keep its current contents (`MENUS` in `app/menuBar.ts`), labels and shortcuts. A menu command SHALL run in the focused window.
2. On Windows and Linux, where the menu bar belongs to each window, each window SHALL show the same menus with the same behavior.
3. Commands that opened or focused another process today (Open in New Window, Open Recent, the project-already-open path) SHALL open or focus a window in the same process.

## R6. Application lifecycle
1. WHEN the last window closes THEN on macOS Goi SHALL keep running with no windows (standard app behavior), and on Windows/Linux Goi SHALL quit.
2. Quit (Cmd-Q / File ▸ Exit) SHALL run each window's close flow in turn. IF the user cancels any of them THEN quit SHALL stop and the remaining windows SHALL stay open.
3. "Reopen last project on startup" SHALL reopen every project that was open when Goi last quit, each in its own window. The first project goes in the first window.
4. Crash recovery (journal of unsaved buffers) SHALL keep working per project.

## R7. Platform behavior
1. Windows: all Goi windows SHALL group under one taskbar button (one AppUserModelID). The installer, file associations and Start-menu entry SHALL keep working.
2. Linux: windows SHALL group under the `goi.desktop` launcher (WM_CLASS / Wayland app id `goi`). The deb/rpm/tar.gz packages SHALL keep working.
3. macOS: file associations, the Open/Save sheets (bundle identifier), the appearance (dark mode) integration and the universal build SHALL keep working.

## R8. No regressions
1. Every feature in M1–M4 acceptance documents SHALL work unchanged in each window, including when two or more project windows are open at once.
2. Bound method signatures seen by the frontend SHALL keep their current shape where possible. The frontend SHALL continue to reach the backend only through `lib/api.ts`.
3. Architecture rules SHALL hold: `internal/*` (except `internal/rpc`) never imports Wails, and backend → UI updates go through `events.Bus`.
4. Backend tests (`go test -race ./internal/...`), frontend tests, type-check and the E2E suites (smoke, m2, m3, m4) SHALL pass. The E2E harness SHALL work against v3's dev mode.
5. Startup to first paint SHALL be no slower than today (measured on this machine), and opening a second project window SHALL be faster than starting a second process is today.

## R9. Tooling and release
1. Dev, build and binding-generation commands SHALL be documented for v3 in `CLAUDE.md` and `.claude/steering/tech.md`, replacing `wails dev` / `wails build` / `wails generate module`.
2. CI (`ci.yml`) and the release workflow (`release.yml`) SHALL build and package all current targets (macOS universal dmg/zip, Windows NSIS + zips, Linux deb/rpm/tar.gz), and the tag/version check SHALL still work.
3. GoLand run configurations in `.run/` SHALL be updated.

## Acceptance (feature-level)
- With two projects open on macOS: one Dock icon; its menu lists both and switches between them; Cmd-Tab shows one Goi.
- Running `goi --project B` (or double-clicking a `.go` file in B) while Goi has project A open opens B in a second window of the same process (`ps` shows one Goi process).
- Editing, gopls, run/debug, terminal and git work in both windows at once, with no event crossing over (for example, diagnostics or run output from A never appear in B).
- Closing A's window stops A's gopls/processes and leaves B untouched. Quitting with unsaved changes in both prompts for each.
