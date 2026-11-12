package rpc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/gomod"
)

// modSession caches the proxy client and known modules for completion.
type modSession struct {
	mu      sync.Mutex
	proxy   *gomod.Proxy
	known   []string
	knownAt time.Time
}

// ModService supports editing go.mod / go.work files.
type ModService struct {
	app *App
}

func (s *ModService) session(ctx context.Context) (*session, *modSession, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, nil, err
	}
	if sess.mod == nil {
		return nil, nil, errNoProject
	}
	return sess, sess.mod, nil
}

func (s *ModService) goEnv(ctx context.Context, w *window) map[string]string {
	if r, err := w.runner(ctx); err == nil {
		if ge, err := r.GoEnv(ctx); err == nil {
			return ge
		}
	}
	return map[string]string{"GOPROXY": os.Getenv("GOPROXY"), "GOMODCACHE": ""}
}

// Versions lists released versions of a module from the GOPROXY, newest
// first.
func (s *ModService) Versions(ctx context.Context, module string) ([]string, error) {
	sess, ms, err := s.session(ctx)
	if err != nil {
		return []string{}, err
	}
	vctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ms.mu.Lock()
	p := ms.proxy
	ms.mu.Unlock()
	if p == nil {
		p = gomod.NewProxy(s.goEnv(vctx, sess.win)["GOPROXY"])
		ms.mu.Lock()
		ms.proxy = p
		ms.mu.Unlock()
	}
	vs, err := p.Versions(vctx, strings.TrimSpace(module))
	if err != nil {
		return []string{}, err
	}
	return vs, nil
}

// Modules lists module paths known locally (module cache, go.sum files in
// the project) that start with prefix.
func (s *ModService) Modules(ctx context.Context, prefix string) []string {
	sess, ms, err := s.session(ctx)
	if err != nil {
		return []string{}
	}
	ms.mu.Lock()
	known, fresh := ms.known, time.Since(ms.knownAt) < 5*time.Minute
	ms.mu.Unlock()
	if !fresh {
		kctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ge := s.goEnv(kctx, sess.win)
		var sums []string
		for _, m := range sess.manager.Project().Modules {
			sums = append(sums, filepath.Join(m.Dir, "go.sum"))
		}
		sums = append(sums, filepath.Join(sess.root, "go.work.sum"))
		known = gomod.KnownModules(kctx, ge["GOMODCACHE"], sums)
		cancel()
		ms.mu.Lock()
		ms.known, ms.knownAt = known, time.Now()
		ms.mu.Unlock()
	}
	out := []string{}
	for _, m := range known {
		if strings.HasPrefix(m, prefix) {
			out = append(out, m)
			if len(out) >= 500 {
				break
			}
		}
	}
	return out
}
