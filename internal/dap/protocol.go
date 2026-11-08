package dap

// Protocol types for the subset of DAP Goi uses. Lines and columns are
// 1-based (Goi initializes with linesStartAt1/columnsStartAt1).

// InitializeArguments are the client capabilities sent with initialize.
type InitializeArguments struct {
	ClientID                     string `json:"clientID"`
	ClientName                   string `json:"clientName"`
	AdapterID                    string `json:"adapterID"`
	Locale                       string `json:"locale,omitempty"`
	LinesStartAt1                bool   `json:"linesStartAt1"`
	ColumnsStartAt1              bool   `json:"columnsStartAt1"`
	PathFormat                   string `json:"pathFormat"`
	SupportsVariableType         bool   `json:"supportsVariableType"`
	SupportsVariablePaging       bool   `json:"supportsVariablePaging"`
	SupportsRunInTerminalRequest bool   `json:"supportsRunInTerminalRequest"`
	SupportsProgressReporting    bool   `json:"supportsProgressReporting"`
}

// ExceptionBreakpointsFilter is an exception filter the adapter offers.
type ExceptionBreakpointsFilter struct {
	Filter  string `json:"filter"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// Capabilities are the adapter's capabilities.
type Capabilities struct {
	SupportsConfigurationDoneRequest  bool                         `json:"supportsConfigurationDoneRequest"`
	SupportsFunctionBreakpoints       bool                         `json:"supportsFunctionBreakpoints"`
	SupportsConditionalBreakpoints    bool                         `json:"supportsConditionalBreakpoints"`
	SupportsHitConditionalBreakpoints bool                         `json:"supportsHitConditionalBreakpoints"`
	SupportsEvaluateForHovers         bool                         `json:"supportsEvaluateForHovers"`
	ExceptionBreakpointFilters        []ExceptionBreakpointsFilter `json:"exceptionBreakpointFilters"`
	SupportsSetVariable               bool                         `json:"supportsSetVariable"`
	SupportsRestartRequest            bool                         `json:"supportsRestartRequest"`
	SupportsGotoTargetsRequest        bool                         `json:"supportsGotoTargetsRequest"`
	SupportsLogPoints                 bool                         `json:"supportsLogPoints"`
	SupportsExceptionInfoRequest      bool                         `json:"supportsExceptionInfoRequest"`
	SupportsSteppingGranularity       bool                         `json:"supportsSteppingGranularity"`
	SupportsTerminateRequest          bool                         `json:"supportsTerminateRequest"`
	SupportsDelayedStackTraceLoading  bool                         `json:"supportsDelayedStackTraceLoading"`
}

// Source is a source file.
type Source struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	// SourceReference > 0 means the content must be fetched from the adapter.
	SourceReference int `json:"sourceReference,omitempty"`
}

// SourceBreakpoint is a requested line breakpoint.
type SourceBreakpoint struct {
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
	LogMessage   string `json:"logMessage,omitempty"`
}

// FunctionBreakpoint is a requested function breakpoint.
type FunctionBreakpoint struct {
	Name         string `json:"name"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
}

// Breakpoint is the adapter's view of a breakpoint.
type Breakpoint struct {
	ID       int     `json:"id"`
	Verified bool    `json:"verified"`
	Message  string  `json:"message"`
	Source   *Source `json:"source,omitempty"`
	Line     int     `json:"line"`
}

// SetBreakpointsArguments replaces the breakpoints of one source.
type SetBreakpointsArguments struct {
	Source      Source             `json:"source"`
	Breakpoints []SourceBreakpoint `json:"breakpoints"`
}

// BreakpointsResponse lists breakpoint results in request order.
type BreakpointsResponse struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// Thread is a thread (a goroutine, for Delve).
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// StackFrame is one frame.
type StackFrame struct {
	ID                          int     `json:"id"`
	Name                        string  `json:"name"`
	Source                      *Source `json:"source,omitempty"`
	Line                        int     `json:"line"`
	Column                      int     `json:"column"`
	PresentationHint            string  `json:"presentationHint,omitempty"`
	InstructionPointerReference string  `json:"instructionPointerReference,omitempty"`
}

// StackTraceResponse is a page of frames.
type StackTraceResponse struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames"`
}

// Scope is a variable scope of a frame.
type Scope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	Expensive          bool   `json:"expensive"`
}

// Variable is one variable; VariablesReference > 0 has children.
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type"`
	EvaluateName       string `json:"evaluateName"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables"`
	IndexedVariables   int    `json:"indexedVariables"`
}

// EvaluateResponse is an evaluation result.
type EvaluateResponse struct {
	Result             string `json:"result"`
	Type               string `json:"type"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables"`
	IndexedVariables   int    `json:"indexedVariables"`
}

// SetVariableResponse is the new value of a variable.
type SetVariableResponse struct {
	Value              string `json:"value"`
	Type               string `json:"type"`
	VariablesReference int    `json:"variablesReference"`
}

// StoppedEvent body.
type StoppedEvent struct {
	Reason            string `json:"reason"`
	Description       string `json:"description"`
	ThreadID          int    `json:"threadId"`
	Text              string `json:"text"`
	AllThreadsStopped bool   `json:"allThreadsStopped"`
	HitBreakpointIDs  []int  `json:"hitBreakpointIds"`
}

// OutputEvent body.
type OutputEvent struct {
	Category string `json:"category"`
	Output   string `json:"output"`
}

// ExitedEvent body.
type ExitedEvent struct {
	ExitCode int `json:"exitCode"`
}

// BreakpointEvent body.
type BreakpointEvent struct {
	Reason     string     `json:"reason"`
	Breakpoint Breakpoint `json:"breakpoint"`
}

// ExceptionInfoResponse describes the exception at a stop.
type ExceptionInfoResponse struct {
	ExceptionID string `json:"exceptionId"`
	Description string `json:"description"`
	BreakMode   string `json:"breakMode"`
	Details     *struct {
		Message    string `json:"message"`
		TypeName   string `json:"typeName"`
		StackTrace string `json:"stackTrace"`
	} `json:"details,omitempty"`
}
