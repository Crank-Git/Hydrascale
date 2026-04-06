# Host Access: Transparent Access to Tailnet Peers from Host

**Date:** 2026-04-05
**Status:** Approved

## Problem

Hydrascale runs each tailnet in an isolated network namespace. To reach a peer, users must go through `hydrascale exec`, `hydrascale ping`, or similar namespace-scoped commands. The long-standing request is for the host machine to transparently access peers on all managed tailnets — `ping havoc-mars` just works — without any data mixing between tailnets.

## Config

```yaml
version: 2

# Global default — false if omitted
host_access: false

tailnets:
  - id: havoc
    host_access: true    # per-tailnet override
  - id: personal
    # inherits global default (false)

# DNS integration mode for host access
host_dns:
  mode: hosts            # default — manages /etc/hosts entries
  # mode: resolved       # opt-in — registers with systemd-resolved via D-Bus
```

### Rules

- `host_access` at the top level defaults to `false` if omitted (backward compatible).
- Per-tailnet `host_access` overrides the global value when set.
- No host access behavior unless explicitly enabled.
- `host_dns.mode` defaults to `hosts` when `host_access` is enabled for any tailnet. Ignored if no tailnet has `host_access` enabled.

## Architecture

### Approach: Reconciler + Dedicated Module (Hybrid)

The reconciler calls into a new `internal/hostaccess/` package during each cycle. The `hostaccess` package owns all host access logic. The reconciler remains a thin orchestrator.

```
Reconciler.Reconcile()
  → for each tailnet with host_access enabled:
      hostaccess.Sync(tailnetID, statusJSON)
  → for removed/disabled tailnets:
      hostaccess.Teardown(tailnetID)
```

### Package Layout

```
internal/
  hostaccess/
    hostaccess.go       — Manager struct, Sync() and Teardown() entry points
    peers.go            — parse tailscale status JSON → peer map
    peers_test.go       — peer parsing tests (5 cases)
    routes.go           — host route add/replace/delete operations
    routes_test.go      — route sync tests (4 cases)
    hosts.go            — /etc/hosts managed block read/write
    hosts_test.go       — hosts file tests (6 cases)
    resolved.go         — systemd-resolved D-Bus integration
    resolved_test.go    — D-Bus integration tests (3 cases)
    hostaccess_test.go  — integration tests (4 cases)
```

Note: namespace-side iptables (masquerade, DNS DNAT, `/etc/netns/` resolv.conf) are consolidated in `internal/namespaces/ns.go` alongside existing veth iptables rules, not in this package.

### Modified Existing Code

- `internal/config/config.go` — add `HostAccess` bool to top-level and per-tailnet config, add `HostDNS` struct.
- `internal/daemon/daemon.go` — add new `GetStatus(nsName, tailnetID string) (*TailscaleStatus, error)` method to the `Manager` interface. `CheckHealth` remains unchanged. This avoids breaking existing callers and mocks.
- `internal/namespaces/ns.go` — extend `SetupVeth` with optional host access iptables rules (masquerade on tailscale0, DNS DNAT on veth, `/etc/netns/` resolv.conf). All namespace-side iptables consolidated in this file.
- `internal/reconciler/reconciler.go` — new action types, calls `hostaccess.Sync()` and `hostaccess.Teardown()` during cycle. `Shutdown()` extended to call `hostaccess.TeardownAll()` for graceful cleanup.

## Components

### 1. Peer Map

Parses `tailscale status --json` (already queried by the reconciler for health checks) to build a map of peers per tailnet.

```go
type Peer struct {
    Hostname string   // "mars"
    IPv4     string   // "100.98.107.70"
    IPv6     string   // "fd7a:115c:a1e0::1" (may be empty)
    Online   bool
}

type TailnetPeers struct {
    TailnetID      string
    MagicDNSSuffix string   // "taildf854a.ts.net"
    Peers          []Peer
    VethGateway    string   // "10.200.22.2"
    VethHost       string   // "vh5cde1b791fe1"
}
```

