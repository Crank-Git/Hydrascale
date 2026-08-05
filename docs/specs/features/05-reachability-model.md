---
id: reachability-model
feature: Local reachability model
epic: "Epic 5: Local reachability model"
status: planned
issues: []
mockups: []
---

## Purpose

The daemon promises that traffic from one tailnet never enters another. Separate network
stacks deliver that promise for routing. The host `FORWARD` chain does not deliver it for
forwarding.

`internal/namespaces/ns.go:263` inserts one rule per namespace:

```
iptables -I FORWARD 1 -i vh<hash> -j ACCEPT
```

The rule matches on the input interface only. It has no destination match and no output
interface match. Every namespace has a default route to the host
(`internal/namespaces/ns.go:250`). Every namespace has a `MASQUERADE` rule
(`internal/namespaces/ns.go:277`). Host forwarding is enabled per veth device
(`internal/namespaces/ns.go:256`).

A process inside namespace A therefore has a forwarding path to namespace B, to every
peer route that host access installs for B, and to the host local network. The rule is
inserted at position 1, so it also sits above any rule that the operator's own firewall
placed in `FORWARD`.

This feature set replaces that rule with a rule set that the operator controls. The
default is deny.

## User stories

- As the operator, I want a namespace to be unable to reach another namespace unless I
  allow it, so that the isolation the documentation promises is real.
- As the operator, I want to see what the daemon would deny before it denies it, so that
  an upgrade does not cut off a tailnet I depend on.
- As the operator, I want the upgrade to keep my current reachability, so that nothing
  breaks the day I install version 1.0.
- As the operator, I want the daemon's rules to live in their own chain, so that they do
  not sit above my own firewall rules.

## Functional requirements

### The chain

- **FR-access-1** — The daemon creates the iptables chain `HYDRASCALE-FWD` in the
  `filter` table.
- **FR-access-2** — The daemon inserts exactly one rule into `FORWARD`, which jumps to
  `HYDRASCALE-FWD`, and it appends that rule rather than inserting it at position 1.
- **FR-access-3** — The daemon owns every rule in `HYDRASCALE-FWD` and no rule outside
  it, other than the single jump.
- **FR-access-4** — The last rule in `HYDRASCALE-FWD` returns to `FORWARD` when
  `access.mode` is `observe`, and it drops when `access.mode` is `enforce`.
- **FR-access-5** — The daemon creates the chain `HYDRASCALE-OUT` for traffic that a
  namespace sends to the host itself, and it jumps to it from `INPUT`.

### The rule set

- **FR-access-6** — A local rule has a `from`, a `to`, and a list of ports.
- **FR-access-7** — `from` is a tailnet identifier or the literal `host`.
- **FR-access-8** — `to` is a tailnet identifier, the literal `host`, or the literal
  `internet`.
- **FR-access-9** — An empty port list allows every port and every protocol.
- **FR-access-10** — A port entry matches `tcp/<n>`, `udp/<n>`, `tcp/<n>-<m>`, or
  `udp/<n>-<m>`, where each number is between 1 and 65535.
- **FR-access-11** — The daemon rejects a rule where `from` equals `to`.
- **FR-access-12** — The daemon rejects a rule that names a tailnet that the
  configuration does not declare.
- **FR-access-13** — The daemon allows return traffic for an established connection
  without a rule.
- **FR-access-14** — The daemon allows a namespace to reach the DNS forwarder bind
  address without a rule, because DNS is how the product works.

### Compilation

- **FR-access-15** — The daemon compiles the rule set into an ordered list of iptables
  arguments. The compiler is a pure function of the rule set and the namespace addresses.
- **FR-access-16** — The compiler produces the same output for the same input.
- **FR-access-17** — The daemon compares the compiled rule set with the live chain and
  it writes only when they differ.
- **FR-access-18** — The daemon writes the chain with `iptables-restore --noflush` on a
  table that contains only its own chains, so that one write is atomic.

