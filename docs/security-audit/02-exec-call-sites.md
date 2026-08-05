# Security audit — the os/exec call sites

This file is a fragment of the security audit. It answers issue #64. It covers
**FR-audit-8** and **FR-audit-13**. Issue #67 collects every fragment into
`docs/security-audit.md`.

The identifiers run from `SA-20` to `SA-39`. Issue #67 renumbers the complete set.

This audit changes no code. Epic 3 makes the corrections.

## Method

The audit reads every file in `cmd/` and `internal/` that imports `os/exec`. The command
that found the call sites is:

```
rg 'exec\.Command' cmd/ internal/
```

The audit then reads each call site and records the origin of every argument. An origin
is one of four values:

- A constant in the source.
- The configuration file `/etc/hydrascale/config.yaml`.
- A control API request.
- The control server, through the output of `tailscale status --json` or through the
  route table that `tailscaled` writes.

## The count

| Group | Call sites |
|---|---|
| `internal/` through `execx.Runner` | 35 |
| `internal/` direct, against the convention | 23 |
| `cmd/` direct, which the convention permits | 17 |
| `internal/execx/execx.go`, the runner itself | 1 |

The 35 runner calls are 31 in `internal/namespaces/ns.go` and 4 in
`internal/routing/routes.go`.

**Epic 0 moved `internal/namespaces` and `internal/routing` onto the `execx.Runner`
interface.** `internal/routing/routes.go:24-40` holds the `RealManager` and its `runner`
method. Neither package calls `os/exec` now. This audit does not repeat that work. It
records the argument origins that reach the runner in those two packages.

`internal/dns` calls no command. The package holds no `os/exec` import. The forwarder
listens on a socket and it writes no host state.

## The inventory

### Group 1 — `internal/`, direct, against the convention

`CLAUDE.md` states: "Never call `exec.Command` directly in `internal/`." Every call site
in this group is a defect against that rule. The harm is that no test can assert the
argument list.

| File | Lines | Command | Argument origins |
|---|---|---|---|
| `internal/hostaccess/routes.go` | 220, 230 | `ip netns exec <ns> ip [-6] route show table 52` | `nsName` from the reconciler, derived from the tailnet id |
| `internal/hostaccess/routes.go` | 296 | `ip [-6] route get <addr>` | `dest` from the control server |
| `internal/hostaccess/routes.go` | 361, 365, 433, 442 | `ip [-6] route show` | constants |
| `internal/hostaccess/routes.go` | 380, 395 | `ip [-6] route replace <dest> via <gw> dev <veth>` | `dest` from the control server; `gw` and `veth` from the reconciler |
| `internal/hostaccess/routes.go` | 387, 402, 436, 445 | `ip [-6] route del <dest>` | `dest` from the host route table |
| `internal/hostaccess/resolved.go` | 19 | `systemctl is-active --quiet systemd-resolved` | constants |
| `internal/hostaccess/resolved.go` | 36 | `resolvectl domain lo ~<domain>` | `domain` from the control server |
| `internal/hostaccess/resolved.go` | 40, 52 | `resolvectl dns lo 127.0.0.53:5354`, `resolvectl revert lo` | constants |
| `internal/daemon/daemon.go` | 94, 252, 402 | `ip netns exec <ns> tailscale --socket=<path> status --json` | `namespaceName` and `socketPath`, both derived from the tailnet id |
| `internal/daemon/daemon.go` | 149 | `ip netns exec <ns> <self> __nsdaemon --etc-upper <u> --etc-work <w> -- tailscaled …` | `self` from `os.Executable`; every path derived from the tailnet id |
| `internal/daemon/daemon.go` | 338 | `ip netns exec <ns> tailscale --socket=<path> up …` | the auth key through a file, the control URL from the configuration file |
| `internal/daemon/daemon.go` | 424 | `ip netns exec <ns> tailscale --socket=<path> set --accept-dns=<v>` | `v` is `"false"` or `"true"`, both constants |

### Group 2 — `cmd/`, direct, which the convention permits

