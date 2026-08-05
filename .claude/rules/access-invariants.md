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

**Append the jump rule. Never insert it at position 1.** Version 0.9 ran
`iptables -I FORWARD 1 …`, which placed the daemon's rule above every rule the operator's
own firewall had written. An operator who wants Hydrascale to run first can move the jump
themselves.

## Deny is the default

A local rule allows. There is no deny rule. The last rule in `HYDRASCALE-FWD` drops in
`enforce` mode and returns in `observe` mode.

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

Version 1.0 writes IPv4 rules only. The daemon logs at start that it does not filter IPv6
forwarding. State the gap; never let it look covered.