### Modes and migration

- **FR-access-19** — `access.mode` accepts `enforce` and `observe`, and it defaults to
  `enforce`.
- **FR-access-20** — In `observe` mode, the daemon logs each packet that it would drop,
  through an iptables `LOG` rule with the prefix `hydrascale-would-deny: `, and it drops
  nothing.
- **FR-access-21** — When the configuration contains no `access` block, the daemon
  writes the rule set that preserves the reachability of version 0.9 on first start.
- **FR-access-22** — The preserving rule set contains one rule per tailnet, from that
  tailnet to `internet`, with an empty port list.
- **FR-access-23** — The daemon records the event `access.migrated` with the rules it
  wrote.
- **FR-access-24** — The daemon writes the generated `access` block into the
  configuration file, and it keeps a copy of the previous file at
  `<config>.pre-v1.backup`.

### The control API

- **FR-access-25** — `GET /api/access` returns the rule set, the mode, and the compiled
  reachability.
- **FR-access-26** — `PUT /api/access` replaces the whole rule set.
- **FR-access-27** — `PUT /api/access` validates every rule before it writes any rule.
- **FR-access-28** — `PUT /api/access` returns the compiled reachability that the new
  rule set produces, without applying it, when the query parameter `dry_run=true` is set.

## User flows

### The operator upgrades from version 0.9

1. The operator installs the version 1.0 binary and restarts the service.
2. The daemon reads the configuration and finds no `access` block.
3. The daemon writes `<config>.pre-v1.backup`.
4. The daemon builds the preserving rule set, one rule per tailnet to `internet`.
5. The daemon writes the `access` block into the configuration file.
6. The daemon records `access.migrated` and logs each rule that it wrote.
7. The daemon applies the rule set.
8. Each tailnet keeps its internet path. No tailnet can reach another tailnet.

### The operator checks before enforcement

1. The operator sets `access.mode: observe` and restarts the service.
2. The daemon writes the chain with a `LOG` rule and no drop.
3. The operator uses the host normally for a day.
4. The operator reads `journalctl -u hydrascale | grep hydrascale-would-deny`.
5. The operator adds a rule for each path that the log shows and that the operator wants.
6. The operator sets `access.mode: enforce` and restarts the service.

### The reconciler applies a changed rule set

1. The console sends `PUT /api/access`.
2. The daemon validates every rule.
3. The daemon writes the rule set into the configuration file.
4. The reconciler ticks.
5. The reconciler compiles the rule set and compares it with the live chain.
6. The reconciler writes the chain when it differs.
7. The reconciler records `access.applied` with the count of rules.

## Screens & states

This feature set has no screen. `features/07-console-access-editor.md` defines the
screens that drive it.

## Behaviour rules

- Deny is the default. A rule allows. There is no deny rule, because a rule set with
  both is a rule set with an order that the operator must reason about.
- The daemon never writes a rule outside its own chains. An operator firewall rule stays
  where the operator put it.
- The daemon appends the jump rule rather than inserting it at position 1. An operator
  who wants Hydrascale to run first can move the jump.
- A rule set that fails validation is not partially applied. Validate everything, then
  write.
- The migration runs once. The daemon detects it by the presence of the `access` key,
  not by a version marker, so a manually written empty `access` block suppresses it.
- The `internet` destination means every address that is not the host, not a namespace,
  and not a private range that another namespace uses.

## Data touched

| Entity | Change |
|---|---|
| Configuration | New `access` block with `mode` and `rules`. |
| Event | New kinds `access.applied`, `access.migrated`, and `access.write_failed`. |
| Status | New field `access` with the mode and the rule count. |

## Interfaces

### `GET /api/access`