| File | Lines | Command | Argument origins |
|---|---|---|---|
| `cmd/hydrascale/init.go` | 138 | `<self> install` | `os.Executable` |
| `cmd/hydrascale/init.go` | 161 | `ip netns exec <ns> tailscale --socket=<sock> up --accept-dns=true --authkey=<key>` | the auth key from the environment variable |
| `cmd/hydrascale/init.go` | 192 | `ip netns exec <ns> tailscale --socket=<sock> status` | the tailnet id, which `config.IsValidID` accepts at line 220 |
| `cmd/hydrascale/init.go` | 254, 259 | `groupadd -f <group>`, `usermod -aG <group> <user>` | the operator answer at the prompt |
| `cmd/hydrascale/init.go` | 273 | `exec.LookPath` for `tailscale`, `tailscaled`, `iptables`, `ip` | constants |
| `cmd/hydrascale/init.go` | 283, 300, 313, 345 | `sysctl -w net.ipv4.ip_forward=1`, `tailscale set --accept-dns=false`, `uname -r`, `tailscale debug prefs` | constants |
| `cmd/hydrascale/main.go` | 553 | `ip netns exec <ns> <argv>` | `argv` from the operator command line; the id passes `config.IsValidID` at line 548 |
| `cmd/hydrascale/nsdaemon.go` | 59 | `exec.LookPath(cmdArgs[0])`, then `syscall.Exec` at line 63 | `cmdArgs` from the parent command line |
| `cmd/hydrascale/uninstall.go` | 56, 58, 93, 113 | `systemctl stop\|disable\|daemon-reload\|is-active hydrascale` | constants |
| `cmd/hydrascale/uninstall.go` | 121 | `ip netns exec <ns> tailscale --socket=<sock> logout` | the tailnet id from the configuration file |

### Group 3 — `internal/`, through `execx.Runner`

| File | Count | Commands | Argument origins |
|---|---|---|---|
| `internal/namespaces/ns.go` | 31 | `ip netns`, `ip link`, `ip addr`, `ip route`, `sysctl`, `iptables` | `namespaceName` is `"ns-" + tailnetID` (`ns.go:66-67`); `hostVeth` and `nsVeth` are `"vh"`/`"vn"` plus 12 hexadecimal characters of a hash (`ns.go:179`); the addresses come from `infra_subnet`, which `config.LoadConfig` validates as an IPv4 CIDR |
| `internal/routing/routes.go` | 4 | `ip netns exec <ns> tailscale status --json`, `ip route show`, `ip route replace <dest>`, `ip route del <dest>` | `dest` from the control server; `addRoute` validates it with `parseCIDR` at `routes.go:206-209`, and `deleteRoute` at line 214 receives a destination that `parseRouteOutput` already validated |

## The findings

### SA-20 — `internal/hostaccess` calls `os/exec` 17 times

- **Severity**: medium
- **Where**: `internal/hostaccess/routes.go:220`, `internal/hostaccess/routes.go:296`,
  `internal/hostaccess/routes.go:380`, `internal/hostaccess/resolved.go:19`,
  `internal/hostaccess/resolved.go:36`, and 12 more sites in the same two files
- **Harm**: The package writes host routes and host DNS configuration. No test can assert
  the argument list of a direct call, so a wrong argument reaches the host and no test
  reports it. The route commands run against the host main route table, and a wrong
  destination removes host connectivity.
- **Epic 3**: Yes. Epic 3 moves the package onto `execx.Runner`, in the same manner as
  Epic 0 moved `internal/namespaces` and `internal/routing`.

### SA-21 — `internal/daemon` calls `os/exec` 6 times

- **Severity**: medium
- **Where**: `internal/daemon/daemon.go:94`, `internal/daemon/daemon.go:149`,
  `internal/daemon/daemon.go:252`, `internal/daemon/daemon.go:338`,
  `internal/daemon/daemon.go:402`, `internal/daemon/daemon.go:424`
- **Harm**: The package starts `tailscaled` and it runs `tailscale up` with the auth key.
  No test asserts the argument list, so a change that puts the auth key back into `argv`
  passes the test suite. `daemon.go:338` is the one command in the code base that handles
  a secret.
- **Epic 3**: Yes. Epic 3 moves the package onto `execx.Runner`.

### SA-22 — The auth key reaches `argv` in `hydrascale init`

- **Severity**: high
- **Where**: `cmd/hydrascale/init.go:161-162`
- **Harm**: The command is
  `tailscale --socket=<sock> up --accept-dns=true --authkey=<key>`. The key is one
  argument, so it appears in `/proc/<pid>/cmdline`. Any local user reads that file while
  the command runs. The key then joins a tailnet as a new node.
  `internal/daemon/daemon.go:316-336` already solves this problem for the daemon path: it
  writes the key to a 0600 file and it passes `--auth-key=file:<path>`. See
  `buildTailscaleUpArgs` at `internal/daemon/daemon.go:271-288`. The `init` path did not
  get the same correction.
