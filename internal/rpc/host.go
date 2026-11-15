package rpc

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/workspace"
)

// projectEnv describes the window's project to pack capabilities.
func (w *window) projectEnv() pack.ProjectEnv {
	p := pack.ProjectEnv{Settings: w.settings.Effective()}
	if sess, err := w.current(); err == nil {
		p.Root = sess.root
	}
	return p
}

// toolEnv is the environment of programs started for the window's project:
// the pack toolchain's, else the app's own.
func (w *window) toolEnv(ctx context.Context) []string {
	if tc := w.app.pack.Toolchain; tc != nil {
		if env, err := tc.Env(ctx, w.projectEnv()); err == nil {
			return env
		}
	}
	return os.Environ()
}

// toolEnvTimeout bounds resolving the toolchain environment.
const toolEnvTimeout = 20 * time.Second

// host is the pack.Host of an App.
type host struct{ a *App }

var _ pack.Host = host{}

func (h host) Session(ctx context.Context) (pack.Session, error) {
	s, err := h.a.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	return packSession{s}, nil
}

func (h host) Settings(ctx context.Context) (settings.Settings, error) {
	w, err := h.a.windowFor(ctx)
	if err != nil {
		return settings.Settings{}, err
	}
	return w.settings.Effective(), nil
}

func (h host) SetSetting(ctx context.Context, key string, value any) error {
	w, err := h.a.windowFor(ctx)
	if err != nil {
		return err
	}
	return w.settings.Set(w.settingsLayerForProject(), key, value)
}

func (h host) Env(ctx context.Context) []string {
	w, err := h.a.windowFor(ctx)
	if err != nil {
		return os.Environ()
	}
	ectx, cancel := context.WithTimeout(context.Background(), toolEnvTimeout)
	defer cancel()
	return w.toolEnv(ectx)
}

func (h host) Background() context.Context { return h.a.ctx }
func (h host) Log() *slog.Logger           { return h.a.log }
func (h host) ToolsDir() string            { return h.a.toolsDir() }

// packSession is a session as pack services see it.
type packSession struct{ s *session }

func (p packSession) Root() string                { return p.s.root }
func (p packSession) Project() workspace.Project  { return p.s.manager.Project() }
func (p packSession) Settings() settings.Settings { return p.s.win.settings.Effective() }
func (p packSession) Env(ctx context.Context) []string {
	return p.s.win.toolEnv(ctx)
}

func (p packSession) State(key string, create func() any) any {
	p.s.stateMu.Lock()
	defer p.s.stateMu.Unlock()
	if p.s.state == nil {
		p.s.state = map[string]any{}
	}
	v, ok := p.s.state[key]
	if !ok {
		v = create()
		p.s.state[key] = v
	}
	return v
}

// stateHolder is embedded in session for pack service state.
type stateHolder struct {
	stateMu sync.Mutex
	state   map[string]any
}
