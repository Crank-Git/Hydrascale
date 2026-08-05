---
name: Hydrascale v1.0
slug: hydrascale-v1
repo: Crank-Git/Hydrascale
status: approved
spec_version: 2
created: 2026-08-04
approved: 2026-08-04
html_generated: 2026-08-04
branch_model: dev-and-live
features:
  - id: foundation
    file: features/00-foundation.md
  - id: desktop-client-removal
    file: features/01-desktop-client-removal.md
  - id: security-audit
    file: features/02-security-audit.md
  - id: security-fixes
    file: features/03-security-fixes.md
  - id: dns-integrity
    file: features/04-dns-integrity.md
  - id: reachability-model
    file: features/05-reachability-model.md
  - id: console-foundation
    file: features/06-console-foundation.md
  - id: console-access-editor
    file: features/07-console-access-editor.md
  - id: upstream-policy
    file: features/08-upstream-policy.md
  - id: docs-and-release
    file: features/09-docs-and-release.md
---

# Hydrascale v1.0

## Overview

Hydrascale lets one Linux host join several Tailscale tailnets at the same time. The
daemon creates a network namespace for each tailnet. The daemon runs a separate
`tailscaled` process inside each namespace. A reconciler loop drives the live host
toward a declared YAML state. A DNS forwarder resolves names across every tailnet.

Version 1.0 makes three changes. First, it replaces the desktop client with a console
that the daemon serves. Second, it gives the operator direct control over reachability,
both on the host and in the upstream tailnet policy. Third, it closes the gap between
the isolation the documentation promises and the isolation the host enforces.

The third change is the reason for the release. The daemon currently inserts an
unrestricted `ACCEPT` rule into the host `FORWARD` chain for every namespace. A process
inside one namespace can therefore reach another namespace and the host network. The
separate network stacks prevent an accidental route leak. They do not prevent a
deliberate one. Version 1.0 makes reachability explicit, denies it by default, and shows
the operator what is allowed.

## Terms

| Term | Part of speech | Meaning in this project | Do not use |
|---|---|---|---|
| tailnet | noun | One Tailscale or Headscale network that the host joins. | network, VPN, mesh |
| namespace | noun | The Linux network namespace that the daemon creates for one tailnet. | netns, container, sandbox, jail |
| daemon | noun | The `hydrascale` process that runs the reconciler and the control API. | server, service, agent, backend |
| host | noun | The Linux machine that runs the daemon. | machine, box, node, system |
| operator | noun | The person who administers the host. | user, admin, customer |
| console | noun | The web interface that the daemon serves on the loopback address. | GUI, web app, dashboard, UI, front end |
| desktop client | noun | The Wails application in `gui/`, which version 1.0 removes. | GUI, desktop app, client app |
| terminal interface | noun | The Bubble Tea interface that `hydrascale tui` starts. | TUI, terminal UI, curses interface |
| control socket | noun | The Unix socket at `/var/lib/hydrascale/api.sock`. | API socket, socket file, IPC socket |
| control server | noun | The Tailscale coordination server, or a Headscale instance. | coordinator, control plane, server |
| peer | noun | A device in a tailnet other than this host. | device, machine, client |
| reachability | noun | The condition where traffic from one source arrives at one destination. | connectivity, access, routing |
| local rule | noun | One reachability rule that the daemon enforces on the host with iptables. | ACL, firewall rule, filter |
| policy | noun | The huJSON access-control document that a control server holds for a tailnet. | ACL file, policy file, ruleset |
| rule set | noun | The complete set of local rules for one host. | config, ACL, policy |
| stage | verb | To record an edit in the console without sending it to the daemon. | draft, queue, buffer |
| apply | verb | To send staged edits to the daemon so the reconciler enforces them. | save, commit, push, submit |
| converge | verb | To reach the state that the configuration declares. | settle, sync, stabilize |
| reconcile | verb | To compare the live host against the configuration and to act on the difference. | sync, refresh, update |
| security audit | noun | The written review of the code that Epic 2 produces. | pentest, review, assessment |
| finding | noun | One defect that the security audit records. | issue, bug, vulnerability |
| brand asset | noun | One design token file, icon, or logo that the console uses. | branding, design file, resource |
| overlay mount | noun | The OverlayFS mount that the daemon places on `/etc` inside a namespace. | shield, DNS shield, overlay hack |
| test host | noun | The machine at `phobos` that verifies a change against a real kernel. | staging, dev box, lab machine |

