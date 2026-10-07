---
name: hydrascale-troubleshoot
description: Find the cause of a Hydrascale fault with read-only commands, and print the command that repairs it. Use when a tailnet has no IPv6, makes no direct connection, resolves no name, loses traffic after Docker or tailscaled starts, or cannot read or write its policy.
allowed-tools: Read, Bash(hydrascale version:*), Bash(hydrascale status:*), Bash(sudo hydrascale status:*), Bash(sudo hydrascale diff:*), Bash(sudo hydrascale list:*), Bash(journalctl -u hydrascale:*), Bash(sudo iptables -S:*), Bash(sudo ip6tables -S:*), Bash(sudo iptables -t nat -S:*), Bash(sudo ip6tables -t nat -S:*), Bash(ip -6 route show:*), Bash(sysctl -n:*), Bash(pgrep -a tailscaled:*), Bash(resolvectl status:*), Bash(sudo findmnt --task:*)
---

# Troubleshoot Hydrascale

This skill finds the cause of a fault on a host that runs Hydrascale. It holds five
diagnoses. Each diagnosis holds a read-only check, the cause that the output proves, and
the command that repairs the cause.

**Warning — a repair command changes the host.** Print each repair command for the
operator. Run a repair command only when the operator asks for that command in the same
turn.

## The commands you run

The `allowed-tools` field above names read-only commands. None of them changes the host.

- Run each command that `allowed-tools` names without a question.
- Ask the operator before you run another read-only command, such as `sudo curl` or
  `sudo hydrascale tailscale <tailnet-id> -- netcheck`.
- Never read the secrets file `/etc/hydrascale/secrets.yaml`. It holds each credential.

`allowed-tools` names no `hydrascale tailscale` rule. A rule for
`sudo hydrascale tailscale` also allows `up` and `logout`, and both change a tailnet.
`allowed-tools` names `journalctl` without `sudo` for the same reason, because
`sudo journalctl` also allows `--vacuum-time`, which deletes the journal.

## Read the state first

1. Run `hydrascale version`. It states the version that the host runs.
2. Run `sudo hydrascale status`. It names each tailnet and its state.
3. Run `sudo hydrascale diff`. It names each action that a reconciliation performs.

The page https://crank-git.github.io/Hydrascale/reference/command-line/ states each
command.

## Read the events

The daemon writes each event to the log in the form `[<type>] <tailnet>: <message>`. The
tailnet part is absent when the event concerns the whole host. Read one event type:

```sh
journalctl -u hydrascale --since "-1d" | grep 'ipv6.state' | tail -n 5
```

Always pass `--since`. On a host with a large journal, a read of the whole journal of the
unit can take a minute or more, and `--since "-1d"` takes seconds. The daemon records
`ipv6.state` once after it starts, and again when the state changes. If a read of one day
holds no such line, ask the operator, then run
`systemctl show -p ActiveEnterTimestamp hydrascale`. Pass that start time to `--since`.

If `journalctl` prints no line of Hydrascale, the account can be outside the group that
reads the journal. Ask the operator before you run the same command with `sudo`.

Two other sources hold the same events:

- `GET /api/events` on the control socket returns the last 1000 events. Ask before you run
  `sudo curl --unix-socket /var/lib/hydrascale/api.sock http://unix/api/events`.
- When the configuration file sets `event_log`, that file holds one JSON line per event.

The page https://crank-git.github.io/Hydrascale/reference/events/ states each event type.

## Diagnosis: IPv6

The symptom: a namespace reaches no IPv6 address, or `netcheck` in a namespace reports
`IPv6: no`.

### Check

```sh
journalctl -u hydrascale --since "-1d" | grep 'ipv6.state' | tail -n 1
ip -6 route show default
sysctl -n net.ipv6.conf.all.force_forwarding
sudo ip6tables -t nat -S POSTROUTING
```

### Cause

The event `ipv6.state` states whether the IPv6 path is on, and why. The message starts
with `on:` and the mode, or with `off:` and the reason.

| `ipv6.state` message | Cause |
|---|---|
| `off: the host holds no IPv6 default route` | `ip -6 route show default` prints no route, so the host holds no upstream device. |
| `off: the kernel holds no force_forwarding, and the configuration file does not set ipv6: true` | The kernel is older than Linux 6.17, and the configuration file does not set the `ipv6` key. `sysctl -n net.ipv6.conf.all.force_forwarding` fails on such a kernel. |
| `on: force_forwarding` or `on: all_forwarding` | The path is on. Read the NAT66 rule. |

The upstream device is the device that each IPv6 default route names. The NAT66 rule gives
the namespace the global IPv6 address of the host. `ip6tables -t nat -S POSTROUTING` holds
one rule per namespace in this form:

```
-A POSTROUTING -s fd5c:9a3e:7b10:<veth index in hex>::/64 -j MASQUERADE
```

The daemon writes the NAT66 rule again on each tick when it is absent. If the rule stays
absent, read the event `access.write_failed`. A message that starts with `IPv6:` names the
command that failed. Report that message to the operator.

