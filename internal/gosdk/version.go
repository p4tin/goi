package gosdk

import (
	"strconv"
	"strings"
)

type parsedVersion struct {
	nums    [3]int
	preRank int // 0 beta, 1 rc, 2 release
	preNum  int
}

// parseVersion parses go1.26.0, go1.26, go1.26rc1, go1.21beta2. Unparseable
// parts are treated as zero.
func parseVersion(v string) parsedVersion {
	v = strings.TrimPrefix(v, "go")
	p := parsedVersion{preRank: 2}
	for _, pre := range []struct {
		tag  string
		rank int
	}{{"beta", 0}, {"rc", 1}} {
		if i := strings.Index(v, pre.tag); i >= 0 {
			p.preRank = pre.rank
			p.preNum, _ = strconv.Atoi(v[i+len(pre.tag):])
			v = v[:i]
			break
		}
	}
	for i, part := range strings.SplitN(v, ".", 3) {
		p.nums[i], _ = strconv.Atoi(part)
	}
	return p
}

// CompareVersions compares Go version strings, returning -1, 0 or 1.
// Pre-releases (beta < rc) sort below the release of the same version.
func CompareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := range pa.nums {
		if c := cmpInt(pa.nums[i], pb.nums[i]); c != 0 {
			return c
		}
	}
	if c := cmpInt(pa.preRank, pb.preRank); c != 0 {
		return c
	}
	return cmpInt(pa.preNum, pb.preNum)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
