# Networking

This page states how the daemon connects each namespace to the host and to the internet.

## IP forwarding

The host needs `net.ipv4.ip_forward=1`, so that the traffic of a namespace reaches the
internet and the control server. The daemon does not set this value on the host. Enable
it for this boot:

```bash
sudo sysctl -w net.ipv4.ip_forward=1
```

To keep it across a restart, write `/etc/sysctl.d/99-hydrascale.conf`:

```
net.ipv4.ip_forward = 1
```

The preflight checks of `hydrascale init` read the value, and they offer to do both
steps.

## Veth pairs

A veth pair connects each namespace to the host. The namespace of a tailnet is
`ns-<id>`. The interface names come from a short hash of the namespace name: `vh<hash>`
on the host side, and `vn<hash>` inside the namespace. The hash has 12 hexadecimal
characters, so each name stays inside the Linux limit of 15 characters.

Each pair takes a `/30` block from the infra subnet, which defaults to `10.200.0.0/16`.
The veth index of the namespace, from 1 to 254, selects the block. The index also comes
from the hash of the namespace name.

If `10.200.0.0/16` collides with a route on the network, set `infra_subnet` to a free
range. The value must be an IPv4 range, with a prefix length of 16 or less.

## NAT and masquerade

The daemon writes these rules in the `nat` table of the host for each namespace:

- **MASQUERADE**, for the IPv4 traffic from the `/30` block of the namespace. The traffic
  leaves the host with the address of the host interface that sends it.
- **NAT66**, when the IPv6 path is on. The IPv6 traffic of the namespace then leaves with
  the global address of the host. See [IPv6](#ipv6).
- **One DNAT rule per address family** in `PREROUTING`. It sends inbound UDP for the
  listen port of the namespace to its `tailscaled`. See
  [Direct connections and the listen port](#direct-connections-and-the-listen-port).

The reconciler checks these rules on every tick, and it writes a rule again when it is
absent.

## Direct connections and the listen port

The `tailscaled` of each namespace listens on a fixed UDP port: 41641 plus the veth index
of the namespace. The port is therefore from 41642 to 41895. The host forwards inbound
UDP for that port to the namespace, for IPv4 and for IPv6. The DNAT rule applies only to
a packet that arrives on a device other than a host side veth, for an address of the
host. A peer then reaches the namespace directly, as it reaches the `tailscaled` of the
host on 41641.

Read the port of a namespace:

```bash
sudo ip netns exec ns-<id> ss -lunp
```

**Warning — the host forwards each inbound UDP packet for that port to the namespace.** A
service of the host that listens on a port from 41642 to 41895 receives none of that
traffic.

## IPv6

Each namespace gets an IPv6 path when the host holds an IPv6 default route. The daemon
gives each namespace a `/64` from the unique local prefix `fd5c:9a3e:7b10::/48`. The host
translates that address to its own global address with one NAT66 rule. The local rules
apply to IPv6 as they apply to IPv4: the daemon writes `HYDRASCALE-FWD` and
`HYDRASCALE-OUT` in the IPv6 filter table too.

The host must forward IPv6, and the kernel decides how:

- **A kernel with the `force_forwarding` key, which is Linux 6.17 or later.** The daemon
  sets `force_forwarding` on each upstream device and on each host side veth device. The
  host keeps its own router advertisements. No configuration key is necessary.
- **An older kernel.** Only `net.ipv6.conf.all.forwarding` forwards IPv6. That key stops
  each device with `accept_ra` 1 from accepting a router advertisement, so the host can
  lose its own IPv6 default route. The daemon therefore sets it only when you add the key
  `ipv6: true`. It first changes `accept_ra` from 1 to 2 on each device.

```yaml
ipv6: true
```

The event `ipv6.state` states whether the path is on, and why. Read it with
`journalctl -u hydrascale | grep ipv6.state`. A host with no IPv6 default route reports
`off: the host holds no IPv6 default route`.

**Warning — `force_forwarding` on the upstream device lets the host forward internet
traffic to any other device of the host.** If the host did not forward IPv6 before, the
daemon drops that traffic in `HYDRASCALE-FWD`. A shutdown resets `force_forwarding` on
each upstream device before it removes the chains.

## Docker

Docker sets the policy of the `FORWARD` chain to `DROP`, which stops the traffic between a
namespace and the host. The daemon sends every forwarded packet into `HYDRASCALE-FWD`
before that policy applies. A local rule that allows the path therefore keeps it open.
The operator writes no iptables rule by hand.

Version 0.9 wrote an `ACCEPT` rule per namespace into `FORWARD`. Version 1.0 removes
those rules after a write of the rule set in the mode `enforce`. In the mode `observe`,
the daemon keeps them.

The daemon inserts its jump rule at position 1 of `FORWARD` and of `INPUT`. That position
is not stable, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` each take
position 1 after the daemon starts. The reconciler reads the position on every tick. It
records the event `access.jump_displaced` when the position changes, and it writes the
jump rule again when the parent chain holds none. It moves no rule of the operator.
