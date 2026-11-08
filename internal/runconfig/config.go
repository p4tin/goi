// Package runconfig models run configurations (Go Build, Go Test, Go Remote,
// Shell script, Makefile), stores them as TOML files, validates them and
// turns them into concrete command plans and Delve launch arguments.
package runconfig

import (
	"strings"
)

// Type is a run configuration type.
type Type string

const (
	TypeApp    Type = "app"
	TypeTest   Type = "test"
	TypeRemote Type = "remote"
	TypeShell  Type = "shell"
	TypeMake   Type = "make"
)

// Kinds of Go targets.
const (
	KindPackage   = "package"
	KindFile      = "file"
	KindDirectory = "directory"
	KindFunction  = "function"
)

// Before-launch task types.
const (
	TaskConfig   = "config"
	TaskGenerate = "generate"
	TaskCommand  = "command"
)

// Task runs before a configuration launches.
type Task struct {
	Type    string `toml:"type" json:"type"`
	Config  string `toml:"config,omitempty" json:"config"`
	Package string `toml:"package,omitempty" json:"package"`
	Command string `toml:"command,omitempty" json:"command"`
}

// Config is one run configuration. Fields not used by its type are ignored.
type Config struct {
	Name string `toml:"name" json:"name"`
	Type Type   `toml:"type" json:"type"`
	// Shared configurations live in the project's .goi/runConfigurations.
	Shared bool `toml:"-" json:"shared"`
	// Temporary configurations were created from a gutter marker or context.
	Temporary bool `toml:"temporary,omitempty" json:"temporary"`
	// Created orders temporary configurations (Unix milliseconds).
	Created int64 `toml:"created,omitempty" json:"created"`

	// Go target.
	Kind      string   `toml:"kind,omitempty" json:"kind"`
	Package   string   `toml:"package,omitempty" json:"package"`
	Directory string   `toml:"directory,omitempty" json:"directory"`
	Files     []string `toml:"files,omitempty" json:"files"`
	// Packages lists several import paths for a function-kind test run
	// (reruns across packages); it overrides Package.
	Packages []string `toml:"packages,omitempty" json:"packages"`
	// Pattern is the -run regexp of a function-kind test configuration.
	Pattern string `toml:"pattern,omitempty" json:"pattern"`

	// Process.
	WorkingDir string            `toml:"workingDir,omitempty" json:"workingDir"`
	Args       string            `toml:"args,omitempty" json:"args"`
	Env        map[string]string `toml:"env,omitempty" json:"env"`
	EnvFile    string            `toml:"envFile,omitempty" json:"envFile"`

	// Build.
	BuildFlags    string   `toml:"buildFlags,omitempty" json:"buildFlags"`
	Tags          []string `toml:"tags,omitempty" json:"tags"`
	Output        string   `toml:"output,omitempty" json:"output"`
	RunAfterBuild *bool    `toml:"runAfterBuild,omitempty" json:"runAfterBuild"`
	InputFile     string   `toml:"inputFile,omitempty" json:"inputFile"`

	// Test.
	Race      bool   `toml:"race,omitempty" json:"race"`
	Short     bool   `toml:"short,omitempty" json:"short"`
	Count     int    `toml:"count,omitempty" json:"count"`
	Timeout   string `toml:"timeout,omitempty" json:"timeout"`
	TestFlags string `toml:"testFlags,omitempty" json:"testFlags"`
	Bench     string `toml:"bench,omitempty" json:"bench"`
	Benchmem  bool   `toml:"benchmem,omitempty" json:"benchmem"`
	Fuzz      string `toml:"fuzz,omitempty" json:"fuzz"`
	FuzzTime  string `toml:"fuzzTime,omitempty" json:"fuzzTime"`

	// Remote.
	Host string `toml:"host,omitempty" json:"host"`
	Port int    `toml:"port,omitempty" json:"port"`

	// Shell / Makefile.
	Script      string   `toml:"script,omitempty" json:"script"`
	ScriptText  string   `toml:"scriptText,omitempty" json:"scriptText"`
	Interpreter string   `toml:"interpreter,omitempty" json:"interpreter"`
	Makefile    string   `toml:"makefile,omitempty" json:"makefile"`
	Targets     []string `toml:"targets,omitempty" json:"targets"`

	BeforeLaunch []Task `toml:"beforeLaunch,omitempty" json:"beforeLaunch"`
}

// RunsAfterBuild reports whether an app configuration executes the binary.
func (c Config) RunsAfterBuild() bool { return c.RunAfterBuild == nil || *c.RunAfterBuild }

// Debuggable reports whether the configuration type can be debugged.
func (c Config) Debuggable() bool {
	return c.Type == TypeApp || c.Type == TypeTest || c.Type == TypeRemote
}

// Normalize fills nil slices and maps (bound DTOs must not carry nil) and
// trims names.
func (c Config) Normalize() Config {
	c.Name = strings.TrimSpace(c.Name)
	if c.Files == nil {
		c.Files = []string{}
	}
	if c.Packages == nil {
		c.Packages = []string{}
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	if c.Targets == nil {
		c.Targets = []string{}
	}
	if c.Env == nil {
		c.Env = map[string]string{}
	}
	if c.BeforeLaunch == nil {
		c.BeforeLaunch = []Task{}
	}
	return c
}

// key is the case-insensitive identity of a configuration name.
func key(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Slug maps a name to a file base name: letters, digits, '-' and '.' are
// kept, everything else becomes '_'.
func Slug(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), ".")
	if s == "" {
		s = "config"
	}
	return s
}
