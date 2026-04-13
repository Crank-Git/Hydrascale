# Per-Tailnet MagicDNS Routing Design

**Date:** 2026-04-13
**Issue:** #17 — 100.100.100.100 last-write-wins across multiple tailnets

## Problem

`SetupVeth` runs `ip route replace 100.100.100.100 via <nsGW> dev <hostVeth>` for every namespace. The last namespace to set up its veth owns the host route to MagicDNS. DNS queries from the host only reach one tailnet's MagicDNS resolver regardless of which tailnet the queried hostname belongs to.

## Approach: Wire the DNS Forwarder to per-tailnet veth endpoints

Hydrascale's forwarder already supports `SetDomainRoutes(map[string]string)` for per-domain upstream routing. Each namespace already has a DNAT rule that redirects UDP/53 arriving on its veth to `100.100.100.100:53` inside that namespace. So `vethGW:53` (e.g., `10.200.0.2:53`) is already that tailnet's MagicDNS endpoint — the kernel handles the namespace hop.

### DNS Query Flow After Fix

```
host process
  → Hydrascale forwarder (already owns DNS)
    → matches "corp.ts.net" suffix → routes to 10.200.0.2:53 (corp-prod vethGW)
      → namespace DNAT: 100.100.100.100:53 inside corp-prod namespace
        → MagicDNS answers with correct Tailscale IP
```

The `ip route replace 100.100.100.100` host route is no longer added — the forwarder owns per-tailnet DNS dispatch.

## Components Changed

### 1. `internal/hostaccess/hostaccess.go`

- Define `DNSForwarder` interface: `SetDomainRoutes(map[string]string)`
- Add `forwarder DNSForwarder` field to `Manager`
- Add `WithForwarder(f DNSForwarder)` functional option (forwarder is optional; if nil, skip SetDomainRoutes)
- In `syncDNS()`: build `map[string]string{magicDNSSuffix → nsGW+":53"}` for all active tailnets, call `forwarder.SetDomainRoutes(routes)`
- In `SetupVeth()`: remove `ip route replace 100.100.100.100` call

### 2. `cmd/hydrascale/main.go`

- In `serveCmd`: pass the already-constructed forwarder to the hostaccess manager via `WithForwarder`

## Interface

```go
// DNSForwarder is defined in the hostaccess package to avoid import cycles.
type DNSForwarder interface {
    SetDomainRoutes(routes map[string]string)
}
```

`*dns.Forwarder` satisfies this interface. No changes needed to `internal/dns/forwarder.go`.

## Error Handling

- If no forwarder is wired (`WithForwarder` not called): `syncDNS` skips `SetDomainRoutes`. Existing resolved/system-DNS mode continues unchanged.
- If a tailnet has no `MagicDNSSuffix`: skip that tailnet in the routes map (already the case — empty strings make bad map keys).
- Route updates are non-destructive: each sync cycle replaces the entire map atomically (forwarder holds a mutex).

## Testing

- `TestSyncDNS_SetsDomainRoutes`: mock `DNSForwarder`, verify `SetDomainRoutes` called with correct suffix→vethGW mapping
- `TestSyncDNS_NoForwarder`: nil forwarder wired, verify no panic, existing behavior unchanged
- `TestSyncDNS_EmptySuffix`: tailnet with empty MagicDNSSuffix excluded from routes map
