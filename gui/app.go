package main

import "context"

// App is the Wails-bound backend. Its exported methods are callable from the
// frontend as window.go.main.App.<Method>().
type App struct {
	ctx context.Context
	src DataSource
}

// NewApp wires the app to a data source.
func NewApp(src DataSource) *App {
	return &App{src: src}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetDashboard returns the whole list view in one call.
func (a *App) GetDashboard() (Dashboard, error) {
	return a.src.Dashboard()
}

// GetTailnetDetail returns the detail view for one tailnet.
func (a *App) GetTailnetDetail(id string) (TailnetDetail, error) {
	return a.src.TailnetDetail(id)
}
