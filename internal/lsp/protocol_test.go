package lsp

import (
	"encoding/json"
	"testing"
)

func TestMarkupVariants(t *testing.T) {
	cases := map[string]string{
		`"plain"`:                                     "plain",
		`{"kind":"markdown","value":"**b**"}`:         "**b**",
		`{"kind":"plaintext","value":"a*b"}`:          `a\*b`,
		`{"language":"go","value":"func F()"}`:        "```go\nfunc F()\n```",
		`["x",{"language":"go","value":"var v int"}]`: "x\n\n```go\nvar v int\n```",
		`null`: "",
	}
	for in, want := range cases {
		var m Markup
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if string(m) != want {
			t.Errorf("%s: got %q, want %q", in, m, want)
		}
	}
	b, _ := json.Marshal(Markup("x"))
	if string(b) != `"x"` {
		t.Errorf("marshal = %s", b)
	}
}

func TestLocationsVariants(t *testing.T) {
	r := `{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}`
	cases := map[string]int{
		`null`: 0,
		`{"uri":"file:///a.go","range":` + r + `}`:                                                1,
		`[{"uri":"file:///a.go","range":` + r + `},{"uri":"file:///b.go","range":` + r + `}]`:     2,
		`[{"targetUri":"file:///c.go","targetRange":` + r + `,"targetSelectionRange":` + r + `}]`: 1,
	}
	for in, n := range cases {
		var l Locations
		if err := json.Unmarshal([]byte(in), &l); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if len(l) != n || l == nil {
			t.Errorf("%s: got %d (%v)", in, len(l), l)
		}
	}
	var l Locations
	json.Unmarshal([]byte(`[{"targetUri":"file:///c.go","targetRange":`+r+`,"targetSelectionRange":`+r+`}]`), &l)
	if l[0].URI != "file:///c.go" || l[0].Range.Start.Character != 2 {
		t.Errorf("link = %+v", l[0])
	}
}

func TestCompletionListVariants(t *testing.T) {
	var c CompletionList
	json.Unmarshal([]byte(`[{"label":"a"}]`), &c)
	if len(c.Items) != 1 || c.IsIncomplete {
		t.Errorf("array: %+v", c)
	}
	json.Unmarshal([]byte(`{"isIncomplete":true,"items":[{"label":"a","documentation":{"kind":"markdown","value":"d"}}]}`), &c)
	if !c.IsIncomplete || c.Items[0].Documentation != "d" {
		t.Errorf("list: %+v", c)
	}
	json.Unmarshal([]byte(`null`), &c)
	if c.Items == nil {
		t.Error("nil items for null")
	}
}

func TestCodeActionsWrapsCommands(t *testing.T) {
	var a CodeActions
	in := `[{"title":"Run","command":"gopls.run","arguments":[1]},{"title":"Fix","kind":"quickfix","command":{"title":"Fix","command":"gopls.fix"}}]`
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || a[0].Command == nil || a[0].Command.Command != "gopls.run" || a[1].Kind != "quickfix" || a[1].Command.Command != "gopls.fix" {
		t.Fatalf("got %+v", a)
	}
}

func TestPrepareRenameVariants(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		ph   string
		line int
	}{
		{`null`, false, "", 0},
		{`{"start":{"line":3,"character":1},"end":{"line":3,"character":4}}`, true, "", 3},
		{`{"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}},"placeholder":"x"}`, true, "x", 2},
		{`{"defaultBehavior":true}`, true, "", 0},
	}
	for _, c := range cases {
		var p PrepareRenameResult
		if err := json.Unmarshal([]byte(c.in), &p); err != nil {
			t.Fatal(err)
		}
		if p.OK != c.ok || p.Placeholder != c.ph || p.Range.Start.Line != c.line {
			t.Errorf("%s: got %+v", c.in, p)
		}
	}
}

func TestParameterLabels(t *testing.T) {
	var s SignatureInformation
	in := `{"label":"f(a int, b string)","parameters":[{"label":[2,7]},{"label":"b string"}]}`
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	p0, p1 := s.Parameters[0], s.Parameters[1]
	if p0.Start != 2 || p0.End != 7 || p0.Label != "" || p1.Label != "b string" || p1.Start != -1 {
		t.Fatalf("got %+v %+v", p0, p1)
	}
}

func TestWorkspaceEditDropsFileOps(t *testing.T) {
	in := `{"documentChanges":[
	  {"textDocument":{"uri":"file:///a.go","version":2},"edits":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"x"}]},
	  {"kind":"create","uri":"file:///b.go"}
	],"changes":{"file:///c.go":[]}}`
	var w WorkspaceEdit
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.DocumentChanges) != 1 || w.DocumentChanges[0].Edits[0].NewText != "x" || *w.DocumentChanges[0].TextDocument.Version != 2 {
		t.Fatalf("got %+v", w)
	}
	if _, ok := w.Changes["file:///c.go"]; !ok {
		t.Error("changes lost")
	}
}

func TestDiagnosticDataRoundTrips(t *testing.T) {
	in := `{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"severity":1,"code":"UnusedVar","source":"compiler","message":"m","data":{"fixes":[1,2]}}`
	var d Diagnostic
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(d)
	var back map[string]any
	json.Unmarshal(out, &back)
	if back["code"] != "UnusedVar" || back["data"] == nil {
		t.Fatalf("round trip lost fields: %s", out)
	}
}
