# Security audit — the control API routes

This file is one fragment of the security audit. Issue #67 collects every fragment into
`docs/security-audit.md` and it renumbers every finding. The identifiers here run from
`SA-1` to `SA-19`.

The audit reads code. It ran no command on the test host. Every finding therefore states
`Reproduced: no`, as `FR-audit-6` requires.

Severity describes the harm, never the size of the fix.

## Scope

`internal/api/server.go` registers ten routes at `internal/api/server.go:50-61`. The
feature file `docs/specs/features/02-security-audit.md:104` cites the range `47-59`. The
current code holds the same ten routes at `50-61`.

The daemon serves these routes on the control socket at
`/var/lib/hydrascale/api.sock` (`internal/api/types.go:12`). The daemon binds no loopback
address today. `internal/api/server.go:88` opens a Unix socket and nothing else. Epic 6
adds the console listener. Finding `SA-13` records the threat model for that listener.

## What each route validates

| Method and path | Handler | Mutating | What the handler validates | What the handler does not validate |
|---|---|---|---|---|
| `GET /api/status` | `handleStatus` (`internal/api/server.go:130`) | no | The method equals `GET`. | The response body. The response carries the auth key of every tailnet. See `SA-1`. |
| `GET /api/events` | `handleEvents` (`internal/api/server.go:162`) | no | The method equals `GET`. | Nothing else. The route takes no input. |
| `POST /api/reconcile` | `handleReconcile` (`internal/api/server.go:173`) | yes | The method equals `POST`. | Nothing else. The route takes no body. |
| `POST /api/tailnet/add` | `handleTailnetAdd` (`internal/api/server.go:231`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. `control_url` is an absolute `http` or `https` URL. | The character set of `id`. The value of `auth_key`. The value of `exit_node`. The `https` scheme rule that the loader enforces. See `SA-3`, `SA-4`, `SA-9`, `SA-10`. |
| `POST /api/tailnet/remove` | `handleTailnetRemove` (`internal/api/server.go:282`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. `id` names a tailnet in the configuration file. | The character set of `id`. The membership test rejects an unknown value, so no unvalidated value reaches a path. |
| `POST /api/tailnet/connect` | `handleTailnetConnect` (`internal/api/server.go:327`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. | The character set of `id`. Whether `id` names a tailnet. See `SA-11`. |
| `POST /api/tailnet/disconnect` | `handleTailnetDisconnect` (`internal/api/server.go:348`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `id` is not empty. | The character set of `id`. Whether `id` names a tailnet. The value reaches a file path. See `SA-2`. |
| `POST /api/config/dns` | `handleConfigDNS` (`internal/api/server.go:368`) | yes | The method equals `POST`. The body is 1 MiB or smaller. The body parses as JSON. `bind_address`, when it is not empty, is a loopback `host:port` value. | The value of `mode`. Any string reaches the configuration file. See `SA-6`. |
| `GET /api/config` | `handleConfig` (`internal/api/server.go:410`) | no | The method equals `GET`. | Nothing else. The handler replaces a non-empty auth key with `***` at `internal/api/server.go:434-436`. |
| `GET /api/tailnet/{id}/detail` | `handleTailnetDetail` (`internal/api/server.go:451`) | no | The route pattern restricts the method to `GET`. `id` is not empty. `id` names a tailnet in the configuration file (`internal/reconciler/reconciler.go:448-450`). | The character set of `id`. The membership test runs first, so no unvalidated value reaches a path. |

Four routes are read-only. Six routes mutate state. `POST /api/reconcile` accepts no
body. The other five mutating routes accept a JSON body. No route reads a header for an
identity, a token, or an origin.

## Paths that the daemon builds from request input

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

## Findings

### SA-1 — `GET /api/status` returns the auth key of every tailnet

**Severity: high.** **Reproduced: no.**

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

### SA-2 — `POST /api/tailnet/disconnect` builds a file path from an unvalidated identifier

**Severity: high.** **Reproduced: no.**

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
that the root daemon can read. `SA-5` describes how weak the process test is. Together
the two findings let that account stop a chosen root process and remove a chosen file
named `tailscaled.pid`.

**Epic 3.** Yes. `FR-fix-11` and `FR-fix-13` cover it. Issue #71 builds it.

### SA-3 — `POST /api/tailnet/add` writes an unvalidated identifier into the configuration file

**Severity: high.** **Reproduced: no.**

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

**Epic 3.** Yes. `FR-fix-9`, `FR-fix-10`, and `FR-fix-11` cover it. Issue #71 builds it.

### SA-4 — The route and the loader disagree about the control URL scheme

**Severity: high.** **Reproduced: no.**

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

### SA-5 — `validatePID` accepts any process whose command line contains `tailscaled`

**Severity: medium.** **Reproduced: no.**

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

### SA-6 — `POST /api/config/dns` writes `mode` without validation

**Severity: medium.** **Reproduced: no.**

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

### SA-7 — A failed mutating route returns HTTP 200

**Severity: medium.** **Reproduced: no.**

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

### SA-8 — No route checks `Content-Type` or `Origin`

**Severity: medium.** **Reproduced: no.**

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

### SA-9 — `POST /api/tailnet/add` writes the auth key into the configuration file

**Severity: low.** **Reproduced: no.**

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

### SA-10 — `POST /api/tailnet/add` accepts `exit_node` and no code reads it

**Severity: low.** **Reproduced: no.**

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
the `Other defects` section that
`docs/specs/features/02-security-audit.md:124` describes.

### SA-11 — `POST /api/tailnet/connect` reports success for an unknown tailnet

**Severity: low.** **Reproduced: no.**

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

### SA-12 — An error response returns the internal error text and the configuration path

**Severity: low.** **Reproduced: no.**

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

### SA-13 — The console threat model — accepted

**Severity: high. Status: accepted by the maintainer. Reproduced: no.**

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

## Other defects

`SA-10` is a functional defect rather than a security defect. It is listed above with its
severity so that issue #67 can move it into the `Other defects` section without a second
read of the code.

## What this fragment does not cover

- `FR-audit-8` — every `os/exec` call site. Another fragment covers it. This fragment
  names the call sites that request input reaches: `internal/daemon/daemon.go:94`,
  `:252`, `:338`.
- `FR-audit-9` — every file mode and socket mode. Another fragment covers it. This
  fragment names the modes that a route sets: `internal/config/config.go:283` and
  `:288`.
- `FR-audit-11` — the teardown path.
- `FR-audit-13` — the IPv6 gap in the firewall rules.
