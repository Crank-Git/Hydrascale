---
name: tailnet-exec
description: Send a command into the namespace of one tailnet rather than to the host network. Use when asked to reach a peer, to run a command against a tailnet address, or to run a tailscale command for one tailnet.
---

# Route a command to one tailnet

The host joins several tailnets. The daemon holds one namespace per tailnet. A bare
command reaches the host network, so its result describes the host rather than a tailnet.
A routing form sends the command into the namespace of the tailnet that you name.

The page https://crank-git.github.io/Hydrascale/reference/command-line/ states each
form and each flag in full.

## Read the state first

Run these two commands before the first routed command:

1. Run `sudo hydrascale list`. It prints the ID and the alias of each tailnet.
2. Run `sudo hydrascale status`. It prints the desired state and the actual state.

`hydrascale list` reads the configuration file. It therefore names a tailnet that is not
connected. Read `hydrascale status` for the state of each tailnet that `hydrascale list`
names.

`hydrascale status` asks the daemon first. When the control socket does not answer,
`hydrascale status` reads the configuration file and the host directly. A failed
`hydrascale status` is therefore a permission fault or a fault of the configuration file.
It is not proof that the daemon does not run. Report the error text to the operator.

A namespace exists without the daemon. A namespace that `hydrascale status` lists
therefore still takes a routing form, because a routing form needs the namespace rather
than the daemon.

## Each routing form needs root

`exec`, `ping`, `ssh`, and `tailscale` each need root, because each runs
`ip netns exec`. When the account holds no root permission, print the form with `sudo`
for the operator.

## The five routing forms

| Form | Result |
|---|---|
| `sudo hydrascale exec <tailnet-id> -- <command>` | The command runs inside the namespace of the tailnet. |
| `sudo hydrascale tailscale <tailnet-id> -- <arguments>` | The `tailscale` command runs inside the namespace, against the socket of that tailnet. |
| `sudo hydrascale ping <tailnet-id> <target>` | `tailscale ping` reaches the peer `<target>` from that namespace. |
| `sudo hydrascale ssh <tailnet-id> <target>` | `tailscale ssh` reaches the peer `<target>` from that namespace. |
| `hydrascale wrap <service-name> <tailnet-id>` | The command prints a systemd drop-in that runs a service inside the namespace. |

The first four forms take the tailnet as the first argument. `hydrascale wrap` takes it
as the second argument. Read the tailnet from `hydrascale list`.

## A tailnet alias in place of the ID

Every routing form takes a tailnet alias in place of the tailnet ID. The configuration
file declares the alias in the key `tailnets[].alias`. The namespace name stays
`ns-<ID>`, also when you name the alias. `hydrascale wrap` and `hydrascale env` take the
alias too.

A routing form reads the configuration file to resolve the alias. The form reads the
file also when you name the ID. The form therefore needs read access to
`/etc/hydrascale`, and on most hosts only root can read that directory. When the form
cannot read the file, it fails with an error that tells you to
"run the command with sudo". Print the form with `sudo` for the operator.

When the configuration file names no tailnet that matches, the form uses the argument as
the ID. `ip netns exec` then reports the namespace that it cannot find.

## The separator `--`

`hydrascale exec` and `hydrascale tailscale` need the separator `--`. Each returns an
error when the separator is absent:

- `hydrascale exec` returns `exec requires a -- separator before the command`.
- `hydrascale tailscale` returns
  `tailscale requires a -- separator before the arguments`.

`hydrascale ping` and `hydrascale ssh` take positional arguments, and they need no
separator. `hydrascale wrap` takes two positional arguments, and it needs no separator.

```sh
sudo hydrascale exec personal -- curl -s http://peer:8080
sudo hydrascale tailscale personal -- status
sudo hydrascale ping personal peer
sudo hydrascale ssh personal peer
hydrascale wrap nginx personal
```

## `hydrascale wrap` prints, and `--apply` changes the host

`hydrascale wrap <service-name> <tailnet-id>` prints a systemd drop-in, and it writes no
file. The drop-in runs the service inside the namespace of the tailnet.

`hydrascale wrap <service-name> <tailnet-id> --apply` writes the drop-in to
`/etc/systemd/system/<service-name>.service.d/hydrascale.conf`. This changes the host,
and it needs root. Print the `--apply` form for the operator, and do not run it. After
the operator writes the drop-in, the operator edits the `ExecStart=` line of the file. The
operator then reloads systemd and restarts the service.

## A short name for a peer

A peer of a tailnet that holds an alias can have the short name
`<peer>.<alias>.ts.internal`. The host resolves that name only when all of these
conditions are true:

- `resolver.resolve_aliases` is `true`.
- `host_dns.mode` is `resolved`.
- The tailnet holds an alias and `host_access: true`.

The daemon gives the alias zone to the resolver of the host alone. A command on the host
network can therefore use the short name. Inside a namespace, use the MagicDNS name or the
tailnet address of the peer. The page
https://crank-git.github.io/Hydrascale/concepts/dns/ states the alias zone in the section
"The alias zone".

## `hydrascale switch` changes no state

`hydrascale switch <tailnet-id>` prints the namespace name of the tailnet, and it changes
no state. The shell of the operator stays on the host network. A child process cannot
move its parent shell into a namespace.

Use a routing form for each command instead. One routed command carries no state to the
next command.

## A tailnet that holds no namespace

When the tailnet holds no namespace, the routing form fails with the message of
`ip netns exec`. Read `hydrascale status` for the state of that tailnet, and report that
state to the operator. Start no tailnet yourself.

## A daemon on another host

A routing form runs on the host that holds the namespace. Send the form through SSH when
the namespace is on another host:

```sh
ssh <host> sudo hydrascale exec <tailnet-id> -- <command>
```

Run `ssh <host> sudo hydrascale list` and `ssh <host> sudo hydrascale status` first. The
reason is the same as on the local host. The SSH account needs root permission on
`<host>`, because each routing form runs `ip netns exec` there.
