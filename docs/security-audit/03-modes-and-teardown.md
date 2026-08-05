# Security audit — the file modes, the socket modes, and the teardown paths

This file is a fragment of the security audit. It answers issue #65. It covers
**FR-audit-9** and **FR-audit-11**. Issue #67 collects every fragment into
`docs/security-audit.md`.

The identifiers run from `SA-40` to `SA-59`. Issue #67 renumbers the complete set.

This audit changes no code. Epic 3 makes the corrections.

## Method

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

## Table 1 — every mode and every owner that the daemon sets

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
therefore sets the mode, and a later writer leaves that mode in place. `SA-42` records the
result.

## Table 2 — every resource that setup creates, against its teardown step

| Resource | Setup | Teardown | Can the teardown report a failure? |
|---|---|---|---|
| The namespace `ns-<id>` | `internal/namespaces/ns.go:81` | `internal/namespaces/ns.go:113` | Yes |
| The veth pair `vh<hash>`/`vn<hash>` | `internal/namespaces/ns.go:231` | `internal/namespaces/ns.go:315` | Yes |
| The host address on the veth | `internal/namespaces/ns.go:241` | The veth removal removes it | Yes |
| The namespace address and default route | `internal/namespaces/ns.go:251`, `internal/namespaces/ns.go:261` | The namespace removal removes both | Yes |
| The forwarding sysctl on the veth | `internal/namespaces/ns.go:266` | The veth removal removes the key | Not applicable |
| The host `FORWARD` accept rule | `internal/namespaces/ns.go:273` | `internal/namespaces/ns.go:309` | No — see `SA-29` |
| The host `FORWARD` return rule | `internal/namespaces/ns.go:278` | `internal/namespaces/ns.go:310` | No — see `SA-29` |
| The host `POSTROUTING` masquerade rule | `internal/namespaces/ns.go:285` | `internal/namespaces/ns.go:311` | No — see `SA-29` |
| The namespace `tailscale0` masquerade rule | `internal/namespaces/ns.go:346` | `internal/namespaces/ns.go:459` | No — `SA-45`, `SA-46` |
| The namespace DNS DNAT rule, UDP | `internal/namespaces/ns.go:353` | `internal/namespaces/ns.go:460` | No — `SA-45`, `SA-46` |
| The namespace DNS DNAT rule, TCP | `internal/namespaces/ns.go:360` | `internal/namespaces/ns.go:461` | No — `SA-45`, `SA-46` |
| `/etc/netns/<ns>/resolv.conf` | `internal/namespaces/ns.go:400` | `internal/namespaces/ns.go:120` and `internal/namespaces/ns.go:464` | No — `SA-47` |
| `/etc/netns/<ns>` | `internal/namespaces/ns.go:389` | `internal/namespaces/ns.go:121` and `internal/namespaces/ns.go:465` | No — `SA-47` |
| The `tailscaled` process | `internal/daemon/daemon.go:159` | `internal/daemon/daemon.go:181` | Yes |
| `<state>/tailscaled.pid` | `internal/daemon/daemon.go:166` | `internal/daemon/daemon.go:202`, `208`, `214`, `228`, `235` | No — `SA-49` |
| `<state>/tailscaled.sock` | `tailscaled` creates it | `internal/daemon/daemon.go:120` on the next start, and the state directory removal | No |
| `<state>/tailscaled.state` | `tailscaled` creates it | The state directory removal at `internal/reconciler/reconciler.go:327` | No — `SA-48` |
| `<state>/authkey-*` | `internal/daemon/daemon.go:322` | `internal/daemon/daemon.go:325`, a `defer os.Remove` | No, and the state directory removal covers it |
| `<state>/etc-upper` and `<state>/etc-work` | `cmd/hydrascale/nsdaemon.go:46` and `:49` | `cmd/hydrascale/nsdaemon.go:44` and `:45` on the next start, and the state directory removal | No — `SA-51` |
| The overlay mount on `/etc` | `cmd/hydrascale/nsdaemon.go:55` | The mount namespace ends with the process | Not applicable |
| `/var/lib/hydrascale/state/<id>` | `internal/daemon/daemon.go:114` | `internal/reconciler/reconciler.go:327` | No — `SA-48` |
| The host routes to the peers | `internal/hostaccess/routes.go:333` | `internal/hostaccess/routes.go:432`, through `TeardownAll` at `internal/reconciler/reconciler.go:633` | No — `SA-44` |
| The `/etc/hosts` block | `internal/hostaccess/hosts.go` | `syncDNS`, through `TeardownAll` at `internal/reconciler/reconciler.go:633` | No — `SA-44` |
| The systemd-resolved registration | `internal/hostaccess/resolved.go` | `DeregisterAll` at `internal/hostaccess/hostaccess.go:110` | No — see `SA-29` |
| `/var/lib/hydrascale/api.sock` | `internal/api/server.go:88` | `internal/api/server.go:125` | No — `SA-52` |
| The group ownership on `/var/lib/hydrascale` | `internal/api/server.go:206` and `:209` | None | Not applicable — `SA-41` |
| The event log file | `internal/reconciler/reconciler.go:566` | None; `uninstall` removes `/var/log/hydrascale` | No — `SA-56` |
| `<state>/.hydrascale.lock` | `internal/reconciler/reconciler.go:695` | None; the state directory holds it | Not applicable |
| `/etc/sysctl.d/99-hydrascale.conf` | `cmd/hydrascale/init.go:284` | None | No — `SA-50` |
| `<config path>.bak` | `cmd/hydrascale/init.go:357` | None | No — `SA-43` |
| `/etc/systemd/system/hydrascale.service` | `cmd/hydrascale/main.go:805` | `cmd/hydrascale/uninstall.go:91` | No — `SA-56` |
| `/etc/systemd/system/<svc>.service.d/hydrascale.conf` | `cmd/hydrascale/main.go:711` | None | No — `SA-56` |
| The unix group that `init` creates | `cmd/hydrascale/init.go:254` | None | Not applicable |
| The group membership that `init` grants | `cmd/hydrascale/init.go:259` | None | Not applicable — `SA-40` |

