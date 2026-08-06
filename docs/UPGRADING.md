# Upgrade to version 1.0

This note is for an operator who runs version 0.9 or version 0.10 on a Linux host. It
states what version 1.0 removes, what the daemon writes at the first start, and the order
of the steps.

Read the whole note before you install the binary.

## What version 1.0 removes

- **The desktop client.** Version 1.0 deletes the Wails application in `gui/`. The console
  replaces it.
- **The macOS artifact and the Windows artifact.** The release workflow builds the Linux
  daemon only.
- **The unrestricted `ACCEPT` rule per namespace.** Version 0.9 wrote one such rule into
  the host `FORWARD` chain for each namespace. Version 1.0 writes the chain
  `HYDRASCALE-FWD` instead, and a path that no local rule allows is denied.
- **The stable JSON shape of the `/api/*` routes.** The console is the one supported
  client. A script that reads a route can break.

Version 1.0 removes no configuration key.

### The desktop client stays on the earlier release pages

The version 0.9 release page carries the desktop client. It holds three artifacts:

```
Hydrascale_v0.9.0_linux_amd64.tar.gz
Hydrascale_v0.9.0_macos.zip
Hydrascale_v0.9.0_windows_amd64.exe
```

The version 0.10 release page carries the same three artifacts under the tag `v0.10.0`.
Download the desktop client from one of those two pages. The version 1.0 release page
carries the Linux daemon alone.

## What version 1.0 adds

- **The console.** The daemon serves a web interface at `http://127.0.0.1:9443`.
- **Local rules.** The daemon enforces a declared rule set on the host with iptables.
- **Upstream policy.** The daemon reads and writes the access-control document of a
  control server.

## The console has no authentication

**Warning — the console has no sign-in, and the daemon runs as root.** Any local account
on the host reaches `http://127.0.0.1:9443` and drives the daemon. Give a local account on
this host the trust that root needs.

Three controls limit the exposure:

1. The listener binds a loopback address only. The daemon refuses any other address.
2. Every mutating route requires the header `X-Hydrascale-Console: 1`.
3. The daemon answers HTTP 403 when the `Origin` header names another origin.

Control 2 and control 3 stop a hostile web page. Neither control stops a local account.

To close the listener, set `console.enabled: false`. The control socket keeps serving the
JSON API.

## The configuration file loads without an edit

A version 0.9 configuration file loads on version 1.0 without an edit. Every key that
version 1.0 adds holds a default. The keys `console`, `dns`, and `access` are all
optional, and the daemon serves the console on `127.0.0.1:9443` when the file names no
`console` key.

## The daemon writes an `access` block at the first start

A version 0.9 configuration file holds no `access` key. The daemon detects the absent key
and writes the rule set that preserves the reachability of version 0.9.

**Warning — the daemon rewrites the configuration file at the first start.** Copy the file
to a path of your own before you start version 1.0.

The daemon also writes its own copy at `<config>.pre-v1.backup` before it changes the
file. For the default path that copy is:

```
/etc/hydrascale/config.yaml.pre-v1.backup
```

The copy takes the mode of the source file, because the configuration file can hold an
auth key.

The rule set that the daemon writes holds one rule per tailnet, from that tailnet to
`internet`, with an empty port list. It holds no rule between two tailnets. It names no
mode, so the daemon applies the mode `enforce`.

The migration runs at the start, before the first reconcile. The mode `enforce` therefore
never applies to an empty rule set on an upgraded host.

The daemon records the event `access.migrated` with every rule that it wrote. On a host
with the two tailnets `jbones` and `havoc` the log line reads:

```
[access.migrated] wrote 2 rules: jbones -> internet, havoc -> internet
```

Read the line with:

```bash
sudo journalctl -u hydrascale | grep access.migrated
```

An operator who upgrades without this note still keeps a working host, because the daemon
writes the preserving rule set. The event log states what the daemon wrote.

## The path that the migration does not write

The migrated rule set carries each tailnet to the internet. It carries no traffic from one
tailnet to another tailnet. A host that relies on such a path loses it under the mode
`enforce`. An operator whose version 0.9 host forwarded traffic between two tailnets
therefore writes a rule to restore that path.

Measured on the test host on 2026-08-05, on a host with the two tailnets `jbones` and
`havoc`:

```
sudo ip netns exec ns-jbones ping -c2 -W2 10.200.0.86   -> 2 transmitted, 0 received
sudo ip netns exec ns-havoc  ping -c2 -W2 10.200.0.166  -> 2 transmitted, 0 received
```

The terminal `DROP` rule of `HYDRASCALE-FWD` counted 4 packets and 336 bytes after those
two commands, which is the four packets that the two commands sent. Read the counter of
the chain with:

```bash
sudo iptables -L HYDRASCALE-FWD -v -n
```

Step 16 of the upgrade procedure below writes such a rule. The example there carries
`jbones` to `havoc`.

