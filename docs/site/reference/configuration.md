# The configuration file

The daemon reads the configuration file at `/etc/hydrascale/config.yaml`. Pass
`--config <path>` on any command to read another file. The systemd unit passes the default
path.

This page states every key that the daemon reads. A test compares this page to the
configuration struct of `internal/config`, and it fails when the page omits a key.

## An example file

```yaml
# The schema version. The daemon migrates a version 1 file when the key is absent.
version: 2

# Host access to every tailnet peer (default: false).
host_access: false

# The control server URL for Headscale (default: empty, which means Tailscale).
# control_url: "https://headscale.example.com"

# The subnet of the veth pairs (default: 10.200.0.0/16).
# infra_subnet: "10.200.0.0/16"

# The routing table that holds every host route of the daemon (default: the main table).
# route_table: 53

# Let the daemon set net.ipv6.conf.all.forwarding on a kernel older than Linux 6.17.
# ipv6: true

# The Unix group that reaches the control socket (default: empty, which is root only).
# socket_group: hydrascale

# The root-only file that holds a credential per tailnet.
# secrets_file: "/etc/hydrascale/secrets.yaml"

tailnets:
  - id: "corp-prod"
    alias: "prod"
    exit_node: "node1.example.com"
    host_access: true
    # control_url: "https://headscale.example.com"

access:
  mode: enforce
  rules:
    - from: corp-prod
      to: internet
      ports: []

console:
  enabled: true
  bind_address: "127.0.0.1:9443"

dns:
  allow_unprotected: false

resolver:
  mode: unified
  bind_address: "127.0.0.53:5354"
  resolve_aliases: false

host_dns:
  mode: hosts

# probe_target: "100.100.100.100"
# event_log: /var/log/hydrascale/events.json

reconciler:
  interval: 10s
```

## The top-level keys