`SA-29` in `docs/security-audit/02-exec-call-sites.md` already records that
`internal/namespaces/ns.go:309-311` discards the error of the three host `iptables`
removals. This fragment cites that finding and does not restate it. The findings below
record what the teardown reading shows beyond it.

## The findings

### SA-40 — Membership of `socket_group` is equivalent to root, and no document says so

- **Severity**: high
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

### SA-41 — `applySocketGroup` changes the owner and the mode of the socket's parent directory

- **Severity**: medium
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

### SA-42 — Three writers set the mode of `/etc/hydrascale` and of `config.yaml`, and one mode is implicit

- **Severity**: medium
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

### SA-43 — `backupFile` writes a copy of the configuration file and nothing removes it

- **Severity**: medium
- **Where**: `cmd/hydrascale/init.go:352-358`, `cmd/hydrascale/uninstall.go:84-101`
- **Harm**: `backupFile` reads the configuration file and it writes the bytes to
  `<path>.bak` at mode `0640`. The source file can hold an `auth_key`; see `SA-42`. No step
  in the code removes the backup. `runUninstall` removes `/var/lib/hydrascale`,
  `/var/log/hydrascale`, and the systemd unit. It removes `/etc/hydrascale` only when the
  operator passes `--purge`; see `cmd/hydrascale/uninstall.go:94-100`. An uninstall without
  `--purge` therefore leaves `/etc/hydrascale/config.yaml.bak` on the host, with the auth
  key of every tailnet the operator configured, after every other part of Hydrascale is
  gone.
- **Condition**: The operator uses an `auth_key` in the configuration file, a command calls
  `backupFile`, and the operator later runs `hydrascale uninstall` without `--purge`.
- **Epic 3**: No. Epic 3 records the finding and it makes no change. The removal of the
  backup file belongs with the decision on `auth_key` in `SA-42`.

### SA-44 — `hostaccess.Manager.Teardown` has no caller, so a removed tailnet keeps its `/etc/hosts` entries

- **Severity**: medium
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

### SA-45 — `namespaces.TeardownHostAccess` has no caller, so `host_access: false` leaves its rules in place

- **Severity**: medium
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

### SA-46 — `TeardownHostAccess` cannot report a failure, because its signature returns nothing

- **Severity**: medium
- **Where**: `internal/namespaces/ns.go:446`, `internal/namespaces/ns.go:451`
- **Harm**: Both declarations read `func TeardownHostAccess(nsName string, index int,
  infraSubnet string)` with no result. The function performs six removals: three
  `iptables` commands at `internal/namespaces/ns.go:459-461` and two `os.Remove` calls at
  `internal/namespaces/ns.go:464-465`, and it returns early at
  `internal/namespaces/ns.go:456` when `VethIPs` fails. Not one of the six can reach a
  caller, because there is no result to carry it. This is a step beyond `SA-29`: `SA-29`
  records call sites that assign an error to `_`, and a future correction there needs only
  a changed assignment. Here the signature itself must change before any correction is
  possible. `CLAUDE.md` states: "A best-effort operation that fails silently is a defect."
  The matching setup function, `SetupHostAccess` at `internal/namespaces/ns.go:337`,
  returns an error, so the package is not consistent with itself.
- **Condition**: A caller exists that would act on the failure. `SA-45` records that no
  caller exists yet, so this finding is a defect that the correction for `SA-45` must fix
  first.
- **Epic 3**: Yes. Issue #69 changes the signature to return an error, and it collects the
  six errors and returns them together, as `CLAUDE.md` requires for a cleanup step.

### SA-47 — Four removals under `/etc/netns` discard their errors

- **Severity**: medium
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
  This is beyond `SA-29`, which covers `internal/namespaces/ns.go:309-311` alone.
- **Condition**: The directory `/etc/netns/<ns>` holds a file that Hydrascale did not
  write, or the removal fails for a permission reason.
