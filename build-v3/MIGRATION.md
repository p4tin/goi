# Wails v3 build scaffold: what T5 / T10 must do

This directory was generated with `wails3 init -t vanilla` (Wails CLI v3.1.0, `wails3` at `$(go env GOPATH)/bin/wails3`) and then adapted to Goi. It is not wired in yet.

## T5: move into place

1. **Move the files.**
   - `build-v3/*` → `build/`. The v2-only files it replaces:

     | v2 file (delete) | v3 file |
     |---|---|
     | `build/darwin/Info.plist` (Go template) | `build/darwin/Info.plist` (concrete) |
     | `build/darwin/Info.dev.plist` | `build/darwin/Info.dev.plist` |
     | `build/windows/installer/` (`project.nsi`, `wails_tools.nsh`) | `build/windows/nsis/` |
     | `build/windows/info.json`, `wails.exe.manifest` (templates) | concrete files |
     | `build/package/linux/nfpm.yaml` | `build/package/linux/nfpm.yaml` (paths changed: `./bin/goi`, `${GOARCH}`) |
     | `build/README.md` | — |

     These stay as they are: `build/appicon.svg`, `build/appicon.png`, `build/windows/icon.ico`, `build/darwin/Info.goland.plist`, `build/package/macos/make-dmg.sh` and `build/package/linux/goi.desktop` (identical copies are in build-v3).
   - `build-v3/Taskfile.root.yml` → `./Taskfile.yml`.
   - Delete `build-v3/MIGRATION.md` (fold anything still useful into CLAUDE.md in T14).
2. **Delete `wails.json`.** `scripts/version.sh` reads `build/config.yml` `info.version` first. The `build-v3/config.yml` path in that script can go after the move.
3. **Frontend** (`frontend/package.json`, `frontend/vite.config.ts`):
   - Add the dependency `"@wailsio/runtime": "3.1.0"` (exact).
   - Add the script `"build:dev": "tsc && vite build --minify false --mode development"`. `wails3 build DEV=true` / `wails3 dev` call `npm run build:dev`; `build` stays `tsc && vite build`.
   - Vite: `server: { host: "127.0.0.1", port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true }` and `plugins: [wails("./bindings")]` (`import wails from "@wailsio/runtime/plugins/vite"`). The dev task passes `--port 9245 --strictPort`.
4. **Bindings:** `wails3 generate bindings -clean=true -ts -i` (the `bindings` / `common:generate:bindings` task) writes to `frontend/bindings` (TS interfaces, not classes). It runs `go mod tidy` first, and the build tasks run it automatically. Delete `frontend/wailsjs/`.
5. **`.gitignore`:** add the files the v3 tasks create:
   - `bin/`
   - `build/linux/goi.desktop` (a copy made by `linux:generate:dotdesktop`)
   - `build/linux/appimage/build/`
   - `build/windows/nsis/MicrosoftEdgeWebview2Setup.exe`
   - `wails_windows_*.syso`
   - `frontend/bindings` only if we decide not to commit it. v2 committed `wailsjs`, so committing keeps `go vet` and `tsc` working in a fresh checkout.
