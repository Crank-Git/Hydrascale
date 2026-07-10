# hydrascale-gui

A cross-platform desktop client for Hydrascale, built with [Wails](https://wails.io)
(Go backend + webview frontend).

The Hydrascale daemon is Linux-only (it uses network namespaces), so this app is a
**remote client**: it talks to the daemon's control API and runs natively on macOS,
Windows, or Linux. It is a separate Go module and deliberately does **not** import
`hydrascale/internal/*` — those packages pull in Linux-only code and would break the
cross-platform build. The GUI depends only on the JSON contract.

## Status

Milestone 2, first cut: the window shell renders the dashboard from the Go backend.
Data currently comes from a built-in mock (`datasource.go`) so the app runs standalone
with no daemon attached. The live transport (SSH-tunnel to the daemon's unix socket)
lands next.

## Develop

```sh
cd gui
wails dev      # hot-reload dev window
```

## Build

```sh
cd gui
wails build    # → build/bin/hydrascale-gui.app (macOS), .exe (Windows), binary (Linux)
```

Requires the Wails v2 CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
and its platform prerequisites — run `wails doctor`.

## Layout

- `main.go` — Wails app options, selects the data source, embeds `frontend/dist`
- `app.go` — bound methods callable from JS as `window.go.main.App.*`
- `types.go` — display-ready DTOs (decoupled from the daemon's Go types)
- `datasource.go` — `DataSource` interface + the mock implementation
- `frontend/dist/index.html` — the UI (self-contained; inlined CSS/fonts)
