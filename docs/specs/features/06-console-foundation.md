---
id: console-foundation
feature: Console foundation
epic: "Epic 6: Console foundation"
status: planned
issues: []
mockups: [mockups/01-overview.html, mockups/02-namespace-detail.html, mockups/05-dns-and-settings.html]
---

## Purpose

The daemon serves a console. The console replaces the desktop client. It shows the live
state of every tailnet, it shows the topology, and it lets the operator add, remove,
connect, and disconnect a tailnet.

The console is a static single-page application that `go:embed` places in the daemon
binary. It has no build step and no framework. `spec.md` states why.

This feature set delivers the shell, the data layer, and four views: overview, namespace
detail, DNS, and activity, plus settings. `features/07-console-access-editor.md` adds the
access view.

## User stories

- As the operator, I want to open a browser on the host and see every tailnet, so that I
  do not read YAML to learn the state.
- As the operator, I want the topology as one picture, so that I see which tailnet exists
  and how many peers each has.
- As the operator, I want to add a tailnet without editing a file, so that setup is one
  flow.
- As the operator, I want to see what the daemon did and when, so that I can debug a
  failure.
- As the operator, I want the console to work on a host with no internet route, so that
  an isolated host is still manageable.

## Functional requirements

### Serving

- **FR-console-1** — The daemon serves the console over HTTP on the address in
  `console.bind_address`, which defaults to `127.0.0.1:9443`.
- **FR-console-2** — The daemon refuses to start when `console.bind_address` is not a
  loopback address, and it logs the reason.
- **FR-console-3** — `console.enabled` defaults to `true` and `false` disables the
  listener.
- **FR-console-4** — The daemon serves the JSON API on the console listener and on the
  control socket.
- **FR-console-5** — `go:embed` places `internal/ui/static` in the binary. The daemon
  reads no console file from disk.
- **FR-console-6** — The console requests no resource from a host other than its own
  origin.
- **FR-console-7** — The daemon sets the header
  `Content-Security-Policy: default-src 'self'; img-src 'self' data:` on every console
  response.

### Console request controls

- **FR-console-8** — Every mutating route requires the request header
  `X-Hydrascale-Console: 1`, and it returns HTTP 403 without it.
- **FR-console-9** — A route returns HTTP 403 when the request has an `Origin` header
  whose value is not the console origin.
- **FR-console-10** — The daemon records an event for every mutating request that it
  serves on the console listener.
- **FR-console-11** — The daemon logs the console listener address at start, with the
  statement that the console has no authentication.

### The shell

- **FR-console-12** — The console has a left navigation with the entries overview,
  namespaces, access, activity, and settings.
- **FR-console-13** — The navigation shows the logo and the daemon version.
- **FR-console-14** — The console shows one heading per view and no more than one
  heading of the largest size.
- **FR-console-15** — The console polls `GET /api/status` every 5 seconds and the
  interval is a setting.
- **FR-console-16** — When a poll fails, the console keeps the last state, marks it
  stale, and shows the time of the last success.
- **FR-console-17** — The console never shows invented data. An empty view states what
  would fill it.

### The overview

- **FR-console-18** — The overview shows the count of tailnets, the count of peers, the
  reconciler state, and the time since the last reconcile.
- **FR-console-19** — The overview shows a topology of every tailnet, the host, and the
  internet.
- **FR-console-20** — The topology draws an allowed path as a dotted curve.
- **FR-console-21** — The topology draws no denied path.
- **FR-console-22** — The topology draws no arrowhead, no node icon, no edge label, and
  no minimum map.
- **FR-console-23** — When the operator selects a node, the topology draws that node's
  paths in the accent colour and it mutes every other path.
- **FR-console-24** — The topology has a text equivalent that a screen reader reads,
  which lists each allowed path as a sentence.

### Namespace detail

- **FR-console-25** — The namespace view lists every tailnet with its state, its peer
  count, and its address.