### Repair

If the host holds no upstream device, the repair is on the network of the host. Hydrascale
cannot give the host an IPv6 default route.

If the kernel holds no `force_forwarding`, print this repair for the operator:

```sh
sudo "$EDITOR" /etc/hydrascale/config.yaml   # add the line: ipv6: true
```

The daemon reads the configuration file on each tick. With `ipv6: true`, the daemon sets
`net.ipv6.conf.all.forwarding`. It first changes `accept_ra` from 1 to 2 on each device,
so the host keeps its own IPv6 default route.

The page https://crank-git.github.io/Hydrascale/concepts/networking/ states the IPv6 path
in the section "IPv6".

## Diagnosis: A direct connection

The symptom: `tailscale ping` from a namespace reports `direct connection not
established`, or every connection of the tailnet goes through a relay.

### Check

```sh
pgrep -a tailscaled
sudo iptables -t nat -S PREROUTING
sudo ip6tables -t nat -S PREROUTING
sudo iptables -S HYDRASCALE-FWD
```

Ask the operator, then run `netcheck` inside the namespace:

```sh
sudo hydrascale tailscale <tailnet-id> -- netcheck
```

### Cause

The `tailscaled` of each namespace listens on one UDP port. The port is 41641 plus the
veth index of the namespace, so the port is from 41642 to 41895. The `tailscaled` of the
host keeps 41641. `pgrep -a tailscaled` shows the port of each process in the argument
`--port=<port>`. The argument `--socket` names the tailnet in the path
`/var/lib/hydrascale/state/<tailnet-id>/`.

The host sends inbound UDP for that port to the namespace. Two rules carry each port:

- The DNAT rule in `PREROUTING` of the `nat` table, in this form:
  `-A PREROUTING ! -i vh+ -p udp -m addrtype --dst-type LOCAL -m udp --dport <port> -j DNAT --to-destination <namespace address>:<port>`.
- The forward rule in `HYDRASCALE-FWD`. It holds `--dport <port>` and
  `--ctstate DNAT -j ACCEPT`.

The daemon writes both rules again on each tick when one is absent. A rule that stays
absent therefore means one of these causes:

- The daemon does not run. Ask the operator, then run `systemctl is-active hydrascale`.
  It prints `active` when the daemon runs.
- A write failed. The event `access.write_failed` names the command.

If both rules are present, read the output of `netcheck`. `UDP: false` means that no UDP
answer reached the namespace. A firewall in front of the host, such as the firewall of a
cloud provider, must allow inbound UDP on the port.

### Repair

If the daemon does not run, print this repair for the operator:

```sh
sudo systemctl start hydrascale
```

If a firewall in front of the host stops the port, the repair is in that firewall. Print
the port, and tell the operator to allow inbound UDP on it.

The page https://crank-git.github.io/Hydrascale/concepts/networking/ states the listen
port in the section "Direct connections and the listen port".

## Diagnosis: DNS

The symptom: the host resolves no name of a peer, a split DNS domain answers from the
wrong tailnet, or the resolver configuration of the host changed.

### Check

```sh
sudo hydrascale status
journalctl -u hydrascale --since "-1d" | grep -E 'dns\.unprotected|dns\.split_domain_conflict' | tail -n 5
pgrep -a tailscaled
sudo findmnt --task <pid> /etc
resolvectl status
```

`<pid>` is the process ID of the `tailscaled` of one namespace, from `pgrep -a tailscaled`.
Use the Read tool on `/etc/hosts`. Ask the operator before you run
`sudo cat /etc/hydrascale/config.yaml` to read `host_dns.mode`. The configuration file
holds no credential.

### Cause

**The host DNS mode.** `host_dns.mode` selects how the host resolves the name of a peer.
The daemon writes the names of a tailnet only when the tailnet sets `host_access: true`.

- `hosts`, the default. `/etc/hosts` holds the names between the lines
  `# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT` and `# END HYDRASCALE MANAGED BLOCK`.
- `resolved`. `resolvectl status` lists the host side veth device `vh<hash>` of each
  tailnet. That device holds the MagicDNS suffix and each split DNS domain of the tailnet.

**The overlay mount.** Each `tailscaled` of a namespace runs under an overlay mount on
`/etc`. `findmnt --task <pid> /etc` then shows `overlay` in the column `FSTYPE`. Without
the overlay mount, `tailscaled` can replace `/etc/resolv.conf` of the host. When the
overlay mount fails, the daemon records the event `dns.unprotected`, and the message holds
the reason. The tailnet then enters the error state, unless the configuration file sets
`dns.allow_unprotected: true`. A reason that holds `no such device` means that the kernel
holds no OverlayFS module.

**A split DNS conflict.** Two tailnets can claim one split DNS domain. One tailnet keeps
the domain:

- A MagicDNS suffix and an active alias zone take their domain first.
- Of two split DNS domains, the first tailnet in sorted ID order keeps the domain.

The daemon records the event `dns.split_domain_conflict`. The tailnet of the event lost
the domain. The message names the domain and the tailnet that kept it.

