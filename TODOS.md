# Hydrascale TODOs

## Phase 2: Mesh Mode

**What:** Add optional cross-tailnet routing via veth pairs with iptables policy rules.

**Why:** Namespace isolation is the core value, but some users need selective cross-tailnet communication. Mesh mode bridges namespaces with fine-grained allow rules (port/protocol/direction).

**Details:**
- New `internal/mesh` package: Bridge(), Unbridge(), Policy struct
- Veth pairs with /30 subnets for DNAT between namespaces
- Asymmetric allow semantics (outbound-only per config block)
- iptables FORWARD chain per veth direction
- Reconciler integrates mesh as part of its Diff/Apply loop
- V1 limitation: identical Tailscale IPs across meshed tailnets unsupported
- nftables vs iptables: start with iptables, add nftables later

**Design doc:** `~/.gstack/projects/Crank-Git-Hydrascale/e-main-design-20260330-102333.md` (Phase 2 section)

**Depends on:** Phase 1 reconciler (complete)

---

## Phase 3: Observability API + Web Dashboard

**What:** HTTP JSON API, Prometheus /metrics endpoint, and a web dashboard showing desired-vs-actual state in real time.

**Why:** The reconciler makes the invisible visible. The dashboard is the glass that lets you see every tailnet, route, and daemon health status at a glance.

**Details:**
- `internal/api` package: HTTP JSON on unix socket (optional TCP)
- GET /status, GET /events, GET /diff endpoints
- Prometheus metrics: tailnets_active, daemons_healthy, reconcile_runs_total, reconcile_duration_seconds
- Web dashboard (embedded SPA or htmx) showing desired vs actual state
- Real-time updates via WebSocket or SSE

**Design doc:** `~/.gstack/projects/Crank-Git-Hydrascale/e-main-design-20260330-102333.md` (Phase 3 section)

**Depends on:** Phase 1 reconciler (complete)

---

## Distribution Pipeline

**What:** Automated build and release pipeline for static binaries.

**Why:** Code without distribution is code nobody can use. Users need a one-command install.

**Details:**
- goreleaser config for automated GitHub Releases
- Target platforms: linux/amd64, linux/arm64
- GitHub Actions workflow for release on tag push
- `go install` path for building from source
- AUR package (stretch goal, large homelab overlap)
- Debian/RPM packages (stretch goal)

**Depends on:** Phase 1 reconciler (complete)
