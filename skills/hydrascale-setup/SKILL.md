---
name: hydrascale-setup
description: Read the Hydrascale state on a host and report it, and print each command that changes the host. Use when asked to set up Hydrascale, to explain why a tailnet is down, or to change the configuration file.
allowed-tools: Read, Bash(hydrascale status:*), Bash(hydrascale list:*), Bash(hydrascale diff:*), Bash(hydrascale env:*), Bash(hydrascale version:*), Bash(sudo hydrascale status:*), Bash(sudo hydrascale list:*), Bash(sudo hydrascale diff:*)
---

# Set up Hydrascale

Hydrascale joins one Linux host to several tailnets. The daemon holds one namespace per
tailnet, and it drives the host toward the state that `/etc/hydrascale/config.yaml`
declares.

**Warning — one command that changes the host disconnects every tailnet.** Print such a
command for the operator. Run none of them.

## The commands you run

The `allowed-tools` block above holds five commands. Each command reads the state, and
none of them changes the host.

| Command | Result |
|---|---|
| `hydrascale status` | The desired state and the actual state of each tailnet. |
| `hydrascale list` | The identifier of each tailnet in the configuration file. |
| `hydrascale diff` | The difference between the desired state and the actual state. |
| `hydrascale env <id>` | The environment values for the namespace of one tailnet. |
| `hydrascale version` | The version of the binary. |

The daemon holds the control socket `/var/lib/hydrascale/api.sock` at mode `0600`, so
`hydrascale status` needs `sudo` on a host that names no socket group.

Read the state in this order:

1. Run `hydrascale version`. It names the release that the host runs.
2. Run `hydrascale list`. It reads the configuration file, so it names a tailnet that
   holds no namespace.
3. Run `hydrascale status`. It names each tailnet that is down.
4. Run `hydrascale diff`. It names each action that a reconciliation performs.

A tailnet needs about 20 seconds after a restart of the service before `hydrascale status`
reports `healthy` and `running`. An earlier read reports `down` and `degraded`. Report
that state as normal rather than as a failure.

## Print a mutating command. Run none.

The operator runs every command that changes the host. You print the command, the
precondition, and the risk.

These commands and edits change the host:

- `hydrascale apply`, `hydrascale add <id>`, and `hydrascale remove <id>`.
- `hydrascale install`, and `hydrascale serve`.
- `sudo systemctl start hydrascale`, `sudo systemctl stop hydrascale`, and
  `sudo systemctl reload hydrascale`.
- An edit of `/etc/hydrascale/config.yaml` or of `/etc/hydrascale/secrets.yaml`.

`hydrascale apply --dry-run` prints the actions of a reconciliation, but `apply` is a
mutating command. Run `hydrascale diff` instead. It prints the same difference.

When the operator asks for a command that stops a tailnet, print the command and the
risk. The operator decides.

## The mode `enforce` and the `access` block

**Warning — a configuration file that holds no `access` block loses every path between two
tailnets under the mode `enforce`.** State this risk before you print a command that
starts version 1.0.

The daemon writes a preserving rule set on the first start of version 1.0, and it records
the event `access.migrated`. That rule set carries each tailnet to the internet. It
carries no traffic from one tailnet to another tailnet.

The operator sets the mode `observe` first. The mode `observe` writes a kernel log line
for a packet that no rule allows, and it then accepts that packet. The mode `enforce`
denies that packet. Print this order:

1. Start the service with `sudo systemctl start hydrascale`.
2. Confirm the event `access.migrated` in the journal.
3. Set `access.mode: observe` in `/etc/hydrascale/config.yaml`.
4. Apply the mode with `sudo systemctl reload hydrascale`.
5. Use the host for a day, then read the would-deny log lines.
6. Add one rule for each path that the log names.
7. Set `access.mode: enforce` only after the log names no further path.

The daemon detects the migration by the presence of the `access` key.
`internal/config/migrate.go:72` returns early when the configuration file already holds an
`access` block. An `access` block that the operator writes before the first start
therefore suppresses the migration. `docs/UPGRADING.md` holds both orders and the full
procedure. Read it before you print an upgrade step.

## The console

`internal/api/console.go:15` states the risk of the console:

> The console has no authentication. Any local account on the host can reach the console
> listener and can drive a root daemon.

The console listener binds a loopback address only, and `StartConsole` refuses and logs
any other address. `internal/config/console.go:10` holds the default address
`127.0.0.1:9443`.

An SSH forward reaches the console from another machine. Print this command, and print the
address `http://127.0.0.1:9443` for the browser of the operator:

```sh
ssh -L 9443:127.0.0.1:9443 <host>
```
