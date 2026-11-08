package git

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// FileStatus is one changed file. Index and Work are porcelain status
// letters ('.' = unchanged, M, A, D, R, C, T, U).
type FileStatus struct {
	Path      string `json:"path"`
	Rel       string `json:"rel"`
	Orig      string `json:"orig"`
	Index     string `json:"index"`
	Work      string `json:"work"`
	Conflict  bool   `json:"conflict"`
	Untracked bool   `json:"untracked"`
	Ignored   bool   `json:"ignored"`
}

// Staged reports whether the index differs from HEAD.
func (f FileStatus) Staged() bool {
	return !f.Untracked && !f.Conflict && f.Index != "." && f.Index != ""
}

// Unstaged reports whether the work tree differs from the index.
func (f FileStatus) Unstaged() bool {
	return f.Untracked || (!f.Conflict && f.Work != "." && f.Work != "")
}

// Status is the repository status.
type Status struct {
	Root     string       `json:"root"`
	Branch   string       `json:"branch"`
	Head     string       `json:"head"`
	Upstream string       `json:"upstream"`
	Ahead    int          `json:"ahead"`
	Behind   int          `json:"behind"`
	Detached bool         `json:"detached"`
	Initial  bool         `json:"initial"`
	Op       Op           `json:"op"`
	Files    []FileStatus `json:"files"`
}

// Status runs git status (porcelain v2, NUL separated).
func (r *Repo) Status(ctx context.Context) (Status, error) {
	out, err := r.runEnv(ctx, nil, []string{"GIT_OPTIONAL_LOCKS=0"}, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	st := ParseStatus(out, r.abs)
	st.Root = r.Root
	st.Op = r.CurrentOp()
	return st, nil
}

// ParseStatus parses `git status --porcelain=v2 -z --branch`. abs maps a
// repo-relative path to an absolute one.
func ParseStatus(out []byte, abs func(string) string) Status {
	st := Status{Files: []FileStatus{}}
	recs := bytes.Split(out, []byte{0})
	for i := 0; i < len(recs); i++ {
		rec := string(recs[i])
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '#':
			f := strings.Fields(rec)
			if len(f) < 3 {
				continue
			}
			switch f[1] {
			case "branch.oid":
				if f[2] == "(initial)" {
					st.Initial = true
				} else {
					st.Head = f[2]
				}
			case "branch.head":
				if f[2] == "(detached)" {
					st.Detached = true
				} else {
					st.Branch = f[2]
				}
			case "branch.upstream":
				st.Upstream = f[2]
			case "branch.ab":
				if len(f) >= 4 {
					st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
					st.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
				}
			}
		case '1':
			// 1 XY sub mH mI mW hH hI path
			f := strings.SplitN(rec, " ", 9)
			if len(f) == 9 {
				st.Files = append(st.Files, fileStatus(f[1], f[8], "", abs))
			}
		case '2':
			// 2 XY sub mH mI mW hH hI Xscore path NUL orig
			f := strings.SplitN(rec, " ", 10)
			if len(f) == 10 {
				orig := ""
				if i+1 < len(recs) {
					i++
					orig = string(recs[i])
				}
				st.Files = append(st.Files, fileStatus(f[1], f[9], orig, abs))
			}
		case 'u':
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			f := strings.SplitN(rec, " ", 11)
			if len(f) == 11 {
				fs := fileStatus(f[1], f[10], "", abs)
				fs.Conflict = true
				st.Files = append(st.Files, fs)
			}
		case '?':
			p := rec[2:]
			st.Files = append(st.Files, FileStatus{Path: abs(p), Rel: p, Index: "?", Work: "?", Untracked: true})
		case '!':
			p := rec[2:]
			st.Files = append(st.Files, FileStatus{Path: abs(p), Rel: p, Index: "!", Work: "!", Ignored: true})
		}
	}
	return st
}

func fileStatus(xy, rel, orig string, abs func(string) string) FileStatus {
	fs := FileStatus{Path: abs(rel), Rel: rel, Index: xy[:1], Work: xy[1:2]}
	if orig != "" {
		fs.Orig = abs(orig)
	}
	return fs
}
