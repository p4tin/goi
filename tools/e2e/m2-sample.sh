#!/bin/sh
# Creates the sample module used by m2.json: tools/e2e/m2-sample.sh <dir>
set -e
d="${1:?usage: m2-sample.sh <dir>}"
mkdir -p "$d"
cat > "$d/go.mod" <<'GO'
module example.com/sample

go 1.22
GO
cat > "$d/main.go" <<'GO'
package main

import "fmt"

type Greeter struct {
	Name string
}

func (g Greeter) Greet() string {
	return "hello " + g.Name
}

func helper(n int) int { return n * 2 }

func main() {
	g := Greeter{Name: "goi"}
	fmt.Println(g.Greet(), helper(2))
	var unused int
}
GO
cat > "$d/main_test.go" <<'GO'
package main

import "testing"

func TestHelper(t *testing.T) {
	if helper(2) != 4 {
		t.Fatal("helper")
	}
}
GO
echo "$d"
