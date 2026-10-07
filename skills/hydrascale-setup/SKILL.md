---
name: hydrascale-setup
description: Read the Hydrascale state on a host and report it, and print each command that changes the host. Use when asked to set up Hydrascale, to explain why a tailnet is down, or to change the configuration file.
allowed-tools: Read, Bash(sudo hydrascale status:*), Bash(sudo hydrascale diff:*), Bash(sudo hydrascale list:*), Bash(sudo hydrascale env:*), Bash(hydrascale status:*), Bash(hydrascale version:*)
---

# Set up Hydrascale

Hydrascale joins one Linux host to several tailnets. The daemon holds one namespace per
tailnet, and it drives the host toward the state that `/etc/hydrascale/config.yaml`
declares.

**Warning — one command that changes the host can disconnect every tailnet.** Print such a
command for the operator. Run none of them.

## The commands you run

The `allowed-tools` field above names five read-only commands. None of them changes the
host.

| Command | Result | `sudo` |
|---|---|---|
| `sudo hydrascale status` | The desired state and the actual state of each tailnet. | Yes, unless the account is a member of `socket_group` and the daemon runs. |
| `sudo hydrascale diff` | The difference between the desired state and the actual state, and the difference of the local rule chains. | Yes. |
| `sudo hydrascale list` | The ID and the alias of each tailnet in the configuration file. | Yes. |
| `sudo hydrascale env <id>` | The shell lines for the namespace of one tailnet. | Yes. |
| `hydrascale version` | The version of the binary. | No. |

These commands need `sudo` for three reasons:

- The daemon writes `/etc/hydrascale/config.yaml` at mode `0600`. `list`, `env`, and
  `diff` read that file.
- `diff` also reads the iptables chains, which needs root.
- The control socket `/var/lib/hydrascale/api.sock` has the mode `0600` when no
  `socket_group` is set. `status` reads that socket.

`status` asks the daemon through the control socket first. If the socket does not answer,
`status` reads the configuration file and the host directly. A member of `socket_group`
can run `hydrascale status` without `sudo` while the daemon runs.

Read the state in this order:

1. Run `hydrascale version`. It states the version that the host runs.
2. Run `sudo hydrascale list`. It reads the configuration file, so it names a tailnet that
   holds no namespace.
3. Run `sudo hydrascale status`. It names each tailnet that is down.
4. Run `sudo hydrascale diff`. It names each action that a reconciliation performs.

## The output of `hydrascale status`

`status` prints one row per tailnet with the columns `ID`, `ALIAS`, `NAMESPACE`, `DAEMON`,
`STATE`, and `ERROR`. The `ALIAS` column shows `-` when the tailnet holds no alias.

| `DAEMON` | `STATE` | Meaning |
|---|---|---|
| `healthy` | `running` | The namespace exists and its `tailscaled` answers. |
| `down` | `degraded` | The namespace exists, but its `tailscaled` does not answer. |
| `absent` | `pending` | The configuration file declares the tailnet, but the host holds no namespace for it. |
| `stopped` | `paused` | The operator disconnected the tailnet. |
| `orphan` | `removing` | The host holds a namespace that the configuration file does not declare. |

The `STATE` value `ERROR` replaces `running`, `degraded`, or `pending` when the last
reconciliation of the tailnet failed. The `ERROR` column then holds the message.

A tailnet needs about 20 seconds after a restart of the service before `status` reports
`healthy` and `running`. An earlier read reports `down` and `degraded`. Report that state
as normal rather than as a failure.

## Print a command that changes the host. Run none.

The operator runs every command that changes the host or a tailnet. Print the command, the
precondition, and the risk. Then stop.

These commands change the host or a tailnet:

- `sudo hydrascale apply` reconciles the host once. `hydrascale apply --dry-run` prints the
  actions only, but run `sudo hydrascale diff` instead, because it prints the same
  difference.
- `sudo hydrascale add <id>` adds a tailnet to the configuration file and reconciles.
- `sudo hydrascale remove <id>` removes a tailnet from the configuration file and
  reconciles.
- `sudo hydrascale init` runs the setup wizard.
- `sudo hydrascale init --force` replaces a configuration file that exists. It keeps the
  old file as a `.bak` copy.
- `sudo hydrascale uninstall` removes the service, the namespaces, the rules, and the host
  routes. It keeps `/etc/hydrascale`. It takes three flags:
  - `--yes` skips the confirmation.
  - `--purge` also removes the binary and `/etc/hydrascale`.
  - `--keep-nodes` keeps each tailnet node. Without this flag, the command logs each node
    out of its tailnet.
