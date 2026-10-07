# The console has no authentication

**Warning — the console has no sign-in, and the daemon runs as root.** Any local account on
the host reaches `http://127.0.0.1:9443` and drives the daemon. The operator accepted this
risk for version 1.0. Give a local account on this host the trust that root needs.

The daemon writes this position to the log at every start:

```bash
sudo journalctl -u hydrascale | grep 'the console listens on'
```

## The four controls

Four controls reduce the risk:

1. The listener binds a loopback address only. The daemon refuses any other address, it
   writes the reason to the log, and it stops. `hydrascale apply` refuses the same value.
2. Every mutating route requires the header `X-Hydrascale-Console: 1`. A browser sets no
   custom header on a cross-origin form post.
3. The daemon answers HTTP 403 when the `Origin` header names a host that is not a
   loopback host.
4. The daemon records the event `console.request` for every mutating request, and the
   Activity view shows it. `POST /api/policy/{id}/sections` is the one exception, because
   it changes no state.

Control 2 and control 3 stop a hostile web page. Neither control stops a local account.

The daemon also sets the header
`Content-Security-Policy: default-src 'self'; img-src 'self' data:` on every console
response, so the console loads no resource from another host.

## Close the console

To close the listener, set `console.enabled: false` in the configuration file. The control
socket keeps serving the JSON API. See [The configuration file](../reference/configuration.md).

## The socket group

**Warning — membership of `socket_group` is equivalent to root access.** A member of the
group sends a command to the daemon through the control socket, and the daemon runs as
root. Name a group that holds only trusted operators. See
[Remote access](../operations/remote-access.md).

## The security audit

The [security audit](audit.md) records each finding, its severity, and the epic that
corrects it.