## Goals

1. A process in one namespace cannot reach another namespace unless a local rule allows
   it. The operator can verify this on the test host.
2. The operator can read and change reachability in the console, for local rules and for
   the upstream policy of each tailnet.
3. The daemon reports when it cannot protect the host `/etc/resolv.conf` file. The
   daemon never fails to protect that file without a report.
4. The repository contains no compiled binary, no desktop client, and no private
   development note.
5. A configuration file that works on version 0.9 works on version 1.0 without an edit.
6. The console, the command line interface, and the terminal interface all use the
   brand that `branding/` defines.

## Non-goals

- Version 1.0 does not add an account system to the console. The console serves the
  loopback address only and it has no sign-in.
- Version 1.0 does not add transport security to the console. The operator reaches a
  remote host through an SSH port forward.
- Version 1.0 does not keep the JSON shape of the existing `/api/*` routes stable. The
  console is the only supported client.
- Version 1.0 does not rewrite the Git history. The repository keeps every existing
  commit, tag, and fork.
- Version 1.0 does not manage tailnet devices, tags, users, or auth keys through the
  upstream policy feature. It reads and writes the policy document only.
- Version 1.0 does not run the console on a non-Linux host. The daemon stays Linux-only.

## Users & personas

**The operator.** One person who administers one Linux host. The operator has root
access. The operator runs `hydrascale init`, edits `/etc/hydrascale/config.yaml`, and
watches the daemon converge. The operator needs to see which tailnet reaches which, and
to change it without writing iptables rules.

**The contributor.** A person who sends a pull request. The contributor needs a
repository that builds offline, a test command that runs without root, and a written
design standard that keeps a change on brand.

**The automation account.** A script that drives the control socket. The automation
account needs a stable configuration schema. It does not need a stable HTTP API in
version 1.0.

## Feature map

| Feature set | Spec file | Epic | Mockups |
|---|---|---|---|
| Foundation | `features/00-foundation.md` | Epic 0 | none |
| Desktop client removal | `features/01-desktop-client-removal.md` | Epic 1 | none |
| Security audit | `features/02-security-audit.md` | Epic 2 | none |
| Security fixes | `features/03-security-fixes.md` | Epic 3 | none |
| DNS integrity | `features/04-dns-integrity.md` | Epic 4 | none |
| Reachability model | `features/05-reachability-model.md` | Epic 5 | none |
| Console foundation | `features/06-console-foundation.md` | Epic 6 | `mockups/01-overview.html`, `mockups/02-namespace-detail.html`, `mockups/05-dns-and-settings.html` |
| Console access editor | `features/07-console-access-editor.md` | Epic 7 | `mockups/03-acl-editor.html` |
| Upstream policy control | `features/08-upstream-policy.md` | Epic 8 | `mockups/04-upstream-policy.html` |
| Documentation and release | `features/09-docs-and-release.md` | Epic 9 | none |

## Architecture & stack

### Components after version 1.0

| Component | Path | Purpose |
|---|---|---|
| Command line interface | `cmd/hydrascale` | `init`, `apply`, `serve`, `install`, `uninstall`, `tui`. |
| Reconciler | `internal/reconciler` | The control loop that drives the host toward the configuration. |
| Namespace manager | `internal/namespaces` | Namespace creation, the veth pair, and the iptables rules. |
| Rule engine | `internal/access` (new) | The local rule model, and the iptables chain that enforces it. |
| DNS forwarder | `internal/dns` | The unified resolver across every namespace. |
| Host access | `internal/hostaccess` | Route and hosts-file propagation to the host. |
| Control API | `internal/api` | The HTTP server on the control socket. |
| Console | `internal/ui` (new) | The static console that `go:embed` places in the binary. |
| Upstream policy client | `internal/policy` (new) | The Tailscale and Headscale policy clients. |
| Terminal interface | `internal/tui` | The Bubble Tea interface. |

