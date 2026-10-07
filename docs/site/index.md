# Hydrascale

Hydrascale lets one Linux host join more than one tailnet at the same time.

![The Overview view of the console. The names and the addresses are placeholders.](images/console-overview.png)

## What Hydrascale does

The daemon creates one network namespace for each tailnet, and it runs a separate
`tailscaled` in each namespace. The traffic of one tailnet stays inside the network stack
of its namespace. Overlapping IP ranges, separate firewall rules, and separate routing
tables therefore work with no extra step.

The operator declares the tailnets in a YAML configuration file. The reconciler drives
the live host toward that file:

- If the operator adds a tailnet to the file, the daemon creates its namespace.
- If the operator removes a tailnet, the daemon deletes the namespace, stops the
  process, and removes the routes.

The DNS forwarder answers names across every tailnet, so the host reaches a peer by name.

## The reconciler

The reconciler runs as a control loop. On each tick it reads the configuration, reads the
live host, computes the difference, and applies the smallest set of actions. The event
log records every action.

A tailnet that fails three times in a row enters an error state. The reconciler then
skips that tailnet, so one broken tailnet stops no other tailnet. To reset the error
state, run `sudo hydrascale apply`. Or, in the Namespaces view of the console, open the
tailnet and select **Connect**.

## Version 1.0 and later

Version 1.0 adds three things to the control loop:

- **The console.** The daemon serves a web interface on `127.0.0.1:9443`. The console
  shows the namespaces, the local rules, the upstream policy, and the event list.
- **Local rules.** The daemon enforces a declared rule set on the host with iptables. The
  daemon denies a path that no rule allows. See [Local rules](concepts/local-rules.md).
- **Upstream policy.** The daemon reads, validates, and writes the policy that the
  control server of a tailnet holds. See [Upstream policy](concepts/upstream-policy.md).

Version 1.4 gives each namespace an IPv6 path. It also forwards a UDP port of the host to
each namespace, so a peer connects to a namespace directly over IPv4 and IPv6. See
[Networking](concepts/networking.md).

## Next step

To install the daemon, read [Get started](get-started/index.md).