The mode `observe` finds that path before the mode `enforce` denies it. The mode `observe`
writes a kernel log line for a packet that no rule allows, and it then accepts that
packet. The path stays open for the length of the observation.

The tail of each daemon chain accepts in the mode `observe`, therefore the policy of
`FORWARD` and the policy of `INPUT` receive no packet that a namespace device carries.
Version 0.9 wrote `ACCEPT` rules into `FORWARD`, so the mode `observe` restores the
reachability of version 0.9. A packet that the tail accepts reaches no later chain, and
`ts-forward`, `DOCKER-USER` and `DOCKER-FORWARD` do not see it. The mode `enforce` is the
default and it does not change.

## The upgrade procedure

**Warning — step 8 starts the daemon in the mode `enforce`.** A path from one tailnet to
another tailnet stops for the length of step 9 and step 10. To avoid that interval, read
"To upgrade with no enforcement at all" below.

1. Read the release note.
2. Stop the service with `sudo systemctl stop hydrascale`.
3. Copy `/etc/hydrascale/config.yaml` to a path of your own.
4. Download the version 1.0 archive from the GitHub Releases page.
5. Unpack the archive with `tar xzf hydrascale_*.tar.gz`.
6. Install the binary with `sudo install hydrascale /usr/local/bin/`.
7. Confirm the version with `hydrascale version`.
8. Start the service with `sudo systemctl start hydrascale`.
9. Confirm the event `access.migrated` in the journal.
10. Set `access.mode: observe` in `/etc/hydrascale/config.yaml`.
11. Apply the mode with `sudo systemctl reload hydrascale`.
12. Wait 90 seconds.
13. Read the state with `sudo hydrascale status`.
14. Use the host for a day.
15. Read the would-deny log lines with the command below.
16. Add one rule for each path that the log names.
17. Apply the rules with `sudo systemctl reload hydrascale`.
18. Use the host for a second day.
19. Confirm that the log names no further path.
20. Set `access.mode: enforce` in `/etc/hydrascale/config.yaml`.
21. Apply the mode with `sudo systemctl reload hydrascale`.

Step 11 to step 20 run in the mode `observe`, which drops no packet of a namespace.
Step 14 therefore keeps every path of the host open for that day, and the log names each
path that the mode `enforce` denies at step 21.

Step 15 reads the paths that the mode `enforce` denies:

```bash
journalctl -u hydrascale | grep hydrascale-would-deny
```

The mode `observe` rate-limits that log to 60 packets each minute.

A rule that step 16 adds takes this form:

```yaml
access:
  mode: observe
  rules:
    - from: jbones
      to: internet
    - from: havoc
      to: internet
    - from: jbones
      to: havoc
      ports: ["tcp/22", "tcp/443"]
```

The Access view of the console shows the same rule set, stages an edit, and applies it.

## `hydrascale status` reports `down` for up to 90 seconds

A tailnet needs up to 90 seconds after a restart of the service before `hydrascale status`
reports `healthy` and `running`. An earlier read reports `down` and `degraded`. That is
the normal state of a tailnet that still authenticates, and it is not a failure.

Wait 90 seconds, then read the state again:

```bash
sleep 90 && sudo hydrascale status
```

A tailnet that still reports `down` after 90 seconds has a real failure. Read the events:

```bash
sudo journalctl -u hydrascale -n 50
```

## To upgrade with no enforcement at all

The daemon detects the migration by the presence of the `access` key. An `access` block
that the operator writes by hand therefore suppresses the migration, and the daemon writes
no copy at `<config>.pre-v1.backup`.

Use this order to start version 1.0 with the mode `observe` on the first tick:

1. Stop the service with `sudo systemctl stop hydrascale`.
2. Copy `/etc/hydrascale/config.yaml` to a path of your own.
3. Install the version 1.0 binary.
4. Add an `access` block with `mode: observe` to the configuration file.
5. Add one rule per tailnet, from that tailnet to `internet`.
6. Start the service with `sudo systemctl start hydrascale`.

Continue at step 12 of the upgrade procedure.

## To roll back

**Warning — read "`hydrascale status` reports `down` for up to 90 seconds" before you roll
back.** A tailnet that reports `down` within 90 seconds of a restart needs more time
rather than a rollback.

1. Stop the service with `sudo systemctl stop hydrascale`.
2. Download the version 0.10 archive from the GitHub Releases page.
3. Install the version 0.10 binary over `/usr/local/bin/hydrascale`.
4. Restore the configuration file from `/etc/hydrascale/config.yaml.pre-v1.backup`.
5. Start the service with `sudo systemctl start hydrascale`.
6. Wait 90 seconds.
7. Read the state with `sudo hydrascale status`.

Version 0.10 ignores the keys `access`, `console`, and `dns`, so a restore of the copy is
optional. Restore the copy when you want the file to match the running version.
