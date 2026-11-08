package gopls

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/p4tin/goi/internal/lsp"
)

// FileEdit is the set of text edits for one file.
type FileEdit struct {
	Path  string         `json:"path"`
	Edits []lsp.TextEdit `json:"edits"`
}

// Location is an lsp.Location with the URI converted to an OS path.
type Location struct {
	Path  string    `json:"path"`
	Range lsp.Range `json:"range"`
}

// ErrOverlappingEdits is returned when two edits touch the same text.
var ErrOverlappingEdits = errors.New("gopls: overlapping edits")

// FileEdits flattens a workspace edit into per-file edits sorted by path.
func FileEdits(we lsp.WorkspaceEdit) []FileEdit {
	by := map[string][]lsp.TextEdit{}
	for uri, edits := range we.Changes {
		p := URIToPath(uri)
		by[p] = append(by[p], edits...)
	}
	for _, dc := range we.DocumentChanges {
		p := URIToPath(dc.TextDocument.URI)
		by[p] = append(by[p], dc.Edits...)
	}
	out := make([]FileEdit, 0, len(by))
	for p, edits := range by {
		if edits == nil {
			edits = []lsp.TextEdit{}
		}
		out = append(out, FileEdit{Path: p, Edits: edits})
	}
	slices.SortFunc(out, func(a, b FileEdit) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// ApplyEdits applies LSP text edits (UTF-16 positions) to text. Positions
// past the end of a line clamp to the line end; lines past the end of the
// text clamp to the end of the text.
func ApplyEdits(text string, edits []lsp.TextEdit) (string, error) {
	if len(edits) == 0 {
		return text, nil
	}
	starts := lineStarts(text)
	type span struct {
		from, to int
		text     string
		idx      int
	}
	spans := make([]span, len(edits))
	for i, e := range edits {
		from := Offset(text, starts, e.Range.Start)
		to := Offset(text, starts, e.Range.End)
		if to < from {
			return "", fmt.Errorf("gopls: edit %d ends before it starts", i)
		}
		spans[i] = span{from, to, e.NewText, i}
	}
	slices.SortStableFunc(spans, func(a, b span) int {
		if a.from != b.from {
			return a.from - b.from
		}
		return a.idx - b.idx
	})
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for i, s := range spans {
		if i > 0 && s.from < spans[i-1].to {
			return "", ErrOverlappingEdits
		}
		b.WriteString(text[last:s.from])
		b.WriteString(s.text)
		last = s.to
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

func lineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// Offset converts an LSP position to a byte offset in text, given the byte
// offsets of each line start (see lineStarts).
func Offset(text string, starts []int, p lsp.Position) int {
	if p.Line >= len(starts) {
		return len(text)
	}
	if p.Line < 0 {
		return 0
	}
	off := starts[p.Line]
	end := len(text)
	if p.Line+1 < len(starts) {
		end = starts[p.Line+1] - 1 // the '\n'
		if end > off && text[end-1] == '\r' {
			end--
		}
	}
	units := 0
	for off < end && units < p.Character {
		r, size := utf8.DecodeRuneInString(text[off:])
		if r >= 0x10000 {
			units += 2
		} else {
			units++
		}
		off += size
	}
	return off
}
