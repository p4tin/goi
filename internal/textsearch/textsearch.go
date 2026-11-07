// Package textsearch implements Find/Replace in Path: line-based regular
// expression search over a list of files, with file masks, and the
// matching replacement. Search and replace both work line by line so that
// `^`, `$` and `\s` mean the same thing in both.
package textsearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Query describes a search.
type Query struct {
	Pattern       string `json:"pattern"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	// Mask is a comma-separated list of base-name globs; a leading "!"
	// excludes ("*.go, !*_test.go"). Empty matches every file.
	Mask string `json:"mask"`
	// Replace is the replacement; with Regex, $1 / ${name} refer to groups.
	Replace string `json:"replace"`
}

// Match is one occurrence. Col and End are UTF-16 offsets within the line;
// PreviewStart is the UTF-16 offset of Preview's first character.
type Match struct {
	Path         string `json:"path"`
	Line         int    `json:"line"` // 1-based
	Col          int    `json:"col"`
	End          int    `json:"end"`
	Preview      string `json:"preview"`
	PreviewStart int    `json:"previewStart"`
}

// Limits on what is searched.
const (
	MaxFileSize    = 5 << 20
	maxPreviewLine = 300
	binarySniff    = 8000
)

// ErrEmptyPattern is returned for an empty search string.
var ErrEmptyPattern = errors.New("textsearch: empty pattern")

// Compile builds the line regexp for q.
func Compile(q Query) (*regexp.Regexp, error) {
	if q.Pattern == "" {
		return nil, ErrEmptyPattern
	}
	pat := q.Pattern
	if !q.Regex {
		pat = regexp.QuoteMeta(pat)
	}
	if q.WholeWord {
		pat = `\b(?:` + pat + `)\b`
	}
	if !q.CaseSensitive {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("textsearch: invalid pattern: %w", err)
	}
	return re, nil
}

// Mask is a parsed file mask.
type Mask struct {
	include, exclude []string
}

// ParseMask parses a comma-separated glob list.
func ParseMask(s string) Mask {
	var m Mask
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "!") {
			if g := strings.TrimSpace(p[1:]); g != "" {
				m.exclude = append(m.exclude, g)
			}
			continue
		}
		m.include = append(m.include, p)
	}
	return m
}

// Matches reports whether the file at p passes the mask (by base name).
func (m Mask) Matches(p string) bool {
	base := filepath.Base(p)
	for _, g := range m.exclude {
		if ok, _ := path.Match(g, base); ok {
			return false
		}
	}
	if len(m.include) == 0 {
		return true
	}
	for _, g := range m.include {
		if ok, _ := path.Match(g, base); ok {
			return true
		}
	}
	return false
}

// Search scans paths in order and returns up to limit matches. Binary files
// and files larger than MaxFileSize are skipped.
func Search(ctx context.Context, paths []string, q Query, limit int) ([]Match, bool, error) {
	return SearchWith(ctx, paths, q, limit, nil)
}

// SearchWith is Search with buffers overriding file contents (unsaved
// editor buffers, keyed by path).
func SearchWith(ctx context.Context, paths []string, q Query, limit int, buffers map[string]string) ([]Match, bool, error) {
	re, err := Compile(q)
	if err != nil {
		return []Match{}, false, err
	}
	mask := ParseMask(q.Mask)
	out := []Match{}
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return out, false, err
		}
		if !mask.Matches(p) {
			continue
		}
		text, ok := buffers[p]
		if !ok {
			b, readable := readText(p)
			if !readable {
				continue
			}
			text = string(b)
		}
		var truncated bool
		out, truncated = searchText(out, p, text, re, limit)
		if truncated {
			return out, true, nil
		}
	}
	return out, false, nil
}

func readText(p string) ([]byte, bool) {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() || st.Size() > MaxFileSize {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	sniff := b
	if len(sniff) > binarySniff {
		sniff = sniff[:binarySniff]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return nil, false
	}
	return b, true
}

// SearchText finds matches of q in text (used for open, unsaved buffers).
func SearchText(p, text string, q Query, limit int) ([]Match, bool, error) {
	re, err := Compile(q)
	if err != nil {
		return []Match{}, false, err
	}
	out, truncated := searchText([]Match{}, p, text, re, limit)
	return out, truncated, nil
}

func searchText(out []Match, p, text string, re *regexp.Regexp, limit int) ([]Match, bool) {
	text = strings.TrimPrefix(text, "\ufeff")
	lineNo := 0
	for len(text) > 0 || lineNo == 0 {
		lineNo++
		line := text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			text = ""
		}
		line = strings.TrimSuffix(line, "\r")
		for _, loc := range re.FindAllStringIndex(line, -1) {
			if loc[1] == loc[0] {
				continue
			}
			if limit > 0 && len(out) >= limit {
				return out, true
			}
			out = append(out, makeMatch(p, lineNo, line, loc[0], loc[1]))
		}
		if text == "" {
			break
		}
	}
	return out, false
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func makeMatch(p string, lineNo int, line string, from, to int) Match {
	m := Match{Path: p, Line: lineNo, Col: utf16Len(line[:from]), End: utf16Len(line[:to])}
	if len(line) <= maxPreviewLine {
		m.Preview = line
		return m
	}
	start := from - 100
	if start < 0 {
		start = 0
	}
	for start > 0 && !utf8.RuneStart(line[start]) {
		start--
	}
	end := start + maxPreviewLine
	if end < to {
		end = to
	}
	if end > len(line) {
		end = len(line)
	}
	for end < len(line) && !utf8.RuneStart(line[end]) {
		end++
	}
	m.Preview = line[start:end]
	m.PreviewStart = utf16Len(line[:start])
	return m
}

// Template returns the replacement template for q: regex replacements keep
// $1 group references; literal ones have "$" escaped.
func Template(q Query) string {
	if q.Regex {
		return q.Replace
	}
	return strings.ReplaceAll(q.Replace, "$", "$$")
}

// ReplaceText replaces every match of re in text, line by line, expanding
// tmpl (see Template). It returns the new text and the replacement count.
// Line endings (\n or \r\n) are preserved.
func ReplaceText(text string, re *regexp.Regexp, tmpl string) (string, int) {
	var b strings.Builder
	b.Grow(len(text))
	count := 0
	for {
		line, rest, more := strings.Cut(text, "\n")
		cr := strings.HasSuffix(line, "\r")
		body := strings.TrimSuffix(line, "\r")
		last := 0
		for _, sub := range re.FindAllStringSubmatchIndex(body, -1) {
			if sub[1] == sub[0] {
				continue
			}
			b.WriteString(body[last:sub[0]])
			b.Write(re.ExpandString(nil, tmpl, body, sub))
			last = sub[1]
			count++
		}
		b.WriteString(body[last:])
		if cr {
			b.WriteByte('\r')
		}
		if !more {
			break
		}
		b.WriteByte('\n')
		text = rest
	}
	return b.String(), count
}
