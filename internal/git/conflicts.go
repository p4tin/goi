package git

import (
	"strings"
	"time"
)

func unixRFC3339(sec int64) string { return time.Unix(sec, 0).UTC().Format(time.RFC3339) }

// Block is one conflict in a file with conflict markers. Line numbers are
// 0-based indices of the marker lines; Base is -1 without a diff3 base.
type Block struct {
	Start       int    `json:"start"`
	Base        int    `json:"base"`
	Mid         int    `json:"mid"`
	End         int    `json:"end"`
	OursLabel   string `json:"oursLabel"`
	TheirsLabel string `json:"theirsLabel"`
}

// Conflicts finds conflict blocks (<<<<<<< / ||||||| / ======= / >>>>>>>).
// Unterminated blocks are ignored.
func Conflicts(text string) []Block {
	lines := strings.Split(text, "\n")
	blocks := []Block{}
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "<<<<<<<") {
			continue
		}
		b := Block{Start: i, Base: -1, Mid: -1, End: -1, OursLabel: strings.TrimSpace(strings.TrimPrefix(lines[i], "<<<<<<<"))}
		j := i + 1
		for ; j < len(lines); j++ {
			l := lines[j]
			switch {
			case strings.HasPrefix(l, "|||||||") && b.Mid < 0 && b.Base < 0:
				b.Base = j
			case strings.HasPrefix(l, "=======") && b.Mid < 0:
				b.Mid = j
			case strings.HasPrefix(l, ">>>>>>>") && b.Mid >= 0:
				b.End = j
				b.TheirsLabel = strings.TrimSpace(strings.TrimPrefix(l, ">>>>>>>"))
			case strings.HasPrefix(l, "<<<<<<<"):
				j = len(lines) // nested start: treat as unterminated
			}
			if b.End >= 0 || j >= len(lines) {
				break
			}
		}
		if b.End >= 0 {
			blocks = append(blocks, b)
			i = b.End
		}
	}
	return blocks
}

// Resolve replaces conflict block k of text with "ours", "theirs" or
// "both" (ours then theirs).
func Resolve(text string, b Block, side string) string {
	lines := strings.Split(text, "\n")
	oursEnd := b.Mid
	if b.Base >= 0 {
		oursEnd = b.Base
	}
	ours := lines[b.Start+1 : oursEnd]
	theirs := lines[b.Mid+1 : b.End]
	var repl []string
	switch side {
	case "ours":
		repl = ours
	case "theirs":
		repl = theirs
	default:
		repl = append(append([]string{}, ours...), theirs...)
	}
	out := append(append(append([]string{}, lines[:b.Start]...), repl...), lines[b.End+1:]...)
	return strings.Join(out, "\n")
}
