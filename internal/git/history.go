package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Branch is a local or remote branch.
type Branch struct {
	Name     string `json:"name"`
	Remote   bool   `json:"remote"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream"`
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	Date     string `json:"date"`
}

// Branches lists local and remote branches (remote HEAD symrefs skipped).
func (r *Repo) Branches(ctx context.Context) ([]Branch, error) {
	const f = "%(HEAD)%00%(refname)%00%(refname:short)%00%(upstream:short)%00%(objectname:short)%00%(subject)%00%(committerdate:iso-strict)"
	out, err := r.run(ctx, nil, "for-each-ref", "--format="+f, "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	branches := []Branch{}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		p := strings.Split(line, "\x00")
		if len(p) < 7 {
			continue
		}
		if strings.HasSuffix(p[1], "/HEAD") {
			continue
		}
		branches = append(branches, Branch{
			Current: p[0] == "*", Remote: strings.HasPrefix(p[1], "refs/remotes/"),
			Name: p[2], Upstream: p[3], Hash: p[4], Subject: p[5], Date: p[6],
		})
	}
	return branches, nil
}

// Checkout switches to ref. newBranch creates it from ref (or HEAD when
// ref is ""). A remote branch ("origin/x") checked out without newBranch
// creates the tracking branch "x".
func (r *Repo) Checkout(ctx context.Context, ref, newBranch string) error {
	switch {
	case newBranch != "" && ref != "":
		_, err := r.run(ctx, nil, "checkout", "-b", newBranch, ref)
		return err
	case newBranch != "":
		_, err := r.run(ctx, nil, "checkout", "-b", newBranch)
		return err
	}
	if remote, branch, ok := r.remoteBranch(ctx, ref); ok {
		if _, err := r.run(ctx, nil, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
			_, err := r.run(ctx, nil, "checkout", branch)
			return err
		}
		_, err := r.run(ctx, nil, "checkout", "--track", "-b", branch, remote+"/"+branch)
		return err
	}
	_, err := r.run(ctx, nil, "checkout", ref)
	return err
}

func (r *Repo) remoteBranch(ctx context.Context, ref string) (string, string, bool) {
	remote, branch, ok := strings.Cut(ref, "/")
	if !ok {
		return "", "", false
	}
	if _, err := r.run(ctx, nil, "rev-parse", "--verify", "-q", "refs/remotes/"+ref); err != nil {
		return "", "", false
	}
	return remote, branch, true
}

// RenameBranch renames a local branch.
func (r *Repo) RenameBranch(ctx context.Context, from, to string) error {
	_, err := r.run(ctx, nil, "branch", "-m", from, to)
	return err
}

// DeleteBranch deletes a local branch (force also deletes unmerged ones).
// ErrUnmerged reports an unmerged branch without force.
func (r *Repo) DeleteBranch(ctx context.Context, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := r.run(ctx, nil, "branch", flag, name)
	var ge *Error
	if errors.As(err, &ge) && strings.Contains(ge.Stderr, "not fully merged") {
		return ErrUnmerged
	}
	return err
}

// ErrUnmerged means a branch has commits not merged into HEAD.
var ErrUnmerged = errors.New("git: the branch is not fully merged")

// Commit is one log entry.
type Commit struct {
	Hash    string   `json:"hash"`
	Short   string   `json:"short"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Email   string   `json:"email"`
	Date    string   `json:"date"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Refs    string   `json:"refs"`
}

// LogQuery selects commits.
type LogQuery struct {
	Path string `json:"path"`
	Skip int    `json:"skip"`
	Max  int    `json:"max"`
	Grep string `json:"grep"`
	Ref  string `json:"ref"`
}

const (
	recSep   = "\x1e"
	fieldSep = "\x1f"
)

// Log lists commits, newest first.
func (r *Repo) Log(ctx context.Context, q LogQuery) ([]Commit, error) {
	if q.Max <= 0 {
		q.Max = 200
	}
	format := strings.Join([]string{"%H", "%h", "%P", "%an", "%ae", "%aI", "%s", "%b", "%D"}, fieldSep) + recSep
	args := []string{"log", "--format=" + format, "-n", strconv.Itoa(q.Max), "--skip", strconv.Itoa(q.Skip)}
	if q.Grep != "" {
		args = append(args, "-i", "--grep="+q.Grep)
	}
	if q.Ref != "" {
		args = append(args, q.Ref)
	}
	if q.Path != "" {
		args = append(args, "--follow", "--", r.rel(q.Path))
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && (strings.Contains(ge.Stderr, "does not have any commits") || strings.Contains(ge.Stderr, "bad default revision")) {
			return []Commit{}, nil
		}
		return nil, err
	}
	return ParseLog(string(out)), nil
}

// ParseLog parses Log's format.
func ParseLog(out string) []Commit {
	commits := []Commit{}
	for _, rec := range strings.Split(out, recSep) {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, fieldSep)
		if len(f) < 9 {
			continue
		}
		parents := strings.Fields(f[2])
		if parents == nil {
			parents = []string{}
		}
		commits = append(commits, Commit{Hash: f[0], Short: f[1], Parents: parents, Author: f[3], Email: f[4], Date: f[5], Subject: f[6], Body: strings.TrimSpace(f[7]), Refs: strings.TrimSpace(f[8])})
	}
	return commits
}