- `sudo hydrascale wrap <service> <id> --apply` installs a systemd drop-in that runs a
  service in the namespace of one tailnet. Without `--apply`, `wrap` prints the drop-in
  only.
- `sudo hydrascale tui` opens the terminal interface. Its keys add, connect, disconnect,
  and remove a tailnet.
- `sudo hydrascale install` writes the systemd unit, and `sudo hydrascale serve` runs the
  daemon.
- `sudo systemctl start hydrascale`, `sudo systemctl stop hydrascale`,
  `sudo systemctl restart hydrascale`, and `sudo systemctl reload hydrascale`.
- An edit of `/etc/hydrascale/config.yaml` or of `/etc/hydrascale/secrets.yaml`.

These console actions also change the host or a tailnet. Name the view and the action for
the operator. Do not use the console yourself.

- **Connect** in the Namespaces view starts a tailnet again and reconciles the host.
- **Disconnect** in the Namespaces view stops a tailnet.
- **Remove** in the Namespaces view removes a tailnet.
- **Apply** in the Access view writes the local rules to the host.
- **Push** in the Policy view replaces the policy document on the control server.

**Warning — Push changes the policy of every device in the tailnet, not only this host.**
State this risk before you name Push.

## The configuration keys

The operator sets these keys in `/etc/hydrascale/config.yaml` most often:

| Key | Effect |
|---|---|
| `tailnets[].alias` | A second name of the tailnet. Every command that takes a tailnet ID also takes the alias. |
| `resolver.resolve_aliases` | When `true`, the resolver answers `<host>.<alias>.ts.internal` for each tailnet that holds an alias. |
| `route_table` | The routing table that holds every host route of the daemon. When the key is absent, the daemon writes into the main table. |
| `ipv6` | When `true`, the daemon sets `net.ipv6.conf.all.forwarding` on a kernel older than Linux 6.17. |
| `host_dns.mode` | `hosts` writes the peer names into `/etc/hosts`. `resolved` registers the peer domains with `systemd-resolved`. |
| `socket_group` | The Unix group that reaches the control socket without root. |

**Warning — membership of `socket_group` gives the access of root.** A member sends a
command to the daemon, and the daemon runs as root. State this risk before you print a
change to `socket_group`. See
https://crank-git.github.io/Hydrascale/operations/remote-access/.

The configuration page states every other key:
https://crank-git.github.io/Hydrascale/reference/configuration/.

## The console

The daemon serves the console at `http://127.0.0.1:9443` by default. The console listener
binds a loopback address only, and the daemon refuses any other address.

**Warning — the console has no authentication.** Any local account on the host can reach
the console and drive the root daemon. See
https://crank-git.github.io/Hydrascale/security/console/.

An SSH tunnel reaches the console from another machine. Print this command for the
operator, and print the address `http://127.0.0.1:9443` for the browser:

```sh
ssh -L 9443:127.0.0.1:9443 <host>
```

The tunnel works on any free local port, because the daemon accepts an `Origin` header
that names a loopback host on any port. If the local port 9443 is in use, replace the first
`9443` with a free port, and use that port in the browser address.

## Upgrade from version 0.9 or 0.10

This section applies only to a host that still runs version 0.9 or version 0.10. A host
that runs version 1.0 or later does not need it. `hydrascale version` states the version.

**Warning — a configuration file that holds no `access` block loses every path between two
tailnets under the mode `enforce`.** State this risk before you print a command that
starts version 1.0 or later on such a host.

At the first start, the daemon writes a preserving rule set and records the event
`access.migrated`. That rule set carries each tailnet to the internet. It carries no
traffic from one tailnet to another tailnet.

The daemon detects the migration by the absence of the `access` key. An `access` block
that the operator writes before the first start therefore suppresses the migration.

The operator sets the mode `observe` first. The mode `observe` writes a kernel log line for
a packet that no rule allows, and it then accepts that packet. Print this order:

1. Start the service with `sudo systemctl start hydrascale`.
2. Confirm the event `access.migrated` in the journal.
3. Set `access.mode: observe` in `/etc/hydrascale/config.yaml`.
4. Apply the mode with `sudo systemctl reload hydrascale`.
5. Use the host for a day.
6. Read the would-deny log lines.
7. Add one local rule for each path that the log names.
8. Set `access.mode: enforce` only after the log names no further path.

The upgrade page holds both orders and the full procedure. Read it before you print an
upgrade step: https://crank-git.github.io/Hydrascale/operations/upgrade/.