```json
{
  "mode": "enforce",
  "rules": [
    { "from": "jbones", "to": "homelab", "ports": ["tcp/22", "tcp/443"] },
    { "from": "jbones", "to": "internet", "ports": [] }
  ],
  "nodes": [
    { "id": "jbones", "kind": "tailnet", "peers": 6, "veth": "10.99.0.2" },
    { "id": "homelab", "kind": "tailnet", "peers": 3, "veth": "10.99.0.6" },
    { "id": "host", "kind": "host" },
    { "id": "internet", "kind": "internet" }
  ]
}
```

### `PUT /api/access`

The request body has the same `mode` and `rules` fields. The response is the new
`GET /api/access` body. A validation failure returns HTTP 400 and
`{"error": "<message>"}`.

### iptables

The compiler emits, for a rule from tailnet `a` to tailnet `b` with port `tcp/22`:

```
-A HYDRASCALE-FWD -i vh<hash-a> -o vh<hash-b> -p tcp --dport 22 -j ACCEPT
```

The chain opens with the established-connection rule and closes with the policy rule:

```
-A HYDRASCALE-FWD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
…
-A HYDRASCALE-FWD -j DROP
```

`iptables-restore` accepts a rule file on standard input. The `--noflush` flag keeps
chains that the file does not name. The behaviour is documented in
`iptables-restore(8)`.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The chain write fails halfway. | It cannot. `iptables-restore` applies the file as one transaction. A failure leaves the previous chain in place and records `access.write_failed`. |
| The host runs `nftables` through the `iptables-nft` compatibility layer. | The commands work, because the compatibility layer implements them. Epic 5 verifies this on the test host and records the kernel and the backend. |
| An operator firewall reloads and removes the jump rule. | The reconciler detects the missing jump on the next tick and re-adds it. It records an event so that the operator sees the conflict. |
| A tailnet is removed while a rule names it. | The daemon drops the rules that name the removed tailnet and records an event. It does not refuse to start. |
| Two tailnets use overlapping peer address ranges. | The rules match on the veth device rather than on the address, so overlap does not matter. This is why the compiler uses interfaces. |
| The operator sets `access.mode: observe` on a busy host. | The `LOG` rule can fill the journal. The compiler adds `-m limit --limit 60/minute` to the `LOG` rule. |
| IPv6 traffic. | Version 1.0 writes IPv4 rules only. The daemon logs at start that IPv6 forwarding is not filtered, so the gap is stated rather than hidden. |

## Acceptance criteria

- [ ] The compiler is a pure function and a test asserts its exact output for a fixed
      rule set.
- [ ] A test asserts that the compiler rejects `tcp/0`, `tcp/65536`, and `tcp/22-21`.
- [ ] A test asserts that the compiler rejects a rule where `from` equals `to`.
- [ ] A test asserts that the daemon inserts one jump rule into `FORWARD` and appends it.
- [ ] A test asserts that a rule set with no `access` block produces the preserving rule
      set with one rule per tailnet to `internet`.
- [ ] A test asserts that the daemon writes `<config>.pre-v1.backup` before it changes
      the configuration file.
- [ ] `PUT /api/access` with an invalid rule returns HTTP 400 and changes nothing.
- [ ] `PUT /api/access?dry_run=true` returns the compiled result and changes nothing.
- [ ] On the test host with two tailnets and no rule between them,
      `ip netns exec ns-a ping -c1 <veth address of b>` fails.
- [ ] On the test host, after a rule from `a` to `b` is added, the same command succeeds.
- [ ] On the test host, `ip netns exec ns-a curl -sS https://example.com` succeeds with
      the `internet` rule and fails without it.
- [ ] On the test host, `observe` mode writes `hydrascale-would-deny` lines to the
      journal and drops nothing.
- [ ] On the test host, the daemon re-adds the jump rule after `iptables -F FORWARD`.

## Out of scope

- IPv6 rules. The gap is recorded and logged.
- Per-peer rules. A rule names a tailnet, not a device inside it. Per-peer control is
  what the upstream policy does, and `features/08-upstream-policy.md` covers it.
- A rule that denies. The model is allow-only.
- Rate limiting or shaping.

## Open questions

None.
