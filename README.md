# Hydrascale

<p align="center">
  <img src="internal/ui/static/brand/logo-lime.svg" alt="Hydrascale logo" width="120">
</p>

<p align="center">Run multiple Tailscale tailnets simultaneously on a single Linux machine.</p>

Hydrascale lets one Linux host join several Tailscale tailnets at the same time. It is for
the operator who administers a host that must reach a work tailnet, a home tailnet, and a
customer tailnet at once. The daemon holds each tailnet in its own network namespace, it
enforces the reachability rules that the operator declares, and it serves a console on the
loopback address.

## Table of Contents

- [What Hydrascale does](#what-hydrascale-does)
- [Requirements](#requirements)
- [Install](#install)
- [Quick start](#quick-start)
- [The console](#the-console)
- [Local rules](#local-rules)
- [Upstream policy](#upstream-policy)
- [Credentials](#credentials)
- [Host access](#host-access)
- [Headscale and a custom control server](#headscale-and-a-custom-control-server)
- [Configuration reference](#configuration-reference)
- [Remote access](#remote-access)
- [Networking](#networking)
- [Command line](#command-line)
- [Environment variables](#environment-variables)
- [Control API](#control-api)
- [Daemon mode](#daemon-mode)
- [Architecture](#architecture)
- [Uninstall](#uninstall)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## What Hydrascale does

Hydrascale creates one network namespace per tailnet and runs a separate `tailscaled`
inside each. Traffic of one tailnet stays inside its own network stack. Overlapping IP
ranges, independent firewall rules, and separate routing tables therefore work without an
extra step.

The operator declares the tailnets in a YAML configuration file. The reconciler drives the
live host toward that file. Add a tailnet to the file and the daemon creates it. Remove a
tailnet and the daemon deletes the namespace, stops the process, and removes the routes. A
unified DNS resolver answers names across every tailnet, so the host reaches a peer by
name.

The reconciler runs as a control loop. On each tick it reads the configuration, reads the
live host, computes the difference, and applies the smallest set of actions. A tailnet that
fails three times in a row enters an error state, and the reconciler skips it until the
operator resets it. One broken tailnet therefore stops no other tailnet. The event log
records every action.

Version 1.0 adds three things to that loop:

- **The console.** The daemon serves a web interface on `127.0.0.1:9443`. The console shows
  the namespaces, the local rules, the upstream policy, and the event list.
- **Local rules.** The daemon enforces a declared rule set on the host with iptables. A
  path that no rule allows is denied.
- **Upstream policy.** The daemon reads, validates, and writes the access-control document
  that the control server of a tailnet holds.

## Requirements

- **Linux.** A network namespace is a Linux kernel feature.
- **Go 1.26 or later**, to build from source.
- **Root, or the capability `CAP_NET_ADMIN`.**
- **Tailscale.** The commands `tailscaled` and `tailscale` must be in `$PATH`.
- **iproute2.** The daemon runs `ip` to manage a namespace.
- **iptables.** The daemon writes the NAT rules and the forward rules.
- **Kernel network namespace support** (`CONFIG_NET_NS`), which every current kernel holds.
- **Kernel policy routing** (`CONFIG_IP_MULTIPLE_TABLES`, `CONFIG_IPV6_MULTIPLE_TABLES`).
  [Host access](#host-access) needs it to propagate an accepted subnet route to the host.
  Most distribution kernels hold it. Some single-board kernels omit it, and route
  propagation then changes nothing.
- **IP forwarding.** Run `sudo sysctl -w net.ipv4.ip_forward=1`.

## Install

### A released binary

Download a binary from the
[GitHub Releases](https://github.com/Crank-Git/Hydrascale/releases) page:

```bash
tar xzf hydrascale_*.tar.gz
sudo install hydrascale /usr/local/bin/
```

### A build from source

```bash
go install hydrascale/cmd/hydrascale@latest
```

Or clone the repository and build it:

```bash
git clone https://github.com/Crank-Git/Hydrascale.git
cd hydrascale
go build -o hydrascale ./cmd/hydrascale
sudo install hydrascale /usr/local/bin/
```

## Quick start

The interactive wizard is the shortest path. It runs the preflight checks, writes the
configuration, sets up the authentication, starts the first tailnet, and confirms that the
tailnet authenticated:

```bash
sudo hydrascale init
```

To configure the host by hand instead:

1. Write a configuration file at `/etc/hydrascale/config.yaml`:

```yaml
version: 2
tailnets:
  - id: corp-prod
  - id: homelab
    exit_node: exit-us.example.com
access:
  mode: enforce
  rules:
    - from: corp-prod
      to: internet
    - from: homelab
      to: internet
resolver:
  mode: unified
reconciler:
  interval: 10s
```

2. Apply the configuration once:

```bash
sudo hydrascale apply
```

3. Or run the daemon, which reconciles on every tick:

```bash
sudo hydrascale serve
```

4. Open the console at `http://127.0.0.1:9443`.

5. Read the state:

```bash
sudo hydrascale status
```

A tailnet needs up to 90 seconds after a start of the daemon before `hydrascale status`
reports `healthy` and `running`. An earlier read reports `down` and `degraded`, which is
the normal state of a tailnet that still authenticates.

## The console

The daemon serves the console on the loopback address. The console is a static single-page
application inside the binary, so it makes no request to another host and it needs no build
step. It holds six views: Overview, Namespaces, Access, Policy, Activity, and Settings.

The daemon serves the console when the configuration file holds no `console` key, so a
version 0.9 file needs no edit. The default address is `127.0.0.1:9443`:

```yaml
console:
  enabled: true                  # default: true
  bind_address: "127.0.0.1:9443" # default: 127.0.0.1:9443
```

The daemon serves the JSON API on the console listener as well as on the control socket.

To confirm the listener, read the status code:

```bash
curl -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9443/    # prints 200
```

The daemon writes the address and the authentication position to the log at every start:

```bash
sudo journalctl -u hydrascale | grep 'the console listens on'
```

### The console has no authentication

**Warning — the console has no sign-in, and the daemon runs as root.** Any local account on
the host reaches `http://127.0.0.1:9443` and drives the daemon. The operator accepted this
risk for version 1.0. Give a local account on this host the trust that root needs.

Four controls reduce the risk:

1. The listener binds a loopback address only. The daemon refuses any other address, it
   writes the reason to the log, and `hydrascale apply` refuses the same value.
2. Every mutating route requires the header `X-Hydrascale-Console: 1`. A browser sets no
   custom header on a cross-origin form post.
3. The daemon answers HTTP 403 when the `Origin` header names another origin.
4. The daemon records one event for every mutating request, and the Activity view shows it.

Control 2 and control 3 stop a hostile web page. Neither control stops a local account.

To close the listener, set `console.enabled: false`. The control socket keeps serving the
JSON API.

## Local rules

A local rule is one reachability rule that the daemon enforces on the host with iptables.
The daemon owns the chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT`, and one jump rule into
each of `FORWARD` and `INPUT`. It writes no other rule, and it moves no rule of the
operator.

The rule set holds no deny rule, because deny is the default. A path that no rule allows is
denied.

```yaml
access:
  mode: enforce        # default: enforce. The other mode is observe.
  rules:
    - from: corp-prod
      to: internet     # every port and both protocols
    - from: host
      to: corp-prod
      ports: ["tcp/22", "tcp/443"]
    - from: homelab
      to: corp-prod
      ports: ["udp/1-1024"]
```

- `from` names a tailnet that the file declares, or the literal `host`. It never names
  `internet`.
- `to` names a tailnet that the file declares, or the literal `host`, or the literal
  `internet`.
- `from` and `to` must differ.
- `ports` holds entries of the form `tcp/<n>`, `udp/<n>`, `tcp/<n>-<m>`, or `udp/<n>-<m>`.
  A port number is between 1 and 65535. An empty list allows every port and both protocols.

The daemon validates the whole block before it writes a rule. A rule that names an unknown
tailnet stops the load, and the error names every failure together.

### The two modes

`enforce` drops a packet that no rule allows. `observe` writes a kernel log line for that
packet and drops nothing. Read the lines of the mode `observe` with:

```bash
journalctl -u hydrascale | grep hydrascale-would-deny
```

The mode `observe` rate-limits the log to 60 packets each minute.

**Warning — a first start on a host with no `access` key changes reachability.** When the
configuration file holds no `access` key, the daemon writes one. It copies the file to
`<config>.pre-v1.backup` first. The rule set it writes holds one rule per tailnet, from that
tailnet to `internet`, in the mode `enforce`. No tailnet reaches another tailnet, and the
host reaches no tailnet. Set `access.mode: observe` before the first start of version 1.0,
read the log for a day, then add the rules the log names.

The Access view of the console shows the rule set, stages an edit, and applies it. The
reconciler writes the changed rule set on the next tick.

## Upstream policy

A policy is the huJSON access-control document that a control server holds for a tailnet.
The Policy view of the console reads that document, validates it, and writes it back. This
is the upstream half of reachability; the local rules are the half that this host enforces.

The daemon takes the control server kind from the control URL of the tailnet. A tailnet
that declares no `control_url` is a Tailscale tailnet. Every other tailnet is a Headscale
tailnet.

The daemon sends the document to the validate route of the control server before it writes.
A document that validate rejects returns HTTP 400 with the answer of the control server,
and the write route receives no request.

A Tailscale write carries the `ETag` value of the read in the `If-Match` header. When the
policy changed between the read and the write, the control server answers HTTP 412 and the
console reports a conflict. The console re-reads the document and keeps the text of the
operator, so that the operator compares the two.

**A Headscale control server needs `policy.mode: "database"` for a policy write.** A server
that runs the file policy mode rejects `PUT /api/v1/policy`, and the daemon returns the
message of the control server word for word:

```json
{"code":2,"message":"update is disabled for modes other than 'database'","details":[]}
```

`hscontrol/types/config.go:54` of the Headscale source at tag `v0.29.3` declares the two
values:

```go
PolicyModeDB   = "database"
PolicyModeFile = "file"
```

Policy access also needs Headscale v0.29 or later, because an older server exposes no
policy route.

## Credentials

A credential authenticates the daemon to a control server. A policy read and a policy write
need one. The daemon reads a credential at the moment it needs one and holds none between
requests.

A credential never enters the configuration file, never reaches the log, and never reaches
a control API answer. It lives in the secrets file, which the key `secrets_file` names. The
default path is `/etc/hydrascale/secrets.yaml`. The daemon refuses a secrets file that
grants group access or other access, so the file needs the mode `0600` and the owner root.

```yaml
# /etc/hydrascale/secrets.yaml, mode 0600, owner root
tailnets:
  corp-prod:
    tailscale_oauth_client_id: "..."
    tailscale_oauth_client_secret: "..."
  homelab:
    headscale_api_key: "..."
    headscale_address: "https://headscale.example.com"
```

Write the file with the mode in place:

```bash
sudo install -m 0600 /dev/null /etc/hydrascale/secrets.yaml
sudo "$EDITOR" /etc/hydrascale/secrets.yaml
```

The console writes the same values through `PUT /api/policy/{id}/credentials`.

### A Tailscale credential

The daemon authenticates to the Tailscale API with an OAuth client. Create the client in
the Tailscale admin console, under **Settings → Keys → OAuth clients**.

**A policy write needs three scopes, not one.** Give the client the scopes:

- `policy_file`
- `devices:posture_attributes`
- `devices:core:read`

`https://tailscale.com/kb/1623/`, "Trust credentials", section `Scopes`, states verbatim:

> policy_file The credential has access to read, validate, and modify the tailnet policy
> file. devices:posture_attributes and devices:core:read are required when using this
> scope. Endpoints from policy_file:read POST /api/v2/tailnet/:tailnet/acl

The Tailscale OpenAPI schema states `OAuth Scope: policy_file.` in the description of
`operationId: setPolicyFile`. Both sources were retrieved on 2026-08-05. A client that
holds `policy_file` alone fails at the write with a permission error that names no cause.

Write the client identifier and the client secret into the secrets file as
`tailscale_oauth_client_id` and `tailscale_oauth_client_secret`. The daemon exchanges them
for an access token at `https://api.tailscale.com/api/v2/oauth/token`, and that token
expires after one hour.

### A Headscale credential

Create an API key on the Headscale host:

```bash
headscale apikeys create --expiration 90d
```

Write the key into the secrets file as `headscale_api_key`, and write the base address of
the control server as `headscale_address`. The daemon sends the key as a bearer token.

Set `policy.mode: "database"` in the Headscale configuration before a policy write. A
server in the file policy mode serves a policy read and refuses a policy write.

### An environment variable overrides a file value

An environment variable overrides the matching file value, and it overrides that value
alone. `<ID>` is the tailnet identifier in upper case, with each dash replaced by an
underscore.

| Variable | Overrides |
|---|---|
| `HYDRASCALE_TS_CLIENT_ID_<ID>` | `tailscale_oauth_client_id` |
| `HYDRASCALE_TS_CLIENT_SECRET_<ID>` | `tailscale_oauth_client_secret` |
| `HYDRASCALE_HS_API_KEY_<ID>` | `headscale_api_key` |

## Host access

Each tailnet stays inside its own namespace by default. The host reaches a peer through a
namespace-scoped command such as `hydrascale exec` or `hydrascale ping`. **Host access**
changes that: the host reaches the peers of every managed tailnet directly.

```bash
# Without host access
sudo hydrascale ping havoc bigboy    # works, but needs the wrapper

# With host access
ping havoc-bigboy
ssh havoc-mars
curl http://havoc-webserver:8080
```

Enable it globally, or per tailnet:

```yaml
# Global: the host reaches every tailnet
host_access: true

# Or per tailnet
tailnets:
  - id: corp-prod
    host_access: true     # the host reaches this tailnet
  - id: personal
    host_access: false    # isolated (default)
```

A local rule still governs the path. A host that reaches a tailnet needs a rule from `host`
to that tailnet.

### How host access works

The daemon does four things on each reconciliation tick of a tailnet that holds host
access:

1. **Host routes.** The daemon adds a host route for the Tailscale address of each peer,
   for IPv4 and for IPv6, through the veth pair of the namespace. The kernel then sends a
   packet to the right namespace.

2. **Namespace masquerade.** The daemon adds an iptables masquerade rule inside the
   namespace on `tailscale0`. Traffic of the host then carries the Tailscale address of the
   namespace, and `tailscaled` forwards it to the peer.

3. **DNS entries.** The daemon writes `/etc/hosts` entries, so the host resolves a peer by
   name.

4. **A MagicDNS route per tailnet.** The daemon registers the MagicDNS suffix of each
   tailnet, such as `taildf854a.ts.net`, with the DNS forwarder, and points it at the veth
   gateway of that tailnet. A PREROUTING DNAT rule on the veth carries the query to the
   MagicDNS resolver of that namespace at `100.100.100.100`. Several tailnets therefore
   answer MagicDNS queries on one host.

The daemon syncs the routes and the DNS entries on every tick. It removes them at shutdown,
and when host access is disabled.

### The DNS lifecycle of tailscaled

Two subtleties matter for a MagicDNS route per tailnet, and the daemon handles both.

**Namespace upstreams.** Each namespace gets `/etc/netns/<ns>/resolv.conf` with the real
upstream resolvers of the host. The daemon reads them from
`/run/systemd/resolve/resolv.conf` or from `/etc/resolv.conf`, it removes a loopback
address, and it falls back to `1.1.1.1`. The address `100.100.100.100` must not go into
that file: `tailscaled` removes its own address as a self-loop, an empty resolver chain
returns SERVFAIL for every query, and the daemon then answers no name at all.

**A refresh after a restart.** The reconciler restarts an unhealthy `tailscaled`. The new
process loads its state from disk and does not read `resolv.conf` again, which can leave
its MagicDNS proxy stopped. The daemon waits for `BackendState=Running`, then sets
`--accept-dns=false` and `--accept-dns=true`, which rebuilds the resolver chain. DNS
therefore recovers on every restart, and the operator runs no `tailscale set` by hand.

### The overlay mount on /etc

`tailscaled` replaces `/etc/resolv.conf` with a temporary file and a rename whenever its
DNS configuration changes. A bind mount on the single file cannot hold a rename: the new
file lands in the shared `/etc` of the host and replaces the host resolver configuration.

The daemon therefore starts each namespaced `tailscaled` under an overlay mount on `/etc`.
The lower layer is the `/etc` of the host, and the upper layer is a directory per tailnet.
Every write to `/etc`, including that rename, stays inside the namespace, and the process
still reads the `/etc` of the host through the lower layer. The daemon never changes
`/etc/resolv.conf` on the host.

A host that cannot mount OverlayFS fails the start of the namespace, because a namespace
without the overlay mount can replace the resolver configuration of the host. To start such
a namespace anyway, set:

```yaml
dns:
  allow_unprotected: false   # default: false
```

Set `allow_unprotected: true` only on a host where the overlay mount cannot work. The
daemon records the event `dns.unprotected`, and the Overview view of the console shows the
namespace as unprotected.

### Accepted subnet routes

When a peer advertises a subnet route and the `tailscaled` of the namespace runs with
`--accept-routes`, the daemon propagates that route to the host. On each tick it reads the
routing table 52 inside the namespace, which is where `tailscaled` installs an accepted
route, and it adds the matching route on the host through the veth gateway.

Pass `--accept-routes` at the login:

```bash
sudo hydrascale tailscale corp-prod -- up --accept-routes
```

The configuration file needs no change. A subnet route appears on the host within one tick
after the login, and the daemon removes it when the tailnet goes away.

### The naming convention

Each peer takes the name `<tailnet-id>-<hostname>`. Two tailnets with a peer of the same
name therefore produce two different names on the host.

| Tailnet | Peer | Name on the host |
|---------|------|-----------------|
| havoc | bigboy | `havoc-bigboy` |
| havoc | mars | `havoc-mars` |
| personal | pixel 8a | `personal-pixel-8a` |
| personal | nas | `personal-nas` |

The daemon lowercases a name and it replaces each space with a dash.

### The two host DNS modes

The key `host_dns.mode` selects the mode.

**`hosts`, the default.** The daemon owns a marked block in `/etc/hosts`:

```
# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT
100.98.107.70  havoc-mars
fd7a:115c:a1e0::1  havoc-mars
100.73.198.12  havoc-bigboy
# END HYDRASCALE MANAGED BLOCK
```

This mode works on every Linux system. The daemon rewrites the block only when the peer
data changes, and it writes the file atomically. It changes no other entry of `/etc/hosts`.

**`resolved`.** The daemon registers a routing domain with `systemd-resolved` through
`resolvectl`. This mode needs `systemd-resolved`, and it changes no file.

### Teardown

When host access goes away for a tailnet, through a configuration change or a removal, the
daemon removes:

- Every host route of the peers of that tailnet.
- The masquerade rule and the DNS DNAT rule inside the namespace.
- The entries of that tailnet in `/etc/hosts`, or the `systemd-resolved` registration.

A graceful shutdown removes the same state.

### Compatibility

- **A standard Linux distribution.** Every feature works, including a MagicDNS name per
  tailnet.
- **Tegra and Jetson**, and any kernel without `xt_connmark`. Every feature works. The DNS
  forwarder and the veth DNAT rule carry a MagicDNS query without `xt_connmark`, so a name
  such as `mars.taildf854a.ts.net` resolves.
- **A host without `systemd-resolved`.** Use the mode `hosts`, which is the default.

## Headscale and a custom control server

The key `control_url` points a tailnet at [Headscale](https://github.com/juanfont/headscale)
or at another Tailscale-compatible control server. A self-hosted tailnet therefore runs
beside a tailnet of the Tailscale coordination server.

Set the key per tailnet, or set a global default:

```yaml
version: 2
control_url: "https://headscale.example.com"   # the default for every tailnet

tailnets:
  - id: homelab
    # takes the global control_url

  - id: corp-infra
    control_url: "https://headscale.corp.internal"
    # takes its own Headscale instance

  - id: personal
    # no control_url: the Tailscale coordination server
```

A per-tailnet `control_url` overrides the global default. A tailnet that declares neither
joins the Tailscale coordination server. The URL needs the scheme `https`, unless the host
is a loopback address.

### Log in without an auth key

Without a pre-authentication key, log in after the daemon creates the namespace. Start the
daemon, or run `apply`, then run the login for each tailnet:

```bash
# Start the daemon, which creates the namespaces and starts tailscaled
sudo hydrascale serve &

# Log in to a Headscale tailnet
sudo hydrascale tailscale corp-prod -- up --login-server https://headscale.example.com

# Log in to the Tailscale coordination server
sudo hydrascale tailscale personal -- up
```

The daemon prints the authentication URL. Open it in a browser and approve the device. The
namespace stays up, and the daemon manages it from that point.

### What to check with Headscale

- **The auth key format.** A Headscale auth key has a different form from a Tailscale
  `tskey-auth-*` key. The daemon checks no form and passes the key to `tailscale up` through
  `TS_AUTHKEY`. Use the key type of the control server.

- **MagicDNS.** Host access DNS resolution depends on the DNS configuration of the control
  server. Headscale serves MagicDNS, and its suffix and its behaviour can differ from
  Tailscale. When a name does not resolve, read the Headscale DNS configuration.

- **DERP relays.** Tailscale runs its own global DERP relay network. Headscale uses the same
  relays, its own relays, or both. When two peers cannot connect, read the DERP map of the
  Headscale instance. A direct connection through STUN works without the control server.

## Configuration reference

```yaml
# The schema version. The daemon migrates a v1 file when the key is absent.
version: 2

# Transparent host access to every tailnet peer (default: false).
host_access: false

# The control server URL for Headscale (default: empty, which means Tailscale).
# It applies to every tailnet that declares none. It needs the scheme https,
# unless the host is a loopback address.
# control_url: "https://headscale.example.com"

# The subnet of the veth pairs between the host and the namespaces
# (default: 10.200.0.0/16). Change it when 10.200.0.0/16 collides with a route on
# the network. It must be an IPv4 CIDR of at least /16.
# infra_subnet: "10.200.0.0/16"

# The Unix group that reaches the control socket (default: empty, which is root only).
# Warning: membership of this group is equivalent to root access on this host, because a
# member sends a command to the daemon and the daemon runs as root.
# A client that is not root needs it, and a client that reaches the socket over an SSH
# forward needs it. The daemon then makes /var/lib/hydrascale and api.sock
# group-accessible. See "Remote access" below.
# socket_group: hydrascale

# The root-only file that holds a credential per tailnet
# (default: /etc/hydrascale/secrets.yaml). The file needs the mode 0600 and the owner
# root. See "Credentials" above.
# secrets_file: "/etc/hydrascale/secrets.yaml"

# The tailnets that the daemon manages
tailnets:
  - id: "corp-prod"                # unique; letters, digits, dots, hyphens, underscores; 63 characters at most
    exit_node: "node1.example.com" # optional exit node name
    auth_key: "tskey-auth-xxxxx"   # optional auth key for an unattended setup
    host_access: true              # optional, and it overrides the global value
    # control_url: "https://headscale.example.com"  # optional per-tailnet control server

# The local rule set (default: the mode enforce, and the preserving rules that the
# migration writes). See "Local rules" above.
access:
  mode: enforce                    # enforce (default) or observe
  rules:
    - from: corp-prod              # a tailnet, or the literal host
      to: internet                 # a tailnet, or the literal host, or the literal internet
      ports: []                    # empty allows every port and both protocols

# The console listener. See "The console" above.
console:
  enabled: true                    # default: true
  bind_address: "127.0.0.1:9443"   # default: 127.0.0.1:9443, and it must be a loopback address

# The DNS protection setting. See "The overlay mount on /etc" above.
dns:
  allow_unprotected: false         # default: false

# The DNS resolver
resolver:
  mode: unified                    # the one mode the daemon runs
  bind_address: "127.0.0.53:5354"  # optional, and it defaults to 127.0.0.53:5354

# The host DNS mode, which host access uses
host_dns:
  mode: hosts                      # hosts (default) writes /etc/hosts entries
  # mode: resolved                 # registers with systemd-resolved through resolvectl

# The address that each namespace sends one packet to, so that the status answer reports
# measured reachability. An empty value selects a public default. Declare an IP address
# and never a name.
# probe_target: "100.100.100.100"

# The reconciler
reconciler:
  interval: 10s                    # the tick of the control loop, as a Go duration
```

## Remote access

The daemon serves the control API on the control socket `/var/lib/hydrascale/api.sock`. The
socket is root only by default: the mode is `0600`, and no other account traverses
`/var/lib/hydrascale`. A client that is not root needs group access to the socket. A client
on another machine reaches the socket over an SSH forward, and the SSH account needs the
same group access.

**Warning — membership of `socket_group` is equivalent to root access.** A member of the
group sends a command to the daemon, and the daemon runs as root. The member creates a
namespace, writes a host route, and runs a command as root. Name a group that holds only
trusted operators. The daemon refuses to start when `socket_group` names the root group.

Create the group and add the account to it:

```bash
sudo groupadd hydrascale
sudo usermod -aG hydrascale "$USER"    # log out and in for it to take effect
# set `socket_group: hydrascale` in the config, then restart the daemon
```

`hydrascale init` offers this as a step. With `socket_group` set, the daemon owns the socket
`root:hydrascale 0660` and makes the directory group-traversable. A member of the group then
reaches the control API without root.

**From another machine.** Forward the socket over SSH, then send the request to the
forwarded path. The SSH account must be a member of the socket group on the host:

```bash
ssh -L /tmp/hydrascale.sock:/var/lib/hydrascale/api.sock user@linux-host
curl --unix-socket /tmp/hydrascale.sock http://unix/api/status
```

Forward the console port the same way when the operator wants the console from another
machine:

```bash
ssh -L 9443:127.0.0.1:9443 user@linux-host
```

The console has no authentication, so the SSH forward is the only control on that path.

## Networking

### IP forwarding

The daemon needs `net.ipv4.ip_forward=1` on the host, so that traffic from inside a
namespace reaches the internet for the tailnet coordination. Enable it for this boot:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
```

To keep it across a restart, write `/etc/sysctl.d/99-hydrascale.conf`:

```
net.ipv4.ip_forward = 1
```

### Veth pairs

A veth pair connects each namespace to the host. The interface names come from a short hash
of the tailnet identifier: `vh<hash>` on the host side and `vn<hash>` inside the namespace.
The names therefore stay inside the Linux limit of 15 characters. Each pair takes a `/30`
block from the infra subnet, which defaults to `10.200.0.0/16`.

When `10.200.0.0/16` collides with a route on the network, set `infra_subnet` to a free
range; see [Configuration reference](#configuration-reference). The value must be IPv4 and
at least a `/16`.

### NAT and masquerade

The daemon adds an iptables MASQUERADE rule per namespace, so that outbound traffic of the
namespace goes through the default interface of the host and reaches the internet.

### Docker

Docker sets the policy of the `FORWARD` chain to `DROP`, which stops the traffic between a
namespace and the host. The daemon detects this and adds an `ACCEPT` rule per namespace in
the `FORWARD` chain. The operator writes no iptables rule by hand.

The daemon inserts its jump rule at position 1 of `FORWARD` and of `INPUT`. That position is
not stable, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` each take position 1
after the daemon starts. The reconciler reads the position on every tick, records the event
`access.jump_displaced` when the position changes, and writes the jump rule again when the
parent chain holds none.

## Command line

```
hydrascale add <id>                   Add a tailnet to the config and reconcile
hydrascale apply                      Reconcile once
hydrascale diff                       Show what a reconcile would change
hydrascale env <tailnet-id>           Print the shell environment of a tailnet namespace
hydrascale exec <tailnet-id> -- <cmd> Run a command inside the namespace of a tailnet
hydrascale init                       Run the first-run wizard
hydrascale install                    Install the daemon as a systemd service
hydrascale list                       List the configured tailnets
hydrascale ping <tailnet-id> <target> Ping a peer from inside the namespace of a tailnet
hydrascale remove <id>                Remove a tailnet from the config and reconcile
hydrascale serve                      Run the daemon and the control loop
hydrascale ssh  <tailnet-id> <target> Open an SSH session to a peer through a namespace
hydrascale status                     Show the declared state and the live state
hydrascale switch <id>                Switch the default namespace of the tailscale command
hydrascale tailscale <tailnet-id> -- <args>
                                      Run a tailscale command inside the namespace of a tailnet
hydrascale tui                        Open the terminal interface, which needs a running daemon
hydrascale uninstall                  Remove Hydrascale from the host
hydrascale version                    Print the version
hydrascale wrap <service> <tailnet-id>
                                      Write a systemd drop-in that isolates a service
```

Pass `--config <path>` on any command to name another configuration file. The default is
`/etc/hydrascale/config.yaml`, which the systemd unit also passes.

The namespace-scoped subcommands `exec`, `ping`, `ssh`, and `tailscale` replace a raw
`ip netns exec` line:

```bash
# Before
sudo ip netns exec ns-personal tailscale --socket=/var/lib/hydrascale/state/personal/tailscaled.sock ping Mars

# After
sudo hydrascale ping personal Mars
```

## Environment variables

| Variable | Description |
|---|---|
| `HYDRASCALE_AUTHKEY_<ID>` | Overrides the `auth_key` of the tailnet `<ID>`. `<ID>` is the identifier in upper case, with each dash replaced by an underscore. For the tailnet `corp-prod`, set `HYDRASCALE_AUTHKEY_CORP_PROD=tskey-auth-xxxxx`. |
| `HYDRASCALE_TS_CLIENT_ID_<ID>` | Overrides `tailscale_oauth_client_id` in the secrets file. |
| `HYDRASCALE_TS_CLIENT_SECRET_<ID>` | Overrides `tailscale_oauth_client_secret` in the secrets file. |
| `HYDRASCALE_HS_API_KEY_<ID>` | Overrides `headscale_api_key` in the secrets file. |

## Control API

The daemon serves the JSON API on the control socket `/var/lib/hydrascale/api.sock`, and on
the console listener at `127.0.0.1:9443`. The `tui` command and the `status` command use the
control socket. Every mutating route on the console listener requires the header
`X-Hydrascale-Console: 1`.

| Route | Method | Description |
|---|---|---|
| `/api/status` | GET | The declared state and the live state of every tailnet |
| `/api/events` | GET | The recent events of the reconciler |
| `/api/reconcile` | POST | Run one reconciliation now |
| `/api/tailnet/add` | POST | Add a tailnet to the config and reconcile |
| `/api/tailnet/remove` | POST | Remove a tailnet from the config and reconcile |
| `/api/tailnet/connect` | POST | Clear the error state and reconnect a tailnet |
| `/api/tailnet/disconnect` | POST | Stop the process of a tailnet and keep the config |
| `/api/tailnet/{id}/detail` | GET | The peers, the routes, and the addresses of one tailnet |
| `/api/tailnet/{id}/removal-plan` | GET | What a removal of one tailnet deletes |
| `/api/config` | GET | The current configuration, with every credential removed |
| `/api/config/dns` | POST | Change the resolver configuration |
| `/api/dns` | GET | The resolver state and the DNS protection state per namespace |
| `/api/access` | GET, PUT | Read and write the local rule set |
| `/api/policy` | GET | The control server kind and the write availability per tailnet |
| `/api/policy/{id}` | GET, PUT | Read and write the policy of one tailnet |
| `/api/policy/{id}/validate` | POST | Validate a policy document at the control server |
| `/api/policy/{id}/credentials` | PUT | Write the credential of one tailnet |

Every mutating route validates the whole request body before it changes anything. A failure
returns HTTP 400 and the body `{"error": "<message>"}`.

## Daemon mode

### The systemd unit

```bash
sudo mkdir -p /var/lib/hydrascale
sudo cp contrib/hydrascale.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hydrascale
```

The unit at `contrib/hydrascale.service` runs the daemon with ambient capabilities and the
systemd sandbox settings. `hydrascale install` writes the same unit.

### SIGHUP

SIGHUP makes the daemon read the configuration file and reconcile at once, without a wait
for the next tick. Under systemd, `systemctl reload hydrascale` sends the signal.

### A graceful shutdown

SIGINT and SIGTERM cancel the control loop. The daemon stops every tailnet process
concurrently, with a timeout of 30 seconds, and exits.

### Monitoring

```bash
sudo systemctl status hydrascale
sudo journalctl -u hydrascale -f
```

## Architecture

```
                      +-----------------------+
                      |    config.yaml        |
                      |  (declared state)     |
                      +-----------+-----------+
                                  |
                                  v
                      +-----------+-----------+
                      |     Reconciler        |
                      |  read the config      |
                      |  read the live host   |
                      |  compute the diff     |
                      |  apply the actions    |
                      +-+--------+----------+-+
                        |        |          |
               +--------+   +---+---+   +--+--------+
               v             v           v
    +----------+--+  +------+------+  +-+----------+
    |  Namespace  |  |   Daemon    |  |  Routing   |
    |  Manager    |  |   Manager   |  |  Manager   |
    | (ip netns)  |  | (tailscaled)|  | (netlink)  |
    +-------------+  +-------------+  +------------+
          |                |                |
          v                v                v
    ns-corp-prod     tailscaled         route sync
    ns-homelab       per namespace      per namespace
```

Each tick of the reconciler:

1. **Read** the declared state from `config.yaml`.
2. **Read** the live state: the namespaces that exist, the processes that are healthy, the
   routes that are installed, and the position of the jump rule.
3. **Compute** the difference, which produces a list of actions.
4. **Apply** the actions in order, and count the failures per tailnet.
5. After three failures in a row, place the tailnet in an error state and skip it until the
   operator resets it.

The reconciler takes a file lock before each tick, so two ticks never change the host at
once.

## Uninstall

`uninstall` stops every tailnet, deletes the namespaces, the veth pairs, the iptables
rules, the host routes, and the DNS entries, then removes the systemd service and
`/var/lib/hydrascale`:

```bash
sudo hydrascale uninstall
```

The command logs each tailnet node out, so no node stays in the tailnet admin console. Pass
`--keep-nodes` to keep them. A plain `uninstall` keeps the binary and `/etc/hydrascale`, so
the command runs again. Pass `--purge` to remove those too, and `--yes` to skip the
confirmation.

## Troubleshooting

**`bind: address in use` for the control socket**
A daemon that crashed left the socket file. Delete it and start again:
```bash
sudo rm /var/lib/hydrascale/api.sock
sudo hydrascale serve
```

**The console does not open**
Read the log for the bind address. The daemon refuses a `console.bind_address` that is not
a loopback host and a port, and it refuses a port that another process holds:
```bash
sudo journalctl -u hydrascale | grep console
```

**A tailnet cannot reach the internet after an upgrade to version 1.0**
The rule set denies the path. Read the rules, and set `access.mode: observe` to see the
paths that the mode `enforce` denies:
```bash
sudo journalctl -u hydrascale | grep hydrascale-would-deny
```

**`hydrascale status` reports `down` and `degraded` after a restart**
A tailnet needs up to 90 seconds after a start of the daemon to authenticate. Wait 90
seconds and read the state again:
```bash
sleep 90 && sudo hydrascale status
```
A tailnet that still reports `down` after 90 seconds has a real failure. Read the events:
```bash
sudo journalctl -u hydrascale -n 50
```

**A policy write returns a permission error on Tailscale**
The OAuth client holds the scope `policy_file` alone. Add `devices:posture_attributes` and
`devices:core:read`, then write the new client identifier and client secret into the
secrets file.

**A policy write fails on Headscale**
The control server runs the file policy mode, and the answer holds the message
`update is disabled for modes other than 'database'`. Set `policy.mode: "database"` in the
Headscale configuration and restart the control server.

**The daemon refuses the secrets file**
The file grants group access or other access. Set the mode and the owner:
```bash
sudo chown root:root /etc/hydrascale/secrets.yaml
sudo chmod 0600 /etc/hydrascale/secrets.yaml
```

**Traffic of a namespace cannot reach the internet**
IP forwarding is off. Read the value and set it:
```bash
sudo sysctl net.ipv4.ip_forward          # it must print 1
sudo sysctl -w net.ipv4.ip_forward=1     # set it
```

**The host resolves no public name, and a tailscaled of the host changes the resolver**
A `tailscaled` that the host runs outside Hydrascale, with `accept-dns` on, rewrites
`/etc/resolv.conf` to `100.100.100.100` alone. When that tailnet holds no public upstream,
the host loses public DNS and the resolver of Hydrascale takes the dead upstream. Turn the
DNS management of that process off, and give `/etc/resolv.conf` back to `systemd-resolved`:
```bash
sudo tailscale set --accept-dns=false
sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
```
`hydrascale init` detects this in the preflight checks and offers to turn it off.

**`failed to listen on /var/lib/hydrascale/api.sock: no such file or directory`**
The state directory does not exist. `hydrascale init` and `hydrascale install` create it. To
create it by hand:
```bash
sudo mkdir -p /var/lib/hydrascale/state
```

**Docker stops the traffic of a namespace**
The daemon adds the `FORWARD` `ACCEPT` rules, but another rule can come first. Read the
chain:
```bash
sudo iptables -L FORWARD -v
```
Look for a `DROP` rule before the `ACCEPT` rules of Hydrascale, and remove it or move it.

**The infra subnet collides with a route**
When `10.200.0.0/16` overlaps a route on the host, the veth setup fails or the traffic goes
to the wrong place. Read the routes:
```bash
ip route | grep 10.200
```
On a match, set `infra_subnet` to a free range:
```yaml
infra_subnet: "10.201.0.0/16"
```
Then restart the daemon. It deletes the namespaces and builds them again with the new
addresses.

**`name not a valid ifname`**
An older version used the whole tailnet identifier as the interface name, which passes the
Linux limit of 15 characters. Install the current release, which uses the hash names
`vh<hash>` and `vn<hash>`.

**MagicDNS returns SERVFAIL for a tailnet name**
First read the log for `refresh_dns` after `start_daemon` on that tailnet:
```bash
journalctl -u hydrascale | grep -E 'start_daemon|refresh_dns'
```
When `refresh_dns` is absent or timed out, `tailscaled` never reached
`BackendState=Running`. The cause is a bad auth key, no network route, or a control server
that does not answer. Read `sudo hydrascale tailscale <id> -- status`. When `refresh_dns`
ran and a query still returns SERVFAIL, read the resolv.conf of the namespace; it must hold
real upstreams and never `100.100.100.100`:
```bash
cat /etc/netns/ns-<id>/resolv.conf
```
As a last step, restart the service with `sudo systemctl restart hydrascale`. The reconciler
runs the DNS refresh again on the next tick.

## License

MIT License. See [LICENSE](LICENSE) for details.
