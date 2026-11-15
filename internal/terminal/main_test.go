package terminal

import (
	"os"
	"testing"

	"github.com/p4tin/goi/internal/product/producttest"
)

func TestMain(m *testing.M) {
	producttest.Set()
	os.Exit(m.Run())
}
