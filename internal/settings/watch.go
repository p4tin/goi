package settings

import "sync"

// FileWatcher is the subset of vfs.Watcher the store needs. It is declared
// here so settings does not depend on vfs.
type FileWatcher interface {
	WatchFile(path string, fn func()) (cancel func(), err error)
}

// Watch reloads the user and project layers whenever their files change on
// disk. When the project path changes (SetProjectPath), the project watch
// follows it. Reload errors are reported by Reload itself.
func (s *Store) Watch(w FileWatcher) (cancel func(), err error) {
	var cancelUser func()
	if p := s.Path(LayerUser); p != "" {
		cancelUser, err = w.WatchFile(p, func() { s.Reload(LayerUser) })
		if err != nil {
			return nil, err
		}
	}

	var mu sync.Mutex
	var projectPath string
	var cancelProject func()
	watchProject := func(p string) error {
		mu.Lock()
		defer mu.Unlock()
		if p == projectPath {
			return nil
		}
		if cancelProject != nil {
			cancelProject()
			cancelProject = nil
		}
		projectPath = p
		if p == "" {
			return nil
		}
		c, err := w.WatchFile(p, func() { s.Reload(LayerProject) })
		if err != nil {
			return err
		}
		cancelProject = c
		return nil
	}
	if err := watchProject(s.Path(LayerProject)); err != nil {
		if cancelUser != nil {
			cancelUser()
		}
		return nil, err
	}
	// SetProjectPath always notifies subscribers; follow the new path.
	unsub := s.Subscribe(func(Settings) { watchProject(s.Path(LayerProject)) })

	return func() {
		unsub()
		if cancelUser != nil {
			cancelUser()
		}
		watchProject("")
	}, nil
}
