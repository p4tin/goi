package runconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SplitArgs splits s into words like a POSIX shell does for quoting:
// whitespace separates words, single quotes are literal, double quotes allow
// \" \\ \$ escapes, and a backslash outside quotes escapes the next
// character. Nothing is expanded.
func SplitArgs(s string) ([]string, error) {
	out := []string{}
	var cur strings.Builder
	inWord := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\':
			if i+1 < len(rs) {
				i++
				cur.WriteRune(rs[i])
			}
			inWord = true
		case r == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated single quote")
			}
			cur.WriteString(string(rs[i+1 : j]))
			i = j
			inWord = true
		case r == '"':
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\"\\$`", rs[j+1]) {
					j++
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated double quote")
			}
			i = j
			inWord = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}

// LoadEnvFile parses a .env file: KEY=VALUE lines, blank lines and # comments
// ignored, an optional "export " prefix, values optionally single- or
// double-quoted (double quotes support \n, \t, \" and \\ escapes), and
// trailing " # comment" after unquoted values.
func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("env file: %w", err)
	}
	defer f.Close()
	return ParseEnv(bufio.NewScanner(f))
}

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

// ParseEnv parses .env lines from sc (see LoadEnvFile).
func ParseEnv(sc *bufio.Scanner) (map[string]string, error) {
	out := map[string]string{}
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || !envKey.MatchString(k) {
			return nil, fmt.Errorf("env file line %d: expected KEY=VALUE", n)
		}
		v = strings.TrimSpace(v)
		switch {
		case len(v) >= 2 && v[0] == '\'' && strings.LastIndexByte(v, '\'') > 0:
			v = v[1:strings.LastIndexByte(v, '\'')]
		case len(v) >= 2 && v[0] == '"' && strings.LastIndexByte(v, '"') > 0:
			v = unescapeDouble(v[1:strings.LastIndexByte(v, '"')])
		default:
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		out[k] = v
	}
	return out, sc.Err()
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// TestRunPattern builds a `go test -run` pattern selecting the given tests,
// whose names are as reported by test2json ("TestA", "TestA/sub_case").
// Each level becomes an anchored alternation of the quoted names at that
// level, joined by "/". Selecting by level can include extra combinations
// (TestA/x and TestB/y also select TestA/y), which go test tolerates; every
// requested test is included.
func TestRunPattern(names []string) string {
	var levels [][]string
	seen := []map[string]bool{}
	for _, n := range names {
		for i, part := range strings.Split(n, "/") {
			if i == len(levels) {
				levels = append(levels, nil)
				seen = append(seen, map[string]bool{})
			}
			if !seen[i][part] {
				seen[i][part] = true
				levels[i] = append(levels[i], regexp.QuoteMeta(part))
			}
		}
	}
	// A top-level test listed without subtests must still run fully: drop
	// deeper levels if any name stops early.
	minDepth := -1
	for _, n := range names {
		d := len(strings.Split(n, "/"))
		if minDepth < 0 || d < minDepth {
			minDepth = d
		}
	}
	parts := make([]string, 0, len(levels))
	for i, l := range levels {
		if minDepth >= 0 && i >= minDepth {
			break
		}
		if len(l) == 1 {
			parts = append(parts, "^"+l[0]+"$")
		} else {
			parts = append(parts, "^("+strings.Join(l, "|")+")$")
		}
	}
	return strings.Join(parts, "/")
}