// ChangedFile is a file changed by a commit.
type ChangedFile struct {
	Path   string `json:"path"`
	Rel    string `json:"rel"`
	Status string `json:"status"`
	Orig   string `json:"orig"`
}

// CommitFiles lists the files a commit changed (against its first parent).
func (r *Repo) CommitFiles(ctx context.Context, hash string) ([]ChangedFile, error) {
	// -m --first-parent: merge commits list their changes against the
	// first parent (plain show gives merges an empty combined diff).
	out, err := r.run(ctx, nil, "show", "--format=", "--name-status", "-M", "-z", "-m", "--first-parent", hash)
	if err != nil {
		return nil, err
	}
	files := []ChangedFile{}
	parts := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	for i := 0; i < len(parts); i++ {
		st := strings.TrimSpace(parts[i])
		if st == "" {
			continue
		}
		if i+1 >= len(parts) {
			break
		}
		cf := ChangedFile{Status: st[:1]}
		if st[0] == 'R' || st[0] == 'C' {
			if i+2 >= len(parts) {
				break
			}
			cf.Orig = r.abs(parts[i+1])
			cf.Rel = parts[i+2]
			i += 2
		} else {
			cf.Rel = parts[i+1]
			i++
		}
		cf.Path = r.abs(cf.Rel)
		files = append(files, cf)
	}
	return files, nil
}

// BlameLine is the commit that last changed one line.
type BlameLine struct {
	Line    int    `json:"line"`
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Summary string `json:"summary"`
}

// Blame annotates path. contents (when non-nil) is blamed instead of the
// work tree file, so unsaved edits show as "Not Committed Yet".
func (r *Repo) Blame(ctx context.Context, path string, contents *string) ([]BlameLine, error) {
	args := []string{"blame", "--porcelain"}
	var stdin *strings.Reader
	if contents != nil {
		args = append(args, "--contents", "-")
		stdin = strings.NewReader(*contents)
	}
	args = append(args, "--", r.rel(path))
	var out []byte
	var err error
	if stdin != nil {
		out, err = r.run(ctx, stdin, args...)
	} else {
		out, err = r.run(ctx, nil, args...)
	}
	if err != nil {
		return nil, err
	}
	return ParseBlame(string(out)), nil
}

// ParseBlame parses `git blame --porcelain`.
func ParseBlame(out string) []BlameLine {
	type info struct{ author, date, summary string }
	infos := map[string]*info{}
	lines := []BlameLine{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var hash string
	var final int
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "\t") {
			in := infos[hash]
			bl := BlameLine{Line: final, Hash: hash}
			if in != nil {
				bl.Author, bl.Date, bl.Summary = in.author, in.date, in.summary
			}
			lines = append(lines, bl)
			continue
		}
		f := strings.Fields(l)
		if len(f) >= 3 && len(f[0]) == 40 && isHex(f[0]) {
			hash = f[0]
			final, _ = strconv.Atoi(f[2])
			if infos[hash] == nil {
				infos[hash] = &info{}
			}
			continue
		}
		in := infos[hash]
		if in == nil {
			continue
		}
		switch {
		case strings.HasPrefix(l, "author "):
			in.author = strings.TrimPrefix(l, "author ")
		case strings.HasPrefix(l, "author-time "):
			if sec, err := strconv.ParseInt(strings.TrimPrefix(l, "author-time "), 10, 64); err == nil {
				in.date = unixRFC3339(sec)
			}
		case strings.HasPrefix(l, "summary "):
			in.summary = strings.TrimPrefix(l, "summary ")
		}
	}
	return lines
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Stash is one stash entry.
type Stash struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

// StashPush stashes changes (untracked files too when asked).
func (r *Repo) StashPush(ctx context.Context, message string, untracked bool) error {
	args := []string{"stash", "push"}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := r.run(ctx, nil, args...)
	return err
}

// Stashes lists stash entries, newest first.
func (r *Repo) Stashes(ctx context.Context) ([]Stash, error) {
	out, err := r.run(ctx, nil, "stash", "list", "--format=%gd"+fieldSep+"%gs"+fieldSep+"%cI")
	if err != nil {
		return nil, err
	}
	list := []Stash{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, fieldSep)
		if len(f) < 3 {
			continue
		}
		var i int
		fmt.Sscanf(f[0], "stash@{%d}", &i)
		list = append(list, Stash{Index: i, Message: f[1], Date: f[2]})
	}
	return list, nil
}

// StashApply applies (pop removes) stash i.
func (r *Repo) StashApply(ctx context.Context, i int, pop bool) error {
	verb := "apply"
	if pop {
		verb = "pop"
	}
	_, err := r.run(ctx, nil, "stash", verb, fmt.Sprintf("stash@{%d}", i))
	return err
}

// StashDrop deletes stash i.
func (r *Repo) StashDrop(ctx context.Context, i int) error {
	_, err := r.run(ctx, nil, "stash", "drop", fmt.Sprintf("stash@{%d}", i))
	return err
}

// StashDiff is the diff a stash would apply.
func (r *Repo) StashDiff(ctx context.Context, i int) (string, error) {
	out, err := r.run(ctx, nil, "stash", "show", "-p", "--include-untracked", fmt.Sprintf("stash@{%d}", i))
	if err != nil {
		// Older git: no --include-untracked for show.
		out, err = r.run(ctx, nil, "stash", "show", "-p", fmt.Sprintf("stash@{%d}", i))
	}
	return string(out), err
}
