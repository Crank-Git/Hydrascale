# Hydrascale – Final Plan

## 1. Overview
Hydrascale is a Linux‑only Go service that lets a single user run multiple Tailscale tailnets simultaneously. It achieves isolation by creating one network namespace per tailnet and launching a dedicated `tailscaled` instance inside that namespace. A unified DNS resolver aggregates all tailnet DNS servers, and routes are synced via `netlink`.

## 2. Key Design Decisions
- **Namespace naming**: `ns‑<tailnet-id>` (e.g., `ns-team-prod`).
- **DNS strategy**: Unified forwarder listening on 127.0.0.1:53, round‑robin/domain‑based forwarding to each namespace’s Tailscale DNS server.
- **Exit‑node handling**: Per‑tailnet exit node is inferred automatically from the daemon’s configuration.

## 3. Architecture Diagram
```
┌───────────────────────┐
│   hydrascale CLI      │  (Cobra)
└────────▲───────────────┘
         │
   add/remove/list     ──►  Service (systemd)
         │                     │
         ▼                     ▼
┌───────────────────────┐  ┌──────────────────────────┐
│ Namespace Manager     │  │ Tailscale Daemon Launcher│
│ (ip netns)            │  │   (tailscaled per ns)     │
└────────▲───────────────┘  └──────────────▲─────────────┘
         │                            │
         ▼                            ▼
┌───────────────────────┐  ┌──────────────────────────┐
│ Routing & Policy Engine │ │ DNS Unified Resolver     │
│ (netlink, route sync)  │ │ (UDP/TCP forwarder)     │
└────────▲───────────────┘  └──────────────────────────┘
         │                            │
         ▼                            ▼
   Namespace‑specific routes & DNS  |  Global /etc/resolv.conf (optional)
```

## 4. Core Modules
| Module | Responsibility |
|--------|----------------|
| Namespace Manager (`internal/ns`) | Create/delete namespaces, maintain mapping `<tailnetID> → <nsName>` |
| Daemon Launcher (`internal/tailscaled`) | Write per‑namespace state file, launch `tailscaled` with `--netns` or `ip netns exec`. |
| Routing Engine (`internal/routing`) | Poll `tailscaled status --json`, parse routes, inject via netlink; handle default route & exit node. |
| DNS Resolver (`internal/dns`) | Unified UDP/TCP server on 127.0.0.1:53; forwards to each namespace’s DNS server (round‑robin/domain mapping). |
| CLI (`cmd/hydrascale`) | Commands: `add <id>`, `remove <id>`, `list`, `switch <ns>`, plus `serve` for daemon mode. |
| Watcher (`internal/watcher`) | Monitor daemons, restart on crash, re‑sync routes. |
| Service Layer | systemd unit (`hydrascale.service`) with `CAP_NET_ADMIN`, runs as user `hydrascale`. |

## 5. Operational Flow
1. **Add Tailnet**: `hydrascale add <id>` → create namespace, launch daemon, sync routes, update DNS resolver.
2. **Remove Tailnet**: `hydrascale remove <id>` → stop daemon, delete namespace.
3. **Switch / Query**: `hydrascale list`; optional `switch <id>` for default namespace.
4. **Exit‑node**: inferred from daemon config; Routing Engine sets it as default route.
5. **Unified DNS**: queries forwarded to appropriate namespace DNS server.

## 6. Configuration Schema (YAML)
```yaml
# /var/lib/hydrascale/config.yaml
tailnets:
  - id: "team-prod"
    exit_node: "node1.example.com"   # optional
  - id: "devops"
    exit_node: null
resolver:
  mode: unified   # only one mode in this release
```

## 7. Testing Strategy
- **Unit**: Go `testing` for namespace ops, route insertion.
- **Integration (mocked)**: Dummy `tailscaled` emitting static JSON; verify route sync.
- **E2E on VM**: Deploy binary, add real tailnet via test account; confirm connectivity.
- **Failure Recovery**: Kill daemon, ensure watcher restarts and routes re‑apply.
- **DNS Forwarding**: `dig @127.0.0.1 example.com`; response should come from a namespace DNS server.

## 8. Deployment Outline
1. `go mod init github.com/yourorg/hydrascale`.
2. Add dependencies: Cobra, netlink, vishvananda/netlink.
3. Build binary: `go build -o hydrascale ./cmd/hydrascale`.
4. Install to `/usr/local/bin`.
5. Create systemd unit (`hydrascale.service`) with `CAP_NET_ADMIN`, run as user `hydrascale`.
6. Enable and start service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable hydrascale
sudo systemctl start hydrascale
```

## 9. Future Enhancements (TODO)
- Seccomp/SELinux hardening profiles.
- Optional per‑namespace DNS stub for isolation.
- Web UI/dashboard to visualize tailnets, routes, and DNS tables.
- Metrics & structured logging for observability.

---
*This file contains the consolidated plan. The next step is to begin implementation following the outlined modules and workflow.*
