package rpc

import (
	"fmt"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/recent"
	"github.com/p4tin/goi/internal/settings"
)

// SettingsService reads and writes layered settings.
type SettingsService struct {
	app *App
}

// Effective returns the merged settings.
func (s *SettingsService) Effective() settings.Settings {
	return s.app.settings.Effective()
}

// Origins maps each dotted key to the layer ("default", "user" or
// "project") its value comes from.
func (s *SettingsService) Origins() map[string]string {
	out := map[string]string{}
	for k, l := range s.app.settings.Origins() {
		out[k] = string(l)
	}
	return out
}

// Set writes key in layer ("user" or "project"). A null value removes it.
func (s *SettingsService) Set(layer string, key string, value any) error {
	l := settings.Layer(layer)
	if l != settings.LayerUser && l != settings.LayerProject {
		return fmt.Errorf("settings: cannot write layer %q", layer)
	}
	return s.app.settings.Set(l, key, value)
}

// SettingsPaths are the files behind each writable layer.
type SettingsPaths struct {
	User    string `json:"user"`
	Project string `json:"project"`
}

// Paths returns the user and project settings files.
func (s *SettingsService) Paths() SettingsPaths {
	return SettingsPaths{
		User:    s.app.settings.Path(settings.LayerUser),
		Project: s.app.settings.Path(settings.LayerProject),
	}
}

// RecentService lists recent projects.
type RecentService struct {
	app *App
}

// List returns recent projects, most recent first.
func (s *RecentService) List() ([]recent.Entry, error) {
	list, err := s.app.recent.List()
	if list == nil {
		list = []recent.Entry{}
	}
	return list, err
}

// Remove forgets a recent project.
func (s *RecentService) Remove(path string) error {
	if err := s.app.recent.Remove(path); err != nil {
		return err
	}
	if list, err := s.app.recent.List(); err == nil {
		s.app.bus.Emit(events.TopicRecentChanged, list)
	}
	return nil
}
