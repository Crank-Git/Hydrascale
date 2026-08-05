# Security audit

This document is the security audit of Hydrascale that Epic 2 produces. It answers
**FR-audit-1** through **FR-audit-14** in `docs/specs/features/02-security-audit.md`.

The audit changes no code. Epic 3 and Epic 5 make the corrections.

The audit holds 49 findings. It collects four earlier fragments, one for each issue in
Epic 2: issue #63 covered the control API, issue #64 covered the `os/exec` call sites,
issue #65 covered the file modes and the teardown paths, and issue #66 reproduced the
forwarding defect on the test host. Issue #67 merged the four fragments into this file and
removed the fragment directory, so that the repository holds one document. The git history
keeps the fragments.

Every finding carries an identifier, a severity, at least one `file:line` reference, the
condition under which it causes harm, whether the audit reproduced it, and which epic
fixes it.

Severity describes the harm. Severity never describes the size of the fix.

## The status of the dispositions

**Every disposition in this document is a proposal. The maintainer has confirmed none of
them.** The date of the proposal is **2026-08-05**.

A proposed disposition is one of four values.

- `propose: fix in Epic 3` — a Hydrascale requirement in
  `docs/specs/features/03-security-fixes.md` covers the finding.
- `propose: fix in Epic 5` — the reachability model in
  `docs/specs/features/05-reachability-model.md` covers the finding.
- `propose: defer` — the finding needs a decision or a change that no epic on the board
  holds today.
- `propose: accept the risk` — the audit proposes that version 1.0 keeps the behaviour.

One finding is not a proposal. `SA-5` records the console threat model, and the maintainer
already accepted that risk. `docs/specs/spec.md:100-101` holds the decision, and the body
of issue #67 records it.

The maintainer confirms or reverses each proposal after this run. Until that answer
arrives, the fourth acceptance criterion of issue #67 is open.

## The summary table

| Identifier | Severity | Area | Reproduced | Proposed disposition | Which epic fixes it |
|---|---|---|---|---|---|
| `SA-1` | high | control API | no | propose: fix in Epic 3 | Epic 3, and Epic 3 must widen its scope |
| `SA-2` | high | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-3` | high | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-4` | high | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-5` | high | console | no | **accepted by the maintainer** | none |
| `SA-6` | high | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-7` | high | modes and teardown | no | propose: fix in Epic 3 | Epic 3 |
| `SA-8` | high | forwarding | **yes** | propose: fix in Epic 5 | Epic 5, which holds no issue today |
| `SA-9` | high | forwarding | **yes** | propose: fix in Epic 5 | Epic 5, which holds no issue today |
| `SA-10` | medium | control API | no | propose: defer | none |
| `SA-11` | medium | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-12` | medium | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-13` | medium | control API | no | propose: defer | Epic 6 |
| `SA-14` | medium | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-15` | medium | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-16` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-17` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-18` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-19` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-20` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-21` | medium | exec call sites | no | propose: fix in Epic 3 | Epic 3 |
| `SA-22` | medium | modes and teardown | no | propose: defer | none |
| `SA-23` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, in part |
| `SA-24` | medium | modes and teardown | no | propose: defer | none |
| `SA-25` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-26` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-27` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-28` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-29` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-30` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-31` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, in part |
| `SA-32` | medium | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-33` | medium | forwarding | **yes** | propose: fix in Epic 5 | Epic 5, which holds no issue today |
| `SA-34` | medium | forwarding | no | propose: accept the risk | none, by the version 1.0 decision |
| `SA-35` | medium | forwarding | **yes** | propose: accept the risk | none, by the version 1.0 decision |
| `SA-36` | low | control API | no | propose: fix in Epic 3 | Epic 3, in part |
| `SA-37` | low | control API | no | propose: fix in Epic 3 | Epic 3 |
| `SA-38` | low | control API | no | propose: accept the risk | none |
| `SA-39` | low | exec call sites | no | propose: fix in Epic 3 | Epic 3, in part |
| `SA-40` | low | exec call sites | no | propose: accept the risk | none |
| `SA-41` | low | exec call sites | no | propose: accept the risk | none |
| `SA-42` | low | modes and teardown | no | propose: fix in Epic 3 | Epic 3, issue #69 |
| `SA-43` | low | modes and teardown | no | propose: defer | none |
| `SA-44` | low | modes and teardown | no | propose: fix in Epic 3 | Epic 3 |
| `SA-45` | low | modes and teardown | no | propose: accept the risk | none |
| `SA-46` | low | modes and teardown | no | propose: defer | none |
| `SA-47` | low | forwarding | no | propose: fix in Epic 3 | Epic 3 |
| `SA-48` | low | forwarding | no | propose: fix in Epic 5 | Epic 5, which holds no issue today |
| `SA-49` | low | other defect | no | propose: defer | none |

The counts are 9 high findings, 26 medium findings, and 14 low findings. The audit
reproduced 4 findings on the test host: `SA-8`, `SA-9`, `SA-33`, and `SA-35`. Every other
finding comes from a read of the code.

## The two facts that change the plan of the following epics

Read these two facts before you plan Epic 3.

**Fact 1 — Epic 5 fixes `SA-8` and `SA-9`, and Epic 5 holds no issue on the board
today.** These two findings are the two high findings that the audit reproduced on a live
host. They need the `HYDRASCALE-FWD` chain and the deny default of
`docs/specs/features/05-reachability-model.md`. Epic 3 does not write that chain, so no
scheduled work fixes these two findings. `SA-33` and `SA-48` are in the same state.

**Fact 2 — `FR-fix-15` does not cover `SA-1`.** `FR-fix-15` states that the control API
never returns the contents of the secrets file. The auth key that `GET /api/status`
returns comes from the configuration file, not from the secrets file. Epic 3 must widen
the scope of issue #71 to redact `StatusResponse.Desired`, or the defect survives the
epic.

## The identifier mapping

The four fragments used four separate ranges. This document renumbers every finding into
one contiguous sequence, ordered by severity and then by area. The issue comments that
Epic 2 wrote cite the old identifiers. This table keeps them traceable.

| Old identifier | Fragment | New identifier |
|---|---|---|
| `SA-1` | `01-control-api.md` | `SA-1` |
| `SA-2` | `01-control-api.md` | `SA-2` |
| `SA-3` | `01-control-api.md` | `SA-3` |
| `SA-4` | `01-control-api.md` | `SA-4` |
| `SA-5` | `01-control-api.md` | `SA-10` |
| `SA-6` | `01-control-api.md` | `SA-11` |
| `SA-7` | `01-control-api.md` | `SA-12` |
| `SA-8` | `01-control-api.md` | `SA-13` |
| `SA-9` | `01-control-api.md` | `SA-36` |
| `SA-10` | `01-control-api.md` | `SA-49` |
| `SA-11` | `01-control-api.md` | `SA-37` |
| `SA-12` | `01-control-api.md` | `SA-38` |
| `SA-13` | `01-control-api.md` | `SA-5` |
| `SA-20` | `02-exec-call-sites.md` | `SA-16` |
| `SA-21` | `02-exec-call-sites.md` | `SA-17` |
| `SA-22` | `02-exec-call-sites.md` | `SA-6` |
| `SA-23` | `02-exec-call-sites.md` | `SA-18` |
| `SA-24` | `02-exec-call-sites.md` | `SA-19` |
| `SA-25` | `02-exec-call-sites.md` | `SA-14` |
| `SA-26` | `02-exec-call-sites.md` | `SA-15` |
| `SA-27` | `02-exec-call-sites.md` | `SA-20` |
| `SA-28` | `02-exec-call-sites.md` | `SA-21` |
| `SA-29` | `02-exec-call-sites.md` | `SA-39` |
| `SA-30` | `02-exec-call-sites.md` | `SA-40` |
| `SA-31` | `02-exec-call-sites.md` | `SA-41` |
| `SA-32` | `02-exec-call-sites.md` | `SA-34` |
| `SA-33` | `02-exec-call-sites.md` | `SA-47` |
| `SA-40` | `03-modes-and-teardown.md` | `SA-7` |
| `SA-41` | `03-modes-and-teardown.md` | `SA-22` |
| `SA-42` | `03-modes-and-teardown.md` | `SA-23` |
| `SA-43` | `03-modes-and-teardown.md` | `SA-24` |
| `SA-44` | `03-modes-and-teardown.md` | `SA-25` |
| `SA-45` | `03-modes-and-teardown.md` | `SA-26` |
| `SA-46` | `03-modes-and-teardown.md` | `SA-27` |
| `SA-47` | `03-modes-and-teardown.md` | `SA-28` |
| `SA-48` | `03-modes-and-teardown.md` | `SA-29` |
| `SA-49` | `03-modes-and-teardown.md` | `SA-30` |
| `SA-50` | `03-modes-and-teardown.md` | `SA-31` |
| `SA-51` | `03-modes-and-teardown.md` | `SA-32` |
| `SA-52` | `03-modes-and-teardown.md` | `SA-42` |
| `SA-53` | `03-modes-and-teardown.md` | `SA-43` |
| `SA-54` | `03-modes-and-teardown.md` | `SA-44` |
| `SA-55` | `03-modes-and-teardown.md` | `SA-45` |
| `SA-56` | `03-modes-and-teardown.md` | `SA-46` |
| `SA-60` | `04-forwarding.md` | `SA-8` |
| `SA-61` | `04-forwarding.md` | `SA-9` |
| `SA-62` | `04-forwarding.md` | `SA-48` |
| `SA-63` | `04-forwarding.md` | `SA-33` |
| `SA-64` | `04-forwarding.md` | `SA-35` |

Two pairs of findings describe the same code from two viewpoints, because two issues
reached the same site by two methods. The audit keeps both members of each pair, and each
member cites the other.

- `SA-3` and `SA-14` both cover the identifier that `POST /api/tailnet/add` writes.
- `SA-2` and `SA-15` both cover the identifier that `POST /api/tailnet/disconnect` joins
  onto a path.

## The control API surface

The audit reads code for this area. It ran no command on the test host. Every finding in
this area therefore states `Reproduced: no`, as `FR-audit-6` requires.

`internal/api/server.go` registers ten routes at `internal/api/server.go:50-61`. The
feature file `docs/specs/features/02-security-audit.md:104` cites the range `47-59`. The
current code holds the same ten routes at `50-61`.

The daemon serves these routes on the control socket at
`/var/lib/hydrascale/api.sock` (`internal/api/types.go:12`). The daemon binds no loopback
address today. `internal/api/server.go:88` opens a Unix socket and nothing else. Epic 6
adds the console listener. Finding `SA-5` records the threat model for that listener.

### What each route validates

