package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Hunk is one @@ section of a unified diff. Lines keep their ' ', '+', '-'
// or '\' prefix.
type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Header   string   `json:"header"`
	Lines    []string `json:"lines"`
}

// FileDiff is the diff of one file.
type FileDiff struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
	// Header holds the "diff --git" .. "+++" lines verbatim.
	Header []string `json:"header"`
	Hunks  []Hunk   `json:"hunks"`
	Binary bool     `json:"binary"`
}

// DiffSpec selects what to compare.
type DiffSpec struct {
	// Path limits the diff to one file (absolute or repo-relative).
	Path string
	// Staged compares the index with HEAD (else the work tree with the index).
	Staged bool
	// Rev compares the work tree with a revision (e.g. "HEAD"); with Staged,
	// the index with that revision.
	Rev string
	// Commit shows a commit against its first parent.
	Commit string
}

// Diff returns unified diff text.
func (r *Repo) Diff(ctx context.Context, s DiffSpec) (string, error) {
	args := []string{"diff", "--no-ext-diff", "--no-color", "-M"}
	switch {
	case s.Commit != "":
		args = []string{"show", "--no-ext-diff", "--no-color", "-M", "--format=", s.Commit}
	case s.Staged:
		args = append(args, "--cached")
		if s.Rev != "" {
			args = append(args, s.Rev)
		}
	case s.Rev != "":
		args = append(args, s.Rev)
	}
	if s.Path != "" {
		args = append(args, "--", r.rel(s.Path))
	}
	out, err := r.run(ctx, nil, args...)
	return string(out), err
}

// Show returns a file's content at rev ("HEAD", a hash, or index stages
// ":0" ":1" ":2" ":3"). A missing file returns ErrNotFound.
func (r *Repo) Show(ctx context.Context, rev, path string) (string, error) {
	spec := rev + ":" + r.rel(path)
	if strings.HasPrefix(rev, ":") {
		spec = rev + ":" + r.rel(path)
	}
	out, err := r.run(ctx, nil, "show", spec)
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && (strings.Contains(ge.Stderr, "does not exist") || strings.Contains(ge.Stderr, "exists on disk, but not in") || strings.Contains(ge.Stderr, "invalid object name") || strings.Contains(ge.Stderr, "not in the index")) {
			return "", ErrNotFound
		}
		return "", err
	}
	return string(out), nil
}

// ErrNotFound means the file doesn't exist at that revision.
var ErrNotFound = errors.New("git: file not found at revision")

// ParseDiff splits unified diff text into files and hunks.
func ParseDiff(text string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	lines := strings.Split(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			flushHunk()
			if cur != nil {
				files = append(files, *cur)
			}
			cur = &FileDiff{Header: []string{l}, Hunks: []Hunk{}}
			a, b := splitGitPaths(strings.TrimPrefix(l, "diff --git "))
			cur.OldPath, cur.NewPath = a, b
		case cur != nil && hunk == nil && strings.HasPrefix(l, "--- "):
			cur.Header = append(cur.Header, l)
			if p := strings.TrimPrefix(l, "--- "); p != "/dev/null" {
				cur.OldPath = strings.TrimPrefix(p, "a/")
			}
		case cur != nil && hunk == nil && strings.HasPrefix(l, "+++ "):
			cur.Header = append(cur.Header, l)
			if p := strings.TrimPrefix(l, "+++ "); p != "/dev/null" {
				cur.NewPath = strings.TrimPrefix(p, "b/")
			}
		case cur != nil && strings.HasPrefix(l, "@@"):
			flushHunk()
			h, ok := parseHunkHeader(l)
			if ok {
				hunk = &h
			}
		case hunk != nil && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") || strings.HasPrefix(l, "\\") || l == ""):
			if l == "" {
				l = " " // some tools strip the space of empty context lines
			}
			hunk.Lines = append(hunk.Lines, l)
		case cur != nil && hunk == nil:
			cur.Header = append(cur.Header, l)
			if strings.HasPrefix(l, "Binary files ") {
				cur.Binary = true
			}
		}
	}
	flushHunk()
	if cur != nil {
		files = append(files, *cur)
	}
	if files == nil {
		files = []FileDiff{}
	}
	return files
}

func splitGitPaths(s string) (string, string) {
	// "a/x b/x" (paths without spaces in the common case).
	if i := strings.Index(s, " b/"); i >= 0 {
		return strings.TrimPrefix(s[:i], "a/"), s[i+3:]
	}
	return s, s
}

func parseHunkHeader(l string) (Hunk, bool) {
	// @@ -a,b +c,d @@ section
	end := strings.Index(l[2:], "@@")
	if end < 0 {
		return Hunk{}, false
	}
	spec := strings.Fields(l[2 : end+2])
	if len(spec) < 2 {
		return Hunk{}, false
	}
	os, ol := parseRange(strings.TrimPrefix(spec[0], "-"))
	ns, nl := parseRange(strings.TrimPrefix(spec[1], "+"))
	return Hunk{OldStart: os, OldLines: ol, NewStart: ns, NewLines: nl, Header: l, Lines: []string{}}, true
}

func parseRange(s string) (int, int) {
	a, b, ok := strings.Cut(s, ",")
	start, _ := strconv.Atoi(a)
	if !ok {
		return start, 1
	}
	n, _ := strconv.Atoi(b)
	return start, n
}

// HunkPatch builds a patch containing only the given hunks (indices into
// fd.Hunks), suitable for `git apply --recount`.
func HunkPatch(fd FileDiff, hunks []int) (string, error) {
	if fd.Binary {
		return "", errors.New("git: binary files can't be staged partially")
	}
	var b bytes.Buffer
	for _, h := range fd.Header {
		b.WriteString(h)
		b.WriteByte('\n')
	}
	n := 0
	for _, i := range hunks {
		if i < 0 || i >= len(fd.Hunks) {
			return "", fmt.Errorf("git: no hunk %d", i)
		}
		h := fd.Hunks[i]
		b.WriteString(h.Header)
		b.WriteByte('\n')
		for _, l := range h.Lines {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		n++
	}
	if n == 0 {
		return "", errors.New("git: no hunks selected")
	}
	return b.String(), nil
}

// ApplyPatch applies a patch to the index (cached) or work tree, optionally
// in reverse (unstage / revert).
func (r *Repo) ApplyPatch(ctx context.Context, patch string, cached, reverse bool) error {
	args := []string{"apply", "--recount", "--whitespace=nowarn"}
	if cached {
		args = append(args, "--cached")
	}
	if reverse {
		args = append(args, "--reverse")
	}
	_, err := r.run(ctx, strings.NewReader(patch), append(args, "-")...)
	return err
}
