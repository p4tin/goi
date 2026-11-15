// Package product holds the identity of the running application (Goi, Pyi,
// …): its display name, program name, bundle identifier and the names of the
// directories and environment variables it owns.
//
// The app's main package sets it once, before anything else runs
// (boot.Main); core packages read it instead of hard-coding a product name.
// Tests that need an identity call Set in TestMain.
package product

import (
	"strings"
	"sync"
)

// Identity describes one application built on the shared core.
type Identity struct {
	// Name is the display name ("Goi"): window titles, About, dialogs.
	Name string `json:"name"`
	// Program is the lower-case program name ("goi"): binary, Linux program
	// name, log prefix, MCP server name.
	Program string `json:"program"`
	// Description is a one-line description of the app.
	Description string `json:"description"`
	// BundleID is the platform bundle identifier ("dev.goi.goi").
	BundleID string `json:"bundleId"`
	// InstanceID prefixes the single-instance id (one process per config
	// directory). Empty means BundleID.
	InstanceID string `json:"-"`
	// Version is the full product version (info.version in the app's
	// build/config.yml).
	Version string `json:"version"`
	// DirName names the per-user config and cache directories:
	// <UserConfigDir>/<DirName>/<DataVersion>.
	DirName string `json:"-"`
	// DataVersion is the major.minor that scopes user data directories.
	DataVersion string `json:"-"`
	// HomeEnv is the environment variable that redirects user data to
	// $HomeEnv/config and $HomeEnv/cache ("GOI_HOME").
	HomeEnv string `json:"homeEnv"`
	// ProjectDir is the project state directory (".goi").
	ProjectDir string `json:"projectDir"`
	// FileAssociations are the extensions the OS opens with this app.
	FileAssociations []string `json:"-"`
}

// MCPTokenEnv is the environment variable that carries the MCP bearer token
// to `<program> mcp-bridge` ("GOI_MCP_TOKEN").
func (id Identity) MCPTokenEnv() string {
	return strings.ToUpper(id.Program) + "_MCP_TOKEN"
}

// SingleInstanceID is the prefix of the single-instance lock id.
func (id Identity) SingleInstanceID() string {
	if id.InstanceID != "" {
		return id.InstanceID
	}
	return id.BundleID
}

var (
	mu  sync.RWMutex
	cur Identity
)

// Set installs the process identity. Call it once, first thing in main.
func Set(id Identity) {
	mu.Lock()
	cur = id
	mu.Unlock()
}

// Current returns the process identity (the zero Identity before Set).
func Current() Identity {
	mu.RLock()
	defer mu.RUnlock()
	return cur
}

// Name is Current().Name, or "the IDE" when no identity is set.
func Name() string {
	if n := Current().Name; n != "" {
		return n
	}
	return "the IDE"
}

// Program is Current().Program, or "ide" when no identity is set.
func Program() string {
	if p := Current().Program; p != "" {
		return p
	}
	return "ide"
}