- **Epic 3**: Yes. Issue #69 collects these errors and returns them with the other teardown
  errors.

### SA-48 — The state directory removal only writes a log line, and the directory holds the node private key

- **Severity**: medium
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

### SA-49 — `StopDaemon` discards the error of five `os.Remove` calls on the PID file

- **Severity**: medium
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

### SA-50 — `init` writes a permanent sysctl file, discards the error, and no step removes the file

- **Severity**: medium
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

### SA-51 — `__nsdaemon` discards the error of both overlay directory removals

- **Severity**: medium
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

### SA-52 — `Server.Shutdown` discards the socket removal error

- **Severity**: low
- **Where**: `internal/api/server.go:116-125`
- **Harm**: `Shutdown` calls `os.Remove(s.socketPath)` and it checks no result, while the
  same function checks and wraps the error of `s.httpServer.Shutdown` two lines above. A
  socket file that survives has mode `0600`, or mode `0660` with the group of `SA-40`, and
  no process listens on it. `Start` handles that state correctly: it dials the socket at
  `internal/api/server.go:72`, it removes the file when the connection is refused at
  `internal/api/server.go:75`, and it fails with "socket %s is already in use" when the
  connection succeeds. The next start therefore recovers, and the harm is a file that
  outlives the daemon.
- **Condition**: `os.Remove` fails on the socket path.
- **Epic 3**: Yes. Issue #69 returns the error together with the shutdown error.

### SA-53 — `syscall.Umask` changes the whole process, and the API server starts in a goroutine

- **Severity**: low
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

### SA-54 — The event log is mode `0644` in a directory that the daemon may create at `0755`

- **Severity**: low
- **Where**: `internal/reconciler/reconciler.go:558-570`, `cmd/hydrascale/main.go:782`,
  `cmd/hydrascale/init.go:96`
- **Harm**: `SetEventLog` creates the directory at `0755` and it opens the file at `0644`.
  Every local user reads the file. The records hold the tailnet id, the event type, and the
  message text; see `internal/reconciler/reconciler.go:667-673`. The message text is the
  error string of a failed action, and an error string from `tailscale up` or from
  `iptables` names the control server, the namespace, and the addresses. The file holds no
  auth key, because the key reaches `tailscale up` through a file and not through argv; see
  `internal/daemon/daemon.go:317-333` and `SA-42`. `hydrascale install` and
  `hydrascale init` both create `/var/log/hydrascale` at `0750`, so the directory is
  usually closed to other users and the `0755` applies only when `SetEventLog` creates the
  directory first. The file mode `0644` applies in every case.
- **Condition**: The operator sets `event_log`, and a local user who is not root reads the
  path.
- **Epic 3**: Yes. Epic 3 opens the event log at `0640` and it creates the directory at
  `0750`, to agree with the two install paths.

### SA-55 — `/etc/netns/<ns>/resolv.conf` is mode `0644` in a directory at mode `0755`

- **Severity**: low
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

### SA-56 — `uninstall` reports the removals that succeed and it reports no removal that fails

- **Severity**: low
- **Where**: `cmd/hydrascale/uninstall.go:84-107`
- **Harm**: Each removal has the form `if err := os.RemoveAll(d); err == nil { removed =
  append(removed, d) }`. A failure adds nothing to the list and it produces no message. The
  command then prints the "Removed:" list and the line "Hydrascale uninstalled." The
  operator reads a success message for an uninstall that left `/var/lib/hydrascale` on the
  host, with the node private key of every tailnet inside it; see `SA-48` for the content
  of that tree. The absence of a path from the list is the only signal, and the operator
  must compare the printed list against the four or six paths the command names in its
  earlier output. The same file discards the results of `systemctl stop`,
  `systemctl disable`, `systemctl daemon-reload`, and `tailscale logout`; `SA-29` already
  records those four sites.
- **Condition**: A removal fails and the operator does not compare the printed list against
  the announced list.
- **Epic 3**: Partly. Epic 3 covers the `internal/` sites, as `SA-29` states. This site is
  in `cmd/` and it reports to the operator on the terminal, so Epic 3 records it and makes
  no change.

## What this fragment does not cover

- The chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT` that `CLAUDE.md` describes. The
  directory `internal/access` does not exist on this branch, so the daemon writes neither
  chain today. The audit records no finding for a resource that no code creates.
- The console on the loopback address. The directory `internal/ui` does not exist on this
  branch.
- The file `/etc/hydrascale/secrets.yaml` that `CLAUDE.md` describes at mode `0600`. No
  file in `cmd/` or in `internal/` reads or writes that path. `SA-42` records the secret
  that the configuration file holds today.
- The environment that `tailscaled` inherits. `SA-27` in
  `docs/security-audit/02-exec-call-sites.md` records it.
- The three host `iptables` removals at `internal/namespaces/ns.go:309-311`. `SA-29` in the
  same file records them, and Table 2 cites that finding.