- **Condition**: A local user reads `/proc/<pid>/cmdline` during the retry that
  `verifyAuth` runs.
- **Epic 3**: Yes. Epic 3 applies the `--auth-key=file:` form at this call site.

### SA-23 — A route destination from the control server reaches `ip` without validation

- **Severity**: medium
- **Where**: `internal/hostaccess/routes.go:380`, `internal/hostaccess/routes.go:395`,
  `internal/hostaccess/routes.go:296`
- **Harm**: The destination comes from `ip route show table 52` inside the namespace.
  `tailscaled` fills that table from the routes that the control server advertises. The
  parser at `internal/hostaccess/routes.go:134-177` removes a known set of destinations,
  but it never confirms that the remaining text is an address or a CIDR. The text then
  becomes one argument of `ip route replace`. `ip` reads a leading hyphen as an option, so
  a crafted destination changes the meaning of the command. `internal/routing` shows the
  correct pattern: `addRoute` calls `parseCIDR` first, at
  `internal/routing/routes.go:206-209`.
- **Condition**: The control server is hostile, or an operator with route-advertisement
  rights on the tailnet advertises a crafted destination.
- **Epic 3**: Yes. Epic 3 validates the destination as an address or a CIDR before the
  command runs.

### SA-24 — A domain from the control server reaches `resolvectl` without validation

- **Severity**: medium
- **Where**: `internal/hostaccess/resolved.go:22-38`, with the origin at
  `internal/hostaccess/hostaccess.go:129-134`
- **Harm**: The domain is `MagicDNSSuffix` from `tailscale status --json`. The code adds
  the prefix `~` only when the value does not already start with `~`. It confirms nothing
  else. The value becomes one argument of `resolvectl domain lo`. A value that starts with
  a hyphen becomes an option of `resolvectl`, and the command then changes a setting that
  the daemon did not intend to change.
- **Condition**: The control server returns a crafted MagicDNS suffix, and the
  configuration selects the `resolved` DNS mode.
- **Epic 3**: Yes. Epic 3 validates the domain as a DNS name before the command runs.

### SA-25 — The control API accepts a tailnet id that the configuration loader rejects

- **Severity**: medium
- **Where**: `internal/api/server.go:243-278`
- **Harm**: `handleTailnetAdd` confirms only that `req.ID` is not empty. It does not call
  `config.IsValidID`. It writes the id into `/etc/hydrascale/config.yaml`. The next
  `config.LoadConfig` rejects the whole file at `internal/config/config.go:124`. The
  daemon then fails every reconcile cycle, and it fails to start after a restart. An
  operator must correct the file by hand.
  The invalid id does not reach a command, because the reconciler reads the configuration
  through `config.LoadConfig` and that call fails first. The harm is the loss of service,
  not the command.
- **Condition**: A caller of the control API sends an id that
  `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$` does not match.
- **Epic 3**: Yes. Epic 3 validates the id in the route, in the same manner as the route
  validates `control_url` at `internal/api/server.go:246`.

### SA-26 — A tailnet id from the control API reaches a path without validation

- **Severity**: medium
- **Where**: `internal/api/server.go:361-364`, `internal/reconciler/reconciler.go:457-459`,
  `internal/daemon/daemon.go:181-215`
- **Harm**: `handleTailnetDisconnect` passes `req.ID` to `Reconciler.StopDaemon`, which
  passes it to `daemon.StopDaemon`. That function joins the id onto
  `/var/lib/hydrascale/state`, reads the file `tailscaled.pid` under the result, sends
  `SIGTERM` to the number it reads, and then removes the file. An id such as `../../tmp/x`
  reads a file outside the state directory. `validatePID` at
  `internal/daemon/daemon.go:495-502` limits the signal to a process whose
  `/proc/<pid>/cmdline` holds the text `tailscaled`, so the signal cannot reach an
  arbitrary process. The file removal has no such limit.
  `internal/reconciler/reconciler.go:282-290` shows the correct pattern: `safeStateDir`
  validates the id and it confirms that the path is a direct child of the state directory.
- **Condition**: A caller of the control API sends an id that holds a path separator.
- **Epic 3**: Yes. Epic 3 applies `safeStateDir`, or an equivalent check, on this path.

### SA-27 — `tailscaled` inherits the complete environment of the daemon