| Method and path | Handler | Mutating | What the handler validates | What the handler does not validate |
|---|---|---|---|---|
| `GET /api/status` | `handleStatus` (`internal/api/server.go:130`) | no | The method equals `GET`. | The response body. The response carries the auth key of every tailnet. See `SA-1`. |
| `GET /api/events` | `handleEvents` (`internal/api/server.go:162`) | no | The method equals `GET`. | Nothing else. The route takes no input. |
| `POST /api/reconcile` | `handleReconcile` (`internal/api/server.go:173`) | yes | The method equals `POST`. | Nothing else. The route takes no body. |
| `POST /api/tailnet/add` | `handleTailnetAdd` (`internal/api/server.go:231`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. `control_url` is an absolute `http` or `https` URL. | The character set of `id`. The value of `auth_key`. The value of `exit_node`. The `https` scheme rule that the loader enforces. See `SA-3`, `SA-4`, `SA-36`, `SA-49`. |
| `POST /api/tailnet/remove` | `handleTailnetRemove` (`internal/api/server.go:282`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. `id` names a tailnet in the configuration file. | The character set of `id`. The membership test rejects an unknown value, so no unvalidated value reaches a path. |
| `POST /api/tailnet/connect` | `handleTailnetConnect` (`internal/api/server.go:327`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. | The character set of `id`. Whether `id` names a tailnet. See `SA-37`. |
| `POST /api/tailnet/disconnect` | `handleTailnetDisconnect` (`internal/api/server.go:348`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. | The character set of `id`. Whether `id` names a tailnet. The value reaches a file path. See `SA-2`. |
| `POST /api/config/dns` | `handleConfigDNS` (`internal/api/server.go:368`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `bind_address`, when it is not empty, is a loopback `host:port` value. | The value of `mode`. Any string reaches the configuration file. See `SA-11`. |
| `GET /api/config` | `handleConfig` (`internal/api/server.go:410`) | no | The method equals `GET`. | Nothing else. The handler replaces a non-empty auth key with `***` at `internal/api/server.go:434-436`. |
| `GET /api/tailnet/{id}/detail` | `handleTailnetDetail` (`internal/api/server.go:451`) | no | The route pattern restricts the method to `GET`. `id` is not empty. `id` names a tailnet in the configuration file (`internal/reconciler/reconciler.go:448-450`). | The character set of `id`. The membership test runs first, so no unvalidated value reaches a path. |

Four routes are read-only. Six routes mutate state. `POST /api/reconcile` accepts no
body. The other five mutating routes accept a JSON body. No route reads a header for an
identity, a token, or an origin.

### Paths that the daemon builds from request input

`FR-audit-10` asks for every path that the daemon builds from request input. The table
lists each one, the route that supplies the value, and the guard that stands between
them.

| Path the daemon builds | Code | Route that supplies the value | Guard |
|---|---|---|---|
| `/var/lib/hydrascale/state/<id>` | `internal/daemon/daemon.go:182` | `POST /api/tailnet/disconnect` | none. See `SA-2`. |
| `/var/lib/hydrascale/state/<id>/tailscaled.pid` | `internal/daemon/daemon.go:183` | `POST /api/tailnet/disconnect` | none. See `SA-2`. |
| `/var/lib/hydrascale/state/<id>` | `internal/reconciler/reconciler.go:285` | `POST /api/tailnet/add`, `POST /api/tailnet/remove`, `POST /api/reconcile` | `safeStateDir` calls `config.IsValidID` and then tests that the result is a direct child of the state directory (`internal/reconciler/reconciler.go:281-289`). |
| `/var/lib/hydrascale/state/<id>` and `/var/lib/hydrascale/state/<id>/tailscaled.sock` | `internal/daemon/daemon.go:88`, `:113`, `:246`, `:321`, `:439-440`, `:446` | `POST /api/tailnet/add`, `POST /api/reconcile`, `GET /api/tailnet/{id}/detail` | The reconciler reads the identifier from `DesiredState`, which calls `LoadConfig`. `LoadConfig` rejects an identifier that does not match `validIDPattern` (`internal/config/config.go:124-126`). |
| `/etc/netns/ns-<id>` and `/etc/netns/ns-<id>/resolv.conf` | `internal/namespaces/ns.go:119-120`, `:388`, `:392`, `:463`, `:465` | `POST /api/tailnet/add`, `POST /api/tailnet/remove`, `POST /api/reconcile` | The same `LoadConfig` guard. `GetNamespaceName` also prefixes the value with `ns-` (`internal/namespaces/ns.go:67`), so a value cannot start with `-` and cannot become a command flag. |
| `/etc/hydrascale/config.yaml` | `internal/api/server.go:252`, `:299`, `:388`, `:416` | every mutating route | The daemon reads the path from `Reconciler.ConfigPath`. No request supplies it. |

The prefix `ns-` at `internal/namespaces/ns.go:67` is the reason no request value reaches
`ip netns` as a flag. Record that fact before Epic 3 changes the naming.

## The `os/exec` call sites

### Method

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

### The count

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

### The summary of the origins

| Origin | Reaches a command at | Validated |
|---|---|---|
| A constant in the source | most sites in every group | Not applicable |
| The tailnet id from the configuration file | every `ip netns exec` site | Yes, `config.LoadConfig` rejects a bad id at `internal/config/config.go:120-125` |
| The tailnet id from a control API request | `internal/api/server.go:243`, `internal/api/server.go:361` | No — see `SA-14` and `SA-15` |
| The `infra_subnet` value from the configuration file | `internal/namespaces/ns.go:240`, `internal/namespaces/ns.go:250` | Yes, `config.LoadConfig` parses it as an IPv4 CIDR at `internal/config/config.go:168-180` |
| The `control_url` value | `internal/daemon/daemon.go:338`, through `buildTailscaleUpArgs` | Yes, `ValidateControlURL` and `isValidControlURL` |
| The auth key | `internal/daemon/daemon.go:338` through a 0600 file, and `cmd/hydrascale/init.go:161` in `argv` | The `init` path exposes the key — see `SA-6` |
| A route destination from the control server | `internal/routing/routes.go:210` and `internal/hostaccess/routes.go:380` | `internal/routing` validates it; `internal/hostaccess` does not — see `SA-18` |
| A MagicDNS suffix from the control server | `internal/hostaccess/resolved.go:36` | No — see `SA-19` |
| An operator answer at a prompt | `cmd/hydrascale/init.go:254`, `cmd/hydrascale/init.go:259` | No — see `SA-41` |
| An operator command line | `cmd/hydrascale/main.go:553`, `cmd/hydrascale/nsdaemon.go:59` | By design; the operator is root |

### A note on the shell

No call site uses a shell. Every site passes a program name and a list of arguments to
`exec.Command`, `exec.CommandContext`, or `syscall.Exec`. A value that holds `;` or `|`
therefore stays one argument. The findings above describe argument confusion, where a
value becomes an option of the program. They do not describe shell command injection,
because no shell is present.

## The file modes, the socket modes, and the teardown paths

### Method

The audit reads every call that sets a mode, sets an owner, or sets the process file mode
creation mask. The command that found the call sites is:

```
rg 'os\.Chmod|os\.Chown|os\.WriteFile|os\.MkdirAll|os\.OpenFile|os\.Create|syscall\.Umask' -g '!vendor' -g '*.go' -g '!*_test.go' .
```

The audit then reads every resource that setup creates, and it finds the step that removes
that resource. A resource with no removal step, or with a removal step that cannot report a
failure, becomes a finding.

Two limits apply to every mode in the tables below.

- `os.MkdirAll`, `os.WriteFile`, `os.OpenFile`, and `os.Create` pass a requested mode. The
  kernel masks that mode with the process file mode creation mask. The daemon runs under
  systemd with the default mask `0022`, so a requested `0755` becomes `0755`, a requested
  `0644` becomes `0644`, and the `0666` that `os.Create` requests becomes `0644`.
- `os.Chmod` sets the mode directly. The mask does not apply.

### Table 1 — every mode and every owner that the daemon sets

The daemon runs as root. The owner is `root:root` for every path, except the two paths that
`applySocketGroup` changes.

| Site | Call | Path | Requested mode | Effective mode | Owner |
|---|---|---|---|---|---|
| `internal/api/server.go:87` | `syscall.Umask` | the process | mask `0077` | — | — |
| `internal/api/server.go:89` | `syscall.Umask` | the process | the saved mask | — | — |
| `internal/api/server.go:93` | `os.Chmod` | `/var/lib/hydrascale/api.sock` | `0600` | `0600` | `root:root` |
| `internal/api/server.go:206` | `os.Chown` | `/var/lib/hydrascale` | — | — | `root:<socket_group>` |
| `internal/api/server.go:209` | `os.Chmod` | `/var/lib/hydrascale` | `0750` | `0750` | `root:<socket_group>` |
| `internal/api/server.go:212` | `os.Chown` | `/var/lib/hydrascale/api.sock` | — | — | `root:<socket_group>` |
| `internal/api/server.go:215` | `os.Chmod` | `/var/lib/hydrascale/api.sock` | `0660` | `0660` | `root:<socket_group>` |
| `internal/config/config.go:283` | `os.MkdirAll` | `/etc/hydrascale` | `0755` | `0755` | `root:root` |
| `internal/config/config.go:288` | `os.CreateTemp` | `/etc/hydrascale/.hydrascale-config-*.yaml` | none, `0600` implicit | `0600` | `root:root` |
| `internal/daemon/daemon.go:114` | `os.MkdirAll` | `/var/lib/hydrascale/state/<id>` | `0700` | `0700` | `root:root` |
| `internal/daemon/daemon.go:166` | `os.WriteFile` | `<state>/tailscaled.pid` | `0600` | `0600` | `root:root` |
| `internal/daemon/daemon.go:322` | `os.CreateTemp` | `<state>/authkey-*` | none, `0600` implicit | `0600` | `root:root` |
| `internal/daemon/daemon.go:328` | `f.Chmod` | `<state>/authkey-*` | `0600` | `0600` | `root:root` |
| `internal/hostaccess/hosts.go:116` | `os.CreateTemp` | `/etc/.hydrascale-hosts-*` | none, `0600` implicit | `0600` | `root:root` |
| `internal/hostaccess/hosts.go:131` | `os.Chmod` | the same temporary file | the mode of `/etc/hosts`, `0644` when the file is absent | as requested | `root:root` |
| `internal/namespaces/ns.go:389` | `os.MkdirAll` | `/etc/netns/<ns>` | `0755` | `0755` | `root:root` |
| `internal/namespaces/ns.go:400` | `os.WriteFile` | `/etc/netns/<ns>/resolv.conf` | `0644` | `0644` | `root:root` |
| `internal/reconciler/reconciler.go:562` | `os.MkdirAll` | the event log directory | `0755` | `0755` | `root:root` |
| `internal/reconciler/reconciler.go:566` | `os.OpenFile` | the event log file | `0644` | `0644` | `root:root` |
| `internal/reconciler/reconciler.go:683` | `os.Create` | `/var/lib/hydrascale/state/.lock-test` | `0666` implicit | `0644` | `root:root` |
| `internal/reconciler/reconciler.go:695` | `os.OpenFile` | `<state>/.hydrascale.lock` | `0600` | `0600` | `root:root` |
| `cmd/hydrascale/nsdaemon.go:46` | `os.MkdirAll` | `<state>/etc-upper` | `0755` | `0755` | `root:root` |
| `cmd/hydrascale/nsdaemon.go:49` | `os.MkdirAll` | `<state>/etc-work` | `0755` | `0755` | `root:root` |
| `cmd/hydrascale/init.go:96` | `os.MkdirAll` | `/etc/hydrascale`, `/var/lib/hydrascale/state`, `/var/log/hydrascale` | `0750` | `0750` | `root:root` |
| `cmd/hydrascale/init.go:284` | `os.WriteFile` | `/etc/sysctl.d/99-hydrascale.conf` | `0644` | `0644` | `root:root` |
| `cmd/hydrascale/init.go:357` | `os.WriteFile` | `<config path>.bak` | `0640` | `0640` | `root:root` |
| `cmd/hydrascale/main.go:708` | `os.MkdirAll` | `/etc/systemd/system/<svc>.service.d` | `0755` | `0755` | `root:root` |
| `cmd/hydrascale/main.go:711` | `os.WriteFile` | the same directory, `hydrascale.conf` | `0644` | `0644` | `root:root` |
| `cmd/hydrascale/main.go:774` | `os.MkdirAll` | `/etc/hydrascale` | `0755` | `0755` | `root:root` |
| `cmd/hydrascale/main.go:778` | `os.MkdirAll` | `/var/lib/hydrascale/state` | `0750` | `0750` | `root:root` |
| `cmd/hydrascale/main.go:782` | `os.MkdirAll` | `/var/log/hydrascale` | `0750` | `0750` | `root:root` |
| `cmd/hydrascale/main.go:799` | `os.WriteFile` | `/usr/local/bin/hydrascale` | `0755` | `0755` | `root:root` |
| `cmd/hydrascale/main.go:805` | `os.WriteFile` | `/etc/systemd/system/hydrascale.service` | `0644` | `0644` | `root:root` |
| `cmd/hydrascale/main.go:830` | `os.WriteFile` | `/etc/hydrascale/config.yaml` | `0640` | `0640` | `root:root` |

The counts are 6 `os.Chmod` calls, 2 `os.Chown` calls, 9 `os.WriteFile` calls, 12
`os.MkdirAll` calls, 2 `os.OpenFile` calls, 4 `os.CreateTemp` calls, 1 `os.Create` call,
and 2 `syscall.Umask` calls. Both `os.Chown` calls are in `applySocketGroup`. The daemon
sets an owner nowhere else.

Three paths get a mode from more than one writer:

- `/etc/hydrascale` is `0755` from `cmd/hydrascale/main.go:774` and from
  `internal/config/config.go:283`, and `0750` from `cmd/hydrascale/init.go:96`.
- `/var/lib/hydrascale/state` is `0750` from `cmd/hydrascale/main.go:778` and from
  `cmd/hydrascale/init.go:96`.
- `/etc/hydrascale/config.yaml` is `0640` from `cmd/hydrascale/main.go:830` and `0600`
  from `internal/config/config.go:288`, through the rename at
  `internal/config/config.go:305`.

`os.MkdirAll` applies its mode only when it creates the directory. The first writer to run
therefore sets the mode, and a later writer leaves that mode in place. `SA-23` records the
result.

### Table 2 — every resource that setup creates, against its teardown step

| Resource | Setup | Teardown | Can the teardown report a failure? |
|---|---|---|---|
| The namespace `ns-<id>` | `internal/namespaces/ns.go:81` | `internal/namespaces/ns.go:113` | Yes |
| The veth pair `vh<hash>`/`vn<hash>` | `internal/namespaces/ns.go:231` | `internal/namespaces/ns.go:315` | Yes |
| The host address on the veth | `internal/namespaces/ns.go:241` | The veth removal removes it | Yes |
| The namespace address and default route | `internal/namespaces/ns.go:251`, `internal/namespaces/ns.go:261` | The namespace removal removes both | Yes |
| The forwarding sysctl on the veth | `internal/namespaces/ns.go:266` | The veth removal removes the key | Not applicable |
| The host `FORWARD` accept rule | `internal/namespaces/ns.go:273` | `internal/namespaces/ns.go:309` | No — see `SA-39` |
| The host `FORWARD` return rule | `internal/namespaces/ns.go:278` | `internal/namespaces/ns.go:310` | No — see `SA-39` |
| The host `POSTROUTING` masquerade rule | `internal/namespaces/ns.go:285` | `internal/namespaces/ns.go:311` | No — see `SA-39` |
| The namespace `tailscale0` masquerade rule | `internal/namespaces/ns.go:346` | `internal/namespaces/ns.go:459` | No — `SA-26`, `SA-27` |
| The namespace DNS DNAT rule, UDP | `internal/namespaces/ns.go:353` | `internal/namespaces/ns.go:460` | No — `SA-26`, `SA-27` |
| The namespace DNS DNAT rule, TCP | `internal/namespaces/ns.go:360` | `internal/namespaces/ns.go:461` | No — `SA-26`, `SA-27` |
| `/etc/netns/<ns>/resolv.conf` | `internal/namespaces/ns.go:400` | `internal/namespaces/ns.go:120` and `internal/namespaces/ns.go:464` | No — `SA-28` |
| `/etc/netns/<ns>` | `internal/namespaces/ns.go:389` | `internal/namespaces/ns.go:121` and `internal/namespaces/ns.go:465` | No — `SA-28` |
| The `tailscaled` process | `internal/daemon/daemon.go:159` | `internal/daemon/daemon.go:181` | Yes |
| `<state>/tailscaled.pid` | `internal/daemon/daemon.go:166` | `internal/daemon/daemon.go:202`, `208`, `214`, `228`, `235` | No — `SA-30` |
| `<state>/tailscaled.sock` | `tailscaled` creates it | `internal/daemon/daemon.go:120` on the next start, and the state directory removal | No |
| `<state>/tailscaled.state` | `tailscaled` creates it | The state directory removal at `internal/reconciler/reconciler.go:327` | No — `SA-29` |
| `<state>/authkey-*` | `internal/daemon/daemon.go:322` | `internal/daemon/daemon.go:325`, a `defer os.Remove` | No, and the state directory removal covers it |
| `<state>/etc-upper` and `<state>/etc-work` | `cmd/hydrascale/nsdaemon.go:46` and `:49` | `cmd/hydrascale/nsdaemon.go:44` and `:45` on the next start, and the state directory removal | No — `SA-32` |
| The overlay mount on `/etc` | `cmd/hydrascale/nsdaemon.go:55` | The mount namespace ends with the process | Not applicable |
| `/var/lib/hydrascale/state/<id>` | `internal/daemon/daemon.go:114` | `internal/reconciler/reconciler.go:327` | No — `SA-29` |
| The host routes to the peers | `internal/hostaccess/routes.go:333` | `internal/hostaccess/routes.go:432`, through `TeardownAll` at `internal/reconciler/reconciler.go:633` | No — `SA-25` |
| The `/etc/hosts` block | `internal/hostaccess/hosts.go` | `syncDNS`, through `TeardownAll` at `internal/reconciler/reconciler.go:633` | No — `SA-25` |
| The systemd-resolved registration | `internal/hostaccess/resolved.go` | `DeregisterAll` at `internal/hostaccess/hostaccess.go:110` | No — see `SA-39` |
| `/var/lib/hydrascale/api.sock` | `internal/api/server.go:88` | `internal/api/server.go:125` | No — `SA-42` |
| The group ownership on `/var/lib/hydrascale` | `internal/api/server.go:206` and `:209` | None | Not applicable — `SA-22` |
| The event log file | `internal/reconciler/reconciler.go:566` | None; `uninstall` removes `/var/log/hydrascale` | No — `SA-46` |
| `<state>/.hydrascale.lock` | `internal/reconciler/reconciler.go:695` | None; the state directory holds it | Not applicable |
| `/etc/sysctl.d/99-hydrascale.conf` | `cmd/hydrascale/init.go:284` | None | No — `SA-31` |
| `<config path>.bak` | `cmd/hydrascale/init.go:357` | None | No — `SA-24` |
| `/etc/systemd/system/hydrascale.service` | `cmd/hydrascale/main.go:805` | `cmd/hydrascale/uninstall.go:91` | No — `SA-46` |
| `/etc/systemd/system/<svc>.service.d/hydrascale.conf` | `cmd/hydrascale/main.go:711` | None | No — `SA-46` |
| The unix group that `init` creates | `cmd/hydrascale/init.go:254` | None | Not applicable |
| The group membership that `init` grants | `cmd/hydrascale/init.go:259` | None | Not applicable — `SA-7` |

`SA-39` already records that `internal/namespaces/ns.go:309-311` discards the error of the
three host `iptables` removals. Table 2 cites that finding and does not restate it.

## The forwarding rules on the test host

### Method

The audit did **not** deploy a build. The test host already ran the daemon with two
tailnets, and the condition that this area examines was present. A deploy therefore
adds risk and it adds no information.

The audit observed the running host and it changed no host state. Every command is a read
of state, an ICMP echo, or one TCP connection attempt.

The test host is `phobos` at `192.168.1.221`. Its host name is `mars`. It holds two
namespaces, `ns-havoc` and `ns-jbones`, and each namespace holds one `tailscaled` that
joins a different tailnet.

### The state before the work

```
$ systemctl is-active hydrascale
active

$ sha256sum /usr/local/bin/hydrascale
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale

$ ip netns list
ns-havoc (id: 1)
ns-jbones (id: 0)
```

### The state after the work

```
$ systemctl is-active hydrascale
active

$ sha256sum /usr/local/bin/hydrascale
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale

$ ip netns list
ns-havoc (id: 1)
ns-jbones (id: 0)
```

The three values are the same before and after. The audit left the host as it found it.
The `FORWARD` chain is also the same before and after; the chain is quoted below.

### The addresses

```
$ ip -o addr show | grep -E "vh|veth"
6: vh02a1edb1c461    inet 10.200.0.165/30 scope global vh02a1edb1c461\       valid_lft forever preferred_lft forever
6: vh02a1edb1c461    inet6 fe80::607b:46ff:fef5:1172/64 scope link \       valid_lft forever preferred_lft forever
8: vh5cde1b791fe1    inet 10.200.0.85/30 scope global vh5cde1b791fe1\       valid_lft forever preferred_lft forever
8: vh5cde1b791fe1    inet6 fe80::64b9:71ff:feac:205/64 scope link \       valid_lft forever preferred_lft forever
```

```
$ sudo ip netns exec ns-havoc ip -o addr show
4: tailscale0    inet 100.121.171.43/32 scope global tailscale0\       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fd7a:115c:a1e0::fd35:ab2c/128 scope global \       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fe80::608d:bf55:5c76:88ac/64 scope link stable-privacy \       valid_lft forever preferred_lft forever
7: vn5cde1b791fe1    inet 10.200.0.86/30 scope global vn5cde1b791fe1\       valid_lft forever preferred_lft forever
7: vn5cde1b791fe1    inet6 fe80::644d:80ff:fe20:278b/64 scope link \       valid_lft forever preferred_lft forever

$ sudo ip netns exec ns-jbones ip -o addr show
4: tailscale0    inet 100.94.158.62/32 scope global tailscale0\       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fd7a:115c:a1e0::b736:9e40/128 scope global \       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fe80::a8d9:914f:9014:5f82/64 scope link stable-privacy \       valid_lft forever preferred_lft forever
5: vn02a1edb1c461    inet 10.200.0.166/30 scope global vn02a1edb1c461\       valid_lft forever preferred_lft forever
5: vn02a1edb1c461    inet6 fe80::b84e:84ff:fe9d:d3ee/64 scope link \       valid_lft forever preferred_lft forever
```

| Namespace | Host side | Host address | Namespace side | Namespace address |
|---|---|---|---|---|
| `ns-havoc` | `vh5cde1b791fe1` | `10.200.0.85` | `vn5cde1b791fe1` | `10.200.0.86` |
| `ns-jbones` | `vh02a1edb1c461` | `10.200.0.165` | `vn02a1edb1c461` | `10.200.0.166` |

The addresses agree with the values that the project manager recorded before the work.

### The chain

```
$ sudo iptables -S FORWARD
-P FORWARD DROP
-A FORWARD -j ts-forward
-A FORWARD -j DOCKER-USER
-A FORWARD -j DOCKER-FORWARD
-A FORWARD -o vh5cde1b791fe1 -m state --state RELATED,ESTABLISHED -j ACCEPT
-A FORWARD -i vh5cde1b791fe1 -j ACCEPT
-A FORWARD -o vh02a1edb1c461 -m state --state RELATED,ESTABLISHED -j ACCEPT
-A FORWARD -i vh02a1edb1c461 -j ACCEPT
```

```
$ sudo iptables -L FORWARD -v -n --line-numbers
Chain FORWARD (policy DROP 0 packets, 0 bytes)
num   pkts bytes target     prot opt in     out     source               destination
1     6142  679K ts-forward  0    --  *      *       0.0.0.0/0            0.0.0.0/0
2     6142  679K DOCKER-USER  0    --  *      *       0.0.0.0/0            0.0.0.0/0
3     6144  679K DOCKER-FORWARD  0    --  *      *       0.0.0.0/0            0.0.0.0/0
4     1527  206K ACCEPT     0    --  *      vh5cde1b791fe1  0.0.0.0/0            0.0.0.0/0            state RELATED,ESTABLISHED
5     1559  142K ACCEPT     0    --  vh5cde1b791fe1 *       0.0.0.0/0            0.0.0.0/0
6     1514  183K ACCEPT     0    --  *      vh02a1edb1c461  0.0.0.0/0            0.0.0.0/0            state RELATED,ESTABLISHED
7     1550  149K ACCEPT     0    --  vh02a1edb1c461 *       0.0.0.0/0            0.0.0.0/0
```

The supporting host state is:

```
$ sysctl net.ipv4.ip_forward net.ipv4.conf.vh5cde1b791fe1.forwarding net.ipv4.conf.vh02a1edb1c461.forwarding
net.ipv4.ip_forward = 1
net.ipv4.conf.vh5cde1b791fe1.forwarding = 1
net.ipv4.conf.vh02a1edb1c461.forwarding = 1

$ sudo iptables -t nat -S POSTROUTING
-P POSTROUTING ACCEPT
-A POSTROUTING -j ts-postrouting
-A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE
-A POSTROUTING -s 10.200.0.164/30 -j MASQUERADE
-A POSTROUTING -s 10.200.0.84/30 -j MASQUERADE
```

```
$ sudo ip netns exec ns-havoc ip route show
default via 10.200.0.85 dev vn5cde1b791fe1
10.200.0.84/30 dev vn5cde1b791fe1 proto kernel scope link src 10.200.0.86
```

The namespace holds a default route to the host, the host forwards, and the host
translates the namespace source address. The two rules at positions 5 and 7 accept on the
input interface alone. They hold no destination match and no output interface match.

### The answers to the forwarding questions

| Question | Answer |
|---|---|
| Did a namespace reach another namespace? | Yes. Both directions, `ttl=63`. See `SA-8`. |
| Did a namespace reach the host local network? | Yes. Two hosts answered. See `SA-9`. |
| Did a namespace reach the second tailnet? | Not demonstrated. See `SA-48`. |
| Where do the daemon rules sit in `FORWARD`? | Positions 4 to 7 of 7, below `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD`, although the code inserts each rule at position 1. See `SA-33`. |

The README claim that traffic from one tailnet never leaks into another holds for the
route table and for the network stack of each namespace. It does not hold for forwarding
on the host.

## The findings

The findings run in one sequence from `SA-1` to `SA-48`, ordered by severity and then by
area. `SA-49` is not a security defect; the `Other defects` section holds it.

### The high findings

#### SA-1 — `GET /api/status` returns the auth key of every tailnet

- **Severity**: high
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. The defect returns a reusable
  credential to every caller of a route, and Epic 3 owns the control API corrections.

`StatusResponse.Desired` has the type `map[string]config.Tailnet`
(`internal/api/types.go:16`). `config.Tailnet` carries the field `AuthKey`
(`internal/config/config.go:31`). The struct declares a `yaml` tag and no `json` tag, so
`encoding/json` writes the Go field name. `internal/api/server.go:148-159` encodes the
value and returns it.

A probe confirms the encoded shape:

```
{"ID":"corp","ExitNode":"","AuthKey":"tskey-auth-SECRET","HostAccess":null,"ControlURL":""}
```

`GET /api/config` redacts the same value at `internal/api/server.go:434-436`. `GET
/api/status` does not.

**Condition for harm.** The configuration file holds an auth key, and an account other
than root reaches the control socket. `socket_group` grants that reach
(`internal/config/config.go:74`, `internal/api/server.go:97-103`). The account then reads
a reusable Tailscale auth key and joins its own device to the tailnet.

**Epic 3.** Partly. `FR-fix-15` states that the control API never returns the contents of
the secrets file. This auth key comes from the configuration file rather than from the
secrets file, so no current requirement covers it. Epic 3 issue #71 must redact
`Desired` as well, or the defect survives the epic.

#### SA-2 — `POST /api/tailnet/disconnect` builds a file path from an unvalidated identifier

- **Severity**: high
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `FR-fix-11` and `FR-fix-13` already
  name the correction, and issue #71 holds the work.

`internal/api/server.go:360-365` tests only that `req.ID` is not empty. It then calls
`s.reconciler.StopDaemon(req.ID)`. `internal/reconciler/reconciler.go:457-459` passes the
value to `r.dm.Stop`. `internal/daemon/daemon.go:182-183` joins the value onto the state
directory:

```go
stateDir := filepath.Join(DefaultStateDir, tailnetID)
pidPath := filepath.Join(stateDir, "tailscaled.pid")
```

`filepath.Join` resolves `..` elements, so the identifier `../../../../tmp/attacker`
produces `/tmp/attacker/tailscaled.pid`. The route runs no membership test, so the value
never meets `config.IsValidID`. The guard that `safeStateDir` applies
(`internal/reconciler/reconciler.go:281-289`) covers the reconciler path only. It does
not cover this route.

The daemon then reads that file, parses a process identifier, tests it with
`validatePID`, and sends `SIGTERM` and later `SIGKILL`
(`internal/daemon/daemon.go:213`, `:227`). The daemon runs as root. The daemon also
removes the file it read (`internal/daemon/daemon.go:202`, `:208`, `:214`, `:228`,
`:235`).

**Condition for harm.** An account reaches the control socket and can create a directory
that the root daemon can read. `SA-10` describes how weak the process test is. Together
the two findings let that account stop a chosen root process and remove a chosen file
named `tailscaled.pid`.

`SA-15` records the same call chain from the `os/exec` viewpoint.

**Epic 3.** Yes. `FR-fix-11` and `FR-fix-13` cover it. Issue #71 builds it.

#### SA-3 — `POST /api/tailnet/add` writes an unvalidated identifier into the configuration file

- **Severity**: high
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. One request stops the daemon from
  managing every tailnet, and `FR-fix-9` through `FR-fix-11` name the correction.

`internal/api/server.go:243-246` tests only that `req.ID` is not empty.
`internal/api/server.go:266-277` appends the value to the configuration and calls
`config.SaveConfig`. `SaveConfig` validates no identifier
(`internal/config/config.go:268-311`).

`LoadConfig` does validate. `internal/config/config.go:124-126` rejects an identifier
that does not match `validIDPattern` at `internal/config/config.go:19`. The write happens
first and the rejection happens after it.

The result is a configuration file that the daemon cannot read. Every later call to
`LoadConfig` fails, so these routes fail as well:

- `GET /api/status` returns HTTP 500 (`internal/api/server.go:136-140`).
- `GET /api/config` returns HTTP 500 (`internal/api/server.go:417-421`).
- `POST /api/tailnet/remove` cannot undo the change (`internal/api/server.go:300-303`).
- `POST /api/reconcile` returns the load error on every cycle
  (`internal/reconciler/reconciler.go:390-392`).
- The reconciler loop logs the same error every interval
  (`internal/reconciler/reconciler.go:427-429`).

Only an edit of `/etc/hydrascale/config.yaml` by hand restores service.

**Condition for harm.** An account reaches the control socket and sends one request with
an identifier such as `My Net`. The daemon stops managing every tailnet until the
operator edits the file. The route reports HTTP 200 with `{"ok":false}`, so the caller
sees a soft failure rather than a rejection.

`SA-14` records the same route from the `os/exec` viewpoint.

**Epic 3.** Yes. `FR-fix-9`, `FR-fix-10`, and `FR-fix-11` cover it. Issue #71 builds it.

#### SA-4 — The route and the loader disagree about the control URL scheme

- **Severity**: high
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `FR-fix-12` names the correction and
  it settles the loopback exception at the same time.

`isValidControlURL` accepts the scheme `http` or `https`
(`internal/api/server.go:223-229`):

```go
return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
```

`ValidateControlURL` accepts `https` only (`internal/config/config.go:250-252`):

```go
if u.Scheme != "https" {
	return fmt.Errorf("control_url %q must use https scheme", raw)
}
```

`POST /api/tailnet/add` calls the first function at `internal/api/server.go:247`. It
writes the value at `internal/api/server.go:270`. `LoadConfig` calls the second function
at `internal/config/config.go:132`.

**Condition for harm.** An operator adds a tailnet with the control URL
`http://headscale.example.com`. The route accepts the value and writes it. The daemon
then reaches the state that `SA-3` describes: the configuration file no longer loads and
every route fails. The narrower harm is the plain `http` scheme itself, which sends the
auth key to the control server without transport security.

**Epic 3.** Yes. `FR-fix-12` covers it, and it also settles the loopback exception that
neither function holds today.

#### SA-5 — The console threat model — accepted

- **Severity**: high
- **Reproduced**: no
- **Area**: console
- **Disposition**: **accepted by the maintainer.** This finding is not a proposal. The
  maintainer accepted the risk before this run, and `docs/specs/spec.md:100-101` holds
  the decision.

This finding records a decision. It does not ask for a change.

`docs/specs/spec.md:100-101` states the decision:

> Version 1.0 does not add an account system to the console. The console serves the
> loopback address only and it has no sign-in.

The daemon runs as root. Epic 6 makes the daemon serve the console on a loopback address.
The console drives the same handlers that this audit covers, so it can add a tailnet,
remove a tailnet, stop a daemon, and write the resolver configuration. No handler tests
an identity (`internal/api/server.go:130-515`).

**Condition for harm.** Any local account on the host connects to the loopback port. That
account then holds the full control API of a root daemon. The same holds for any process
that account runs, including a browser extension and any program that can send an HTTP
request. `socket_group` grants the same reach over the control socket today
(`internal/api/server.go:97-103`, `internal/config/config.go:70-74`).

**The accepted position.** The operator is the single administrator of a single host and
already holds root (`docs/specs/spec.md:114-115`). A sign-in that protects root from
root adds a credential store and no protection. The operator reaches a remote host
through an SSH port forward rather than through a network listener
(`docs/specs/spec.md:102-103`).

**Epic 3.** No. Epic 3 does not change this. `FR-fix-5` and `FR-fix-6` make the daemon
and the documentation state that `socket_group` membership equals root, which writes the
same fact down for the socket.

#### SA-6 — The auth key reaches `argv` in `hydrascale init`

- **Severity**: high
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. The daemon path already holds the
  correct pattern, so the change is small and it removes a credential from `argv`.
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

#### SA-7 — Membership of `socket_group` is equivalent to root, and no document says so

- **Severity**: high
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #70 holds the work, and
  `FR-fix-5`, `FR-fix-6`, and `FR-fix-8` already state the sentence to write.
- **Where**: `internal/api/server.go:97-103`, `internal/api/server.go:192-221`,
  `README.md:337-362`, `README.md:309-313`, `cmd/hydrascale/init.go:242-268`
- **Harm**: `applySocketGroup` gives the named group mode `0660` on
  `/var/lib/hydrascale/api.sock` and mode `0750` on the parent directory. No route on the
  server checks an identity. A member of the group therefore reaches every mutating route,
  and the daemon runs every resulting command as root. The route
  `POST /api/tailnet/add` accepts a `control_url`, so a group member points a tailnet at a
  control server of its choice. The daemon then runs `ip netns exec ... tailscaled` as
  root against that server. Membership of the group is equivalent to root on the host.
  Three documents describe the feature and none states the equivalence. `README.md:337-362`
  calls it "Remote Access" and says "A member of the group then reaches the control API
  without root". `README.md:309-313` describes the key in the example configuration file.
  `cmd/hydrascale/init.go:242-268` prints "Enable non-root access via a unix group?" and it
  prints no warning. `docs/specs/features/02-security-audit.md:29` already states the
  conclusion, so the specification agrees with this finding.
- **Condition**: The operator sets `socket_group`, and a user who is not trusted with root
  is a member of that group. `cmd/hydrascale/init.go:253-259` offers to create the group
  and to add `$SUDO_USER` to it, with the default answer "yes", so the common path reaches
  this state.
- **Epic 3**: Yes. Issue #70 writes the sentence. `docs/specs/features/03-security-fixes.md`
  holds it as **FR-fix-5**, **FR-fix-6**, and **FR-fix-8**.

#### SA-8 — A namespace reaches another namespace

- **Severity**: high
- **Reproduced**: **yes, on the test host**
- **Area**: forwarding
- **Proposed disposition**: `propose: fix in Epic 5`. The correction needs the
  `HYDRASCALE-FWD` chain and the deny default, and only Epic 5 holds that design.

`internal/namespaces/ns.go:273` writes one rule for each namespace:

```
iptables -I FORWARD 1 -i vh<hash> -j ACCEPT
```

The rule accepts every packet that enters the host on that interface, to every
destination. The default route at `internal/namespaces/ns.go:261` sends every packet from
the namespace to the host, and `internal/namespaces/ns.go:266` enables forwarding on the
host side of the pair. The host route table holds a route to each other namespace.

The audit ran, in both directions:

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 10.200.0.166
PING 10.200.0.166 (10.200.0.166) 56(84) bytes of data.
64 bytes from 10.200.0.166: icmp_seq=1 ttl=63 time=0.113 ms

--- 10.200.0.166 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.113/0.113/0.113/0.000 ms
exit=0
```

```
$ sudo ip netns exec ns-jbones ping -c1 -W2 10.200.0.86
PING 10.200.0.86 (10.200.0.86) 56(84) bytes of data.
64 bytes from 10.200.0.86: icmp_seq=1 ttl=63 time=0.099 ms

--- 10.200.0.86 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.099/0.099/0.099/0.000 ms
exit=0
```

The audit also reached the host side of the other pair:

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 10.200.0.165
PING 10.200.0.165 (10.200.0.165) 56(84) bytes of data.
64 bytes from 10.200.0.165: icmp_seq=1 ttl=64 time=0.052 ms

--- 10.200.0.165 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.052/0.052/0.052/0.000 ms
exit=0
```

The first two replies carry `ttl=63`. The initial value is 64, therefore the packet
crossed one router: the host forwarded it from one veth pair to the other. The third reply
carries `ttl=64`, because `10.200.0.165` is an address of the host itself and the packet
did not cross the `FORWARD` chain.

**The mechanism.** The chain policy is `DROP`, and the packet from `ns-havoc` to
`10.200.0.166` does not match a rule that a different service owns. It matches rule 5,
`-i vh5cde1b791fe1 -j ACCEPT`, because the rule tests the input interface alone. The reply
matches rule 6, the `RELATED,ESTABLISHED` rule of the other pair. The `DROP` policy
therefore stops nothing here. The policy only stops a packet that enters on an interface
that holds no daemon rule.

**The harm condition.** A process in one namespace reaches the veth address of every other
namespace. The harm becomes a cross-tailnet harm when a service in a namespace listens on
its veth address, or when the second namespace forwards. See `SA-48` for what the audit
could not demonstrate.

**Fix.** Epic 5, not Epic 3. `docs/specs/features/05-reachability-model.md`
**FR-access-1** to **FR-access-4** replace the unrestricted `ACCEPT` with the
`HYDRASCALE-FWD` chain, and the default in that chain is deny. The Epic 5 exit criterion
in `docs/specs/spec.md:464` states that "the test host proves that a namespace cannot
reach another namespace without a rule". **Epic 5 holds no issue on the board today.**

#### SA-9 — A namespace reaches the host local network

- **Severity**: high
- **Reproduced**: **yes, on the test host**
- **Area**: forwarding
- **Proposed disposition**: `propose: fix in Epic 5`. The same deny default corrects this
  finding and `SA-8` together.

The same rule at `internal/namespaces/ns.go:273` accepts a packet to any destination, and
`internal/namespaces/ns.go:285` translates the namespace source address to the host
address. A namespace therefore reaches the host local network as though it were the host.

The audit made one connection attempt to the local network gateway, and one to a second
known host:

```
$ sudo ip netns exec ns-havoc nc -vz -w2 192.168.1.1 22
nc: connect to 192.168.1.1 port 22 (tcp) failed: Connection refused
exit=1

$ sudo ip netns exec ns-havoc nc -vz -w2 192.168.1.215 22
Connection to 192.168.1.215 22 port [tcp/ssh] succeeded!
exit=0
```

Both results prove reachability. The gateway answered with a TCP reset, which the tool
reports as "Connection refused"; the packet therefore reached the gateway and the gateway
answered. The second host completed the TCP handshake.

**The harm condition.** A tailnet peer that gains code execution inside one namespace
reaches every host on the operator local network, with the source address of the host. The
local network sees the traffic as host traffic, therefore a local network firewall that
trusts the host trusts the namespace.

**Fix.** Epic 5, as for `SA-8`. `docs/specs/features/05-reachability-model.md` makes the
default deny, therefore a packet to the local network needs an explicit rule. **Epic 5
holds no issue on the board today.**

### The medium findings

#### SA-10 — `validatePID` accepts any process whose command line contains `tailscaled`

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: defer`. No requirement names `validatePID`, and the
  correction for `SA-2` removes the path that a request uses to reach it.

`internal/daemon/daemon.go:495-502`:

```go
func validatePID(pid int) bool {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "tailscaled")
}
```

The test is a substring test on the whole command line. A command line such as
`grep tailscaled /var/log/syslog` passes it. The test does not compare the executable
path and it does not compare the process owner.

`POST /api/tailnet/disconnect` reaches this code through
`internal/daemon/daemon.go:201`. `internal/daemon/daemon.go:460` reaches it as well, on
the daemon start path.

**Condition for harm.** A caller controls the contents of a `tailscaled.pid` file, which
`SA-2` makes possible. The caller writes the process identifier of a chosen process and
names it so that its command line contains `tailscaled`. The root daemon then signals
that process.

**Epic 3.** No. No functional requirement in `docs/specs/features/03-security-fixes.md`
names `validatePID`. `FR-fix-13` removes the path that reaches it from a request, which
reduces the exposure. The weak test itself stays.

#### SA-11 — `POST /api/config/dns` writes `mode` without validation

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `FR-fix-9` requires every mutating
  route to validate its body, and this route is one of them.

`internal/api/server.go:395-397` copies the field with no test:

```go
if req.Mode != "" {
	cfg.Resolver.Mode = req.Mode
}
```

The route validates `bind_address` at `internal/api/server.go:381-386` and it validates
nothing else. `LoadConfig` applies no rule to `Resolver.Mode` either; it sets the default
`unified` only when the value is empty (`internal/config/config.go:150-152`).

`cmd/hydrascale/main.go:427` reads `cfg.Resolver.BindAddress`. No code compares
`Resolver.Mode` against a known set, so an unknown mode stays in the configuration file
and the resolver behaviour does not match what the file declares.

**Condition for harm.** A caller sets a mode that no code recognises. The operator reads
the configuration file, believes the resolver runs in that mode, and draws a wrong
conclusion about which names resolve. The daemon reports no error.

**Epic 3.** Yes. `FR-fix-9` requires every mutating route to validate its body before it
acts. Issue #71 must name the permitted mode values.

#### SA-12 — A failed mutating route returns HTTP 200

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `FR-fix-10` names the correction, and
  the console of Epic 6 depends on a correct status code.

`writeReconcileResponse` always returns HTTP 200 (`internal/api/server.go:183-190`). It
sets `ok` to `false` and it puts the error text in `message`. Five kinds of failure
report through it:

- the duplicate tailnet test (`internal/api/server.go:261`),
- the unknown tailnet test (`internal/api/server.go:315`),
- the configuration load failure (`internal/api/server.go:255`, `:302`, `:391`),
- the configuration save failure (`internal/api/server.go:275`, `:320`, `:403`),
- the reconcile failure (`internal/api/server.go:279`, `:324`, `:345`, `:407`).

The routes that do return HTTP 400 use `http.Error`, which writes a plain text body
(`internal/api/server.go:244`, `:248`, `:295`, `:340`, `:361`, `:383`). The project
convention in `CLAUDE.md` requires the body `{"error": "<message>"}`.

**Condition for harm.** A client treats HTTP 200 as success. The console, the terminal
interface, and any script must read the `ok` field to learn that the change did not
happen. A client that does not read it reports a change that the daemon refused.

**Epic 3.** Yes. `FR-fix-10` covers it.

#### SA-13 — No route checks `Content-Type` or `Origin`

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: defer`. Epic 6 adds the loopback listener that
  creates the exposure, so the origin test belongs with that work.

Each mutating route decodes the body directly (`internal/api/server.go:239`, `:290`,
`:335`, `:356`, `:376`). No handler reads the `Content-Type` header of the request. No
handler reads the `Origin` header or the `Sec-Fetch-Site` header. No handler sets
`DisallowUnknownFields`, so an unknown field passes without a report.

The daemon binds a Unix socket today (`internal/api/server.go:88`), so no browser reaches
these routes. Epic 6 binds the console to a loopback address. After that change a web
page in the operator browser can send a form request to the loopback port. A form request
with the encoding `text/plain` carries a body that these handlers parse as JSON, and the
browser sends it without a preflight request.

**Condition for harm.** Epic 6 binds the console, the operator opens a page that another
host serves, and that page posts to the console port. The request removes a tailnet or
writes a control URL. The severity becomes high on the day the loopback listener exists.

**Epic 3.** No. No functional requirement in Epic 3 names an origin test.
`docs/specs/features/06-console-foundation.md` owns the console listener, so the test
belongs with it. Record this finding for the console work.

#### SA-14 — The control API accepts a tailnet id that the configuration loader rejects

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `SA-3` records the same route at
  high severity, and one correction closes both findings.
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

#### SA-15 — A tailnet id from the control API reaches a path without validation

- **Severity**: medium
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `SA-2` records the same call chain
  at high severity, and one correction closes both findings.
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

#### SA-16 — `internal/hostaccess` calls `os/exec` 17 times

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. Epic 0 already moved two packages
  onto `execx.Runner`, so the pattern and the test approach exist.
- **Where**: `internal/hostaccess/routes.go:220`, `internal/hostaccess/routes.go:296`,
  `internal/hostaccess/routes.go:380`, `internal/hostaccess/resolved.go:19`,
  `internal/hostaccess/resolved.go:36`, and 12 more sites in the same two files
- **Harm**: The package writes host routes and host DNS configuration. No test can assert
  the argument list of a direct call, so a wrong argument reaches the host and no test
  reports it. The route commands run against the host main route table, and a wrong
  destination removes host connectivity.
- **Condition**: A change alters the argument list of one of the 17 sites, and the test
  suite stays green. `SA-18` and `SA-21` are two defects that this state already hides.
- **Epic 3**: Yes. Epic 3 moves the package onto `execx.Runner`, in the same manner as
  Epic 0 moved `internal/namespaces` and `internal/routing`.

#### SA-17 — `internal/daemon` calls `os/exec` 6 times

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. One of the six call sites handles the
  auth key, so a test that asserts the argument list has direct security value.
- **Where**: `internal/daemon/daemon.go:94`, `internal/daemon/daemon.go:149`,
  `internal/daemon/daemon.go:252`, `internal/daemon/daemon.go:338`,
  `internal/daemon/daemon.go:402`, `internal/daemon/daemon.go:424`
- **Harm**: The package starts `tailscaled` and it runs `tailscale up` with the auth key.
  No test asserts the argument list, so a change that puts the auth key back into `argv`
  passes the test suite. `daemon.go:338` is the one command in the code base that handles
  a secret.
- **Condition**: A change puts the auth key back into `argv` at
  `internal/daemon/daemon.go:338`, and no test reports it. `SA-6` records the same defect
  at the `init` call site, where it is present today.
- **Epic 3**: Yes. Epic 3 moves the package onto `execx.Runner`.

#### SA-18 — A route destination from the control server reaches `ip` without validation

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. `internal/routing` already holds the
  correct pattern, so the change copies an existing guard.
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

#### SA-19 — A domain from the control server reaches `resolvectl` without validation

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. The value comes from a host the
  operator does not control, and a DNS name test is small.
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

#### SA-20 — `tailscaled` inherits the complete environment of the daemon

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. `AuthorizeDaemon` already sets a
  minimal environment, so the correction copies one line.
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

#### SA-21 — No command in `internal/hostaccess` carries a timeout

- **Severity**: medium
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3`. The move onto `execx.Runner` for
  `SA-16` carries a context, so this finding closes with that work.
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

#### SA-22 — `applySocketGroup` changes the owner and the mode of the socket's parent directory

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: defer`. No issue in Epic 3 corrects the
  directory-wide change or the absent restore step, and the current caller passes a
  directory the daemon owns.
- **Where**: `internal/api/server.go:204-211`
- **Harm**: The function computes `dir := filepath.Dir(socketPath)` and it then applies
  `os.Chown(dir, 0, gid)` and `os.Chmod(dir, 0750)`. The path is a parameter of
  `api.NewServer`, not a constant of the function. Today `cmd/hydrascale/main.go:485`
  passes `api.DefaultSocketPath`, which is `/var/lib/hydrascale` plus `api.sock`, so the
  directory is one the daemon owns. A future caller that passes a socket in `/run` or in
  `/var/run` makes the daemon change the owner and the mode of a shared system directory.
  The function also has no step that restores the previous owner or the previous mode, so a
  later change of `socket_group` to the empty value leaves the old group in place until the
  operator corrects it by hand.
- **Condition**: A caller passes a socket path whose parent directory holds files that
  belong to another program, or the operator clears `socket_group` and expects the earlier
  ownership to return.
- **Epic 3**: Partly. Issue #70 covers the documentation of `socket_group`. The
  directory-wide change and the absent restore step are recorded here and no issue in
  Epic 3 corrects them.

#### SA-23 — Three writers set the mode of `/etc/hydrascale` and of `config.yaml`, and one mode is implicit

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3` for the explicit mode and the three
  directory modes. The removal of `auth_key` from the configuration file needs a separate
  decision, which this audit does not make.
- **Where**: `internal/config/config.go:283`, `internal/config/config.go:288`,
  `internal/config/config.go:305`, `cmd/hydrascale/main.go:774`,
  `cmd/hydrascale/main.go:830`, `cmd/hydrascale/init.go:96`, `internal/config/config.go:31`
- **Harm**: `internal/config/config.go:31` declares `AuthKey string` with the YAML key
  `auth_key`, and `README.md:318` documents that key in the example configuration file. The
  configuration file therefore holds a secret on the documented path. `CLAUDE.md` states
  that an auth key "never enter[s] the configuration file", so the code and the convention
  disagree. The mode of that file depends on the writer. `cmd/hydrascale/main.go:830` writes
  it at `0640`. `SaveConfig` writes a temporary file with `os.CreateTemp` at
  `internal/config/config.go:288` and it renames that file over the target at
  `internal/config/config.go:305`. `os.CreateTemp` takes no mode argument, so the mode is
  the `0600` that the standard library chooses. `SaveConfig` calls no `os.Chmod`, unlike
  `internal/hostaccess/hosts.go:131`, which sets the mode of its temporary file before the
  rename. The mode of a file that holds a secret therefore comes from a default of the
  standard library, and every `SaveConfig` call silently replaces the mode that the operator
  set. The directory mode has the same problem: `0755` from
  `cmd/hydrascale/main.go:774` and from `internal/config/config.go:283`, and `0750` from
  `cmd/hydrascale/init.go:96`. `os.MkdirAll` applies a mode only when it creates the
  directory, so the mode comes from whichever command the operator ran first.
- **Condition**: The operator writes an auth key into `config.yaml`, then sets a stricter
  mode on that file by hand, and the daemon or the command line later calls `SaveConfig`.
- **Epic 3**: Partly. Epic 3 sets an explicit mode on the temporary file in `SaveConfig`
  and it makes the three directory modes agree. The removal of `auth_key` from the
  configuration file is a separate decision and this audit does not make it.

#### SA-24 — `backupFile` writes a copy of the configuration file and nothing removes it

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: defer`. The removal of the backup file belongs with
  the `auth_key` decision in `SA-23`, and no epic holds that decision today.
- **Where**: `cmd/hydrascale/init.go:352-358`, `cmd/hydrascale/uninstall.go:84-101`
- **Harm**: `backupFile` reads the configuration file and it writes the bytes to
  `<path>.bak` at mode `0640`. The source file can hold an `auth_key`; see `SA-23`. No step
  in the code removes the backup. `runUninstall` removes `/var/lib/hydrascale`,
  `/var/log/hydrascale`, and the systemd unit. It removes `/etc/hydrascale` only when the
  operator passes `--purge`; see `cmd/hydrascale/uninstall.go:94-100`. An uninstall without
  `--purge` therefore leaves `/etc/hydrascale/config.yaml.bak` on the host, with the auth
  key of every tailnet the operator configured, after every other part of Hydrascale is
  gone.
- **Condition**: The operator uses an `auth_key` in the configuration file, a command calls
  `backupFile`, and the operator later runs `hydrascale uninstall` without `--purge`.
- **Epic 3**: No. Epic 3 records the finding and it makes no change. The removal of the
  backup file belongs with the decision on `auth_key` in `SA-23`.

#### SA-25 — `hostaccess.Manager.Teardown` has no caller, so a removed tailnet keeps its `/etc/hosts` entries

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work, and the
  host resolves a name for a tailnet it no longer joins until the correction lands.
- **Where**: `internal/hostaccess/hostaccess.go:77-90`,
  `internal/reconciler/reconciler.go:633`
- **Harm**: `Manager.Teardown(tailnetID)` deletes the tailnet from `m.activeTailnets`, it
  calls `RemoveAllHostRoutes`, and it calls `syncDNS`. This command finds its callers:

  ```
  rg -n 'ha\.Teardown|TeardownAll|\.Teardown\(' -g '!vendor' -g '!*_test.go' .
  ```

  The command returns `internal/reconciler/reconciler.go:633`, which calls `TeardownAll`,
  and it returns the two declarations. Nothing calls the per-tailnet `Teardown`. The
  reconciler removes a tailnet through `ActionDeleteNS` at
  `internal/reconciler/reconciler.go:313-331`, and that branch calls `r.ns.Delete` and the
  state directory removal only. The tailnet therefore stays in `m.activeTailnets` until the
  daemon stops, and `syncDNS` keeps writing its names into the `/etc/hosts` block. The host
  resolves a name to an address on a tailnet it no longer joins, and the operator sees no
  message. The host routes go away for a different reason: the kernel removes a route when
  `internal/namespaces/ns.go:315` deletes the veth device that the route uses.
- **Condition**: The operator removes one tailnet while the daemon keeps running, and
  `host_access` is enabled for that tailnet.
- **Epic 3**: Yes. Issue #69 calls `Teardown` from the `ActionDeleteNS` branch.

#### SA-26 — `namespaces.TeardownHostAccess` has no caller, so `host_access: false` leaves its rules in place

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work. The
  operator sets a reachability value to false and the reachability does not change.
- **Where**: `internal/namespaces/ns.go:445-466`,
  `internal/reconciler/reconciler.go:222-225`,
  `internal/reconciler/reconciler.go:306-312`
- **Harm**: `SetupHostAccess` writes three `iptables` rules inside the namespace: a
  masquerade on `tailscale0` at `internal/namespaces/ns.go:346` and two DNS DNAT rules at
  `internal/namespaces/ns.go:353` and `internal/namespaces/ns.go:360`.
  `TeardownHostAccess` removes those three rules. This command finds its callers:

  ```
  rg -n 'TeardownHostAccess' -g '!vendor' .
  ```

  The command returns the two declarations at `internal/namespaces/ns.go:446` and
  `internal/namespaces/ns.go:451`, and the one internal call at
  `internal/namespaces/ns.go:447`. No package outside `internal/namespaces` calls it. The
  reconciler emits `ActionSyncHostAccess` only when `cfg.TailnetHostAccess(id)` is true;
  see `internal/reconciler/reconciler.go:223`. When the operator sets `host_access: false`
  on a live tailnet, the reconciler stops emitting the action and it emits no teardown, so
  the three rules stay. The DNS DNAT rules redirect every port 53 packet that enters the
  namespace on the veth to `100.100.100.100`, and the masquerade rule keeps forwarding host
  traffic to the peers. The operator set the value to false and the reachability did not
  change. Namespace deletion is the only step that removes the rules, because the rules
  belong to the namespace.
- **Condition**: The operator sets `host_access: false` on a tailnet that is running with
  `host_access: true`, and the operator does not delete the tailnet.
- **Epic 3**: Yes. Issue #69 emits a teardown action when `host_access` changes to false.

#### SA-27 — `TeardownHostAccess` cannot report a failure, because its signature returns nothing

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 must change the signature
  before the correction for `SA-26` can report anything.
- **Where**: `internal/namespaces/ns.go:446`, `internal/namespaces/ns.go:451`
- **Harm**: Both declarations read `func TeardownHostAccess(nsName string, index int,
  infraSubnet string)` with no result. The function performs six removals: three
  `iptables` commands at `internal/namespaces/ns.go:459-461` and two `os.Remove` calls at
  `internal/namespaces/ns.go:464-465`, and it returns early at
  `internal/namespaces/ns.go:456` when `VethIPs` fails. Not one of the six can reach a
  caller, because there is no result to carry it. This is a step beyond `SA-39`: `SA-39`
  records call sites that assign an error to `_`, and a future correction there needs only
  a changed assignment. Here the signature itself must change before any correction is
  possible. `CLAUDE.md` states: "A best-effort operation that fails silently is a defect."
  The matching setup function, `SetupHostAccess` at `internal/namespaces/ns.go:337`,
  returns an error, so the package is not consistent with itself.
- **Condition**: A caller exists that would act on the failure. `SA-26` records that no
  caller exists yet, so this finding is a defect that the correction for `SA-26` must fix
  first.
- **Epic 3**: Yes. Issue #69 changes the signature to return an error, and it collects the
  six errors and returns them together, as `CLAUDE.md` requires for a cleanup step.

#### SA-28 — Four removals under `/etc/netns` discard their errors

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work, and a stale
  `resolv.conf` changes name resolution for a later namespace.
- **Where**: `internal/namespaces/ns.go:120`, `internal/namespaces/ns.go:121`,
  `internal/namespaces/ns.go:464`, `internal/namespaces/ns.go:465`
- **Harm**: `Delete` removes `/etc/netns/<ns>/resolv.conf` and then `/etc/netns/<ns>`, and
  it assigns both errors to `_`. `TeardownHostAccess` repeats the same pair and discards
  both results as well. `os.Remove` on a directory fails when the directory is not empty.
  `ip netns exec` bind-mounts every file under `/etc/netns/<ns>` over `/etc`, so a file that
  the operator or a future version places there survives the namespace. The directory then
  survives the namespace too, and it holds a `resolv.conf` that names the upstream servers
  of the earlier host configuration. A later namespace with the same tailnet id inherits
  that stale `resolv.conf` through the same bind mount, and `WriteNamespaceResolvConf` at
  `internal/namespaces/ns.go:387` overwrites it only when the reconciler reaches
  `ActionCreateNS` or `ActionStartDaemon`. The operator sees no message in either path.
  This is beyond `SA-39`, which covers `internal/namespaces/ns.go:309-311` alone.
- **Condition**: The directory `/etc/netns/<ns>` holds a file that Hydrascale did not
  write, or the removal fails for a permission reason.
- **Epic 3**: Yes. Issue #69 collects these errors and returns them with the other teardown
  errors.

#### SA-29 — The state directory removal only writes a log line, and the directory holds the node private key

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work. The
  reconcile cycle reports success for a removal that did not happen.
- **Where**: `internal/reconciler/reconciler.go:325-330`
- **Harm**: The `ActionDeleteNS` branch calls `os.RemoveAll(stateDir)` and, on a failure,
  it calls `log.Printf` and it returns `nil`. The reconcile cycle therefore reports success
  for a removal that did not happen. The directory `/var/lib/hydrascale/state/<id>` holds
  `tailscaled.state`, which holds the node private key of that tailnet, and it holds the
  `etc-upper` overlay tree. The operator removed the tailnet from the configuration file,
  the console and `hydrascale status` show it as gone, and the key stays on disk. The mode
  of the directory is `0700` and the owner is `root:root`; see
  `internal/daemon/daemon.go:114`. Only root reads the key, so this is a persistence
  finding rather than a disclosure finding. The `safeStateDir` guard at
  `internal/reconciler/reconciler.go:281-291` is correct and this finding does not question
  it.
- **Condition**: `os.RemoveAll` fails. A file under the directory that is held open, an
  immutable attribute, or a read-only mount produces this result.
- **Epic 3**: Yes. Issue #69 returns the error and it records the failure as a tailnet
  error state, so the console shows it.

#### SA-30 — `StopDaemon` discards the error of five `os.Remove` calls on the PID file

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work, and a stale
  PID file lets the daemon signal the `tailscaled` of another tailnet.
- **Where**: `internal/daemon/daemon.go:202`, `internal/daemon/daemon.go:208`,
  `internal/daemon/daemon.go:214`, `internal/daemon/daemon.go:228`,
  `internal/daemon/daemon.go:235`
- **Harm**: Every exit path of `StopDaemon` calls `os.Remove(pidPath)` and none checks the
  result. Two of the five paths return `nil`, so a caller sees a clean stop while the file
  is still on disk. `StartDaemon` at `internal/daemon/daemon.go:164-170` then writes a new
  PID over the old one, so a normal restart corrects the state. The harm appears when the
  daemon does not start again: the next `StopDaemon` reads the stale number and it sends
  `SIGTERM` to whatever process now holds that number. `validatePID` at
  `internal/daemon/daemon.go:495-502` limits the signal to a process whose
  `/proc/<pid>/cmdline` holds the text `tailscaled`. That check does not identify which
  tailnet the process serves, so the signal reaches the `tailscaled` of another tailnet, or
  the host's own `tailscaled`, and that tailnet goes down. The condition is narrow, because
  the operating system must reuse the number and it must give it to a `tailscaled`.
- **Condition**: The PID file removal fails, the process number is reused, and the reusing
  process is a `tailscaled`.
- **Epic 3**: Yes. Issue #69 checks the result of each removal and it returns the failure.
  A stronger correction writes the tailnet id beside the number and checks both.

#### SA-31 — `init` writes a permanent sysctl file, discards the error, and no step removes the file

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3` for the two discarded results and the
  message. The removal of the file in `uninstall` is a change of behaviour, and the
  maintainer decides it.
- **Where**: `cmd/hydrascale/init.go:283-285`, `cmd/hydrascale/uninstall.go:84-101`
- **Harm**: `runPreflight` offers to enable IP forwarding, with the default answer "yes".
  On "yes" it runs `sysctl -w net.ipv4.ip_forward=1` and it writes
  `/etc/sysctl.d/99-hydrascale.conf` with the text `net.ipv4.ip_forward=1`. Both results go
  to `_`, so a write that fails still prints "enabled" at `cmd/hydrascale/init.go:285`. The
  operator believes the setting persists across a restart when it does not. The reverse
  case is the durable one: the file persists, `runUninstall` removes
  `/var/lib/hydrascale`, `/var/log/hydrascale`, the systemd unit, and with `--purge` the
  binary and `/etc/hydrascale`, and it never names this file. The host therefore forwards
  every IPv4 packet between every interface after Hydrascale is uninstalled. Forwarding is
  a host-wide reachability setting, and the operator enabled it for Hydrascale alone.
- **Condition**: The operator answers "yes" at the forwarding prompt, and the operator
  later runs `hydrascale uninstall`.
- **Epic 3**: Partly. Epic 3 checks the two results and it corrects the message. The
  removal of the file in `uninstall` is a change of behaviour that this audit records and
  does not make.

#### SA-32 — `__nsdaemon` discards the error of both overlay directory removals

- **Severity**: medium
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 holds the work, and the
  failure path stops host name resolution, which issue #28 already corrected once.
- **Where**: `cmd/hydrascale/nsdaemon.go:42-51`
- **Harm**: `runNsDaemon` calls `os.RemoveAll(upper)` and `os.RemoveAll(work)` and it
  assigns both errors to `_`. The comment above the two lines states the reason for the
  removals: "overlay requires an empty workdir, and a stale upper resolv.conf would shadow
  the current one until tailscaled rewrites it." The code therefore names the failure it
  must prevent and it does not check that it prevented it. When the removal of `work`
  fails, the `syscall.Mount` at `cmd/hydrascale/nsdaemon.go:55` fails, the code writes a
  line to standard error and it continues; see `cmd/hydrascale/nsdaemon.go:56-58`. The
  child then runs with the host's shared `/etc`, and `tailscaled --accept-dns=true`
  replaces the host's `/etc/resolv.conf` with a file that names `100.100.100.100`. That
  address is reachable inside the namespace only, so host name resolution stops. This is
  the failure that issue #28 corrected. The daemon writes the line to standard error of the
  `ip netns exec` child, and `StartDaemon` sets `cmd.Stderr = nil` at
  `internal/daemon/daemon.go:151`, so the line reaches no log and no operator.
- **Condition**: `os.RemoveAll` fails on `etc-upper` or on `etc-work`, or the overlay mount
  fails for another reason.
- **Epic 3**: Yes. Issue #69 checks both removals. The suppressed message at
  `internal/daemon/daemon.go:151` is a second correction that Epic 3 makes at the same site.

#### SA-33 — The daemon inserts at position 1, and the convention states that it appends

- **Severity**: medium
- **Reproduced**: **yes, in the code and on the test host**
- **Area**: forwarding
- **Proposed disposition**: `propose: fix in Epic 5`. **FR-access-2** appends the jump
  rule. `CLAUDE.md` needs a note until Epic 5 lands, because it states the Epic 5
  behaviour as though it is the current behaviour.

`CLAUDE.md` states, under `### iptables`:

> The daemon owns the chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT`, and one jump rule into
> each of `FORWARD` and `INPUT`. It writes no other rule. It appends the jump rather than
> inserting it at position 1, so an operator firewall rule keeps its position.

The code does the opposite. `internal/namespaces/ns.go:273` and
`internal/namespaces/ns.go:278` both insert:

```
iptables -I FORWARD 1 -i vh<hash> -j ACCEPT
iptables -I FORWARD 1 -o vh<hash> -m state --state RELATED,ESTABLISHED -j ACCEPT
```

There is no `HYDRASCALE-FWD` chain and no `HYDRASCALE-OUT` chain in the code. A search of
`cmd/` and `internal/` for either name returns nothing. The chains that `CLAUDE.md`
describes are the Epic 5 design, and the file states them as though they are the current
behaviour.

**The position is not stable.** The rules are at positions 4 to 7 on the test host, at the
bottom of the chain, although the daemon inserted each one at position 1. `ts-forward`,
`DOCKER-USER`, and `DOCKER-FORWARD` start after the daemon, and each one takes position 1
in turn. The position of a daemon rule therefore depends on the order in which the
services start, and it changes when a service restarts.

**The harm condition.** Two harms follow, and they are opposite. When the daemon starts
last, its unrestricted `ACCEPT` sits above an operator firewall rule and it defeats that
rule for every packet from a namespace. When the daemon starts first, a later service can
place a `DROP` above the daemon rules and break every namespace. Neither outcome is
declared, and no test covers either one.

**Fix.** Epic 5 **FR-access-2** appends the jump rule. `CLAUDE.md` describes the state
after Epic 5 lands and it needs a note that says so until then.

#### SA-34 — The daemon writes no IPv6 firewall rule

- **Severity**: medium
- **Reproduced**: no
- **Area**: forwarding
- **Proposed disposition**: `propose: accept the risk`. Version 1.0 keeps the IPv6 gap by
  decision; see `docs/specs/features/05-reachability-model.md:231`. `SA-47` adds the log
  line that states the gap to the operator.
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

#### SA-35 — No rule covers IPv6, and each namespace holds an IPv6 tailnet address

- **Severity**: medium
- **Reproduced**: **yes, on the test host**
- **Area**: forwarding
- **Proposed disposition**: `propose: accept the risk`, together with `SA-34`. Epic 5 must
  read this finding before it writes the rule set, so that the deny the console shows
  matches the deny the host enforces.
- **Where**: `internal/namespaces/ns.go:272-285` and `internal/namespaces/ns.go:345-361`
  write every firewall rule with `iptables`; `internal/hostaccess/routes.go:393-399`
  propagates the IPv6 address of every peer to the host route table

`SA-34` records that the daemon writes no IPv6 firewall rule, and `SA-47` records that the
daemon does not log the gap at start. This finding adds the host evidence, and it does not
repeat the analysis.

Each namespace holds an IPv6 tailnet address, quoted in **The addresses** above:
`fd7a:115c:a1e0::fd35:ab2c` in `ns-havoc` and `fd7a:115c:a1e0::b736:9e40` in `ns-jbones`.
Each veth pair holds an IPv6 link-local address. Every rule that this document quotes is
an `iptables` rule, therefore every rule applies to IPv4 alone.

**The harm condition.** The `HYDRASCALE-FWD` design of Epic 5 denies by default. A design
that writes `iptables` alone leaves IPv6 at the `ip6tables` policy of the host, therefore
an operator who reads the console sees a deny that IPv6 does not carry out.

**Fix.** Epic 5 for the rule set, together with `SA-34` and `SA-47`. The audit records the
group together so that the Epic 5 work does not repeat the IPv4 shape in `ip6tables` terms
alone.

### The low findings

#### SA-36 — `POST /api/tailnet/add` writes the auth key into the configuration file

- **Severity**: low
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #71 must state which store the
  route writes to, because `FR-fix-14` through `FR-fix-17` describe the secrets file and no
  requirement changes this write path.

`internal/api/server.go:267` copies `req.AuthKey` into `config.Tailnet.AuthKey`, and
`internal/api/server.go:274` saves the configuration file. `CLAUDE.md` states that an
auth key never enters the configuration file, and that it lives in
`/etc/hydrascale/secrets.yaml` at mode `0600` or in an environment variable.
`internal/config/config.go:233-239` already supports the environment variable form.

`SaveConfig` writes through `os.CreateTemp`, which creates the file at mode `0600`
(`internal/config/config.go:288`). The rename keeps that mode. The parent directory gets
mode `0755` (`internal/config/config.go:283`). The key is therefore readable by root
only, which is why this finding is low rather than high.

**Condition for harm.** The operator copies `/etc/hydrascale/config.yaml` into a backup,
a support archive, or a version control repository. The auth key travels with it.

**Epic 3.** Partly. `FR-fix-14` through `FR-fix-17` describe the secrets file and its
mode. No requirement removes the auth key from the write path of this route. Issue #71
must state which store the route writes to.

#### SA-37 — `POST /api/tailnet/connect` reports success for an unknown tailnet

- **Severity**: low
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: fix in Epic 3`. `FR-fix-9` requires the route to
  validate its body, and a membership test is part of that.

`internal/api/server.go:339-345` tests that `req.ID` is not empty, calls
`s.reconciler.ResetError(req.ID)`, and then runs a full reconcile. `ResetError` deletes
three map entries (`internal/reconciler/reconciler.go:480-486`). A delete of an absent
key does nothing and reports nothing. The route then returns `{"ok":true}` when the
reconcile succeeds.

The identifier reaches map operations only, so it reaches no path and no command.

**Condition for harm.** A caller sends a wrong identifier. The route reports success. The
tailnet that the caller meant to connect stays in its error state.

**Epic 3.** Yes. `FR-fix-9` requires the route to validate its body, and a membership
test is part of that.

#### SA-38 — An error response returns the internal error text and the configuration path

- **Severity**: low
- **Reproduced**: no
- **Area**: control API
- **Proposed disposition**: `propose: accept the risk`. The socket is root-only by
  default, so the caller already holds more than the message gives it.

`internal/api/server.go:138` and `:144` return the reconciler error text.
`internal/api/server.go:419` returns the configuration load error, which
`internal/config/config.go:110` wraps around the operating system error. That error names
`/etc/hydrascale/config.yaml`. `internal/api/server.go:240` returns the JSON parse error
text.

**Condition for harm.** An account that reaches the control socket learns the
configuration path and the parse state of the file. The socket is root-only by default,
so the account already holds more than the message gives it. The value of the finding is
the record, not the risk.

**Epic 3.** No. No requirement covers the error text. The audit records it and Epic 3
does not have to act.

#### SA-39 — Nine commands discard the error that they return

- **Severity**: low
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: fix in Epic 3` for the `internal/` sites. The `cmd/`
  sites report to the operator on the terminal, and the audit proposes to defer those.
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

#### SA-40 — `__nsdaemon` executes any argument list as root

- **Severity**: low
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: accept the risk`. A root caller runs any program
  without this command, so the finding is a constraint to record rather than a defect to
  correct.
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

#### SA-41 — Two prompt answers reach `groupadd` and `usermod` without validation

- **Severity**: low
- **Reproduced**: no
- **Area**: exec call sites
- **Proposed disposition**: `propose: accept the risk`. The operator runs `hydrascale
  init` as root and types the value, so the harm is a confusing failure.
- **Where**: `cmd/hydrascale/init.go:252-260`
- **Harm**: The group name and the user name come from the prompt. Neither value is
  checked. A value that starts with a hyphen becomes an option of `groupadd` or of
  `usermod`. The operator runs `hydrascale init` as root and the operator types the value,
  so the operator can already run any command. The harm is a confusing failure, not a rise
  in privilege.
- **Condition**: The operator types a value that starts with a hyphen.
- **Epic 3**: No. Epic 3 records the finding and it makes no change.

#### SA-42 — `Server.Shutdown` discards the socket removal error

- **Severity**: low
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. Issue #69 returns the error together
  with the shutdown error, and the change is one line.
- **Where**: `internal/api/server.go:116-125`
- **Harm**: `Shutdown` calls `os.Remove(s.socketPath)` and it checks no result, while the
  same function checks and wraps the error of `s.httpServer.Shutdown` two lines above. A
  socket file that survives has mode `0600`, or mode `0660` with the group of `SA-7`, and
  no process listens on it. `Start` handles that state correctly: it dials the socket at
  `internal/api/server.go:72`, it removes the file when the connection is refused at
  `internal/api/server.go:75`, and it fails with "socket %s is already in use" when the
  connection succeeds. The next start therefore recovers, and the harm is a file that
  outlives the daemon.
- **Condition**: `os.Remove` fails on the socket path.
- **Epic 3**: Yes. Issue #69 returns the error together with the shutdown error.

#### SA-43 — `syscall.Umask` changes the whole process, and the API server starts in a goroutine

- **Severity**: low
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: defer`. No requirement covers the mask, and every
  resulting mode is more restrictive than the requested mode.
- **Where**: `internal/api/server.go:85-90`, `cmd/hydrascale/main.go:488-492`
- **Harm**: `syscall.Umask` sets a value for the process, not for the goroutine. `Start`
  sets the mask to `0077` at `internal/api/server.go:87`, it calls `net.Listen`, and it
  restores the saved value at `internal/api/server.go:89`. `cmd/hydrascale/main.go:488`
  calls `apiServer.Start()` inside a goroutine, and the reconciler loop runs at the same
  time. A file that another goroutine creates inside that window gets the mask `0077`
  instead of the systemd default `0022`. The event log at
  `internal/reconciler/reconciler.go:566` becomes `0600` instead of `0644`, and
  `/etc/netns/<ns>/resolv.conf` at `internal/namespaces/ns.go:400` becomes `0600` instead of
  `0644`. Both results are more restrictive than the requested mode, and every reader of
  both files is root, so the harm is a mode that does not match the source. The window is
  the duration of one `net.Listen` on a unix socket. The restore is not a `defer`, so a
  panic between the two lines leaves the mask at `0077` for the life of the process; no
  panic path exists there today.
- **Condition**: A goroutine creates a file during the `net.Listen` call.
- **Epic 3**: No. Epic 3 records the finding. A correction sets the socket mode with
  `os.Chmod` alone, which `internal/api/server.go:93` already does, and it removes the mask
  change.

#### SA-44 — The event log is mode `0644` in a directory that the daemon may create at `0755`

- **Severity**: low
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: fix in Epic 3`. The two install paths already create
  the directory at `0750`, so the correction makes one writer agree with the other two.
- **Where**: `internal/reconciler/reconciler.go:558-570`, `cmd/hydrascale/main.go:782`,
  `cmd/hydrascale/init.go:96`
- **Harm**: `SetEventLog` creates the directory at `0755` and it opens the file at `0644`.
  Every local user reads the file. The records hold the tailnet id, the event type, and the
  message text; see `internal/reconciler/reconciler.go:667-673`. The message text is the
  error string of a failed action, and an error string from `tailscale up` or from
  `iptables` names the control server, the namespace, and the addresses. The file holds no
  auth key, because the key reaches `tailscale up` through a file and not through argv; see
  `internal/daemon/daemon.go:317-333` and `SA-23`. `hydrascale install` and
  `hydrascale init` both create `/var/log/hydrascale` at `0750`, so the directory is
  usually closed to other users and the `0755` applies only when `SetEventLog` creates the
  directory first. The file mode `0644` applies in every case.
- **Condition**: The operator sets `event_log`, and a local user who is not root reads the
  path.
- **Epic 3**: Yes. Epic 3 opens the event log at `0640` and it creates the directory at
  `0750`, to agree with the two install paths.

#### SA-45 — `/etc/netns/<ns>/resolv.conf` is mode `0644` in a directory at mode `0755`

- **Severity**: low
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: accept the risk`. The file discloses no value that
  the reader cannot already read from a world-readable host file.
- **Where**: `internal/namespaces/ns.go:389`, `internal/namespaces/ns.go:400`
- **Harm**: Every local user reads the file and lists the directory. The content is the
  list of upstream name servers that `resolveHostUpstreams` reads from
  `/run/systemd/resolve/resolv.conf` or from `/etc/resolv.conf`; see
  `internal/namespaces/ns.go:412-444`. Both source files are world-readable on a default
  Linux host, so the file discloses no value that the reader cannot already read. The
  directory name discloses the namespace name, and the namespace name holds the tailnet id;
  see `internal/namespaces/ns.go:66`. `ip netns list` gives the same list to any user, so
  that disclosure is also not new. The finding is recorded for completeness, because the
  mode is wider than the mode of every other file that the daemon writes for its own use.
- **Condition**: A local user lists `/etc/netns`.
- **Epic 3**: No. Epic 3 records the mode and it makes no change.

#### SA-46 — `uninstall` reports the removals that succeed and it reports no removal that fails

- **Severity**: low
- **Reproduced**: no
- **Area**: modes and teardown
- **Proposed disposition**: `propose: defer`. The site is in `cmd/` and it reports to the
  operator on the terminal, so Epic 3 records it and makes no change.
- **Where**: `cmd/hydrascale/uninstall.go:84-107`
- **Harm**: Each removal has the form `if err := os.RemoveAll(d); err == nil { removed =
  append(removed, d) }`. A failure adds nothing to the list and it produces no message. The
  command then prints the "Removed:" list and the line "Hydrascale uninstalled." The
  operator reads a success message for an uninstall that left `/var/lib/hydrascale` on the
  host, with the node private key of every tailnet inside it; see `SA-29` for the content
  of that tree. The absence of a path from the list is the only signal, and the operator
  must compare the printed list against the four or six paths the command names in its
  earlier output. The same file discards the results of `systemctl stop`,
  `systemctl disable`, `systemctl daemon-reload`, and `tailscale logout`; `SA-39` already
  records those four sites.
- **Condition**: A removal fails and the operator does not compare the printed list against
  the announced list.
- **Epic 3**: Partly. Epic 3 covers the `internal/` sites, as `SA-39` states. This site is
  in `cmd/` and it reports to the operator on the terminal, so Epic 3 records it and makes
  no change.

#### SA-47 — The daemon does not log the IPv6 gap at start

- **Severity**: low
- **Reproduced**: no
- **Area**: forwarding
- **Proposed disposition**: `propose: fix in Epic 3`. Two documents already require the
  line, and the line is the operator's only signal that `SA-34` and `SA-35` exist.
- **Where**: `cmd/hydrascale/main.go` holds the start path and it emits no such line
- **Harm**: `.claude/rules/access-invariants.md` and
  `docs/specs/features/05-reachability-model.md:231` both require a log line at start that
  states that the daemon does not filter IPv6 forwarding. The code emits no such line. The
  operator therefore sees a set of local rules and believes that the rules cover all
  traffic. The command `rg -i 'IPv6' cmd/` returns no line in the start path.
- **Condition**: The operator reads the rule set and assumes that it covers IPv6.
- **Epic 3**: Yes. Epic 3 adds the log line together with the chain.

#### SA-48 — Delivery into the second tailnet is not demonstrated

- **Severity**: low, because the audit did not reproduce it. The audit records the
  mechanism and the uncertainty, not a result.
- **Reproduced**: no
- **Area**: forwarding
- **Proposed disposition**: `propose: fix in Epic 5`. Epic 5 removes the dependence on a
  kernel default, and it adds a test that asserts the value inside the namespace.

`SA-8` proves that the host forwards a packet from one namespace into the other. It does
not prove that the second namespace passes the packet on to its own tailnet. The audit
tried and the result is inconclusive.

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 100.91.107.38
PING 100.91.107.38 (100.91.107.38) 56(84) bytes of data.

--- 100.91.107.38 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1

$ sudo ip netns exec ns-havoc ping -c1 -W2 100.114.149.115
PING 100.114.149.115 (100.114.149.115) 56(84) bytes of data.

--- 100.114.149.115 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1
```

Both addresses are peers of the `ns-jbones` tailnet, and the host route table sends both
through `ns-jbones`. Neither peer answered `ns-havoc`. **Neither peer answered `ns-jbones`
either**, therefore the negative result says nothing about containment:

```
$ sudo ip netns exec ns-jbones ping -c1 -W2 100.91.107.38
PING 100.91.107.38 (100.91.107.38) 56(84) bytes of data.

--- 100.91.107.38 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1

$ sudo ip netns exec ns-jbones ping -c1 -W2 100.114.149.115
PING 100.114.149.115 (100.114.149.115) 56(84) bytes of data.

--- 100.114.149.115 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1
```

A peer that does not answer its own namespace cannot show whether a different namespace
reaches it. **The audit therefore does not claim a cross-tailnet reproduction.**

The audit did establish where the packet stops. The host sends the packet into
`ns-jbones`, and `ns-jbones` does not forward:

```
$ sudo ip netns exec ns-havoc ip route get 100.91.107.38
100.91.107.38 via 10.200.0.85 dev vn5cde1b791fe1 src 10.200.0.86 uid 0
    cache

$ sudo ip netns exec ns-jbones sysctl net.ipv4.ip_forward
net.ipv4.ip_forward = 0

$ sudo ip netns exec ns-jbones iptables -S FORWARD
-P FORWARD ACCEPT
-A FORWARD -j ts-forward
```

`net.ipv4.ip_forward = 0` inside `ns-jbones` stops a forwarded packet in that namespace.
The daemon sets forwarding on the host side of the pair at
`internal/namespaces/ns.go:266`; it sets no equivalent value inside the namespace.

**The harm condition.** The containment that stops the packet is a kernel default inside
the namespace, not a rule that the daemon writes and not a rule that a test asserts. A
later change that enables forwarding inside a namespace, for a subnet router or for an
exit node, removes the containment silently and turns `SA-8` into a cross-tailnet leak.

**Fix.** Epic 5 removes the dependence on the default, because the deny happens in
`HYDRASCALE-FWD` on the host before the packet reaches the second namespace. A test that
asserts the value inside the namespace is worth adding with that work.

## Other defects

This section holds a defect that is not a security defect. Epic 3 does not have to fix it.
`docs/specs/features/02-security-audit.md:124` describes the section.

### SA-49 — `POST /api/tailnet/add` accepts `exit_node` and no code reads it

- **Severity**: low
- **Reproduced**: no
- **Area**: other defect, a functional defect
- **Proposed disposition**: `propose: defer`. The defect misinforms the operator about the
  path of the traffic, and no security requirement covers it.

The route stores `req.ExitNode` at `internal/api/server.go:269` and the loader keeps it
at `internal/config/config.go:30`. `buildTailscaleUpArgs` builds the argument list for
`tailscale up` and it does not add an exit node flag
(`internal/daemon/daemon.go:271-288`). The only readers display the value:
`cmd/hydrascale/main.go:376-377` and `internal/tui/model.go:935-936`.

**Condition for harm.** The operator sets an exit node through the route or through the
terminal interface (`internal/tui/model.go:205`). The console and the command line
interface both display it. No traffic uses it. The operator believes the host sends
traffic through an exit node when it does not.

**Epic 3.** No. This is a functional defect rather than a security defect. It belongs in
this `Other defects` section that
`docs/specs/features/02-security-audit.md:124` describes.

## What the audit does not cover

- The chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT` that `CLAUDE.md` describes. The
  directory `internal/access` does not exist on this branch, so the daemon writes neither
  chain today. The audit records no finding for a resource that no code creates. `SA-33`
  records the divergence between `CLAUDE.md` and the code.
- The console on the loopback address. The directory `internal/ui` does not exist on this
  branch. `SA-5` records the threat model of the listener that Epic 6 adds.
- The file `/etc/hydrascale/secrets.yaml` that `CLAUDE.md` describes at mode `0600`. No
  file in `cmd/` or in `internal/` reads or writes that path. `SA-23` records the secret
  that the configuration file holds today.
- A cross-tailnet delivery. `SA-48` records the attempt, the commands, the output, and the
  reason the result is inconclusive.



