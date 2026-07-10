package main

import (
	"context"
	"sync"
)

// App is the Wails-bound backend. Its exported methods are callable from the
// frontend as window.go.main.App.<Method>().
type App struct {
	ctx      context.Context
	mu       sync.Mutex
	settings Settings
	src      DataSource
	tun      *tunnel
}

// NewApp builds the app. The data source/tunnel is established in startup,
// once the Wails runtime (and the app's full process environment) is up —
// spawning ssh during construction, before wails.Run, is unreliable.
func NewApp(s Settings) *App {
	return &App{settings: s, src: newMockSource()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyConnection()
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tun != nil {
		a.tun.close()
	}
}

// applyConnection rebuilds the data source (and ssh tunnel) from a.settings.
// The caller must hold a.mu, or be in the constructor.
func (a *App) applyConnection() error {
	if a.tun != nil {
		a.tun.close()
		a.tun = nil
	}
	if a.settings.Mode != "socket" {
		a.src = newMockSource()
		return nil
	}
	sock := a.settings.SocketPath
	if a.settings.SSHHost != "" {
		t, err := startTunnel(a.settings.SSHHost, a.settings.RemoteSocket)
		if err != nil {
			a.src = errSource{err}
			return err
		}
		a.tun = t
		sock = t.localSock
	}
	a.src = newSocketSource(sock)
	return nil
}

func (a *App) source() DataSource {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.src
}

// GetDashboard returns the whole list view in one call.
func (a *App) GetDashboard() (Dashboard, error) { return a.source().Dashboard() }

// GetTailnetDetail returns the detail view for one tailnet.
func (a *App) GetTailnetDetail(id string) (TailnetDetail, error) {
	return a.source().TailnetDetail(id)
}

// AddTailnet creates a tailnet from the wizard's submission.
func (a *App) AddTailnet(req AddTailnetRequest) error { return a.source().AddTailnet(req) }

// RemoveTailnet tears a tailnet down.
func (a *App) RemoveTailnet(id string) error { return a.source().RemoveTailnet(id) }

// GetSettings returns the current connection settings.
func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

// SaveSettings persists new settings and reconnects. A non-nil error (e.g. an
// SSH forward that won't come up) is surfaced to the UI.
func (a *App) SaveSettings(s Settings) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings = s
	if err := saveSettings(s); err != nil {
		return err
	}
	return a.applyConnection()
}