### Console stack

The console is a static single-page application at `internal/ui/static/`. It contains
`index.html`, `app.js`, `app.css`, and `tokens.css`. The `go:embed` directive places
these files in the daemon binary. The console uses no build step, no package manager,
and no framework.

This choice beats a bundled framework for four reasons. The repository keeps its
offline vendored build. The release keeps one artifact. Continuous integration keeps one
toolchain. The console loads no code from a network. The cost is that the operator must
write the topology renderer by hand in SVG, and that the React components in
`branding/design_handoff_hydrascale_brand/component_reference/` become a reference rather
than a dependency.

The same pattern already works in the operator's Nydus project, where a hand-written SVG
renderer draws a topology from one JSON endpoint.

### How the console reaches the daemon

The daemon serves the console over HTTP on `127.0.0.1`. The port comes from the
configuration and defaults to `9443`. The daemon serves the existing JSON API on the same
listener. The daemon keeps serving the JSON API on the control socket.

The console has no authentication. Any local process can reach the listener and can
drive the daemon. `Cross-cutting concerns` records this as an accepted risk and lists the
controls that reduce it.

### Fonts

`branding/design_handoff_hydrascale_brand/tokens/fonts.css` loads Space Grotesk and Space
Mono from Google Fonts. The console must not make that request. An operator console for a
network tool must not contact a third party, and the daemon must work on a host with no
internet route. Epic 6 either self-hosts the two font families as WOFF2 files inside the
binary, or it falls back to `system-ui` and `ui-monospace`. `Risks & open questions`
records the licence check that decides which.

### Deploy target

There is no hosted deploy target. A release is a Git tag that GoReleaser turns into a
Linux binary on the GitHub Releases page. The test host `phobos` at `192.168.1.221`
verifies a change against a real kernel. The operator rolls a change back on the test
host by stopping the systemd service.

## Data model

### Configuration, `/etc/hydrascale/config.yaml`

Version 1.0 adds keys. Version 1.0 removes no key and renames no key. A version 0.9
configuration file loads without an edit, and every new key has a default.

```yaml
# existing keys, unchanged
state_dir: /var/lib/hydrascale
infra_subnet: 10.99.0.0/16
socket_group: ""
control_url: ""
resolver:
  mode: unified
  bind_address: 127.0.0.53:5354
tailnets:
  - id: jbones
    auth_key: ""
    control_url: ""
    host_access: true

# new in v1.0
console:
  enabled: true
  bind_address: 127.0.0.1:9443

access:
  # "enforce" applies the rule set. "observe" logs what it would deny and denies nothing.
  mode: enforce
  rules:
    - from: jbones
      to: homelab
      ports: ["tcp/22", "tcp/443"]
    - from: jbones
      to: host
      ports: []

secrets_file: /etc/hydrascale/secrets.yaml
```

### Local rule

| Field | Type | Constraint |
|---|---|---|
| `from` | string | A tailnet id in `tailnets`, or the literal `host`. |
| `to` | string | A tailnet id in `tailnets`, the literal `host`, or the literal `internet`. |
| `ports` | list of string | Each entry matches `tcp/<n>`, `udp/<n>`, or `tcp/<n>-<m>`. An empty list allows every port. |

A local rule allows traffic. There is no deny rule. The daemon denies everything that no
local rule allows. A rule where `from` equals `to` is invalid.

### Secrets file, `/etc/hydrascale/secrets.yaml`

The daemon creates this file with mode `0600` and owner `root`. The control API never
returns its contents. The console writes it through a dedicated route.

```yaml
tailnets:
  jbones:
    tailscale_oauth_client_id: ""
    tailscale_oauth_client_secret: ""
  corp:
    headscale_api_key: ""
    headscale_address: "https://headscale.example.net"
```

An environment variable overrides the matching file value. The variable names follow the
existing `HYDRASCALE_AUTHKEY_<ID>` pattern.

### Event

The reconciler already records an event per action. Version 1.0 adds three event kinds:
`access.applied`, `dns.unprotected`, and `policy.pushed`.

## Cross-cutting concerns

### The console has no authentication

