# Fix: Goi/Pyi run configurations can't find Info.goland.plist

## Symptom
Running the `Goi` or `Pyi` run configuration (`.goi/runConfigurations/Goi.toml`, `Pyi.toml`) fails at link time:

```
ld: file cannot be open()ed, errno=2 (No such file or directory) path=apps/goi/build/darwin/Info.goland.plist
```

They should build and launch the app with the bundle identifier embedded, so macOS open/save sheets stay open (the earlier fix: embed the plist with a `-sectcreate` build flag, not `CGO_LDFLAGS`).

## Root cause
The `-sectcreate` path in `buildFlags` is relative to the repo root (`apps/goi/build/darwin/Info.goland.plist`). Goi's Go Build plan runs `go build` in the package directory (`Plan.Build.Dir = <root>/apps/goi`, from `gorun.ResolveTarget`). The external linker resolves relative paths against that directory, so it looks for `apps/goi/apps/goi/build/darwin/Info.goland.plist`.

I checked it by hand: from `apps/goi`, the repo-relative path fails with the error above, and the package-relative `build/darwin/Info.goland.plist` links fine. Before the app-pack split the package was the repo root, so a repo-relative path happened to work. My earlier check ran `go build ./apps/goi` from the repo root, which isn't how Goi runs it.

GoLand's `.run/Goi.run.xml` and `.run/Pyi.run.xml` use `$PROJECT_DIR$/apps/<app>/…` (absolute), so they aren't affected.

## Reproduction strategy
Add a test in `packs/golang/gorun` that loads the repo's `.goi/runConfigurations` and plans every Go Build configuration with `Root` = the repo. For each `-sectcreate,__TEXT,__info_plist,<path>` in the build arguments, it resolves `<path>` against `Plan.Build.Dir` (the linker's working directory) and asserts the file exists. Today it fails for Goi and Pyi.

## Fix (after the red test)
Make the paths package-relative in both TOML files (`build/darwin/Info.goland.plist`). Correct the CLAUDE.md sentence that says the path is "from the repo root", and the comment in both `Info.goland.plist` files.

## Steps
- [x] Red: repo run-configuration test fails
- [x] Green: package-relative paths
- [x] Regression: gorun/runconfig tests, docs
