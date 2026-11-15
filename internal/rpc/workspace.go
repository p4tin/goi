package rpc

import (
	"context"

	"github.com/p4tin/goi/internal/workspace"
)

// WorkspaceService exposes the open project model.
type WorkspaceService struct {
	app *App
}

// Project returns the open project, or nil on the Welcome screen.
func (s *WorkspaceService) Project(ctx context.Context) *workspace.Project {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil
	}
	p := sess.manager.Project()
	return &p
}

// AddContentRoot adds an extra top-level folder to the project.
func (s *WorkspaceService) AddContentRoot(ctx context.Context, path string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.AddContentRoot(path)
}

// RemoveContentRoot removes an extra top-level folder.
func (s *WorkspaceService) RemoveContentRoot(ctx context.Context, path string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.RemoveContentRoot(path)
}

// SetExcludes replaces the project's exclude globs.
func (s *WorkspaceService) SetExcludes(ctx context.Context, globs []string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	return sess.manager.SetExcludes(globs)
}