- **FR-console-26** — Selecting a tailnet opens a panel with the peers, the MagicDNS
  name, the control server, the host access state, and the recent events.
- **FR-console-27** — The panel closes when the selection clears.
- **FR-console-28** — The operator can connect and disconnect a tailnet from the panel.
- **FR-console-29** — The operator can remove a tailnet from the panel, through a dialog
  that names every command the removal runs.
- **FR-console-30** — The removal dialog states that the node stays authorized on the
  control server.

### Add a tailnet

- **FR-console-31** — The console has an add flow with the fields identifier, auth key,
  control server URL, exit node, and host access.
- **FR-console-32** — The add flow validates the identifier against the same rule that
  the daemon uses, before it sends the request.
- **FR-console-33** — The add flow never displays an auth key after the operator submits
  it.

### DNS and activity and settings

- **FR-console-34** — The DNS view shows the resolver mode, the bind address, the
  upstream servers, and the protected state of every namespace.
- **FR-console-35** — The DNS view shows a warning when the host `resolv.conf` file
  changed.
- **FR-console-36** — The activity view lists events, newest first, with the time, the
  kind, the tailnet, and the message.
- **FR-console-37** — The settings view shows the configuration path, the socket path,
  the console address, the poll interval, and the version.
- **FR-console-38** — The settings view states that the console has no authentication
  and that any local account can reach it.

### Brand

- **FR-console-39** — The console loads the brand tokens from
  `internal/ui/static/brand/tokens/`.
- **FR-console-40** — The console uses the accent colour for one thing per view: the
  affirmative action, or the current selection, or an allowed path.
- **FR-console-41** — The console shows a state as a coloured dot and a lowercase word.
- **FR-console-42** — The console renders every machine value in the mono typeface.
- **FR-console-43** — The console honours `prefers-reduced-motion`.
- **FR-console-44** — The console contains no emoji.

## User flows

### The operator opens the console

1. The operator runs `hydrascale serve` or starts the systemd service.
2. The daemon logs the console address and the no-authentication statement.
3. The operator opens `http://127.0.0.1:9443` in a browser.
4. The console requests `GET /api/status`.
5. The console draws the overview.

### The operator reaches a remote host

1. The operator runs `ssh -L 9443:127.0.0.1:9443 user@host`.
2. The operator opens `http://127.0.0.1:9443` on the local machine.
3. The console works as it does on the host.

### The operator removes a tailnet

1. The operator selects a tailnet in the namespace view.
2. The operator selects **Remove**.
3. The dialog lists `tailscale logout`, the namespace, the veth device, the iptables
   rules, the rule count, and the state directory.
4. The dialog states that the node stays authorized on the control server.
5. The operator confirms.
6. The console sends `POST /api/tailnet/remove`.
7. The reconciler removes the tailnet and the console updates on the next poll.

## Screens & states

### Overview — `mockups/01-overview.html`

| Region | Content |
|---|---|
| Statistics row | Tailnet count, peer count, reconciler state, time since the last reconcile. |
| Topology | Nodes for each tailnet, the host, and the internet. Dotted curves for allowed paths. |
| Recent activity | The five newest events. |

| State | What it shows |
|---|---|
| Empty | No tailnet is configured. The view states that and shows the add action. |
| Loading | The first poll has not returned. The view shows the shell and a quiet placeholder row. |
| Populated | The statistics, the topology, and the activity. |
| Stale | The last known state, a stale marker, and the time of the last success. |
| Error | The daemon is unreachable. The view names the socket path and the console address. |

### Namespace detail — `mockups/02-namespace-detail.html`

| Region | Content |
|---|---|
| List | One row per tailnet: state dot, identifier, peer count, address. |
| Panel | Peers, MagicDNS name, control server, host access, recent events, actions. |

