# Host access

Each tailnet stays inside its own namespace by default. The host reaches a peer through a
routing form, such as `hydrascale exec` or `hydrascale ping`. **Host access** changes
that: the host reaches the peers of every managed tailnet directly.

```bash
# Without host access
sudo hydrascale ping havoc bigboy    # works, but needs the routing form

# With host access
ping havoc-bigboy
ssh havoc-mars
curl http://havoc-webserver:8080
```

## Enable host access

Enable host access for every tailnet, or for one tailnet:

```yaml
# Global: the host reaches every tailnet
host_access: true

# Or per tailnet
tailnets:
  - id: corp-prod
    host_access: true     # the host reaches this tailnet
  - id: personal
    host_access: false    # isolated
```

The global `host_access` defaults to `false`. A tailnet that sets no `host_access` takes
the global value.

A local rule still governs the path. If the host reaches a tailnet, it needs a rule from
`host` to that tailnet. See [Local rules](../concepts/local-rules.md).

## How host access works

On each reconciliation tick of a tailnet with host access, the daemon does these things:

1. **Host routes.** The daemon adds a host route for the Tailscale address of each peer,
   for IPv4 and for IPv6, through the veth pair of the namespace. The kernel then sends a
   packet to the right namespace. The daemon writes each route into the main table, or
   into the table that `route_table` declares. See
   [A dedicated route table](#a-dedicated-route-table).
2. **Namespace masquerade.** The daemon adds an iptables masquerade rule inside the
   namespace on `tailscale0`. The traffic of the host then carries the Tailscale address
   of the namespace, and `tailscaled` forwards it to the peer.
3. **Names of the peers.** In the host DNS mode `hosts`, the daemon writes an entry in
   `/etc/hosts` for each peer. In the mode `resolved`, the daemon registers the domains of
   the tailnet with `systemd-resolved`. See
   [The two host DNS modes](../concepts/dns.md#the-two-host-dns-modes).
4. **A MagicDNS route per tailnet.** The daemon registers the MagicDNS suffix of each
   tailnet, such as `taildf854a.ts.net`, with the DNS forwarder. The forwarder sends a
   query for that suffix to the veth gateway of the tailnet. A `PREROUTING` DNAT rule on
   the veth sends the query to the MagicDNS resolver of that namespace at
   `100.100.100.100`. Several tailnets therefore answer MagicDNS queries on one host.
5. **A short name per tailnet.** If `resolver.resolve_aliases` is set, the daemon answers
   `<peer>.<alias>.ts.internal` with the tailnet address of that peer. See
   [Short names for a peer](../concepts/dns.md#short-names-for-a-peer).

The daemon brings the routes and the names up to date on every tick.

## Accepted subnet routes

A peer can advertise a subnet route. If the `tailscaled` of the namespace runs with
`--accept-routes`, the daemon propagates that route to the host. On each tick, the daemon
reads the routing table 52 inside the namespace, where `tailscaled` installs an accepted
route. Then it adds the matching route on the host through the veth pair.

Pass `--accept-routes` at the login:

```bash
sudo hydrascale tailscale corp-prod -- up --accept-routes
```

The configuration file needs no change. A subnet route appears on the host within one
tick after the login. The daemon removes it when the tailnet goes away.

## The names of the peers

Each peer takes the name `<tailnet-id>-<hostname>`. Two tailnets with a peer of the same
name therefore give two different names on the host.

| Tailnet | Peer | Name on the host |
|---------|------|-----------------|
| havoc | bigboy | `havoc-bigboy` |
| havoc | mars | `havoc-mars` |
| personal | pixel 8a | `personal-pixel-8a` |
| personal | nas | `personal-nas` |

The daemon writes the host name of the peer in lower case, and it replaces each space
with a dash. It keeps the tailnet identifier as the configuration file writes it.

## Teardown

The daemon removes the host access state in three cases.

If the operator sets `host_access: false` for a tailnet, the daemon removes:

- Every host route of the peers of that tailnet.
- The masquerade rule and the DNS DNAT rules inside the namespace.
- The entries of that tailnet in `/etc/hosts`, or its `systemd-resolved` registration.

The namespace stays, and the other tailnets keep their routes and names.

If the operator removes a tailnet from the configuration file, the daemon removes:

- Every host route of the peers of that tailnet.
- The namespace, with the masquerade rule and the DNS DNAT rules inside it.
- The entries of that tailnet in `/etc/hosts`, or its `systemd-resolved` registration.

A graceful shutdown removes every host route, and every name of a peer. If `route_table`
declares a table, the shutdown also removes the two routing policy rules and empties that
table.

## A dedicated route table

The key `route_table` names the route table, which holds every route that the daemon
writes on the host. To keep the main table, leave the key out. The main table is the
behaviour of version 0.9.

53 is the suggested value, because `tailscaled` already uses the table 52 inside each
namespace. The kernel reserves 253, 254, and 255, and the daemon refuses each of them.
The daemon also refuses a negative value, and a value above 4294967294.

```yaml
route_table: 53
```

A route in a table other than the main table reaches no packet until a routing policy
rule sends a lookup to that table. The daemon therefore owns one routing policy rule per
address family:

```
ip rule add priority 32000 from all lookup 53
ip -6 rule add priority 32000 from all lookup 53
```

The priority 32000 comes after every rule of `tailscaled`, which uses 5210 to 5270. It
comes before the main table, which the kernel reads at 32766. A host that runs its own
`tailscaled` therefore keeps its precedence, and a route of the daemon still takes
precedence over the main table. The daemon reads the rule list on each tick, and it adds
no second copy. A shutdown removes each rule and empties the table.

## Compatibility

- **A standard Linux distribution.** Every feature works, which includes a MagicDNS name
  per tailnet.
- **Tegra and Jetson**, and any kernel without `xt_connmark`. Every feature works. The
  DNS forwarder and the veth DNAT rule carry a MagicDNS query without `xt_connmark`, so a
  name such as `mars.taildf854a.ts.net` resolves.
- **A host without `systemd-resolved`.** Use the mode `hosts`, which is the default.

## DNS

[DNS](../concepts/dns.md) states how the host resolves the name of a peer: the short
names, the DNS lifecycle of `tailscaled`, the overlay mount on `/etc`, and the two host
DNS modes.
