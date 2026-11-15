package rpc

import (
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/product/producttest"
)

// testPack is the pack rpc tests run with.
func testPack() pack.Pack {
	return pack.Pack{Identity: producttest.Identity}
}