- **Severity**: medium
- **Where**: `internal/daemon/daemon.go:149-161`
- **Harm**: `StartDaemon` sets no `cmd.Env`, so the child receives every environment
  variable of the daemon. The daemon holds an auth key in
  `HYDRASCALE_AUTHKEY_<ID>` for each tailnet that uses one; see
  `cmd/hydrascale/init.go:106`. Every one of those variables reaches `tailscaled`, and it
  reaches every process that `tailscaled` starts. A user who reads
  `/proc/<pid>/environ` for that process reads the keys of every tailnet, not only the key
  of its own tailnet. `AuthorizeDaemon` at `internal/daemon/daemon.go:340-343` already sets
  a minimal environment for its own child, with the comment "Minimal environment for the
  child process to avoid leaking parent env vars". `StartDaemon` did not get the same
  correction.
- **Condition**: The operator supplies an auth key through an environment variable, and a
  local user reads `/proc/<pid>/environ` of the `tailscaled` process. The file is
  root-only on a default kernel, so the reader must be root or must share the user of the
  daemon.
- **Epic 3**: Yes. Epic 3 sets a minimal `cmd.Env` at this call site.

### SA-28 — No command in `internal/hostaccess` carries a timeout

- **Severity**: medium
- **Where**: `internal/hostaccess/routes.go:220`, `internal/hostaccess/routes.go:230`,
  `internal/hostaccess/routes.go:296`, `internal/hostaccess/routes.go:361`, and the other
  9 sites in the file
- **Harm**: Each call uses `exec.Command`, not `exec.CommandContext`. A command that does
  not return blocks `SyncHostRoutes`, which blocks the reconcile cycle. The daemon then
  stops converging, and it reports no error. `internal/routing/routes.go:75-78` and
  `internal/daemon/daemon.go:91-95` both apply a 5-second timeout, so the pattern exists
  in the code base.
- **Condition**: `ip netns exec` blocks. A namespace in an unclean state produces this
  result.
- **Epic 3**: Yes. The move onto `execx.Runner` carries a context into every call, because
  `execx.Runner.Run` takes one; see `internal/execx/execx.go:13-18`.

### SA-29 — Nine commands discard the error that they return

- **Severity**: low
- **Where**: `cmd/hydrascale/init.go:161`, `cmd/hydrascale/init.go:283`,
  `cmd/hydrascale/init.go:300`, `cmd/hydrascale/uninstall.go:56`,
  `cmd/hydrascale/uninstall.go:58`, `cmd/hydrascale/uninstall.go:93`,
  `cmd/hydrascale/uninstall.go:121`, `internal/hostaccess/resolved.go:52`,
  `internal/namespaces/ns.go:309-311`
- **Harm**: Each call assigns the error to `_`. `CLAUDE.md` states: "A best-effort
  operation that fails silently is a defect." The teardown sites are the ones that matter:
  `internal/namespaces/ns.go:309-311` removes three `iptables` rules, and a failure there
  leaves an `ACCEPT` rule on the host after the tailnet is gone. The operator sees no
  message.
- **Condition**: The removal fails for a reason other than a rule that is already absent.
- **Epic 3**: Partly. Epic 3 covers the `internal/` sites. The `cmd/` sites report to the
  operator on the terminal and they are lower value.

### SA-30 — `__nsdaemon` executes any argument list as root

- **Severity**: low
- **Where**: `cmd/hydrascale/nsdaemon.go:40-64`
- **Harm**: The command resolves `cmdArgs[0]` with `exec.LookPath` and it then calls
  `syscall.Exec`. It holds no list of permitted programs. The command is hidden, and
  `internal/daemon/daemon.go:134-144` is the only caller. The binary is not set-user-id,
  so a caller must already be root to run it. A root caller runs any program without this
  command. The finding is therefore a note on the design, and it is not a path that raises
  a privilege.
- **Condition**: A future change makes the binary set-user-id, or a future change lets a
  non-root path reach the command.
- **Epic 3**: No. Epic 3 makes no change here. Epic 3 records the constraint that the
  binary must never become set-user-id.

### SA-31 — Two prompt answers reach `groupadd` and `usermod` without validation

- **Severity**: low
- **Where**: `cmd/hydrascale/init.go:252-260`
- **Harm**: The group name and the user name come from the prompt. Neither value is
  checked. A value that starts with a hyphen becomes an option of `groupadd` or of
  `usermod`. The operator runs `hydrascale init` as root and the operator types the value,
  so the operator can already run any command. The harm is a confusing failure, not a rise
  in privilege.
- **Condition**: The operator types a value that starts with a hyphen.
- **Epic 3**: No. Epic 3 records the finding and it makes no change.

### SA-32 — The daemon writes no IPv6 firewall rule

