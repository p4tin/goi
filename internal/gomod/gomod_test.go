package gomod

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestProxyURLs(t *testing.T) {
	cases := map[string][]string{
		"":                          {"https://proxy.golang.org"},
		"https://a.example/,direct": {"https://a.example"},
		"https://a|https://b":       {"https://a", "https://b"},
		"off":                       nil,
		"direct":                    nil,
		"https://a,off,https://b":   {"https://a"},
	}
	for in, want := range cases {
		if got := ProxyURLs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("ProxyURLs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVersionsFromProxy(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/github.com/!burnt!sushi/toml/@v/list":
			w.Write([]byte("v1.2.0\nv1.10.0\nv0.3.1\nv1.3.0-0.20240101000000-abcdefabcdef\nnot-a-version\nv1.2.0\n"))
		case "/example.com/empty/@v/list":
			w.Write([]byte(""))
		case "/example.com/empty/@latest":
			w.Write([]byte(`{"Version":"v0.0.0-20240101000000-abcdefabcdef"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := NewProxy(srv.URL + ",direct")
	ctx := context.Background()
	vs, err := p.Versions(ctx, "github.com/BurntSushi/toml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(vs, []string{"v1.10.0", "v1.2.0", "v0.3.1"}) {
		t.Fatalf("versions = %v", vs)
	}
	p.Versions(ctx, "github.com/BurntSushi/toml")
	if hits.Load() != 1 {
		t.Fatalf("cache miss: %d requests", hits.Load())
	}
	if vs, err := p.Versions(ctx, "example.com/empty"); err != nil || len(vs) != 1 {
		t.Fatalf("latest fallback = %v %v", vs, err)
	}
	if _, err := p.Versions(ctx, "example.com/missing"); err == nil {
		t.Fatal("404 should fail")
	}
	if _, err := NewProxy("off").Versions(ctx, "x"); err != ErrNoProxy {
		t.Fatalf("off = %v", err)
	}
}

func TestKnownModules(t *testing.T) {
	mc := t.TempDir()
	for _, d := range []string{"cache/download/github.com/!burnt!sushi/toml/@v", "cache/download/golang.org/x/mod/@v", "cache/download/sumdb/sum.golang.org/lookup"} {
		os.MkdirAll(filepath.Join(mc, filepath.FromSlash(d)), 0o755)
	}
	sum := filepath.Join(t.TempDir(), "go.sum")
	os.WriteFile(sum, []byte("github.com/stretchr/testify v1.9.0 h1:abc=\ngithub.com/stretchr/testify v1.9.0/go.mod h1:def=\n"), 0o644)
	got := KnownModules(context.Background(), mc, []string{sum, "/nope/go.sum"})
	want := []string{"github.com/BurntSushi/toml", "github.com/stretchr/testify", "golang.org/x/mod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownModules = %v", got)
	}
}