| Key | Default | Description |
|---|---|---|
| `version` | `2` | The schema version. When the key is absent, the daemon reads the file as version 1 and migrates it to version 2. |
| `host_access` | `false` | Host access to the peers of every tailnet. The key `tailnets[].host_access` overrides it for one tailnet. |
| `control_url` | empty | The control server URL for every tailnet that declares none. An empty value selects the Tailscale coordination server. The URL needs the scheme `https`, unless the host is a loopback address. |
| `infra_subnet` | `10.200.0.0/16` | The subnet of the veth pairs between the host and the namespaces. The value is an IPv4 CIDR of `/16` or larger. Change it when `10.200.0.0/16` collides with a route on the network. |
| `route_table` | absent | The routing table that holds every route that the daemon writes on the host. See [The route table](#the-route-table). |
| `ipv6` | `false` | Lets the daemon set `net.ipv6.conf.all.forwarding` on a kernel older than Linux 6.17. A newer kernel holds `force_forwarding`, and it gets the IPv6 path of each namespace without this key. |
| `socket_group` | empty | The Unix group that reaches the control socket. An empty value keeps the socket root only. See [Remote access](../operations/remote-access.md). |
| `secrets_file` | `/etc/hydrascale/secrets.yaml` | The root-only file that holds a credential per tailnet. The file needs the mode `0600` and the owner root. |
| `probe_target` | `1.1.1.1` | The address that each namespace sends one packet to, so that the status answer reports measured reachability. Declare an IP address. The daemon refuses a name. |
| `event_log` | absent | The path of a file that receives each event as one JSON line. See [Events](events.md). |
| `tailnets` | empty | The tailnets that the daemon manages. See [Tailnets](#tailnets). |
| `access` | see [Local rules](#local-rules) | The local rule set. |
| `console` | see [The console](#the-console) | The console listener. |
| `dns` | see [DNS protection](#dns-protection) | The DNS protection setting. |
| `resolver` | see [The resolver](#the-resolver) | The DNS resolver. |
| `host_dns` | see [Host DNS](#host-dns) | The host DNS mode, which host access uses. |
| `reconciler` | see [The reconciler](#the-reconciler) | The control loop. |
| `mesh` | see [Mesh](#mesh) | A key that the daemon reads and does not act on. |

**Warning — membership of `socket_group` is equivalent to root access.** A member of the
group sends a command to the daemon, and the daemon runs as root. Name a group that holds
only trusted operators.

## Tailnets

Each item of `tailnets` declares one tailnet.

| Key | Default | Description |
|---|---|---|
| `tailnets[].id` | required | The unique identifier of the tailnet. It holds letters, digits, dots, hyphens, and underscores. It starts with a letter or a digit, and it holds 63 characters or fewer. |
| `tailnets[].alias` | empty | A second name of the tailnet. It holds letters, digits, hyphens, and underscores. An alias is unique, and it is the identifier of no tailnet. |
| `tailnets[].exit_node` | empty | The name of the exit node of the tailnet. |
| `tailnets[].auth_key` | empty | An auth key for an unattended setup. The environment variable `HYDRASCALE_AUTHKEY_<ID>` overrides it. See [Environment variables](environment.md). |
| `tailnets[].host_access` | the global `host_access` | Host access for this tailnet. The value overrides the global key. |
| `tailnets[].control_url` | the global `control_url` | The control server URL of this tailnet. The same rule for the scheme applies. |
| `tailnets[].publish` | empty | The ports of the host that the peers of this tailnet reach. An entry is `tcp/<n>` or `udp/<n>`. The tailnet needs host access, and a local rule `from: <tailnet>, to: host` must cover each entry. |

When `resolver.resolve_aliases` is `true`, an alias is also a DNS label. The alias then
holds no underscore, and two aliases that differ by case alone are a conflict.

## Local rules

The `access` block holds the local rule set.

| Key | Default | Description |
|---|---|---|
| `access.mode` | `enforce` | `enforce` drops a packet that no rule allows. `observe` logs that packet with the prefix `hydrascale-would-deny: ` and drops nothing. |
| `access.rules` | empty | The list of rules. A path that no rule allows is denied, because the model holds no deny rule. |
| `access.rules[].from` | required | The source: a tailnet identifier, or the literal `host`. |
| `access.rules[].to` | required | The destination: a tailnet identifier, the literal `host`, or the literal `internet`. |
| `access.rules[].ports` | empty | The ports that the rule allows. An entry has the form `tcp/<n>`, `udp/<n>`, `tcp/<n>-<m>`, or `udp/<n>-<m>`, for example `tcp/22`. An empty list allows every port and both protocols. |

When the file holds no `access` key, the daemon writes a rule set at the first start. That
rule set keeps the reachability of version 0.9. The daemon first writes a copy of the file,
and it records the event `access.migrated`. An empty `access` block that the operator
writes stops this migration.

## The console

| Key | Default | Description |
|---|---|---|
| `console.enabled` | `true` | Opens the console listener. The value `false` closes the listener, and the control socket keeps the JSON API. |
| `console.bind_address` | `127.0.0.1:9443` | The address of the console listener. The value is a loopback IP address and a port. The daemon refuses another address, and it refuses a name such as `localhost`. |

A version 0.9 file holds no `console` key, so the daemon serves the console on the default
address with no edit. The console has no authentication. Read
[The console has no authentication](../security/console.md) before you change these keys.

## DNS protection

| Key | Default | Description |
|---|---|---|
| `dns.allow_unprotected` | `false` | Lets a namespace start when the overlay mount on `/etc` fails. With `false`, the daemon places the tailnet of a failed mount in the error state. The daemon records `dns.unprotected` in both cases. |

## The resolver

| Key | Default | Description |
|---|---|---|
| `resolver.mode` | `unified` | The resolver mode. `unified` is the one mode that the daemon runs. |
| `resolver.bind_address` | `127.0.0.53:5354` | The address of the resolver. The value is a loopback IP address and a port. |
| `resolver.resolve_aliases` | `false` | Makes the resolver answer the short name `<host>.<alias>.ts.internal` for each tailnet that holds an alias. |

## Host DNS

| Key | Default | Description |
|---|---|---|
| `host_dns.mode` | `hosts` when a tailnet has host access | `hosts` writes the peer names into `/etc/hosts`. `resolved` registers the peer domains with `systemd-resolved` through `resolvectl`. |

## The reconciler

| Key | Default | Description |
|---|---|---|
| `reconciler.interval` | `10s` | The time between two ticks of the control loop, as a Go duration such as `30s` or `1m`. |

## Mesh

| Key | Default | Description |
|---|---|---|
| `mesh.enabled` | `false` | The daemon reads this key and does not act on it. The struct holds it for a later mesh mode. |

## The route table

When the file holds no `route_table` key, the daemon writes each host route into the main
table and writes no routing policy rule.

A declared `route_table` makes the daemon write each host route into that table. The daemon
then owns one routing policy rule per address family, at the priority 32000, which sends a
lookup to that table. The value `53` is the suggested value, because `tailscaled` uses the
table 52 inside each namespace.

The daemon refuses these values:

- A negative number.
- A number above 4294967294.
- `253`, `254`, and `255`, which the kernel reserves for the tables `default`, `main`, and
  `local`.