### Repair

If the kernel holds no OverlayFS module, print this repair for the operator:

```sh
sudo modprobe overlay
sudo systemctl restart hydrascale
```

The restart clears the error state of the tailnet. Each tailnet needs about 20 seconds to
reach `running` again.

**Warning — `dns.allow_unprotected: true` lets `tailscaled` replace the resolver
configuration of the host.** Print that key only when the host cannot mount OverlayFS, and
state the risk.

A split DNS conflict needs a change on the control server. Tell the operator to remove the
domain from the DNS settings of one tailnet. No command of the host repairs it.

The page https://crank-git.github.io/Hydrascale/concepts/dns/ states the host DNS modes,
split DNS, and the overlay mount.

## Diagnosis: A displaced jump rule

The symptom: the traffic of a namespace stops, or a path that no local rule allows stays
open, after Docker or the `tailscaled` of the host starts.

### Check

```sh
journalctl -u hydrascale --since "-1d" | grep 'access.jump_displaced' | tail -n 4
sudo iptables -S FORWARD
sudo iptables -S INPUT
sudo ip6tables -S FORWARD
sudo ip6tables -S INPUT
```

### Cause

The daemon owns two jump rules in each address family: `-A FORWARD -j HYDRASCALE-FWD` and
`-A INPUT -j HYDRASCALE-OUT`. It inserts each jump rule at position 1. Three chains of other
services also take position 1 when their service starts after the daemon:

- `ts-forward`, which the `tailscaled` of the host adds.
- `DOCKER-USER` and `DOCKER-FORWARD`, which Docker adds.

The daemon reads the position on each tick. It records the event `access.jump_displaced`
when the position changes. The message has one of two forms:

- `the jump rule of FORWARD is at position <n>, below <targets>`. Each target is a chain
  or a target such as `ACCEPT`.
- `FORWARD held no jump rule of the daemon, therefore the daemon wrote it at position 1`.

The daemon moves no rule of the operator, so it does not move the jump rule back.

A displaced jump rule is a fault only when a rule above it ends the path of a packet of a
namespace. Read each chain that the message names with `sudo iptables -S <chain>`. An
`ACCEPT` or a `DROP` that matches the packet ends the path before the local rules apply.
A chain that returns each packet is not a fault.

### Repair

**Warning — a wrong position deletes a rule of the operator.** Read the position `<n>` from
the event or from `sudo iptables -S FORWARD` first. The position counts the `-A FORWARD`
lines up to the jump rule.

Print this repair for the operator. The first command inserts a second jump rule at
position 1, so the old jump rule moves to position `<n+1>`:

```sh
sudo iptables -I FORWARD 1 -j HYDRASCALE-FWD
sudo iptables -D FORWARD <n+1>
```

For `INPUT`, use `INPUT` and `HYDRASCALE-OUT`. For IPv6, use `ip6tables`. The other
service takes position 1 again when it starts again.

The page
https://crank-git.github.io/Hydrascale/operations/troubleshooting/#a-rule-of-another-service-comes-before-the-jump-rule
states this repair. The page https://crank-git.github.io/Hydrascale/concepts/local-rules/
states the chains of the daemon.

## Diagnosis: A rejected credential

The symptom: the console cannot read or write the policy of a tailnet.

### Check

Ask the operator, then read the credential state of each tailnet:

```sh
sudo curl --unix-socket /var/lib/hydrascale/api.sock http://unix/api/policy
```

The answer holds no credential value. Each row holds `id`, `kind`, `credential_state`, and
`reason`.

### Cause

| `credential_state` | Meaning |
|---|---|
| `absent` | The tailnet holds no credential. `reason` names the keys of the secrets file and the environment variables. |
| `rejected` | The tailnet holds a credential that the control server takes for no request. `reason` states why. |
| `usable` | The tailnet holds a credential of the right shape, and the control server refused no request yet. |

An absent credential is not a fault. A tailnet without a credential works. Only the policy
routes need a credential. Report the state, and change nothing.

A `rejected` credential has one of these reasons:

- The value of `tailscale_oauth_client_secret` does not start with `tskey-client`. A device
  authentication key is a different value, and it reaches no API.
- `the control server refused the credential: <message>`. The control server answered HTTP
  401 to a read or a write.

A `usable` credential with a permission error on a write is a different fault. The
credential is valid, and its scopes do not cover the write.

### Repair

Print this repair for the operator:

```sh
sudo "$EDITOR" /etc/hydrascale/secrets.yaml
```

The operator writes the new credential into the file, or through the console. The daemon
reads the credential at each request, so it needs no restart. A refusal of the control
server stays in the state `rejected` until the control server accepts a request, or until
the console writes a new credential.

The page https://crank-git.github.io/Hydrascale/guides/credentials/ states each credential
and its scopes.

## No fault found

If no check finds a cause, tell the operator which checks ran and what each one showed.
Change nothing. Link the page
https://crank-git.github.io/Hydrascale/operations/troubleshooting/, which states more
symptoms and their causes.
