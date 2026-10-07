# Events

The daemon records an event when the state of the host changes and when a step fails. This
page states every event type that the daemon records. A test reads the source of
`internal/`, and it fails when this page omits an event type.

## Where an event goes

Each event holds a time, a type, a tailnet identifier, and a message. The tailnet
identifier is empty when the event concerns the whole host.

- **The log.** The daemon writes each event to the standard error stream in the form
  `[<type>] <tailnet>: <message>`. Under systemd, read it with
  `sudo journalctl -u hydrascale`.
- **The control API.** `GET /api/events` returns the recent events. The daemon keeps the
  last 1000 events in memory, and it removes the oldest event first.
- **The console.** The Activity view shows the recent events.
- **The event log file.** When the configuration file declares `event_log`, the daemon
  appends each event to that file as one JSON line. The line holds the keys `time`,
  `type`, `tailnet`, and `message`. The daemon creates the directory at the mode `0750`
  and the file at the mode `0640`.

The message of an event holds no credential. The message of a failed step can name the
control server, the namespace, and the addresses.

## The control loop

| Event type | The daemon records it when |
|---|---|
| `reconcile_start` | A reconcile starts: a tick of the control loop, `hydrascale apply`, or a `SIGHUP`. |
| `reconcile_apply` | A reconcile finds a difference. The message states the count of actions. |
| `reconcile_complete` | A reconcile ends. The message states `no changes needed` or the count of applied actions. |
| `action_ok` | One action of a reconcile succeeds. The tailnet field names the tailnet, and the message names the action. |
| `action_failed` | One action of a reconcile fails. The message holds the action and the error. After three failures in a row, the daemon places the tailnet in the error state. |
| `loop_stopped` | The control loop stops, because the daemon stops. |

## The tailnet processes

| Event type | The daemon records it when |
|---|---|
| `daemon_stopped` | The daemon stops the `tailscaled` of a tailnet. The message states `disconnected (paused)` after a disconnect, or `shutdown` when the daemon stops. |
| `shutdown_complete` | The daemon stopped every `tailscaled` at shutdown. |
| `shutdown_timeout` | The shutdown did not stop every `tailscaled` within 30 seconds. |
| `teardown.failed` | A step of a teardown fails. The daemon continues the remaining steps, and the message holds every failure. |

## Local rules

| Event type | The daemon records it when |
|---|---|
| `access.applied` | The control API writes a rule set and the next reconcile succeeds. The message states the count of rules. |
| `access.written` | The daemon writes the chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT`. The message states the mode and the count of rules. A message that starts with `IPv6:` concerns the ip6tables chains. |
| `access.write_failed` | A write of the local rules fails. The rule set does not compile, or an iptables command fails. A message that starts with `IPv6:` concerns the IPv6 path. |
| `access.rule_dropped` | A rule names a tailnet that the configuration file no longer declares. The daemon skips the rule and applies the others. |
| `access.migrated` | The daemon writes the rule set that keeps the reachability of version 0.9 into a file that holds no `access` key. The message lists the rules. |
| `access.jump_displaced` | The jump rule of the daemon is no longer at position 1 of `FORWARD` or `INPUT`, or the parent chain holds no jump rule. The daemon records one event for one change of position. |
| `access.path_repaired` | The daemon writes again a host rule of the forward path of a namespace that lost it. The message lists the rules. |
| `access.namespace_forwarding` | The value of `net.ipv4.ip_forward` in a namespace differs from the value that the configuration requires. A tailnet with host access requires `1`, and another tailnet requires `0`. |
| `rules.reaped` | The daemon removes the rules of a namespace that no longer exists, or the forward rules that version 0.9 wrote. The message states the count. |

## IPv6

| Event type | The daemon records it when |
|---|---|
| `ipv6.state` | The IPv6 state of the host changes. The message starts with `on:` and the mode, or with `off:` and the reason. |
| `ipv6.forwarding` | The daemon sets `force_forwarding` on an upstream device, or it sets `net.ipv6.conf.all.forwarding` to `1`. |

## DNS

| Event type | The daemon records it when |
|---|---|
| `dns.unprotected` | The overlay mount on `/etc` fails for a namespace. The message holds the reason. The daemon records the event again only when the reason changes. |
| `dns.refresh_waits` | The DNS refresh of a tailnet waits, because `tailscaled` is not in the Running state. The daemon records it one time for one wait. |
| `dns.host_file_changed` | The host `resolv.conf` file changed. The message holds the first line of the previous file and of the current file. The daemon does not write the host file. |
| `dns.host_file_error` | The daemon cannot read the host `resolv.conf` file. |
| `dns.split_domain_conflict` | Two tailnets claim one split DNS domain. The tailnet field names the tailnet that loses the domain, and the message names the winner. |

## The control API and the console

| Event type | The daemon records it when |
|---|---|
| `console.request` | The console listener receives a mutating request. The message holds the method and the path, and no request body. `POST /api/policy/{id}/sections` records no event, because it changes no state. |
| `policy.pushed` | The daemon writes a policy to the control server of a tailnet. The message names the control server kind. |
