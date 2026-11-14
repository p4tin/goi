package agent

import (
	"errors"
	"runtime"
	"strings"
)

// SplitCommandLine splits s into words like a POSIX shell without
// expansion: whitespace separates words, single quotes keep everything
// literally, double quotes allow \" and \\, and a backslash outside quotes
// escapes the next character. On Windows a backslash outside quotes is
// literal (paths), so only quotes group words.
func SplitCommandLine(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	escapes := runtime.GOOS != "windows"
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\'':
			inWord = true
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				cur.WriteRune(rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated single quote")
			}
			i = j
		case r == '"':
			inWord = true
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' && j+1 < len(rs) && (rs[j+1] == '"' || rs[j+1] == '\\') {
					j++
				}
				cur.WriteRune(rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated double quote")
			}
			i = j
		case r == '\\' && escapes:
			if i+1 >= len(rs) {
				return nil, errors.New("trailing backslash")
			}
			inWord = true
			i++
			cur.WriteRune(rs[i])
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			inWord = true
			cur.WriteRune(r)
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, errors.New("empty command line")
	}
	return out, nil
}

// JoinCommandLine quotes args for display or for typing into a shell.
func JoinCommandLine(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n\"'\\$`&|;<>()*?[]{}!#~") {
			parts[i] = a
			continue
		}
		if runtime.GOOS == "windows" {
			parts[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		} else {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}
