package index

import (
	"runtime"
	"strings"
	"sync"
)

// Scoring weights (fzf-style): every matched character earns scoreMatch plus
// a boundary bonus; runs of adjacent matches earn bonusConsecutive; gaps
// between matches cost gapStart then gapExtend per skipped byte.
const (
	scoreMatch       = 16
	bonusSegment     = 10 // after '/' or at the start
	bonusSeparator   = 8  // after '_', '-', '.', ' '
	bonusCamel       = 10 // lower→Upper transition (a hump counts as a word start)
	bonusConsecutive = 8
	bonusBasename    = 4 // per char matched inside the basename
	bonusBasePrefix  = 20
	gapStart         = 3
	gapExtend        = 1
	recentBoostMax   = 40
	negInf           = -1 << 30
)

type scratch struct {
	score []int
	from  []int
}

// Search returns up to limit files matching query, best first. Query
// whitespace separates terms that must all match. recent (absolute paths,
// most recent first) boosts recently used files; an empty query returns the
// recent files that are indexed.
func (ix *Index) Search(query string, limit int, recent []string) []Match {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	boost := make(map[string]int, len(recent))
	for i, p := range recent {
		if b := recentBoostMax - 2*i; b > 0 {
			if _, ok := boost[p]; !ok {
				boost[p] = b
			}
		}
	}

	terms := strings.Fields(asciiLower(query))
	if len(terms) == 0 {
		var out []Match
		for _, p := range recent {
			if i, ok := ix.pos[p]; ok && len(out) < limit {
				out = append(out, ix.match(ix.entries[i], boost[p], nil))
			}
		}
		return out
	}

	shards := runtime.GOMAXPROCS(0)
	if len(ix.entries) < 2048 {
		shards = 1
	}
	results := make([][]Match, shards)
	var wg sync.WaitGroup
	chunk := (len(ix.entries) + shards - 1) / shards
	for s := 0; s < shards; s++ {
		lo, hi := s*chunk, min((s+1)*chunk, len(ix.entries))
		if lo >= hi {
			continue
		}
		wg.Add(1)
		go func(s, lo, hi int) {
			defer wg.Done()
			var sc scratch
			var local []Match
			for _, e := range ix.entries[lo:hi] {
				total, ok := 0, true
				var pos []int
				for _, t := range terms {
					sv, p := scoreEntry(&sc, e, t)
					if sv == negInf {
						ok = false
						break
					}
					total += sv
					pos = append(pos, p...)
				}
				if !ok {
					continue
				}
				total += boost[e.abs] - strings.Count(e.rel, "/") - len(e.rel)/16
				local = append(local, ix.match(e, total, pos))
			}
			sortMatches(local)
			if len(local) > limit {
				local = local[:limit]
			}
			results[s] = local
		}(s, lo, hi)
	}
	wg.Wait()

	var all []Match
	for _, r := range results {
		all = append(all, r...)
	}
	sortMatches(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// scoreEntry scores the lowercased term against one entry, returning negInf
// when term isn't a subsequence of the path.
func scoreEntry(sc *scratch, e entry, q string) (int, []int) {
	text := e.lower
	n, m := len(text), len(q)
	if m == 0 || m > n || !isSubsequence(text, q) {
		return negInf, nil
	}
	if cap(sc.score) < n*m {
		sc.score = make([]int, n*m)
		sc.from = make([]int, n*m)
	}
	S, F := sc.score[:n*m], sc.from[:n*m]

	for i := 0; i < m; i++ {
		run, runFrom := negInf, -1 // best M[i-1][k] - gap(j-k-1) over k <= j-2
		for j := 0; j < n; j++ {
			if i > 0 && j >= 2 {
				if run != negInf {
					run -= gapExtend
				}
				if prev := S[(i-1)*n+j-2]; prev != negInf && prev-gapStart > run {
					run, runFrom = prev-gapStart, j-2
				}
			}
			idx := i*n + j
			if text[j] != q[i] {
				S[idx] = negInf
				continue
			}
			cs := scoreMatch + boundaryBonus(e.rel, j)
			if j >= e.baseStart {
				cs += bonusBasename
			}
			if i == 0 {
				S[idx], F[idx] = cs, -1
				continue
			}
			best, from := negInf, -1
			if j >= 1 {
				if prev := S[(i-1)*n+j-1]; prev != negInf {
					best, from = prev+bonusConsecutive, j-1
				}
			}
			if run != negInf && run > best {
				best, from = run, runFrom
			}
			if best == negInf {
				S[idx] = negInf
				continue
			}
			S[idx], F[idx] = best+cs, from
		}
	}

	best, bj := negInf, -1
	for j := 0; j < n; j++ {
		if v := S[(m-1)*n+j]; v > best {
			best, bj = v, j
		}
	}
	if best == negInf {
		return negInf, nil
	}
	pos := make([]int, m)
	for i, j := m-1, bj; i >= 0; i-- {
		pos[i] = j
		j = F[i*n+j]
	}
	if strings.HasPrefix(text[e.baseStart:], q) {
		best += bonusBasePrefix
	}
	return best, pos
}

func boundaryBonus(orig string, j int) int {
	if j == 0 {
		return bonusSegment
	}
	prev, cur := orig[j-1], orig[j]
	switch {
	case prev == '/':
		return bonusSegment
	case prev == '_' || prev == '-' || prev == '.' || prev == ' ':
		return bonusSeparator
	case 'a' <= prev && prev <= 'z' && 'A' <= cur && cur <= 'Z':
		return bonusCamel
	case !isAlnum(prev) && isAlnum(cur):
		return bonusSeparator
	}
	return 0
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

func isSubsequence(text, q string) bool {
	i := 0
	for j := 0; j < len(text) && i < len(q); j++ {
		if text[j] == q[i] {
			i++
		}
	}
	return i == len(q)
}
