---
id: dns-integrity
feature: DNS integrity
epic: "Epic 4: DNS integrity"
status: built
issues: [74, 75, 76, 77, 78, 79]
mockups: []
---

## Purpose

The daemon protects the host `/etc/resolv.conf` file with an overlay mount. Each
namespace gets a private `/etc` so that the `tailscaled` process inside it writes its own
`resolv.conf` rather than the host file. Commit `41b8d59` added this in pull request #30.

The operator reports that the host file still changes. The cause is not known. The
overlay mount is best effort: `cmd/hydrascale/nsdaemon.go:56` prints a message to
standard error and continues without protection.

This feature set does three things. It tries to reproduce the defect. It removes the
silent failure path, so that an unprotected host is always reported. It adds a check that
detects a change to the host file whether or not the cause is ever found.

The operator last saw the defect on a Jetson Orin host with a Tegra kernel. The test host
is x86-64. The defect may not reproduce. The plan accounts for that.

**The test host cannot reproduce the defect in its current state.** Its
`/etc/resolv.conf` carries the immutable attribute, as a workaround from the earlier
investigation of issue #28. No process can rewrite an immutable file, so the clobber
cannot happen. Before the reproduction attempt, run `sudo chattr -i /etc/resolv.conf` on
the test host and restore the systemd-resolved stub symbolic link. Record both steps in
`docs/dns-investigation.md`. Restore the immutable attribute when the epic ends, because
the test host needs working DNS for every other epic.

## What exists today

