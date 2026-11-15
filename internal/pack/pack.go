// Package pack defines an application pack: everything that makes one
// product (Goi for Go, Pyi for Python) out of the shared IDE core. A pack
// carries the product identity and the language tooling (project detection,
// SDK/interpreter, language servers, formatter, linter, run types, debugger).
//
// Each app's main package builds a Pack and hands it to boot.Main. Core code
// reads capabilities only through the interfaces here; it never branches on
// which pack is running, and nothing in internal/ imports an apps/ package.
// A nil capability means the app doesn't have it: core hides its UI.
package pack

import "github.com/p4tin/goi/internal/product"

// Identity is the product identity (name, ids, directories).
type Identity = product.Identity

// Pack is one application: identity plus capabilities.
type Pack struct {
	Identity Identity
}
