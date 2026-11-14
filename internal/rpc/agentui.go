package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
)

// AgentRequestEvent is the payload of agent.request: work the backend needs
// the window's frontend to do (editor-aware file access, IDE tools, asking
// the user for permission). The frontend answers with AgentService.Respond.
type AgentRequestEvent struct {
	ID string `json:"id"`
	// Kind is fs.read, fs.write, permission or tool.
	Kind string `json:"kind"`
	// Session is the agent session key ("" for external MCP clients).
	Session string `json:"session"`
	Params  any    `json:"params"`
}

var errWindowClosed = errors.New("the window closed")

type uiReply struct {
	result json.RawMessage
	err    error
}

type uiPending struct {
	kind    string
	session string
	ch      chan uiReply
}

// uiBroker sends requests to a window's frontend and waits for the answers.
type uiBroker struct {
	bus func() events.Bus

	mu      sync.Mutex
	next    int
	pending map[string]*uiPending
	closed  bool
}

func newUIBroker(bus func() events.Bus) *uiBroker {
	return &uiBroker{bus: bus, pending: map[string]*uiPending{}}
}

// request emits an agent.request and waits for its answer, ctx, or timeout
// (0: no timeout).
func (b *uiBroker) request(ctx context.Context, kind, session string, params any, timeout time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errWindowClosed
	}
	b.next++
	id := fmt.Sprintf("r%d", b.next)
	p := &uiPending{kind: kind, session: session, ch: make(chan uiReply, 1)}
	b.pending[id] = p
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	b.bus().Emit(events.TopicAgentRequest, AgentRequestEvent{ID: id, Kind: kind, Session: session, Params: params})
	var expire <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		expire = t.C
	}
	select {
	case r := <-p.ch:
		return r.result, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-expire:
		return nil, fmt.Errorf("the IDE did not answer %s in %s", kind, timeout)
	}
}

// respond answers request id; false if it is no longer pending.
func (b *uiBroker) respond(id string, r uiReply) bool {
	b.mu.Lock()
	p := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if p == nil {
		return false
	}
	p.ch <- r
	return true
}

// cancelSession answers the session's pending permission requests as
// cancelled (its turn was stopped).
func (b *uiBroker) cancelSession(session string) {
	b.mu.Lock()
	var hit []*uiPending
	for id, p := range b.pending {
		if p.session == session && p.kind == "permission" {
			hit = append(hit, p)
			delete(b.pending, id)
		}
	}
	b.mu.Unlock()
	for _, p := range hit {
		p.ch <- uiReply{result: json.RawMessage(`{"outcome":"cancelled"}`)}
	}
}

// close fails every pending request; later requests fail at once.
func (b *uiBroker) close() {
	b.mu.Lock()
	b.closed = true
	pending := b.pending
	b.pending = map[string]*uiPending{}
	b.mu.Unlock()
	for _, p := range pending {
		p.ch <- uiReply{err: errWindowClosed}
	}
}