Refreshed every reconcile cycle. Data comes from a new `GetStatus()` method on the daemon Manager interface (separate from `CheckHealth` to avoid breaking existing callers/mocks). No additional `tailscale status --json` queries — `GetStatus` reuses the same underlying call.

### 2. Host Route Management

For each peer IP, host routes are added via the namespace's veth gateway (both IPv4 and IPv6):

```
ip route replace 100.98.107.70 via 10.200.22.2 dev vh5cde1b791fe1
ip -6 route replace fd7a:115c:a1e0::1 via <ipv6-gw> dev vh5cde1b791fe1
```

- Uses `replace` for idempotency — safe to run every cycle.
- Routes removed when peers disappear or host_access is disabled.
- Full sync each cycle: compare desired (peer map) vs actual (host route table). Add missing, remove stale.

**Route isolation:** When listing host routes for the stale-route cleanup, only consider routes that match BOTH a Hydrascale veth device name (`vh*`) AND the Tailscale CGNAT range (100.64.0.0/10 for IPv4, fd7a:115c:a1e0::/48 for IPv6). This prevents accidental deletion of infrastructure routes like the `100.100.100.100` MagicDNS route created by `SetupVeth`.

### 3. Namespace-Side Setup (One-Time When Enabled)

These rules are added in `internal/namespaces/ns.go` alongside existing veth iptables, conditional on host_access being enabled. Three iptables/config changes inside the namespace:

**Masquerade on tailscale0:** Makes host traffic appear as local tailscale traffic so tailscaled forwards it to peers.
```
ip netns exec ns-havoc iptables -t nat -A POSTROUTING -s 10.200.N.0/30 -o tailscale0 -j MASQUERADE
ip netns exec ns-havoc ip6tables -t nat -A POSTROUTING -s <ipv6-subnet> -o tailscale0 -j MASQUERADE
```

**DNS DNAT on veth:** Forwards DNS queries arriving on the veth to MagicDNS.
```
ip netns exec ns-havoc iptables -t nat -A PREROUTING -i vn<hash> -p udp --dport 53 -j DNAT --to 100.100.100.100
ip netns exec ns-havoc iptables -t nat -A PREROUTING -i vn<hash> -p tcp --dport 53 -j DNAT --to 100.100.100.100
```

**Per-namespace resolv.conf:** Creates `/etc/netns/NAME/resolv.conf` with `nameserver 100.100.100.100`. The `ip netns exec` convention bind-mounts this over `/etc/resolv.conf` for processes in the namespace, enabling MagicDNS where the kernel supports it.

All iptables rules use `-C` (check) before `-A` (append) for idempotency. These are checked each cycle but only inserted if missing.

### 4. DNS Resolution

**Naming convention:** Each peer gets a prefixed short name: `<tailnetid>-<hostname>`. This avoids collisions when multiple tailnets have peers with the same hostname.

- `havoc-mars` → `100.98.107.70`
- `personal-nas` → `100.64.5.3`

FQDNs (`mars.taildf854a.ts.net`) are forwarded to MagicDNS via the namespace's DNS DNAT rules. This works on systems with full iptables/kernel support. On systems where MagicDNS doesn't function (e.g., Tegra/Jetson without `xt_connmark`), FQDN resolution fails gracefully while prefixed short names continue to work.

### 5. Host DNS Integration

Two mutually exclusive modes:

**`hosts` mode (default):**

Hydrascale manages a clearly marked block in `/etc/hosts`:

```
# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT
100.98.107.70  havoc-mars
fd7a:115c:a1e0::1  havoc-mars
100.73.198.12  havoc-bigboy
100.119.89.27  havoc-ns1
100.64.5.3     personal-nas
# END HYDRASCALE MANAGED BLOCK
```

Written only when the managed block content changes (diff before write). Atomic write via temp file + rename. Works on every Linux system.

