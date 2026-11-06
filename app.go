package main

import "context"

// App owns the Wails application lifecycle. Core services are wired in here
// and exposed to the frontend through internal/rpc.
type App struct {
	ctx context.Context
}

// NewApp creates the application.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {}