6. **Plain `go build`** (GoLand "Goi" run config): v3 links `UniformTypeIdentifiers` itself, so drop that framework from `CGO_LDFLAGS`. Keep `-Wl,-sectcreate,__TEXT,__info_plist,build/darwin/Info.goland.plist` (a bundle id is still needed for open/save sheets). Build tags: `production` (no `desktop` tag in v3).
7. **Don't run `wails3 task common:update:build-assets` casually.** It rewrites:
   - `build/darwin/Info*.plist` (it merges existing keys back);
   - `build/windows/info.json`, `wails.exe.manifest`, `nsis/wails_tools.nsh`;
   - `build/linux/nfpm/nfpm.yaml`, `build/linux/desktop`.

   Goi's packaging doesn't use the Linux ones: it uses `build/package/linux/*`. After a version bump, edit `config.yml` `info.version`, then run it, then delete the regenerated `build/linux/desktop` and `build/linux/nfpm/nfpm.yaml`. (The plists, `info.json` and the manifest carry the version, which is why it's worth running.)

## Command map

| v2 | v3 |
|---|---|
| `wails dev` | `wails3 dev` (= `wails3 task dev`; Vite on 9245, app built with `DEV=true` and run as `bin/Goi.dev.app` on macOS) |
| `wails build` | `wails3 build` (host OS, binary `bin/goi[.exe]`) / `wails3 package` (bundle/installer) |
| `wails build -platform darwin/universal` | `wails3 task darwin:package:universal` → `bin/Goi.app` (lipo'd, ad-hoc signed) |
| dmg (`build/package/macos/make-dmg.sh`) | `wails3 task darwin:create:dmg DMG_OUT=dist/Goi-<v>-macos-universal.dmg` (same script) |
| `wails build -platform windows/amd64 -nsis -webview2 embed` | `wails3 task windows:package ARCH=amd64` → `bin/goi-amd64-installer.exe` (NSIS; the WebView2 bootstrapper is generated and bundled automatically) |
| `wails build -platform windows/arm64 … -o goi-arm64.exe` | `wails3 task windows:build ARCH=arm64 OUTPUT=bin/goi-arm64.exe` |
| `wails build -tags webkit2_41` (Linux) | `wails3 task linux:build` (GTK4 + WebKitGTK 6.0 by default; `webkit2_41` is gone). Legacy stack: `EXTRA_TAGS=gtk3`. |
| `nfpm package -f build/package/linux/nfpm.yaml -p deb` | `wails3 task linux:create:deb` / `linux:create:rpm` (`wails3 tool package` with `build/package/linux/nfpm.yaml`; `GOI_VERSION` from `scripts/version.sh`) |
| `wails generate module` | `wails3 task bindings` (or `wails3 generate bindings -ts -i`) |
| `-ldflags "-X …paths.BuildVersion=V"` | pass `BUILD_VERSION=V` to any build/package task (appended to the production ldflags on all three OSes) |
| E2E dev server | `wails3 task build:server DEV=true` → `bin/goi-server`; listens on `WAILS_SERVER_PORT` (see design.md §1.1, S3) |

Output paths moved from `build/bin/` to `bin/` (repo root). Release scripts and `.gitignore` must follow.

## T10: CI / release changes

- Install the CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0` (the env `WAILS_VERSION: v3.1.0`).
- The template's tasks also need **Task**? No: `wails3 task` embeds it.
- **Linux runners:**
  - Change `libgtk-3-dev libwebkit2gtk-4.1-dev` to `libgtk-4-dev libwebkitgtk-6.0-dev`, available on ubuntu-24.04 (ubuntu-latest).
  - Packages now depend on `libgtk-4-1` / `libwebkitgtk-6.0-4` (rpm: `gtk4` / `webkitgtk6.0`), which drops Ubuntu 22.04 / Debian 12 users.
  - To keep them, build with `EXTRA_TAGS=gtk3` and switch the deps in `build/package/linux/nfpm.yaml` to the gtk3 set, which is in the comments there.
- Builds use `-tags production` (v2 used `desktop,production`).
- macOS: `wails3 task darwin:package:universal BUILD_VERSION=$VERSION`, then `darwin:create:dmg DMG_OUT=dist/Goi-$VERSION-macos-universal.dmg`, and zip `bin/Goi.app` as before.
- Windows:
  - `wails3 task windows:package ARCH=amd64 BUILD_VERSION=$VERSION`. The installer lands at `bin/goi-amd64-installer.exe`; rename it to the existing artifact name.
  - Zips come from `bin/goi.exe` and `bin/goi-arm64.exe`.
  - NSIS (`makensis`) must be on PATH, as today.
- Linux:
  - `wails3 task linux:build BUILD_VERSION=$VERSION`, then `linux:create:deb` and `linux:create:rpm`. The output goes to `bin/`, so rename it to the existing artifact names.
  - The tar.gz step packs `bin/goi` plus the desktop file and icon, as before.
- Tag check: `scripts/version.sh "$GITHUB_REF"` is unchanged.
- `wails3 doctor` is a useful first CI step for diagnosing missing system deps.
