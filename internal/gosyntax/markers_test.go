package gosyntax

import (
	"reflect"
	"testing"
)

func TestRunMarkers(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		src      string
		want     []RunMarker
	}{
		{
			name:     "main in package main",
			filename: "main.go",
			src:      "package main\n\nfunc main() {}\n",
			want:     []RunMarker{{KindMain, "main", 3}},
		},
		{
			name:     "main outside package main",
			filename: "x.go",
			src:      "package lib\nfunc main() {}\n",
		},
		{
			name:     "main with params is not runnable",
			filename: "main.go",
			src:      "package main\nfunc main(x int) {}\n",
		},
		{
			name:     "tests only in _test.go",
			filename: "x.go",
			src:      "package x\nimport \"testing\"\nfunc TestA(t *testing.T) {}\n",
		},
		{
			name:     "all kinds",
			filename: "x_test.go",
			src: `package x

import "testing"

func TestA(t *testing.T)      {}
func Test_b(t *testing.T)     {}
func Test(t *testing.T)       {}
func Testfoo(t *testing.T)    {}
func BenchmarkB(b *testing.B) {}
func FuzzF(f *testing.F)      {}
func ExampleE()               {}
func Example_suffix()         {}
func TestMain(m *testing.M)   {}
func TestWrongType(t testing.T) {}
func ExampleWithArg(x int)    {}
`,
			want: []RunMarker{
				{KindTest, "TestA", 5},
				{KindTest, "Test_b", 6},
				{KindTest, "Test", 7},
				{KindBench, "BenchmarkB", 9},
				{KindFuzz, "FuzzF", 10},
				{KindExample, "ExampleE", 11},
				{KindExample, "Example_suffix", 12},
			},
		},
		{
			name:     "aliased testing import",
			filename: "x_test.go",
			src:      "package x\nimport tt \"testing\"\nfunc TestA(t *tt.T) {}\nfunc TestB(t *testing.T) {}\n",
			want:     []RunMarker{{KindTest, "TestA", 3}},
		},
		{
			name:     "dot-imported testing",
			filename: "x_test.go",
			src:      "package x\nimport . \"testing\"\nfunc TestA(t *T) {}\n",
			want:     []RunMarker{{KindTest, "TestA", 3}},
		},
		{
			name:     "methods and generics excluded",
			filename: "x_test.go",
			src:      "package x\nimport \"testing\"\ntype s struct{}\nfunc (s) TestM(t *testing.T) {}\nfunc TestG[T any](t *testing.T) {}\n",
		},
		{
			name:     "results excluded",
			filename: "x_test.go",
			src:      "package x\nimport \"testing\"\nfunc TestR(t *testing.T) error { return nil }\n",
		},
		{
			name:     "doc comment does not shift line",
			filename: "x_test.go",
			src:      "package x\nimport \"testing\"\n\n// TestA does things.\nfunc TestA(t *testing.T) {}\n",
			want:     []RunMarker{{KindTest, "TestA", 5}},
		},
		{
			// The parser recovers an unterminated body, so a test being typed
			// keeps its marker.
			name:     "syntax error tolerated",
			filename: "x_test.go",
			src:      "package x\nimport \"testing\"\nfunc TestA(t *testing.T) {}\nfunc TestB(t *testing.T) {\n",
			want:     []RunMarker{{KindTest, "TestA", 3}, {KindTest, "TestB", 4}},
		},
		{
			name:     "unparseable header yields nothing",
			filename: "x_test.go",
			src:      "not go at all",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunMarkers(tt.filename, tt.src)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