| Item | Reference |
|---|---|
| The overlay mount | `cmd/hydrascale/nsdaemon.go:39-56`. Mounts OverlayFS on `/etc` inside the private mount namespace, then executes the child. |
| The silent continue | `cmd/hydrascale/nsdaemon.go:56`. On a mount error it prints and proceeds. |
| The host `tailscaled` warning | `cmd/hydrascale/init.go:298`. Prints a warning when the host `tailscaled` process has `accept-dns` enabled. |
| The forwarder | `internal/dns/forwarder.go`. Reads the host `resolv.conf` for upstream servers. |
| Related history | `e68ab00` (#22 host DNS clobber), `aa558c7` (#20 MagicDNS on restart), `7a34f62` (#19 per-tailnet MagicDNS). |

## User stories

- As the operator, I want the daemon to tell me when it cannot protect the host DNS
  configuration, so that I do not debug a silent failure.
- As the operator, I want the daemon to tell me when the host DNS configuration changed
  under it, so that I know which process changed it.
- As the operator, I want the preflight check to stop me when the host `tailscaled`
  process will fight the daemon, so that I do not create the conflict.
- As the operator, I want to see the DNS state in the console.

## Functional requirements

### The overlay mount

- **FR-dns-1** — After the daemon mounts the overlay, the daemon verifies the mount.
- **FR-dns-2** — The daemon verifies the mount by reading `/proc/self/mountinfo` and
  finding an `overlay` entry whose mount point is `/etc`.
- **FR-dns-3** — When the overlay mount fails, the child process does not start.
- **FR-dns-4** — When the overlay mount fails, the daemon records the event
  `dns.unprotected` with the tailnet identifier and the mount error.
- **FR-dns-5** — When the overlay mount fails, the reconciler places the tailnet in an
  error state.
- **FR-dns-6** — The configuration key `dns.allow_unprotected` defaults to `false`. When
  it is `true`, the child process starts without the overlay mount and the daemon records
  the event.

### The host file check

- **FR-dns-7** — At start, the daemon records a SHA-256 checksum of the host
  `/etc/resolv.conf` file.
- **FR-dns-8** — On each reconciler tick, the daemon compares the current checksum with
  the recorded checksum.
- **FR-dns-9** — When the checksum changes, the daemon records the event
  `dns.host_file_changed` with the previous first line and the current first line.
- **FR-dns-10** — The daemon records the new checksum as the recorded checksum after it
  reports a change, so that one change produces one event.
- **FR-dns-11** — The control API returns the DNS state, which contains the checksum, the
  time of the last change, and the count of protected namespaces.

### The preflight check

- **FR-dns-12** — `hydrascale init` fails its preflight check when the host `tailscaled`
  process has `accept-dns` enabled.
- **FR-dns-13** — The failure message names the exact command that fixes it:
  `sudo tailscale set --accept-dns=false`.
- **FR-dns-14** — `hydrascale init` accepts a `--force` flag that turns the preflight
  failure into a warning.

### The reproduction

- **FR-dns-15** — `docs/dns-investigation.md` records the reproduction attempt, the
  commands, and the outcome.
- **FR-dns-16** — When the defect does not reproduce on the test host,
  `docs/dns-investigation.md` says so and states what would be needed to reproduce it.

## User flows

### The daemon starts a namespace and the overlay mount fails

1. The daemon runs `hydrascale __nsdaemon` inside the namespace.
2. The child creates the overlay upper directory and the work directory.
3. The child calls `mount`.
4. The `mount` call fails.
5. The child reads `/proc/self/mountinfo` and confirms that `/etc` is not an overlay.
6. The child exits with a non-zero status and a message on standard error.
7. The reconciler records `dns.unprotected` and places the tailnet in an error state.
8. The console shows the tailnet in an error state and shows the event.

### The host file changes

1. Another process writes `/etc/resolv.conf`.
2. The reconciler ticks.
3. The daemon computes the checksum and it differs from the recorded checksum.
4. The daemon records `dns.host_file_changed` with both first lines.
5. The console shows the event and shows a warning on the DNS view.

### The operator runs the preflight check

1. The operator runs `hydrascale init`.
2. The preflight check runs `tailscale status --json` on the host.
3. The check finds `accept-dns` enabled.
4. `hydrascale init` prints the failure and the fix command, then it stops.
5. The operator runs the fix command and runs `hydrascale init` again.

## Screens & states

The DNS view lives in `features/06-console-foundation.md` and uses
`mockups/05-dns-and-settings.html`. This feature set supplies its data.

| State | What the view shows |
|---|---|
| Protected | The resolver bind address, the upstream servers, and every namespace marked protected. |
| One namespace unprotected | The namespace row shows an error dot and the mount error text. |
| Host file changed | A warning above the view with the time and both first lines. |
| Resolver not running | The empty state states that the resolver is not running and names the configuration key that starts it. |

## Behaviour rules

- A failed overlay mount is a failure. The daemon does not continue and pretend the
  namespace is healthy.
- `dns.allow_unprotected` exists for a host whose kernel cannot mount OverlayFS. It is
  opt-in and it is loud.
- The checksum check reports. It does not repair. A daemon that rewrites the host file
  becomes the thing it is protecting against.
- The event records the first line of each version rather than the whole file, because a
  `resolv.conf` file can contain a search domain that names an internal host.

## Data touched

| Entity | Change |
|---|---|
| Configuration | New key `dns.allow_unprotected`, default `false`. |
| Event | New kinds `dns.unprotected` and `dns.host_file_changed`. |
| Status | New field `dns`, containing the checksum, the last change time, and the protected count. |

## Interfaces

`GET /api/dns` returns:

```json
{
  "bind_address": "127.0.0.53:5354",
  "mode": "unified",
  "upstreams": ["1.1.1.1:53"],
  "host_resolv_sha256": "9f2c…",
  "host_resolv_changed_at": "2026-08-04T12:31:08Z",
  "namespaces": [
    { "id": "jbones", "protected": true,  "error": "" },
    { "id": "corp",   "protected": false, "error": "overlay /etc failed: invalid argument" }
  ]
}
```

The daemon reads mount state from `/proc/self/mountinfo`. The format is documented in
`proc_pid_mountinfo(5)`. Field 9 holds the filesystem type and it equals `overlay` for an
OverlayFS mount.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The kernel has no OverlayFS support. | The mount fails with `ENODEV`. The message names `dns.allow_unprotected` as the way to proceed without protection. |
| `/etc/resolv.conf` is a symbolic link to a systemd-resolved stub. | The daemon checksums the target. A change to the link itself also changes the checksum, because the daemon records the link target path with the checksum. |
| `/etc/resolv.conf` does not exist. | The daemon records an empty checksum and reports the file as missing rather than as changed. |
| The overlay upper directory is on a filesystem that OverlayFS rejects as an upper layer. | The mount fails with `EINVAL`. The error text reaches the event, so the operator sees the real reason. |
| The defect does not reproduce on the test host. | `docs/dns-investigation.md` records the negative result. The detection work still ships, because detection is what tells the operator when it happens next. |
| The test host `/etc/resolv.conf` is immutable. | No process can rewrite it, so the clobber cannot happen and a reproduction attempt returns a false negative. Clear the attribute first, and record that the attempt needed it. |
| Issue #28 was already root-caused and fixed by pull request #30. | The overlay mount is that fix. The remaining defect is therefore either the silent failure path at `nsdaemon.go:56`, a host on which the overlay mount cannot succeed, or a second cause. The reproduction attempt must distinguish these before a fix is written. |

## Acceptance criteria

- [ ] A test proves that the child exits non-zero when the mount verification fails.
- [ ] A test proves that the child starts when `/proc/self/mountinfo` shows an `overlay`
      mount on `/etc`.
- [ ] A tailnet whose overlay mount failed appears in an error state in `GET /api/status`.
- [ ] The event list contains `dns.unprotected` with the mount error text.
- [ ] `dns.allow_unprotected: true` starts the child and records the event.
- [ ] A test proves that a change to the host `resolv.conf` file produces exactly one
      `dns.host_file_changed` event.
- [ ] `GET /api/dns` returns the checksum and the per-namespace protected state.
- [ ] `hydrascale init` exits non-zero when the host `tailscaled` process has
      `accept-dns` enabled.
- [ ] `hydrascale init --force` continues with a warning in that case.
- [ ] `docs/dns-investigation.md` records the reproduction attempt and its outcome, and
      it states whether the test host `/etc/resolv.conf` was immutable at the time.
- [ ] `docs/dns-investigation.md` states which of the three candidate causes the
      remaining defect matches, or states that none of them matched.
- [ ] The test host runs the daemon with two tailnets and both report `protected: true`.

## Out of scope

- A change to the DNS forwarder's resolution logic. This feature set covers protection
  and reporting.
- Automatic repair of the host `resolv.conf` file.
- systemd-resolved integration as a replacement for the overlay mount. If Epic 4 finds
  that the overlay approach cannot work, it records the finding and the operator decides
  in a later release.

## Open questions

- Whether the defect reproduces on the test host. `FR-dns-15` records the answer. This
  question does not block the epic, because the detection work is valuable either way.
