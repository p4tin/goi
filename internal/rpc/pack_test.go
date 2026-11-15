package rpc

import (
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/product/producttest"
	"github.com/p4tin/goi/packs/golang"
)

// testPack is the pack rpc tests run with: the test identity plus the Go
// pack (most rpc tests exercise Go projects).
func testPack() pack.Pack {
	return golang.Pack(producttest.Identity)
}
