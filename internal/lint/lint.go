// Package lint runs golangci-lint (v1 or v2) and turns its JSON report into
// issues with absolute paths, 0-based UTF-16 positions and normalized
// suggested fixes. It has no UI knowledge.
package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/p4tin/goi/internal/toolbin"
)

// Module is what Install installs.
const Module = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"

// Name is the executable base name.
const Name = "golangci-lint"

// Pos is a 0-based line and UTF-16 character, like LSP.
type Pos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Edit replaces [Start, End) with NewText.
type Edit struct {
	Start   Pos    `json:"start"`
	End     Pos    `json:"end"`
	NewText string `json:"newText"`
}

// Issue is one finding.
type Issue struct {
	Path     string `json:"path"`
	Start    Pos    `json:"start"`
	End      Pos    `json:"end"`
	Linter   string `json:"linter"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	// Fix holds the suggested edits (same file), if any.
	Fix []Edit `json:"fix"`
}

// Version runs `golangci-lint version` and returns the major version and
// the version string.
func Version(ctx context.Context, bin string) (int, string, error) {
	out, err := exec.CommandContext(ctx, bin, "version").CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("golangci-lint version: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseVersion(string(out))
}

var versionRE = regexp.MustCompile(`version v?(\d+)\.(\d+)\.(\d+)`)

func parseVersion(s string) (int, string, error) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return 0, "", fmt.Errorf("golangci-lint: unrecognized version output %q", strings.TrimSpace(s))
	}
	major, _ := strconv.Atoi(m[1])
	return major, m[1] + "." + m[2] + "." + m[3], nil
}

// Args returns `golangci-lint run` arguments for a major version.
func Args(major int, extra, targets []string) []string {
	var args []string
	if major >= 2 {
		args = []string{"run", "--output.json.path", "stdout", "--output.text.path", "stderr", "--show-stats=false", "--issues-exit-code=0"}
	} else {
		args = []string{"run", "--out-format", "json", "--issues-exit-code=0", "--print-issued-lines=false"}
	}
	args = append(args, extra...)
	return append(args, targets...)
}

type report struct {
	Issues []rawIssue `json:"Issues"`
}

type rawIssue struct {
	FromLinter string `json:"FromLinter"`
	Text       string `json:"Text"`
	Severity   string `json:"Severity"`
	Pos        struct {
		Filename string `json:"Filename"`
		Offset   int    `json:"Offset"`
		Line     int    `json:"Line"`
		Column   int    `json:"Column"`
	} `json:"Pos"`
	LineRange *struct {
		From int `json:"From"`
		To   int `json:"To"`
	} `json:"LineRange"`
	// v1.
	Replacement *struct {
		NeedOnlyDelete bool     `json:"NeedOnlyDelete"`
		NewLines       []string `json:"NewLines"`
		Inline         *struct {
			StartCol  int    `json:"StartCol"`
			Length    int    `json:"Length"`
			NewString string `json:"NewString"`
		} `json:"Inline"`
	} `json:"Replacement"`
	// v2 (analysis.SuggestedFix; byte offsets, NewText base64 []byte).
	SuggestedFixes []struct {
		TextEdits []struct {
			Pos     int    `json:"Pos"`
			End     int    `json:"End"`
			NewText []byte `json:"NewText"`
		} `json:"TextEdits"`
	} `json:"SuggestedFixes"`
}

// fileText caches file contents for position conversion.
type fileText struct {
	cache map[string][]byte
	read  func(string) ([]byte, error)
}

func (f *fileText) get(p string) []byte {
	if b, ok := f.cache[p]; ok {
		return b
	}
	b, err := f.read(p)
	if err != nil {
		b = nil
	}
	f.cache[p] = b
	return b
}

// Parse reads a JSON report (golangci-lint may print text before it on
// errors; the first '{' starts the JSON). Paths are relative to baseDir.
func Parse(out []byte, baseDir string) ([]Issue, error) {
	return parse(out, baseDir, os.ReadFile)
}

func parse(out []byte, baseDir string, read func(string) ([]byte, error)) ([]Issue, error) {
	i := bytes.IndexByte(out, '{')
	if i < 0 {
		return nil, errors.New("golangci-lint: no JSON report in output")
	}
	var r report
	if err := json.NewDecoder(bytes.NewReader(out[i:])).Decode(&r); err != nil {
		return nil, fmt.Errorf("golangci-lint: decode report: %w", err)
	}
	ft := &fileText{cache: map[string][]byte{}, read: read}
	issues := []Issue{}
	for _, ri := range r.Issues {
		p := ri.Pos.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, filepath.FromSlash(p))
		}
		text := ft.get(p)
		line := max(ri.Pos.Line-1, 0)
		startChar := byteColToUTF16(lineBytes(text, line), ri.Pos.Column-1)
		end := wordEnd(text, line, startChar)
		is := Issue{Path: p, Start: Pos{line, startChar}, End: end, Linter: ri.FromLinter, Message: ri.Text, Severity: ri.Severity, Fix: []Edit{}}
		if len(ri.SuggestedFixes) > 0 && text != nil {
			for _, te := range ri.SuggestedFixes[0].TextEdits {
				is.Fix = append(is.Fix, Edit{Start: offsetPos(text, te.Pos), End: offsetPos(text, te.End), NewText: string(te.NewText)})
			}
		} else if ri.Replacement != nil && text != nil {
			is.Fix = replacementEdits(text, ri, line)
		}
		issues = append(issues, is)
	}
	return issues, nil
}

// replacementEdits converts a v1 Replacement into edits.
func replacementEdits(text []byte, ri rawIssue, line int) []Edit {
	rep := ri.Replacement
	if rep.Inline != nil {
		lb := lineBytes(text, line)
		s := byteColToUTF16(lb, rep.Inline.StartCol)
		e := byteColToUTF16(lb, rep.Inline.StartCol+rep.Inline.Length)
		return []Edit{{Start: Pos{line, s}, End: Pos{line, e}, NewText: rep.Inline.NewString}}
	}
	from, to := line, line
	if ri.LineRange != nil {
		from, to = ri.LineRange.From-1, ri.LineRange.To-1
	}
	// Replace whole lines [from, to] including the final newline.
	start := Pos{from, 0}
	end := Pos{to + 1, 0}
	if rep.NeedOnlyDelete {
		return []Edit{{Start: start, End: end}}
	}
	return []Edit{{Start: start, End: end, NewText: strings.Join(rep.NewLines, "\n") + "\n"}}
}

func lineBytes(text []byte, line int) []byte {
	for i := 0; i < line; i++ {
		j := bytes.IndexByte(text, '\n')
		if j < 0 {
			return nil
		}
		text = text[j+1:]
	}
	if j := bytes.IndexByte(text, '\n'); j >= 0 {
		return text[:j]
	}
	return text
}

// byteColToUTF16 converts a 0-based byte column to UTF-16 code units.
func byteColToUTF16(line []byte, col int) int {
	if col <= 0 {
		return 0
	}
	if col > len(line) {
		col = len(line)
	}
	n := 0
	for b := line[:col]; len(b) > 0; {
		r, size := utf8.DecodeRune(b)
		n += len(utf16.Encode([]rune{r}))
		b = b[size:]
	}
	return n
}

// offsetPos converts a byte offset to a position.
func offsetPos(text []byte, off int) Pos {
	if off > len(text) {
		off = len(text)
	}
	if off < 0 {
		off = 0
	}
	line := bytes.Count(text[:off], []byte{'\n'})
	start := bytes.LastIndexByte(text[:off], '\n') + 1
	return Pos{line, byteColToUTF16(text[start:], off-start)}
}

// wordEnd extends a position to the end of the identifier there (or one
// character), for a visible underline.
func wordEnd(text []byte, line, char int) Pos {
	lb := lineBytes(text, line)
	if lb == nil {
		return Pos{line, char + 1}
	}
	runes := utf16.Encode([]rune(string(lb)))
	if char >= len(runes) {
		return Pos{line, len(runes)}
	}
	e := char
	isWord := func(c uint16) bool {
		return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c > 127
	}
	for e < len(runes) && isWord(runes[e]) {
		e++
	}
	if e == char {
		e = len(runes) // not on a word (e.g. a comment): to end of line
	}
	return Pos{line, e}
}

// Spec describes one run.
type Spec struct {
	Bin     string
	Major   int
	Dir     string
	Env     []string
	Targets []string
	Extra   []string
}

// Run lints spec.Targets from spec.Dir and returns the issues. A run that
// fails without a JSON report (bad config, build failure in v1) returns an
// error carrying golangci-lint's message.
func Run(ctx context.Context, s Spec) ([]Issue, error) {
	cmd := exec.CommandContext(ctx, s.Bin, Args(s.Major, s.Extra, s.Targets)...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	issues, perr := Parse(out.Bytes(), s.Dir)
	if perr == nil {
		return issues, nil
	}
	msg := strings.TrimSpace(errb.String())
	if msg == "" {
		msg = strings.TrimSpace(out.String())
	}
	if err != nil {
		return nil, fmt.Errorf("golangci-lint: %s", toolbin.Tail(msg, 2000))
	}
	return nil, perr
}

// Resolve finds golangci-lint (see toolbin.Resolve).
func Resolve(o toolbin.Options) (string, error) {
	o.Name = Name
	return toolbin.Resolve(o)
}
