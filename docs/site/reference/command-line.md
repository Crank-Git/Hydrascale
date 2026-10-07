# The command line

```
hydrascale add <id>                   Add a tailnet to the config and reconcile
hydrascale apply                      Reconcile once
hydrascale diff                       Show what a reconcile would change
hydrascale env <tailnet-id>           Print the exports and the shell function hstn for a tailnet
hydrascale exec <tailnet-id> -- <cmd> Run a command inside the namespace of a tailnet (needs root)
hydrascale init                       Run the first-run wizard
hydrascale install                    Install the daemon as a systemd service
hydrascale list                       List the configured tailnets
hydrascale ping <tailnet-id> <target> Ping a peer from inside the namespace of a tailnet
hydrascale remove <id>                Remove a tailnet from the config and reconcile
hydrascale serve                      Run the daemon and the control loop
hydrascale skills install             Write the agent skills to the skill directory
hydrascale skills list                List the name and the description of each skill
hydrascale ssh  <tailnet-id> <target> Open an SSH session to a peer through a namespace
hydrascale status                     Show the declared state and the live state
hydrascale switch <id>                Print the namespace name for a tailnet (changes no state)
hydrascale tailscale <tailnet-id> -- <args>
                                      Run a tailscale command inside the namespace of a tailnet
hydrascale tui                        Open the terminal interface, which needs a running daemon
hydrascale uninstall                  Remove Hydrascale from the host
hydrascale version                    Print the version
hydrascale wrap <service> <tailnet-id>
                                      Write a systemd drop-in that isolates a service
```

## Flags

Pass `--config <path>` on any command to name another configuration file. The default is
`/etc/hydrascale/config.yaml`, which the systemd unit also passes.

| Command | Flag | Effect |
|---|---|---|
| `apply` | `--dry-run` | Prints the planned actions and runs none of them. |
| `init` | `--force` | Writes over a configuration file that exists, after a copy to `.bak`. It also turns the failure of the `accept-dns` preflight check into a warning. |
| `install` | `--dry-run` | Prints each step and changes nothing. |
| `skills install` | `--dir <path>` | Writes to this directory rather than to `$HOME/.claude/skills`. |
| `skills install` | `--force` | Writes over a skill file that exists. |
| `skills install` | `--dry-run` | Prints each path and writes no file. |
| `uninstall` | `--yes` | Skips the confirmation. |
| `uninstall` | `--purge` | Also removes the binary and `/etc/hydrascale`. |
| `uninstall` | `--keep-nodes` | Keeps each tailnet node, and logs no node out. |
| `wrap` | `--apply` | Writes the drop-in file into the systemd directory. It needs root. |

## A tailnet alias

Each command that takes a `<tailnet-id>` also takes the `alias` of that tailnet. An alias
is unique, and it is the identifier of no tailnet. `hydrascale list` prints the alias, and
`hydrascale status` shows it in the column `ALIAS`.

The commands `exec`, `ping`, `ssh`, `tailscale`, `wrap`, and `env` read the configuration
file to resolve the `<tailnet-id>` argument, which is an identifier or an alias. If only
root can read `/etc/hydrascale`, the command stops with an error that names the file. Run
the command with `sudo` in that case, such as `sudo hydrascale env <tailnet-id>`.

## The routing forms

The namespace-scoped subcommands `exec`, `ping`, `ssh`, and `tailscale` replace a raw
`ip netns exec` line:

```bash
# Before
sudo ip netns exec ns-personal tailscale --socket=/var/lib/hydrascale/state/personal/tailscaled.sock ping Mars

# After
sudo hydrascale ping personal Mars
```
