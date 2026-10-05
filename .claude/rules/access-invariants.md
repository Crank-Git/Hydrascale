---
paths:
  - "internal/access/**/*.go"
  - "internal/namespaces/**/*.go"
  - "internal/routing/**/*.go"
  - "internal/hostaccess/**/*.go"
---

# Host network invariants

These rules exist because version 1.0 fixes a defect that broke each of them. Read
`docs/specs/features/05-reachability-model.md` before you change anything here.

## The daemon owns two chains and two jump rules

The daemon writes rules into `HYDRASCALE-FWD` and `HYDRASCALE-OUT`. It writes one jump
rule into `FORWARD` and one into `INPUT`. It writes no other rule anywhere.

**Insert the jump rule at position 1, and report a displacement.** The operator decided
this on 2026-08-05: "I guess 1, but we need to make sure we catch if this is an issue."
The position is not stable, because `ts-forward`, `DOCKER-USER` and `DOCKER-FORWARD` each
take position 1 after the daemon starts. The reconciler reads the position on each tick
and records `access.jump_displaced` when the position changes. It moves no rule of the
operator.

The jump at position 1 sends every forwarded packet of the host into `HYDRASCALE-FWD`,
therefore the chain opens with `! -i vh+ ! -o vh+ -j RETURN`. The daemon filters a packet
that involves a namespace device, and it returns every other forwarded packet.

## Deny is the default

A local rule allows. There is no deny rule. The last rule in `HYDRASCALE-FWD` drops in
`enforce` mode and accepts in `observe` mode.

**The `observe` tail accepts; it does not return.** A `RETURN` rule gives the packet back
to `FORWARD`, whose policy is `DROP` on a host that runs Docker, therefore the packet
dies one chain later and the mode drops what it promises to keep. Issue #238 measured
this. The same holds for `HYDRASCALE-OUT` and the policy of `INPUT`. The chain opens with
`! -i vh+ ! -o vh+ -j RETURN` and the `HYDRASCALE-OUT` tail matches one namespace device,
therefore the `ACCEPT` applies to the traffic of the daemon alone.

A rule that matches a source only, with no destination match and no output interface
match, allows everything. That was the version 0.9 defect. Match the output interface.

## Match on the interface, not on the address

Two tailnets may use overlapping peer address ranges. A rule that matches an address
therefore matches the wrong namespace. Match `-i vh<hash>` and `-o vh<hash>`.

## The compiler is pure

`Compile` takes the rule set and the node addresses and returns the iptables arguments.
It runs no command, it reads no file, and it holds no state. Every side effect lives at
the edge, in the code that writes the chain.

Purity is load-bearing. The property that matters most is that recompiling an unchanged
rule set changes nothing, and that is only cheap to test on a function that touches
nothing.

## Write the chain as one transaction

Use `iptables-restore --noflush` with a file that names only the daemon's own chains. A
half-written chain is a host with a hole in it. `iptables-restore` applies the file
atomically, so a failure leaves the previous chain in place.

## Never call exec.Command directly

Call the `execx.Runner` that the package holds. A test replaces the runner and asserts
the exact argument list. A direct call cannot be verified, and every rule on this page is
verified by a test that reads the recorded arguments.

## Teardown removes everything setup created

A cleanup step that fails does not stop the remaining steps. Collect the errors and
return them together. A removed namespace that leaves an `ACCEPT` rule behind is a hole
that outlives the tailnet.

Treat "rule does not exist" as success. Treat any other failure as an error and record an
event.

## IPv6

The daemon writes the same two chains in the IPv6 filter table. Read FR-access-29 to
FR-access-41 in `docs/specs/features/05-reachability-model.md` before you change the IPv6
path. The test host measured each rule below on 2026-10-05.

**Write the IPv6 chains before a forwarding key changes.** The chains hold the guard of
each upstream device. A forwarding upstream device without the guard forwards internet
traffic to every other host device.

**Accept neighbor discovery in the IPv6 out chain.** Neighbor discovery is ICMPv6, so the
chain sees it. ARP never enters the IPv4 chain. Without the rule, the closing drop stops
the solicitation for the gateway and the namespace reaches nothing.

**Prefer force_forwarding to all.forwarding.** `net.ipv6.conf.all.forwarding` stops each
device with `accept_ra` 1 from accepting a router advertisement, so the host can lose its
own IPv6 default route. Use `all.forwarding` only when the operator sets `ipv6: true`, and
change `accept_ra` to 2 first.

**Reset force_forwarding before the IPv6 chains go.** A shutdown continues after a failed
step, because a chain that outlives the daemon is a rule that nobody owns.