The operator chose a loopback listener with no sign-in. This is an accepted risk and the
spec states it plainly. Any local account, and any process that a local account runs,
can reach `127.0.0.1:9443` and can drive a root daemon. A web page that the operator
opens in a browser on the host can send requests to that address.

Four controls reduce the risk. Epic 6 implements all four.

1. The listener binds to `127.0.0.1` only. The daemon refuses a non-loopback bind
   address and it logs the refusal.
2. Every mutating route requires the header `X-Hydrascale-Console: 1`. A browser cannot
   set a custom header on a cross-origin form post, so a hostile web page cannot reach a
   mutating route.
3. The daemon rejects a request whose `Origin` header is present and is not the console
   origin.
4. The daemon writes an event for every mutating request, and the console shows the
   event list.

Control 2 and control 3 stop a hostile web page. Neither stops a local account. The spec
does not claim otherwise. `Risks & open questions` records the follow-up.

### Privilege model

The daemon runs as root. It must, because it creates namespaces and it writes iptables
rules. Nothing in version 1.0 lowers that requirement.

The `socket_group` key widens control-socket access to one Unix group. Membership of that
group is equivalent to root on the host. Epic 3 makes the documentation say so, and makes
`hydrascale init` say so at the prompt.

### Command execution

The daemon runs `ip`, `iptables`, `sysctl`, `tailscale`, and `tailscaled` through
`os/exec`. Every argument that comes from the configuration or from the control API must
pass validation before it reaches a command. Epic 2 checks every call site. Epic 3 fixes
what Epic 2 finds.

### Error handling

An operation that cannot complete returns an error. The reconciler records the error as
an event and it places the tailnet in an error state. A best-effort operation that fails
silently is a defect in version 1.0. Epic 3 and Epic 4 remove the silent paths that the
survey already found.

### Validation

Every control API route validates its request body before it acts. A route returns
`400` with a JSON error body when validation fails. The error body has the shape
`{"error": "<message>"}`.

### Logging

The daemon logs to standard error. `journalctl -u hydrascale` reads the log. A log line
never contains an auth key, an OAuth secret, or a Headscale API key.

### Accessibility

The console meets these rules. Every control reaches focus by keyboard. A focus ring uses
`--ring-focus`. Text meets a contrast ratio of 4.5 to 1 against its surface. The topology
view has a text equivalent, because a dotted curve carries no meaning to a screen reader.
The console honours `prefers-reduced-motion`, which `tokens/motion.css` already handles.

### Performance targets

The console first paint completes within 500 ms on the test host. A status poll completes
within 200 ms for 8 tailnets and 200 peers. The rule engine writes a changed rule set
within 2 seconds. The reconciler tick does not grow longer than 1 second because of the
rule engine.

### Security posture

The repository is public. No file in the repository contains a secret, a private
development note, an internal planning document, or a reference to the tools that
generated it. Epic 1 establishes the check. Epic 9 repeats it before the release.

## Environments & config

| Environment | What it is |
|---|---|
| Developer machine | macOS or Linux. Runs `go test ./...` and `go vet ./...`. The daemon does not build on macOS, because it uses `Pdeathsig`. |
| Test host | `phobos` at `192.168.1.221`. Linux, x86-64, passwordless sudo, SSH key access. Runs the real daemon against real tailnets. Rollback is `systemctl stop hydrascale`. |
| Release | A Git tag on `main`. GoReleaser builds the Linux binary and attaches it to the GitHub Release. |

### Environment variables

| Variable | Purpose |
|---|---|
| `HYDRASCALE_AUTHKEY_<ID>` | The auth key for tailnet `<ID>`. Exists today. |
| `HYDRASCALE_TS_CLIENT_ID_<ID>` | The Tailscale OAuth client id for tailnet `<ID>`. New. |
| `HYDRASCALE_TS_CLIENT_SECRET_<ID>` | The Tailscale OAuth client secret for tailnet `<ID>`. New. |
| `HYDRASCALE_HS_API_KEY_<ID>` | The Headscale API key for tailnet `<ID>`. New. |
| `HYDRASCALE_CONFIG` | The configuration path. Exists today. |

