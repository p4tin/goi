//go:build !server

package rpc

// serverMode is set in E2E server builds (browser tabs instead of windows).
const serverMode = false
