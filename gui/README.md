# hydrascale-gui

A cross-platform desktop client for Hydrascale, built with [Wails](https://wails.io)
(Go backend + webview frontend).

The Hydrascale daemon is Linux-only (it uses network namespaces), so this app is a
**remote client**: it talks to the daemon's control API and runs natively on macOS,
Windows, or Linux. It is a separate Go module and deliberately does **not** import
`hydrascale/internal/*` — those packages pull in Linux-only code and would break the
cross-platform build. The GUI depends only on the JSON contract.

## Status

The window renders the dashboard, tailnet detail, add-tailnet wizard, and remove flow,
all round-tripping through Go bound methods.

Two data sources (`DataSource` interface):
- **mock** (`datasource.go`) — built-in fixtures; the default, so the app runs standalone.
- **socket** (`socketsource.go`) — live HTTP over the daemon's unix socket.

## Live data (remote client)

Set `HYDRASCALE_SOCKET` to a daemon control socket and the app talks to it live:

```sh
# Local (on the Linux host running the daemon):
HYDRASCALE_SOCKET=/var/lib/hydrascale/api.sock ./hydrascale-gui

# Remote (macOS/Windows) — forward the daemon's socket over SSH, then point at it:
ssh -N -L /tmp/hydrascale.sock:/var/lib/hydrascale/api.sock user@linux-host
HYDRASCALE_SOCKET=/tmp/hydrascale.sock ./hydrascale-gui
```

Note: the daemon's `api.sock` is `0600` and root-owned, so the SSH login user must be
able to read it — SSH as a user with access, relax the socket mode, or bridge it
(`socat`) as root. A built-in connection UI is planned.

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