An environment variable always wins over the secrets file.

### Seed data

The console must never show invented data. The desktop client shipped demo data and
commit `4cfff21` removed that default. The console repeats the rule: an empty state says
what would fill it, as `BRAND.md` requires.

`contrib/` gains a `dev-config.yaml` that declares two tailnets with no auth key. It
drives the console against a daemon that cannot connect, which is the state that most
needs a designed empty view.

## Testing strategy

| Layer | What it covers | Command |
|---|---|---|
| Unit | Pure logic: configuration parsing, rule compilation, policy document edits, route parsing. | `go test ./...` |
| Unit, console | The console JavaScript, through a Node harness that a Go test starts, as Nydus does. | `go test ./internal/ui/...` |
| Integration | The reconciler against a fake command runner. | `go test -race ./...` |
| Host verification | The real daemon on the test host: namespaces, iptables, DNS, reachability. | `.claude/skills/verify-on-phobos` |

Every command that touches the host goes through an interface that a test can replace.
`internal/namespaces` currently calls `exec.Command` directly. Epic 0 extracts that
interface, because the rule engine in Epic 5 is untestable without it.

"Done" for a change means four things. The unit tests pass. `go vet ./...` passes.
`gofmt -l .` prints nothing. A change that alters host behaviour has a recorded
verification on the test host.

## Epics

### Epic 0: Foundation

Goal: the repository can test the work that follows.
Covers: `features/00-foundation.md`.
Depends on: nothing.
Exit criteria: the `dev` branch exists. Continuous integration runs `gofmt`, `go vet`,
and `go test -race` on a pull request into `dev`. A command runner interface replaces the
direct `exec.Command` calls in `internal/namespaces` and `internal/routing`, with the
existing tests still passing. A `verify-on-phobos` skill deploys a branch build to the
test host and rolls it back.

### Epic 1: Desktop client removal and repository hygiene

Goal: the repository holds only what a public project needs.
Covers: `features/01-desktop-client-removal.md`.
Depends on: Epic 0.
Exit criteria: `gui/` is gone. The release workflow builds the daemon only. The 15 MB
`hydrascale` binary is no longer tracked. `README.md` has no desktop client section. The
brand assets are in the repository and the private design source is not. A repository
hygiene check runs in continuous integration.

### Epic 2: Security audit

Goal: a written list of findings, so that Epic 3 fixes facts rather than guesses.
Covers: `features/02-security-audit.md`.
Depends on: Epic 0.
Exit criteria: `docs/security-audit.md` exists. It covers the control API surface, the
privilege boundary, every `os/exec` call site, file and socket permissions, teardown and
cleanup, and input validation on every route. Each finding has a severity, a `file:line`
reference, and a reproduction note. The operator has agreed which findings Epic 3 fixes.

### Epic 3: Security fixes

Goal: the findings that the operator accepted are fixed.
Covers: `features/03-security-fixes.md`.
Depends on: Epic 2.
Exit criteria: every accepted finding has a test that fails before the fix and passes
after it. The `socket_group` documentation states that group membership equals root.
Teardown removes every rule that setup created, and a test proves it.

### Epic 4: DNS integrity

Goal: the host `/etc/resolv.conf` file is protected, or the daemon says that it is not.
Covers: `features/04-dns-integrity.md`.
Depends on: Epic 0.
Exit criteria: the daemon verifies the overlay mount after it mounts it. A failed
overlay mount produces an event, a log line, and a namespace error state, and it no
longer continues silently. The daemon records a checksum of the host `/etc/resolv.conf`
file each tick and reports a change. `hydrascale init` fails the preflight check when the
host `tailscaled` process has `accept-dns` enabled, instead of printing a warning.

### Epic 5: Local reachability model

Goal: the daemon denies reachability that no local rule allows.
Covers: `features/05-reachability-model.md`.
Depends on: Epic 0, Epic 3.
Exit criteria: the daemon writes a dedicated `HYDRASCALE-FWD` iptables chain instead of
an unrestricted `ACCEPT` in `FORWARD`. The default is deny. `access.mode: observe` logs
what it would deny and denies nothing. On the first start after an upgrade, the daemon
writes the rule set that preserves the previous behaviour, and it records what it wrote.
The test host proves that a namespace cannot reach another namespace without a rule.

