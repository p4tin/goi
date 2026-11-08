package events

import "sync"

// Topics published from the core to the UI.
const (
	TopicFSChanged        = "fs.changed"
	TopicWorkspaceChanged = "workspace.changed"
	TopicSettingsChanged  = "settings.changed"
	TopicRecentChanged    = "recent.changed"
	TopicIndexReady       = "index.ready"
	TopicIndexUpdated     = "index.updated"
	TopicNotify           = "notify"
	TopicMarkersUpdated   = "markers.updated"
	TopicLangStatus       = "lang.status"
	TopicLangDiagnostics  = "lang.diagnostics"
	TopicLangApplyEdit    = "lang.applyEdit"
	TopicRunConfigs       = "run.configs"
	TopicRunState         = "run.state"
	TopicRunOutput        = "run.output"
	TopicRunTest          = "run.test"
)

// Bus publishes a payload under a topic. Implementations must be safe for
// concurrent use and must not block the caller.
type Bus interface {
	Emit(topic string, payload any)
}

// Nop discards every event.
type Nop struct{}

// Emit implements Bus.
func (Nop) Emit(string, any) {}

// Event is one recorded emission.
type Event struct {
	Topic   string
	Payload any
}

// Recorder stores emitted events; intended for tests.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Emit implements Bus.
func (r *Recorder) Emit(topic string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Event{Topic: topic, Payload: payload})
}

// Events returns a copy of the events recorded so far, optionally filtered
// to a single topic when topic is non-empty.
func (r *Recorder) Events(topic string) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Event
	for _, e := range r.events {
		if topic == "" || e.Topic == topic {
			out = append(out, e)
		}
	}
	return out
}

// Notification is the payload of TopicNotify.
type Notification struct {
	ID    string `json:"id"`
	Level string `json:"level"` // info | warn | error
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
}
