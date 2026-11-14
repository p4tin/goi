package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/acp"
)

// TestRealAgent checks protocol compatibility with a real agent:
//
//	GOI_REAL_AGENT="gemini --acp" go test -run RealAgent -v ./internal/agent
//
// It initializes and opens a session (no model call). With
// GOI_REAL_AGENT_PROMPT=1 it also sends one short prompt.
func TestRealAgent(t *testing.T) {
	line := os.Getenv("GOI_REAL_AGENT")
	if line == "" {
		t.Skip("set GOI_REAL_AGENT to an agent command line")
	}
	cmd, err := Resolve(Definition{ID: "real"}, line, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	h := &fakeHost{files: map[string]string{}, dir: dir}
	c, err := Start(Options{Command: cmd, Dir: dir, Env: os.Environ(), Host: h})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	info, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v\nstderr: %s", err, c.StderrTail())
	}
	name := "?"
	if info.AgentInfo != nil {
		name = info.AgentInfo.Name + " " + info.AgentInfo.Version
	}
	t.Logf("agent %s: caps %+v, %d auth methods", name, info.AgentCapabilities, len(info.AuthMethods))
	for _, m := range info.AuthMethods {
		t.Logf("  auth %s (%s) %q", m.ID, m.Type, m.Name)
	}
	ns, err := c.NewSession(ctx, dir, nil)
	if acp.IsAuthRequired(err) {
		t.Skipf("agent needs a login first: %v", err)
	}
	if err != nil {
		t.Fatalf("session/new: %v\nstderr: %s", err, c.StderrTail())
	}
	t.Logf("session %s, modes %+v", ns.SessionID, ns.Modes)
	if os.Getenv("GOI_REAL_AGENT_PROMPT") != "1" {
		return
	}
	stop, err := c.Prompt(ctx, ns.SessionID, []acp.ContentBlock{acp.TextBlock("Reply with the single word PONG and nothing else. Do not use tools.")})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	out := h.text()
	t.Logf("stop %s, reply %q", stop, out)
	if stop != acp.StopEndTurn || !strings.Contains(strings.ToUpper(out), "PONG") {
		t.Errorf("unexpected turn: %s %q", stop, out)
	}
}