### Epic 6: Console foundation

Goal: the daemon serves a branded console that shows the live state.
Covers: `features/06-console-foundation.md`.
Depends on: Epic 1, Epic 5.
Exit criteria: `internal/ui/static` is embedded and served on `127.0.0.1`. The overview,
namespace detail, DNS, activity, and settings views work against a real daemon. The four
console controls in `Cross-cutting concerns` are implemented. The console makes no
external network request.

### Epic 7: Console access editor

Goal: the operator edits local rules by drawing them.
Covers: `features/07-console-access-editor.md`.
Depends on: Epic 5, Epic 6.
Exit criteria: the access view shows the flow overview, the reachability matrix, and the
rule list. An edit is staged and the console shows the staged count. Apply sends the
whole rule set and the reconciler enforces it. Denial is drawn as the absence of a line.

### Epic 8: Upstream policy control

Goal: the operator reads and changes the policy of each tailnet from the console.
Covers: `features/08-upstream-policy.md`.
Depends on: Epic 6.
Exit criteria: the console reads the policy for a Tailscale tailnet and for a Headscale
tailnet. It writes the policy for Tailscale with an `If-Match` precondition. It writes
the policy for Headscale when the control server allows it, and it states the reason when
the control server does not. A push runs the validate step first.

### Epic 9: Documentation, terminal interface restyle, and the release

Goal: version 1.0 ships and the documentation matches it.
Covers: `features/09-docs-and-release.md`.
Depends on: every other epic.
Exit criteria: `README.md` describes the console, the local rules, and the upstream
policy feature, and it uses the brand. `docs/DESIGN.md` states the brand rules for a
contributor. The terminal interface uses the brand palette. The upgrade note tells a
version 0.9 operator what changed. The tag `v1.0.0` builds and the binary runs on the
test host.

## Milestones

| Milestone | Epics | What is shippable |
|---|---|---|
| M1 — Clean base | 0, 1 | The repository builds, tests, and holds no dead artifact. A patch release is safe. |
| M2 — Correct daemon | 2, 3, 4 | The daemon no longer leaks DNS silently and the known security findings are fixed. This is a releasable `v0.10.0` if version 1.0 slips. |
| M3 — Enforced isolation | 5 | The daemon enforces reachability. The command line interface and the configuration are the only way to change it. |
| M4 — Console | 6, 7 | The operator sees and edits local rules in a browser. This is the headline of version 1.0. |
| M5 — Upstream and release | 8, 9 | Upstream policy control works and version 1.0 ships. |

M2 is the point at which the release is worth cutting even if nothing else lands. The
security and DNS work must not wait behind the console.

## Assumptions

1. The operator accepts that `hydrascale` stays a root daemon on a trusted single-user
   host. The console threat model follows from that.
2. The test host `phobos` reproduces the reachability defect, because the defect follows
   from iptables rules rather than from a kernel feature. The DNS defect may not
   reproduce there, because the operator last saw it on a Jetson Orin host with a Tegra
   kernel.
3. No external user depends on the JSON shape of the `/api/*` routes. The operator
   accepted this in the compatibility decision.
4. The `dist/` directory and the `hydrascale` binary at the repository root are build
   output rather than a deliberate artifact.
4a. `vendor/` stays untracked. `.gitignore` line 24 ignores it deliberately. The
   maintainer copies the vendored tree to the test host with `rsync` and builds there
   with `-mod=vendor`, so the offline build serves that loop rather than a clone. A
   clone needs network access, which is normal for a Go project. Version 1.0 changes
   nothing here.
5. Version 1.0 keeps IPv4 rules only for local rules. The survey found IPv6 route
   handling in `internal/hostaccess` but no IPv6 firewall rules. Epic 2 confirms whether
   an IPv6 gap exists.
6. `9443` is free on a typical host. The configuration can change it.

## Risks & open questions

