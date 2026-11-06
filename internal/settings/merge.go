package settings

import (
	"math"
	"strings"
)

// merge deep-merges src into dst. Maps merge per key; any other value
// (including slices) replaces. Provenance is recorded per leaf dotted key.
func merge(dst, src map[string]any, layer Layer, prefix string, prov map[string]Layer) {
	for k, v := range src {
		key := prefix + k
		sm, sIsMap := v.(map[string]any)
		dm, dIsMap := dst[k].(map[string]any)
		if sIsMap && dIsMap {
			merge(dm, sm, layer, key+".", prov)
			if len(dm) > 0 {
				delete(prov, key) // children now carry provenance
			} else if _, ok := prov[key]; !ok {
				prov[key] = layer
			}
			continue
		}
		dst[k] = deepCopy(v)
		for p := range prov {
			if p == key || strings.HasPrefix(p, key+".") {
				delete(prov, p)
			}
		}
		recordLeaves(v, key, layer, prov)
	}
}

func recordLeaves(v any, key string, layer Layer, prov map[string]Layer) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		prov[key] = layer
		return
	}
	for k, c := range m {
		recordLeaves(c, key+"."+k, layer, prov)
	}
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, c := range t {
			m[k] = deepCopy(c)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, c := range t {
			s[i] = deepCopy(c)
		}
		return s
	default:
		return v
	}
}

// normalize converts values arriving from JSON (float64 numbers, typed
// slices/maps) into TOML-friendly generic forms.
func normalize(v any) any {
	switch t := v.(type) {
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t)
		}
		return t
	case float32:
		return normalize(float64(t))
	case int:
		return int64(t)
	case []string:
		s := make([]any, len(t))
		for i, c := range t {
			s[i] = c
		}
		return s
	case []any:
		s := make([]any, len(t))
		for i, c := range t {
			s[i] = normalize(c)
		}
		return s
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, c := range t {
			m[k] = c
		}
		return m
	case map[string][]string:
		m := make(map[string]any, len(t))
		for k, c := range t {
			m[k] = normalize(c)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, c := range t {
			m[k] = normalize(c)
		}
		return m
	default:
		return v
	}
}

// setPath sets (or, when value is nil, deletes) a dotted key in doc.
func setPath(doc map[string]any, parts []string, value any) {
	m := doc
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			if value == nil {
				return
			}
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	last := parts[len(parts)-1]
	if value == nil {
		delete(m, last)
		return
	}
	m[last] = value
}
