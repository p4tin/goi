# Goi

A cross-platform Go IDE built with Wails v2 and CodeMirror 6. Product spec: [SPEC.md](SPEC.md).

## Install

Download the package for your system from the [releases](https://github.com/p4tin/goi/releases):

| System | Package |
|---|---|
| macOS (Apple silicon and Intel) | `Goi-<version>-macos-universal.dmg`: open it and drag Goi to Applications |
| Windows (x64) | `Goi-<version>-windows-amd64-installer.exe`, or the portable `.zip` |
| Windows (ARM64) | `Goi-<version>-windows-arm64.zip` (portable) |
| Linux (x64) | `goi_<version>_amd64.deb` (Debian/Ubuntu), `goi-<version>.x86_64.rpm` (Fedora/RHEL), or the `.tar.gz` |

Goi needs a Go SDK. It offers to install gopls, Delve and golangci-lint into its own tools folder the first time they're needed.

**Unsigned builds.** Until the releases are signed:
- **macOS**: the first time, right-click Goi in Applications and choose **Open**, then **Open** again. Or run `xattr -dr com.apple.quarantine /Applications/Goi.app`.
- **Windows**: SmartScreen may say "Windows protected your PC". Click **More info**, then **Run anyway**.
- **Linux**: the `.deb`/`.rpm` pull in GTK 3 and WebKitGTK 4.1. For the `.tar.gz`, install `libgtk-3-0` and `libwebkit2gtk-4.1-0` (or your distribution's equivalents).

## Release

1. Set `info.productVersion` in `wails.json` (for example `0.1.0`).
2. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
3. The **release** workflow builds every package, checks the tag against `wails.json`, and creates a **draft** GitHub Release with the files and `SHA256SUMS.txt`. Review it and publish.

Run the workflow by hand (Actions → release → Run workflow) to get the packages as artifacts without a release. Signing runs only when these secrets exist: `MACOS_CERTIFICATE` (base64 .p12), `MACOS_CERTIFICATE_PASSWORD`, `MACOS_SIGNING_IDENTITY`, `MACOS_NOTARY_APPLE_ID`, `MACOS_NOTARY_TEAM_ID`, `MACOS_NOTARY_PASSWORD`, `WINDOWS_CERTIFICATE` (base64 .pfx), and `WINDOWS_CERTIFICATE_PASSWORD`.

Build packages locally:
```
wails build -platform darwin/universal && build/package/macos/make-dmg.sh build/bin/Goi.app dist/Goi.dmg
wails build -tags webkit2_41 && GOI_VERSION=0.1.0 nfpm package -f build/package/linux/nfpm.yaml -p deb -t dist/goi.deb   # Linux
wails build -platform windows/amd64 -nsis                                                                                  # Windows (needs NSIS)
```

## Develop

```
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev                  # hot-reload dev mode
wails build                # production build for this OS
go test ./internal/...     # backend tests
cd frontend && npm test    # frontend tests
```

Linux builds need `libgtk-3-dev libwebkit2gtk-4.1-dev` and `wails build -tags webkit2_41`.
