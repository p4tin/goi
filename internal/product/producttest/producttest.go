// Package producttest provides the product identity tests run with. It keeps
// the names existing tests and fixtures expect (GOI_HOME, .goi, "Goi").
package producttest

import "github.com/p4tin/goi/internal/product"

// Identity is the identity tests use.
var Identity = product.Identity{
	Name:        "Goi",
	Program:     "goi",
	Description: "test IDE",
	BundleID:    "dev.goi.test",
	Version:     "0.0.0-test",
	DirName:     "Goi",
	DataVersion: "0.1",
	HomeEnv:     "GOI_HOME",
	ProjectDir:  ".goi",
}

// Set installs Identity; call it from TestMain.
func Set() { product.Set(Identity) }
