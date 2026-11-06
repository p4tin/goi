package gosyntax

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MarkerKind classifies a runnable declaration.
type MarkerKind string

const (
	KindMain    MarkerKind = "main"
	KindTest    MarkerKind = "test"
	KindBench   MarkerKind = "bench"
	KindFuzz    MarkerKind = "fuzz"
	KindExample MarkerKind = "example"
)

// RunMarker is a runnable function found in a Go file. Line is 1-based and
// points at the func keyword.
type RunMarker struct {
	Kind MarkerKind `json:"kind"`
	Name string     `json:"name"`
	Line int        `json:"line"`
}

// RunMarkers returns the runnable functions in src: func main in package
// main, and Test/Benchmark/Fuzz/Example functions in _test.go files, using
// the same naming and signature rules as `go test`. Files with syntax errors
// yield markers for whatever declarations could be parsed.
func RunMarkers(filename, src string) []RunMarker {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if f == nil {
		return nil
	}
	isTestFile := strings.HasSuffix(filename, "_test.go")
	testingName, dotTesting := testingImport(f)

	var out []RunMarker
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Type.TypeParams != nil {
			continue
		}
		if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
			continue
		}
		name := fn.Name.Name
		line := fset.Position(fn.Pos()).Line
		params := fn.Type.Params.List

		if name == "main" && f.Name.Name == "main" && len(params) == 0 {
			out = append(out, RunMarker{KindMain, name, line})
			continue
		}
		if !isTestFile {
			continue
		}
		switch {
		case isTestName(name, "Test") && takesTesting(params, "T", testingName, dotTesting):
			out = append(out, RunMarker{KindTest, name, line})
		case isTestName(name, "Benchmark") && takesTesting(params, "B", testingName, dotTesting):
			out = append(out, RunMarker{KindBench, name, line})
		case isTestName(name, "Fuzz") && takesTesting(params, "F", testingName, dotTesting):
			out = append(out, RunMarker{KindFuzz, name, line})
		case isTestName(name, "Example") && len(params) == 0:
			out = append(out, RunMarker{KindExample, name, line})
		}
	}
	return out
}

// isTestName mirrors cmd/go: name has prefix and the next rune, if any, is
// not lower case ("TestFoo", "Test_foo" and "Test" match; "Testfoo" doesn't).
func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}

// testingImport returns the local name of the "testing" import and whether
// it is dot-imported.
func testingImport(f *ast.File) (name string, dot bool) {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "testing" {
			continue
		}
		if imp.Name == nil {
			return "testing", false
		}
		switch imp.Name.Name {
		case ".":
			return "", true
		case "_":
			continue
		default:
			return imp.Name.Name, false
		}
	}
	return "", false
}

// takesTesting reports whether params is exactly one parameter of type
// *testing.<typ>.
func takesTesting(params []*ast.Field, typ, testingName string, dot bool) bool {
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && testingName != "" && pkg.Name == testingName && x.Sel.Name == typ
	case *ast.Ident:
		return dot && x.Name == typ
	}
	return false
}
