# Goi

A cross-platform Go IDE built with Wails v2 and CodeMirror 6. Product spec: [SPEC.md](SPEC.md).

## Develop

```
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev                  # hot-reload dev mode
wails build                # production build for this OS
go test ./internal/...     # backend tests
cd frontend && npm test    # frontend tests
```

Linux builds need `libgtk-3-dev libwebkit2gtk-4.1-dev` and `wails build -tags webkit2_41`.
