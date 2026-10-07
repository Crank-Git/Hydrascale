# Troubleshooting

Each entry states a symptom, its cause, and the steps that correct it. Read the events
first: `sudo journalctl -u hydrascale -n 50` shows the last 50 lines of the log. The
[Events](../reference/events.md) page states each event type.

## `bind: address in use` for the control socket

A daemon that crashed left the socket file. Delete it and start again:

```bash
sudo rm /var/lib/hydrascale/api.sock
sudo hydrascale serve
```

## The console does not open

Read the log for the bind address. The daemon refuses a `console.bind_address` that is not
a loopback host and a port, and it refuses a port that another process holds:

```bash
sudo journalctl -u hydrascale | grep console
```

## A tailnet cannot reach the internet after an upgrade to version 1.0

The rule set denies the path. Read the rules, and set `access.mode: observe` to see the
paths that the mode `enforce` denies. The mode `observe` writes each line through the
iptables `LOG` target into the kernel log, so read the kernel log:

```bash
sudo journalctl -k | grep hydrascale-would-deny
```

## `hydrascale status` reports `down` and `degraded` after a restart

A tailnet needs about 20 seconds after a start of the daemon to reach `healthy` and
`running`. Wait 30 seconds and read the state again:

```bash
sleep 30 && sudo hydrascale status
```

A tailnet that still reports `down` after 60 seconds has a real failure. Read the events:

```bash
sudo journalctl -u hydrascale -n 50
```

## A policy write returns a permission error on Tailscale

The OAuth client holds the scope `policy_file` alone. Add `devices:posture_attributes` and
`devices:core:read`, then write the new client identifier and client secret into the
secrets file.

## A policy write fails on Headscale

The control server runs the file policy mode, and the answer holds the message
`update is disabled for modes other than 'database'`. Set `policy.mode: "database"` in the
Headscale configuration and restart the control server.

## The daemon refuses the secrets file

The file grants group access or other access. Set the mode and the owner:

```bash
sudo chown root:root /etc/hydrascale/secrets.yaml
sudo chmod 0600 /etc/hydrascale/secrets.yaml
```

## `netcheck` in a namespace reports `IPv6: no`

Read the reason that the daemon records:

```bash
sudo journalctl -u hydrascale | grep ipv6.state
```

`the host holds no IPv6 default route` means the host has no IPv6 upstream.
`the kernel holds no force_forwarding` means the kernel is older than Linux 6.17; add
`ipv6: true` to the configuration file. See [the configuration file](../reference/configuration.md).

## `tailscale ping` reports `direct connection not established`

A peer reaches the namespace on its listen port. Read the port, and confirm the DNAT rule:

```bash
sudo ip netns exec ns-<id> ss -lunp | grep tailscaled
sudo iptables -t nat -S PREROUTING | grep DNAT
```

A firewall in front of the host, such as the firewall of a cloud provider, must allow
inbound UDP on that port.

## Traffic of a namespace cannot reach the internet

IP forwarding is off. Read the value and set it:

```bash
sudo sysctl net.ipv4.ip_forward          # it must print 1
sudo sysctl -w net.ipv4.ip_forward=1     # set it
```

## The host resolves no public name, and a tailscaled of the host changes the resolver

A `tailscaled` that the host runs outside Hydrascale, with `accept-dns` on, rewrites
`/etc/resolv.conf` to `100.100.100.100` alone. When that tailnet holds no public upstream,
the host loses public DNS, and the resolver of Hydrascale takes an upstream that does not answer. Turn the
DNS management of that process off, and give `/etc/resolv.conf` back to `systemd-resolved`:

```bash
sudo tailscale set --accept-dns=false
sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
```

`hydrascale init` detects this in the preflight checks and offers to turn it off.

## `failed to listen on /var/lib/hydrascale/api.sock: no such file or directory`

The state directory does not exist. `hydrascale init` and `hydrascale install` create it. To
create it by hand:

```bash
sudo mkdir -p /var/lib/hydrascale/state
```

## Docker stops the traffic of a namespace

The daemon writes its jump rule at position 1 of `FORWARD`, but another rule can come
first. Read the chain:

```bash
sudo iptables -L FORWARD -v
```

Look for a `DROP` rule before the jump rule of Hydrascale, and remove it or move it. The
daemon records the event `access.jump_displaced` when another rule moves its jump rule
down. See [Events](../reference/events.md).

## The infra subnet collides with a route

When `10.200.0.0/16` overlaps a route on the host, the veth setup fails or the traffic goes
to the wrong place. Read the routes:

```bash
ip route | grep 10.200
```

On a match, set `infra_subnet` to a free range:

```yaml
infra_subnet: "10.201.0.0/16"
```

Then restart the daemon. It deletes the namespaces and builds them again with the new
addresses.

## A host route does not carry traffic

A route in a table other than the main table reaches no packet until a routing policy rule
sends a lookup to that table. Read the rules and the table:

```bash
ip rule
ip -6 rule
ip route show table 53
ip -6 route show table 53
```

The rule list holds `32000: from all lookup 53` for IPv4 and for IPv6, and the table holds
one route per peer. When the rule is absent, read the log for `hostaccess`. When another
rule of the operator already holds the priority 32000, the daemon adds none and it states
the table that the rule looks up.

## `name not a valid ifname`

An older version used the whole tailnet identifier as the interface name, which passes the
Linux limit of 15 characters. Install the current release, which uses the hash names
`vh<hash>` and `vn<hash>`.

## MagicDNS returns SERVFAIL for a tailnet name

First read the log for `refresh_dns` after `start_daemon` on that tailnet:

```bash
journalctl -u hydrascale | grep -E 'start_daemon|refresh_dns'
```

When `refresh_dns` is absent or timed out, `tailscaled` never reached
`BackendState=Running`. The cause is a bad auth key, no network route, or a control server
that does not answer. Read `sudo hydrascale tailscale <id> -- status`. When `refresh_dns`
ran and a query still returns SERVFAIL, read the resolv.conf of the namespace; it must hold
real upstreams and never `100.100.100.100`:

```bash
cat /etc/netns/ns-<id>/resolv.conf
```

As a last step, restart the service with `sudo systemctl restart hydrascale`. The reconciler
runs the DNS refresh again on the next tick.
