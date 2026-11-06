package settings

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed defaults.toml
var defaultsTOML []byte

// Layer identifies where a setting value comes from.
type Layer string

// Layers in increasing priority.
const (
	LayerDefault Layer = "default"
	LayerUser    Layer = "user"
	LayerProject Layer = "project"
)

func (l Layer) priority() int {
	switch l {
	case LayerUser:
		return 1
	case LayerProject:
		return 2
	default:
		return 0
	}
}

// Startup holds application start behavior.
type Startup struct {
	ReopenLastProject bool `toml:"reopenLastProject" json:"reopenLastProject"`
}

// Editor holds editor appearance and limits.
type Editor struct {
	FontFamily         string `toml:"fontFamily" json:"fontFamily"`
	FontSize           int    `toml:"fontSize" json:"fontSize"`
	TabSize            int    `toml:"tabSize" json:"tabSize"`
	SoftWrap           bool   `toml:"softWrap" json:"softWrap"`
	MaxHighlightSizeKB int    `toml:"maxHighlightSizeKB" json:"maxHighlightSizeKB"`
	MaxOpenSizeKB      int    `toml:"maxOpenSizeKB" json:"maxOpenSizeKB"`
}

// Go holds the Go SDK and build environment.
type Go struct {
	GOROOT string            `toml:"goroot" json:"goroot"`
	Env    map[string]string `toml:"env" json:"env"`
	Tags   []string          `toml:"tags" json:"tags"`
	GOOS   string            `toml:"goos" json:"goos"`
	GOARCH string            `toml:"goarch" json:"goarch"`
	CGO    string            `toml:"cgo" json:"cgo"`
	Vendor string            `toml:"vendor" json:"vendor"`
}

// Project holds project layout settings.
type Project struct {
	ContentRoots []string `toml:"contentRoots" json:"contentRoots"`
	Excludes     []string `toml:"excludes" json:"excludes"`
}

// Save holds actions run on save.
type Save struct {
	Format string `toml:"format" json:"format"`
}

// UI holds theme and keymap selection.
type UI struct {
	Theme  string `toml:"theme" json:"theme"`
	Scheme string `toml:"scheme" json:"scheme"`
	Keymap string `toml:"keymap" json:"keymap"`
}

// Settings is the effective, merged configuration.
type Settings struct {
	Startup Startup             `toml:"startup" json:"startup"`
	Editor  Editor              `toml:"editor" json:"editor"`
	Go      Go                  `toml:"go" json:"go"`
	Project Project             `toml:"project" json:"project"`
	Save    Save                `toml:"save" json:"save"`
	UI      UI                  `toml:"ui" json:"ui"`
	Keys    map[string][]string `toml:"keys" json:"keys"`
}

// ChangedEvent is the payload of events.TopicSettingsChanged.
type ChangedEvent struct {
	Layer     Layer    `json:"layer"`
	Effective Settings `json:"effective"`
}

// Provider is read/write access to layered settings.
type Provider interface {
	Effective() Settings
	Origin(key string) Layer
	Set(layer Layer, key string, value any) error
	Subscribe(fn func(Settings)) (cancel func())
}

// decode converts a merged generic document into Settings.
func decode(doc map[string]any) (Settings, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return Settings{}, fmt.Errorf("settings: encode: %w", err)
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, fmt.Errorf("settings: invalid value: %w", err)
	}
	if s.Go.Env == nil {
		s.Go.Env = map[string]string{}
	}
	if s.Keys == nil {
		s.Keys = map[string][]string{}
	}
	return s, nil
}