- **Severity**: medium
- **Where**: `internal/namespaces/ns.go:272-285` and `internal/namespaces/ns.go:345-361`
  write every firewall rule; `internal/hostaccess/routes.go:393-407` and
  `internal/hostaccess/routes.go:442-449` write and remove IPv6 host routes
- **Answer to FR-audit-13**: **An IPv6 gap exists.** The command `ip6tables` appears
  nowhere in `cmd/` or in `internal/`. This command confirms it:

  ```
  rg -c 'ip6tables' --glob '!vendor' .
  ```

  The command returns no line.
- **Detail**: The daemon writes IPv4 rules only. It writes them in two places. The host
  side writes three rules per tailnet at `internal/namespaces/ns.go:272-285`. The
  namespace side writes three rules per tailnet at `internal/namespaces/ns.go:345-361`.
  Both use `iptables`. The forward sysctl at `internal/namespaces/ns.go:266` is
  `net.ipv4.conf.<veth>.forwarding=1`, so it enables IPv4 forwarding only.
- **Harm**: The daemon propagates the IPv6 address of every peer to the host route table,
  at `internal/hostaccess/routes.go:393-399`. The host therefore holds a route to each
  peer over IPv6, and no rule of the daemon applies to the traffic on that route. Epic 3
  writes the local rule set into `HYDRASCALE-FWD` with `iptables`. That chain filters IPv4
  only, so an IPv6 path passes every local rule. `.claude/rules/access-invariants.md`
  states the intent: "Version 1.0 writes IPv4 rules only. The daemon logs at start that it
  does not filter IPv6 forwarding."
- **Condition**: The tailnet carries IPv6, and the operator expects a local rule to apply
  to all traffic.
- **Epic 3**: Partly. Epic 3 writes the IPv4 chain. Version 1.0 keeps the IPv6 gap by
  decision. See `docs/specs/features/05-reachability-model.md:231`.

### SA-33 — The daemon does not log the IPv6 gap at start

- **Severity**: low
- **Where**: `cmd/hydrascale/main.go` holds the start path and it emits no such line
- **Harm**: `.claude/rules/access-invariants.md` and
  `docs/specs/features/05-reachability-model.md:231` both require a log line at start that
  states that the daemon does not filter IPv6 forwarding. The code emits no such line. The
  operator therefore sees a set of local rules and believes that the rules cover all
  traffic. The command `rg -i 'IPv6' cmd/` returns no line in the start path.
- **Condition**: The operator reads the rule set and assumes that it covers IPv6.
- **Epic 3**: Yes. Epic 3 adds the log line together with the chain.

## The summary of the origins

| Origin | Reaches a command at | Validated |
|---|---|---|
| A constant in the source | most sites in every group | Not applicable |
| The tailnet id from the configuration file | every `ip netns exec` site | Yes, `config.LoadConfig` rejects a bad id at `internal/config/config.go:120-125` |
| The tailnet id from a control API request | `internal/api/server.go:243`, `internal/api/server.go:361` | No — see SA-25 and SA-26 |
| The `infra_subnet` value from the configuration file | `internal/namespaces/ns.go:240`, `internal/namespaces/ns.go:250` | Yes, `config.LoadConfig` parses it as an IPv4 CIDR at `internal/config/config.go:168-180` |
| The `control_url` value | `internal/daemon/daemon.go:338`, through `buildTailscaleUpArgs` | Yes, `ValidateControlURL` and `isValidControlURL` |
| The auth key | `internal/daemon/daemon.go:338` through a 0600 file, and `cmd/hydrascale/init.go:161` in `argv` | The `init` path exposes the key — see SA-22 |
| A route destination from the control server | `internal/routing/routes.go:210` and `internal/hostaccess/routes.go:380` | `internal/routing` validates it; `internal/hostaccess` does not — see SA-23 |
| A MagicDNS suffix from the control server | `internal/hostaccess/resolved.go:36` | No — see SA-24 |
| An operator answer at a prompt | `cmd/hydrascale/init.go:254`, `cmd/hydrascale/init.go:259` | No — see SA-31 |
| An operator command line | `cmd/hydrascale/main.go:553`, `cmd/hydrascale/nsdaemon.go:59` | By design; the operator is root |

## A note on the shell

No call site uses a shell. Every site passes a program name and a list of arguments to
`exec.Command`, `exec.CommandContext`, or `syscall.Exec`. A value that holds `;` or `|`
therefore stays one argument. The findings above describe argument confusion, where a
value becomes an option of the program. They do not describe shell command injection,
because no shell is present.
