---
name: tailnet-exec
description: Send a command into the namespace of one tailnet rather than to the host network. Use when asked to reach a peer, to run a command against a tailnet address, or to run a tailscale command for one tailnet.
---

# Route a command to one tailnet

The host joins several tailnets. The daemon holds one namespace per tailnet. A bare
command reaches the host network, so its result describes the host rather than a tailnet.
A routing form sends the command into the namespace of the tailnet that you name.

## Read the state first

Run these two commands before the first routed command:

1. Run `hydrascale list`. It prints the identifier of each tailnet.
2. Run `hydrascale status`. It prints the desired state and the actual state.

`hydrascale list` reads the configuration file. It therefore names a tailnet that is not
connected. Read `hydrascale status` for the state of each tailnet that `hydrascale list`
names.

If `hydrascale status` fails, report the failure. The daemon does not run in that case.
Run the routed command anyway, because `hydrascale exec` needs the namespace rather than
the daemon.

## The five routing forms

| Form | Result |
|---|---|
| `hydrascale exec <id> -- <command>` | The command runs inside the namespace of the tailnet `<id>`. |
| `hydrascale tailscale <id> -- <arguments>` | The `tailscale` command runs inside the namespace, against the socket of that tailnet. |
| `hydrascale ping <id> <target>` | `tailscale ping` reaches the peer `<target>` from that namespace. |
| `hydrascale ssh <id> <target>` | `tailscale ssh` reaches the peer `<target>` from that namespace. |
| `hydrascale wrap <service-name> <tailnet-id>` | The command prints a systemd drop-in that runs a service inside the namespace. |

Each form takes the identifier of the tailnet as its first argument. Read that identifier
from `hydrascale list`.

## The separator `--`

`hydrascale exec` and `hydrascale tailscale` need the separator `--`. Each command calls
`cmd.ArgsLenAtDash()`, and each returns an error when the separator is absent:

- `hydrascale exec` returns `exec requires a -- separator before the command`.
- `hydrascale tailscale` returns
  `tailscale requires a -- separator before the arguments`.

`hydrascale ping` and `hydrascale ssh` take positional arguments, and they need no
separator. `hydrascale wrap` takes two positional arguments, and it needs no separator.

```sh
hydrascale exec personal -- curl -s http://peer:8080
hydrascale tailscale personal -- status
hydrascale ping personal peer
hydrascale ssh personal peer
hydrascale wrap nginx personal
```

`hydrascale exec` runs `ip netns exec`, which needs root. Print the form with `sudo` for
the operator when the account holds no root permission.

## `hydrascale switch` changes no state

`hydrascale switch <id>` prints the namespace name of the tailnet `<id>`, and it changes
no state. The shell of the operator stays on the host network. A child process cannot
move its parent shell into a namespace.

Use a routing form for each command instead. One routed command carries no state to the
next command.

## A tailnet that holds no namespace

When the tailnet holds no namespace, `hydrascale exec` fails with the message of
`ip netns exec`. Read `hydrascale status` for the state of that tailnet, and report that
state to the operator. Start no tailnet yourself.

## A daemon on another host

A routing form runs on the host that holds the namespace. Send the form through SSH when
the daemon runs on another host:

```sh
ssh <host> hydrascale exec <id> -- <command>
```

Run `ssh <host> hydrascale list` and `ssh <host> hydrascale status` first, for the same
reason as on the local host. The SSH account needs root permission on `<host>`, because
`hydrascale exec` runs `ip netns exec` there.
