package runconfig

// TypeHandler plans and validates one configuration type an app pack
// contributes (Goi: "app", "test", "remote").
type TypeHandler interface {
	// Plan turns c into commands.
	Plan(c Config, ctx Context) (Plan, error)
	// Validate returns problems with c's type-specific fields (Validate
	// already checks the name, common paths, arguments and tasks).
	Validate(c Config, ctx Context) []string
	// Debuggable reports whether c can run under the debugger.
	Debuggable(c Config) bool
	// Runnable reports whether c can run without the debugger (Goi's Go
	// Remote attaches only).
	Runnable(c Config) bool
}

// TaskHandler turns a pack's before-launch task (Goi: "generate") into a
// command.
type TaskHandler func(t Task, ctx Context) (*Cmd, error)

// Types are a pack's configuration types and before-launch tasks.
type Types map[Type]TypeHandler

// Debuggable reports whether c's type supports the debugger.
func (ts Types) Debuggable(c Config) bool {
	h := ts[c.Type]
	return h != nil && h.Debuggable(c)
}

// Runnable reports whether c can run without the debugger. Built-in types
// always can.
func (ts Types) Runnable(c Config) bool {
	if c.Type == TypeShell || c.Type == TypeMake {
		return true
	}
	h := ts[c.Type]
	return h != nil && h.Runnable(c)
}
