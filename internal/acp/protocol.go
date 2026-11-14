package acp

import (
	"encoding/json"
)

// ProtocolVersion is the ACP major version Goi speaks.
const ProtocolVersion = 1

// Methods the client calls on the agent.
const (
	MethodInitialize   = "initialize"
	MethodAuthenticate = "authenticate"
	MethodSessionNew   = "session/new"
	MethodPrompt       = "session/prompt"
	MethodCancel       = "session/cancel" // notification
	MethodSetMode      = "session/set_mode"
	MethodSessionClose = "session/close"
)

// Methods the agent calls on the client.
const (
	MethodSessionUpdate     = "session/update" // notification
	MethodRequestPermission = "session/request_permission"
	MethodReadTextFile      = "fs/read_text_file"
	MethodWriteTextFile     = "fs/write_text_file"
	MethodTerminalCreate    = "terminal/create"
	MethodTerminalOutput    = "terminal/output"
	MethodTerminalWait      = "terminal/wait_for_exit"
	MethodTerminalKill      = "terminal/kill"
	MethodTerminalRelease   = "terminal/release"
)

// Implementation names a client or agent program.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// FileSystemCapabilities says which fs/* methods the client implements.
type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// ClientCapabilities is what Goi offers the agent.
type ClientCapabilities struct {
	FS       FileSystemCapabilities `json:"fs"`
	Terminal bool                   `json:"terminal"`
}

// InitializeParams is the initialize request.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
}

// PromptCapabilities says which content blocks a prompt may carry besides
// text and resource links.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// MCPCapabilities says which MCP transports the agent can connect to besides
// stdio.
type MCPCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// AgentCapabilities is what the agent supports.
type AgentCapabilities struct {
	LoadSession        bool               `json:"loadSession,omitempty"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	MCPCapabilities    MCPCapabilities    `json:"mcpCapabilities"`
}

// AuthMethod is one way to log in. Type "terminal" means the client runs the
// agent program itself with Args and Env; otherwise ("agent", the default)
// the client calls authenticate with ID.
type AuthMethod struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// InitializeResult is the agent's answer to initialize.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
	AgentInfo         *Implementation   `json:"agentInfo,omitempty"`
}

// AuthenticateParams is the authenticate request.
type AuthenticateParams struct {
	MethodID string `json:"methodId"`
}

// NameValue is an environment variable or HTTP header.
type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MCPServer is an MCP server the agent should connect to: over HTTP when URL
// is set, otherwise by running Command.
type MCPServer struct {
	Name    string
	URL     string
	Headers []NameValue
	Command string
	Args    []string
	Env     []NameValue
}

// MarshalJSON writes the http or stdio variant.
func (s MCPServer) MarshalJSON() ([]byte, error) {
	if s.URL != "" {
		h := s.Headers
		if h == nil {
			h = []NameValue{}
		}
		return json.Marshal(struct {
			Type    string      `json:"type"`
			Name    string      `json:"name"`
			URL     string      `json:"url"`
			Headers []NameValue `json:"headers"`
		}{"http", s.Name, s.URL, h})
	}
	args, env := s.Args, s.Env
	if args == nil {
		args = []string{}
	}
	if env == nil {
		env = []NameValue{}
	}
	return json.Marshal(struct {
		Name    string      `json:"name"`
		Command string      `json:"command"`
		Args    []string    `json:"args"`
		Env     []NameValue `json:"env"`
	}{s.Name, s.Command, args, env})
}

// UnmarshalJSON reads either variant.
func (s *MCPServer) UnmarshalJSON(b []byte) error {
	var v struct {
		Type    string      `json:"type"`
		Name    string      `json:"name"`
		URL     string      `json:"url"`
		Headers []NameValue `json:"headers"`
		Command string      `json:"command"`
		Args    []string    `json:"args"`
		Env     []NameValue `json:"env"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = MCPServer{Name: v.Name, URL: v.URL, Headers: v.Headers, Command: v.Command, Args: v.Args, Env: v.Env}
	return nil
}

// NewSessionParams is the session/new request.
type NewSessionParams struct {
	Cwd        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// SessionMode is one of the agent's operating modes.
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionModeState lists the modes and the current one.
type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

// NewSessionResult is the agent's answer to session/new.
type NewSessionResult struct {
	SessionID string            `json:"sessionId"`
	Modes     *SessionModeState `json:"modes,omitempty"`
}

// EmbeddedResource is the resource of a "resource" content block.
type EmbeddedResource struct {
	URI      string `json:"uri"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType,omitempty"`
}

// ContentBlock is a prompt or message part: text, resource_link or resource
// (the types Goi sends).
type ContentBlock struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	URI      string            `json:"uri,omitempty"`
	Name     string            `json:"name,omitempty"`
	MimeType string            `json:"mimeType,omitempty"`
	Resource *EmbeddedResource `json:"resource,omitempty"`
}

// TextBlock is a text content block.
func TextBlock(s string) ContentBlock { return ContentBlock{Type: "text", Text: s} }

// PromptParams is the session/prompt request.
type PromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// Stop reasons of a prompt turn.
const (
	StopEndTurn         = "end_turn"
	StopMaxTokens       = "max_tokens"
	StopMaxTurnRequests = "max_turn_requests"
	StopRefusal         = "refusal"
	StopCancelled       = "cancelled"
)

// PromptResult is the agent's answer to session/prompt.
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// SessionParams carries just a session id (session/cancel, session/close).
type SessionParams struct {
	SessionID string `json:"sessionId"`
}

// SetModeParams is the session/set_mode request.
type SetModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

// SessionNotification is a session/update notification. Update stays raw:
// Goi forwards it to the UI.
type SessionNotification struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

// PermissionOption is one choice offered with a permission request. Kind is
// allow_once, allow_always, reject_once or reject_always.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionParams is the session/request_permission request.
type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  json.RawMessage    `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionOutcome is the user's answer: Outcome is "selected" (with
// OptionID) or "cancelled".
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// RequestPermissionResult wraps the outcome.
type RequestPermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// ReadTextFileParams is the fs/read_text_file request. Line is 1-based.
type ReadTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      *int   `json:"line,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
}

// ReadTextFileResult is the answer to fs/read_text_file.
type ReadTextFileResult struct {
	Content string `json:"content"`
}

// WriteTextFileParams is the fs/write_text_file request.
type WriteTextFileParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// CreateTerminalParams is the terminal/create request.
type CreateTerminalParams struct {
	SessionID       string      `json:"sessionId"`
	Command         string      `json:"command"`
	Args            []string    `json:"args,omitempty"`
	Env             []NameValue `json:"env,omitempty"`
	Cwd             string      `json:"cwd,omitempty"`
	OutputByteLimit *int64      `json:"outputByteLimit,omitempty"`
}

// CreateTerminalResult is the answer to terminal/create.
type CreateTerminalResult struct {
	TerminalID string `json:"terminalId"`
}

// TerminalParams names a terminal (output, wait_for_exit, kill, release).
type TerminalParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// ExitStatus is how a terminal command ended.
type ExitStatus struct {
	ExitCode *int   `json:"exitCode"`
	Signal   string `json:"signal,omitempty"`
}

// TerminalOutputResult is the answer to terminal/output.
type TerminalOutputResult struct {
	Output     string      `json:"output"`
	Truncated  bool        `json:"truncated"`
	ExitStatus *ExitStatus `json:"exitStatus,omitempty"`
}