**Coexistence with existing DNS forwarder:** The existing `internal/dns/forwarder.go` handles FQDN domain-based routing and runs on `127.0.0.53:5354`. `/etc/hosts` handles the prefixed short names (`havoc-mars`). These coexist via standard nsswitch.conf ordering (`files dns` — `/etc/hosts` is checked first, then DNS resolvers). No conflict.

**`resolved` mode (opt-in):**

Registers with systemd-resolved via D-Bus as a DNS provider for routing domains (`.ts.net` suffixes and Hydrascale short names). No `/etc/hosts` modification. Only works on systems with systemd-resolved.

### 6. Teardown & Lifecycle

When `host_access` is disabled for a tailnet (config change or removal):

1. Remove all host routes (`100.x.x.x`) pointing via that tailnet's veth
2. Remove namespace iptables rules (tailscale0 masquerade, DNS DNAT)
3. Remove that tailnet's entries from `/etc/hosts` (or deregister from systemd-resolved)
4. Clean up `/etc/netns/NAME/resolv.conf`

When Hydrascale shuts down entirely:

- All host routes for managed peers are removed
- `/etc/hosts` managed block is cleaned up
- systemd-resolved registrations are deregistered (if in resolved mode)

The reconciler calls `hostaccess.Teardown(tailnetID)` alongside existing `DeleteNS`/`StopDaemon` actions.

## Data Flow

```
tailscale status --json (already queried for health checks)
        │
        ▼
   peers.go: extract hostname, IP, MagicDNSSuffix per tailnet
        │
        ├──▶ routes.go: ip route replace 100.x.x.x via veth gateway
        │
        ├──▶ hosts.go: write managed block to /etc/hosts
        │    OR resolved.go: register with systemd-resolved
        │
        └──▶ netns.go: masquerade + DNS DNAT + /etc/netns/ resolv.conf
                        (one-time setup, idempotent)
```

## Error Handling

- Host route failures are logged but do not fail the reconcile cycle — other tailnets proceed.
- `/etc/hosts` write failures are logged as errors. The file is written atomically (write temp, rename).
- Namespace iptables failures are logged but non-fatal (same pattern as existing FORWARD rules).
- systemd-resolved D-Bus failures in `resolved` mode are logged; user is advised to switch to `hosts` mode.

## Testing

Tests split per file (~25 cases total):

**peers_test.go (5 cases):**
- Parse valid JSON with multiple peers (IPv4 + IPv6)
- Empty peer list
- Offline peers (Online=false)
- Malformed JSON input
- Missing MagicDNSSuffix field

**routes_test.go (4 cases):**
- Add new host routes (v4 and v6)
- Remove stale routes
- No-op when routes match desired state
- Route command failure (logged, non-fatal)

**hosts_test.go (6 cases):**
- Insert new managed block into /etc/hosts
- Update existing managed block
- Remove managed block (empty records)
- Preserve non-managed content
- Skip write when block unchanged
- Atomic write failure handling

**resolved_test.go (3 cases):**
- D-Bus connection and domain registration
- D-Bus unavailable (no systemd-resolved)
- Deregister on teardown

**hostaccess_test.go (4 cases):**
- Full Sync: peers → routes → DNS
- host_access=false (no-op)
- Partial failure (route fails, DNS succeeds)
- Teardown idempotent when nothing was set up

**Existing test modifications:**
- `namespaces/ns_test.go` — test SetupVeth with hostAccess=true (masquerade, DNAT, resolv.conf)
- `reconciler/reconciler_test.go` — extend to cover hostaccess.Sync/Teardown calls and Shutdown cleanup

## Compatibility

- **Normal Linux distros:** Full functionality — host routes, DNS, MagicDNS via namespace resolv.conf.
- **Tegra/Jetson (missing xt_connmark):** Host routes and prefixed short names work. MagicDNS FQDN resolution may not work due to kernel limitations. Prefixed short names (`havoc-mars`) provide the primary resolution path.
- **Systems without systemd-resolved:** `hosts` mode works everywhere. `resolved` mode unavailable.