| # | Item | Why it matters | How it resolves |
|---|---|---|---|
| R1 | The Tailscale OAuth scope for a policy write is not confirmed. The OpenAPI schema names `policy_file:read` for `GET /tailnet/{tailnet}/acl` and names no scope for `POST`. | Epic 8 cannot document the credential setup without the scope name. | Epic 8 confirms it from the Tailscale OAuth documentation before it writes the setup guide. Until then the spec does not state a write scope name. |
| R2 | A Headscale `PUT /api/v1/policy` succeeds only when the control server runs `policy.mode: "db"`. | An operator with a file-mode Headscale server cannot write the policy from the console. | Epic 8 detects the mode from the error and shows the reason. The local-file fallback is out of scope for version 1.0. |
| R3 | The DNS defect may not reproduce on the test host. The test host `/etc/resolv.conf` also carries the immutable attribute, from the earlier investigation of issue #28, so no process can rewrite it. | Epic 4 could produce a fix that is never proven, or a false negative that looks like proof. | Epic 4 clears the immutable attribute first, then attempts the reproduction. If the attempt fails, Epic 4 delivers detection and reporting rather than a targeted fix, and it says so. |
| R4 | The Space Grotesk and Space Mono licences must permit embedding in a binary. | The console must not fetch a font from Google. | Epic 6 checks the SIL Open Font License terms. If embedding is not permitted, the console falls back to `system-ui` and `ui-monospace`. |
| R5 | A local account can drive the root daemon through the console listener. | This is the accepted risk. It is not mitigated, only reduced. | The operator accepted it. Epic 2 records it as a finding so that it is written down, and the release note states it. |
| R6 | The default-deny migration may break an install that relies on a path the operator does not know about. | An upgrade could cut off a working tailnet. | Epic 5 writes the preserving rule set on first start and records it as an event. `access.mode: observe` lets the operator check before enforcement. |
| R7 | The rule engine must not fight a host firewall. | An operator who runs `ufw` or `firewalld` may see rules disappear on reload. | Epic 5 uses a dedicated chain with a single jump, so a reload is detectable and repairable. Epic 5 tests this on the test host. |
| R8 | ~~Whether `docs/specs/` should be tracked in a public repository is unresolved.~~ **Resolved 2026-08-04.** | The build loop reads tracked files only, because a worker runs in a git worktree. | The operator tracks `docs/specs/`, `CLAUDE.md`, and `.claude/`. A GitHub issue on a public repository is public in any case, so an untracked spec produces a dead link rather than privacy. |

## Changelog

| Date | Round | Change |
|---|---|---|
| 2026-08-04 | 1 | First draft. Written from three interview rounds and a code survey. |
| 2026-08-04 | 1 | Approved. The operator resolved risk R8: `docs/specs/`, `CLAUDE.md` and `.claude/` are all tracked. |
| 2026-08-04 | 1 | Corrected assumption 4a: `vendor/` is untracked on purpose and this is not a defect. Recorded that the test host `/etc/resolv.conf` is immutable, which blocks the Epic 4 reproduction until the attribute is cleared. |
| 2026-08-05 | 1 | Epic 0 built. `internal/routing` now runs `ip route replace` where it ran `ip route add`, because the acceptance criteria of issue #53 name `replace` and because `replace` succeeds for a destination that is already present. |
| 2026-08-05 | 1 | Epic 0 built. **FR-foundation-6** fixes the `Runner` interface to combined output, so `routing.PollStatus` and `namespaces.ListNamespaces` now read the standard output and the standard error stream together, where they read the standard output alone. No command that they run writes to the standard error stream on success, so no defect is known. Epic 5 must keep this in view. |
| 2026-08-05 | 1 | Epic 0 built. The skill `verify-on-phobos` takes the host as `phobos@192.168.1.221` rather than the bare name `phobos`, which does not resolve. The skill gained an explicit `rollback` command block, which **FR-foundation-12** requires and which the first draft did not hold. |
| 2026-08-05 | 1 | Epic 1 built. The console renders in `system-ui` and `ui-monospace` rather than in `Space Grotesk` and `Space Mono`. The source `tokens/fonts.css` loaded both typefaces from Google Fonts, the console makes no request to another host, and `branding/` holds no font file. Issue #90 holds the decision, which Epic 6 must settle. |
| 2026-08-05 | 1 | Epic 1 built. The compiled daemon leaves the index. The rule `/hydrascale` was already in `.gitignore`; a tracked file overrides an ignore rule, which is why the binary stayed. The 15 MB blob stays in the history, which the non-goals require. |
| 2026-08-05 | 1 | Epic 1 built. Issue #93 records that `.goreleaser.yaml` uses `archives.format`, which GoReleaser version 2 deprecates. The release workflow fails when a later version removes it. |
| 2026-08-05 | 1 | Epic 1 built. The hygiene script omits three rules that `features/01-desktop-client-removal.md` names: `CLAUDE.md`, `.claude/`, and the `Claude Code` text pattern. Risk R8 tracks the first two on purpose, and the third matches the tracked `.claude/` files, so a script holding all three exits 1 on every run. Issue #96 holds the decision. |
| 2026-08-05 | 1 | Epic 4 built. Issue #76 adds the configuration key `dns.allow_unprotected`, default `false`. **FR-dns-5** and **FR-dns-6** read two ways together, so the build settles the conflict: a failed overlay mount always records the event `dns.unprotected`, and it places the tailnet in an error state only when `dns.allow_unprotected` is `false`. A tailnet that the operator opted out of protection stays out of the error state, because an error state that the operator cannot clear makes the opt-in useless. |
| 2026-08-05 | 1 | Epic 4 built. Issue #76 records the overlay mount failure in the file `/var/lib/hydrascale/state/<id>/dns-unprotected`, because `internal/daemon/daemon.go:151` sets `cmd.Stderr = nil` and the standard error stream of the child reaches no journal. Issue #75 records that finding. |

