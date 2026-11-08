#!/bin/sh
# Creates the sample module used by m3.json: tools/e2e/m3-sample.sh <dir>
set -e
d="${1:?usage: m3-sample.sh <dir>}"
mkdir -p "$d/cmd/work" "$d/calc" "$d/worker" "$d/.goi/runConfigurations"
cat > "$d/go.mod" <<'GO'
module example.com/m3

go 1.22
GO
cat > "$d/cmd/work/main.go" <<'GO'
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	fmt.Println("\x1b[32mgreen output\x1b[0m from cmd/work/main.go:10")
	fmt.Fprintln(os.Stderr, "a warning on stderr")
	fmt.Println("type a line:")
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		fmt.Println("you typed:", sc.Text())
	}
	os.Exit(4)
}
GO
cat > "$d/calc/calc.go" <<'GO'
package calc

// Add returns a + b.
func Add(a, b int) int { return a + b }

// Greeting returns a greeting.
func Greeting(name string) string { return "hello " + name }
GO
cat > "$d/calc/calc_test.go" <<'GO'
package calc

import (
	"fmt"
	"testing"
)

func TestAdd(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		if Add(1, 2) != 3 {
			t.Fatal("bad sum")
		}
	})
	t.Run("broken case", func(t *testing.T) {
		if Add(2, 2) != 5 {
			t.Errorf("Add(2, 2) = %d, want 5", Add(2, 2))
		}
	})
}

// TestGreeting fails with testify-style output (without the dependency).
func TestGreeting(t *testing.T) {
	got := Greeting("there")
	if got != "hello world" {
		t.Error(fmt.Sprintf("\n\tError Trace:\tcalc_test.go:25\n\tError:      \tNot equal: \n\t            \texpected: %q\n\t            \tactual  : %q\n\tTest:       \tTestGreeting", "hello world", got))
	}
}

func TestSkipped(t *testing.T) { t.Skip("not yet") }

func BenchmarkAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Add(i, i)
	}
}
GO
cat > "$d/worker/worker_test.go" <<'GO'
package worker

import (
	"sync"
	"testing"
)

func process(i int, out []int, wg *sync.WaitGroup) {
	defer wg.Done()
	v := i * i
	out[i] = v
}

func TestWorkers(t *testing.T) {
	var wg sync.WaitGroup
	out := make([]int, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go process(i, out, &wg)
	}
	wg.Wait()
	if out[3] != 9 {
		t.Fatalf("out[3] = %d", out[3])
	}
}
GO
cat > "$d/.goi/runConfigurations/Work.toml" <<'TOML'
name = "Work"
type = "app"
kind = "package"
package = "./cmd/work"
TOML
cat > "$d/.gitignore" <<'TXT'
*.test
TXT
echo "sample module in $d"
