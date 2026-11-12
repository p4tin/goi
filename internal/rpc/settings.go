package rpc

import (
	"context"
	"fmt"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/recent"
	"github.com/p4tin/goi/internal/settings"
)

// SettingsService reads and writes layered settings.
type SettingsService struct {
	app *App
}

// Effective returns the calling window's merged settings.
func (s *SettingsService) Effective(ctx context.Context) (settings.Settings, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return settings.Settings{}, err
	}
	return w.settings.Effective(), nil
}

// Origins maps each dotted key to the layer ("default", "user" or
// "project") its value comes from.
func (s *SettingsService) Origins(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return out, err
	}
	for k, l := range w.settings.Origins() {
		out[k] = string(l)
	}
	return out, nil
}

// Set writes key in layer ("user" or "project") of the calling window's
// settings. A null value removes it.
func (s *SettingsService) Set(ctx context.Context, layer string, key string, value any) error {
	l := settings.Layer(layer)
	if l != settings.LayerUser && l != settings.LayerProject {
		return fmt.Errorf("settings: cannot write layer %q", layer)
	}
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return err
	}
	return w.settings.Set(l, key, value)
}

// SettingsPaths are the files behind each writable layer.
type SettingsPaths struct {
	User    string `json:"user"`
	Project string `json:"project"`
}

// Paths returns the calling window's user and project settings files.
func (s *SettingsService) Paths(ctx context.Context) (SettingsPaths, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return SettingsPaths{}, err
	}
	return SettingsPaths{
		User:    w.settings.Path(settings.LayerUser),
		Project: w.settings.Path(settings.LayerProject),
	}, nil
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
		s.app.appBus.Emit(events.TopicRecentChanged, list)
	}
	return nil
}