| State | What it shows |
|---|---|
| Empty | No tailnet. The add action. |
| Selected | The panel, open on the right. |
| Not authenticated | The row shows a warning dot and the panel shows the login URL. |
| Error | The row shows a critical dot and the panel shows the last error. |
| Removing | The row is muted and the actions are disabled until the poll confirms. |

### DNS and settings — `mockups/05-dns-and-settings.html`

| Region | Content |
|---|---|
| Resolver | Mode, bind address, upstream servers. |
| Namespaces | One row per namespace with the protected state. |
| Host file | The checksum, the last change time, and the warning when it changed. |
| Settings | Configuration path, socket path, console address, poll interval, version, and the no-authentication statement. |

## Behaviour rules

- The console stages nothing except an access edit. Every other action applies at once,
  because it is a single named action rather than a set of edits.
- The console does not confirm a non-destructive action.
- The console shows one toast per destructive action and none per click.
- The console draws no animation that the operator did not trigger.
- A machine value is never in the sans typeface. A sentence is never in the mono
  typeface.
- The topology places tailnet nodes on the left and the host and the internet on the
  right, as the flow overview in the brand reference does.

## Data touched

| Entity | Change |
|---|---|
| Configuration | New `console` block with `enabled` and `bind_address`. |
| Event | New kind `console.request` for a mutating console request. |

## Interfaces

The console uses the existing routes, plus:

| Method and path | Purpose |
|---|---|
| `GET /api/dns` | The DNS state. `features/04-dns-integrity.md` defines it. |
| `GET /api/access` | The rule set and the node list, which the topology draws. |

The console is served at `/`. Every JSON route stays under `/api/`.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The port is in use. | The daemon fails to start and it names the port and the configuration key. |
| The operator sets `console.bind_address: 0.0.0.0:9443`. | The daemon refuses to start and it states that the console must bind a loopback address. |
| A browser extension injects a request. | The `X-Hydrascale-Console` header requirement and the `Origin` check reject a cross-origin request. Neither stops a local process. |
| The daemon has no tailnet. | Every view shows a designed empty state rather than a blank region. |
| A tailnet has 200 peers. | The panel lists them in a scrolling region and it states the count. |
| The browser has no `color-mix` support. | The tokens use `color-mix` for two values. The console defines a fallback value before each `color-mix` declaration. |
| The poll fails for 60 seconds. | The console keeps the last state, marks it stale, and offers a manual retry. |

## Acceptance criteria

- [ ] `go build ./...` produces one binary that serves the console with no external file.
- [ ] The daemon refuses to start with `console.bind_address: 0.0.0.0:9443`.
- [ ] The console loads at `http://127.0.0.1:9443` and shows the overview.
- [ ] The browser network log shows no request to a host other than the console origin.
- [ ] `POST /api/tailnet/remove` without `X-Hydrascale-Console: 1` returns HTTP 403.
- [ ] A request with `Origin: http://evil.example` returns HTTP 403.
- [ ] The topology draws a dotted curve for each allowed path and no line for a denied
      path.
- [ ] Selecting a node mutes every path that the node does not own.
- [ ] A screen reader reads the topology text equivalent as a list of allowed paths.
- [ ] Every control reaches focus by keyboard and shows a visible focus ring.
- [ ] The removal dialog names `tailscale logout`, the namespace, the veth device, and
      the state directory.
- [ ] The add flow rejects the identifier `My Net` before it sends a request.
- [ ] With the daemon stopped, the console shows the last known state marked stale.
- [ ] With no tailnet configured, every view shows a written empty state.
- [ ] The console JavaScript tests run under `go test ./internal/ui/...`.

## Out of scope

- The access editor. `features/07-console-access-editor.md` owns it.
- The upstream policy views. `features/08-upstream-policy.md` owns them.
- Authentication of any kind.
- A light theme. The brand is a dark operator console and `tokens/colors.css` sets
  `color-scheme: dark`.
- Serving the console over the control socket.

## Open questions

None.