## Issue map

Epics 0 to 4 are on the tracker. Epics 5 to 9 stay `planned` until the first milestones
land, so that the console and policy epics can be re-decomposed if the security audit
changes what they need.

| Epic | Issue | Sub-issues | Feature file |
|---|---|---|---|
| Epic 0: Foundation | #48 | #49 #50 #51 #52 #53 #54 | `features/00-foundation.md` |
| Epic 1: Desktop client removal and repository hygiene | #55 | #56 #57 #58 #59 #60 #61 | `features/01-desktop-client-removal.md` |
| Epic 2: Security audit | #62 | #63 #64 #65 #66 #67 | `features/02-security-audit.md` |
| Epic 3: Security fixes | #68 | #69 #70 #71 #72 #73 | `features/03-security-fixes.md` |
| Epic 4: DNS integrity | #74 | #75 #76 #77 #78 #79 | `features/04-dns-integrity.md` |
| Epic 5: Local reachability model | not issued | — | `features/05-reachability-model.md` |
| Epic 6: Console foundation | not issued | — | `features/06-console-foundation.md` |
| Epic 7: Console access editor | not issued | — | `features/07-console-access-editor.md` |
| Epic 8: Upstream policy control | not issued | — | `features/08-upstream-policy.md` |
| Epic 9: Documentation and release | not issued | — | `features/09-docs-and-release.md` |

### Requirement coverage

| Feature | Requirements | Issues that cover them |
|---|---|---|
| foundation | FR-foundation-1 to 12 | #49 #50 #51 #52 #53 #54 |
| desktop-client-removal | FR-removal-1 to 14 | #56 #57 #58 #59 #60 #61 |
| security-audit | FR-audit-1 to 14 | #63 #64 #65 #66 #67 |
| security-fixes | FR-fix-1 to 19 | #69 #70 #71 #72 #73 |
| dns-integrity | FR-dns-1 to 16 | #75 #76 #77 #78 #79 |

Every one of the 75 requirements in these five features is cited by at least one issue.

### Requirements already satisfied when the issues were created

| Requirement | State |
|---|---|
| FR-foundation-1 | The `dev` branch exists at `89745a9`. The maintainer cut it by hand. |
| FR-foundation-11, FR-foundation-12 | Written as `.claude/skills/verify-on-phobos/SKILL.md`. Issue #54 proves them on the test host. |
| FR-removal-5, FR-removal-11 | `.gitignore` already ignores `/hydrascale` and `branding/`. The files are still tracked, so the removal work remains. |
