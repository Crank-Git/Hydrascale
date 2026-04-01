# Hydrascale

Run multiple Tailscale tailnets simultaneously on a single Linux machine.

## What It Does

Hydrascale lets a single Linux host participate in multiple Tailscale tailnets at the same time. It creates an isolated network namespace for each tailnet and launches a dedicated `tailscaled` instance inside it, so traffic from one tailnet never leaks into another. Overlapping IP ranges, independent firewall rules, and separate routing tables all work out of the box because every tailnet gets its own network stack.

You declare the tailnets you want in a YAML config file and Hydrascale continuously reconciles the running system toward that desired state. Add a tailnet to the config and it appears; remove it and Hydrascale tears down the namespace, stops the daemon, and cleans up routes. A unified DNS resolver aggregates name resolution across all active tailnets so you can reach any peer by hostname regardless of which tailnet it belongs to.

The reconciler runs as a control loop: on each tick it reads the config, inspects the live system, computes a diff, and applies the minimum set of actions needed. Tailnets that fail repeatedly are placed into an error state and skipped until explicitly reset, preventing a single broken tailnet from disrupting the rest. The event log records every action for debugging and future API use.

## Requirements

- **Linux** (network namespaces are a Linux kernel feature)
- **Go 1.24+** (for building from source)
- **Root or CAP_NET_ADMIN** capability
- **Tailscale installed** (`tailscaled` and `tailscale` commands available in `$PATH`)

## Install

### Binary Download

Download a pre-built binary from the [GitHub Releases](https://github.com/yourorg/hydrascale/releases) page:

```bash
tar xzf hydrascale_*.tar.gz
sudo install hydrascale /usr/local/bin/
```

### Build from Source

```bash
go install hydrascale/cmd/hydrascale@latest
```

Or clone and build manually:

```bash
git clone https://github.com/yourorg/hydrascale.git
cd hydrascale
go build -o hydrascale ./cmd/hydrascale
sudo install hydrascale /usr/local/bin/
```

## Quick Start

1. Create a config file at `/var/lib/hydrascale/config.yaml`:

```yaml
version: 2
tailnets:
  - id: corp-prod
    auth_key: tskey-auth-xxxxx   # optional, for unattended auth
  - id: homelab
    exit_node: exit-us.example.com
resolver:
  mode: unified
reconciler:
  interval: 10s
```

2. Apply the config (one-shot):

```bash
sudo hydrascale apply
```

3. Or run as a daemon with continuous reconciliation:

```bash
sudo hydrascale serve
```

## Config Reference

```yaml
# Config schema version (auto-migrated from v1 if omitted)
version: 2

# List of tailnets to manage
tailnets:
  - id: "corp-prod"              # unique identifier (alphanumeric, dots, hyphens, underscores; max 63 chars)
    exit_node: "node1.example.com" # optional exit node hostname
    auth_key: "tskey-auth-xxxxx"   # optional auth key for unattended setup

# DNS resolver settings
resolver:
  mode: unified                  # aggregates DNS across all tailnets
  bind_address: "127.0.0.1:53"  # optional, defaults to 127.0.0.1:53

# Reconciler settings
reconciler:
  interval: 10s                  # how often the control loop runs (Go duration)
```

## CLI Commands

```
hydrascale add <id>        Add a tailnet to config and reconcile
hydrascale remove <id>     Remove a tailnet from config and reconcile
hydrascale list            List all configured tailnets
hydrascale switch <id>     Switch the default namespace for direct tailscale CLI usage
hydrascale diff            Show what would change without applying
hydrascale apply           One-shot reconciliation (apply config to system)
hydrascale status          Show desired vs actual state for all tailnets
hydrascale serve           Start daemon mode (continuous reconciliation loop)
```

Use `--config <path>` on any command to override the default config location (`/var/lib/hydrascale/config.yaml`).

## Daemon Mode

### systemd Setup

```bash
sudo mkdir -p /var/lib/hydrascale
sudo cp contrib/hydrascale.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hydrascale
```

The provided unit file (`contrib/hydrascale.service`) runs Hydrascale with minimal privileges using ambient capabilities and systemd sandboxing.

### SIGHUP Reload

The daemon re-reads the config file on every reconciliation tick, so config changes take effect within one interval. A future release will add explicit SIGHUP handling for immediate reload.

### Graceful Shutdown

Sending SIGINT or SIGTERM causes the daemon to cancel the reconciliation loop and exit cleanly. Namespaces and daemons are left running so tailnet connectivity is preserved across restarts.

### Monitoring

```bash
sudo systemctl status hydrascale
sudo journalctl -u hydrascale -f
```

## Architecture

```
                      +-----------------------+
                      |    config.yaml        |
                      |  (desired state)      |
                      +-----------+-----------+
                                  |
                                  v
                      +-----------+-----------+
                      |     Reconciler        |
                      |  load config          |
                      |  query actual state   |
                      |  compute diff         |
                      |  apply actions        |
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

Each reconciliation cycle:
1. **Load** desired state from `config.yaml`
2. **Query** actual state: which namespaces exist, which daemons are healthy, which routes are installed
3. **Diff** desired vs actual to produce a list of actions (create/delete namespace, start/stop daemon, sync routes)
4. **Apply** actions in order; track per-tailnet failure counts
5. After 3 consecutive failures, a tailnet enters **error state** and is skipped until reset

The reconciler acquires a file lock before each cycle to prevent concurrent mutations.

## License

MIT License. See [LICENSE](LICENSE) for details.
