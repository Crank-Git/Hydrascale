---
name: Hydrascale v1.0
slug: hydrascale-v1
repo: Crank-Git/Hydrascale
status: approved
spec_version: 2
created: 2026-08-04
approved: 2026-08-23
html_generated: 2026-08-24
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
  - id: agent-skills
    file: features/10-agent-skills.md
  - id: policy-document-model
    file: features/11-policy-document-model.md
  - id: visual-acl-editor
    file: features/12-visual-acl-editor.md
  - id: visual-policy-advanced
    file: features/13-visual-policy-advanced.md
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
| forward path | noun | The host rules that carry the traffic of one namespace to the internet. | uplink, egress, NAT path |
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
| crash record | noun | The kernel log that the `pstore` backend keeps across a restart. | crash dump, core dump, panic log, vmcore |
| credential | noun | One value that authenticates the daemon to a control server: an auth key, an OAuth client identifier, an OAuth client secret, or a Headscale API key. | secret, token, key, password |
| secrets file | noun | The root-only file that holds a credential per tailnet, which `secrets_file` names. | secret store, vault, credentials file |
| override | verb | To replace one file value with the value of the matching environment variable. | shadow, mask, take precedence |
| access token | noun | The short-lived bearer token that the Tailscale OAuth token endpoint returns for a credential. | token, API key, bearer |
| ETag value | noun | The value of the `ETag` header that a Tailscale policy read returns, which the write sends back as `If-Match`. | version, hash, revision |
| conflict | noun | The condition where the policy changed between the read and the write, which the control server reports as HTTP 412. | collision, race, clash |
| control server kind | noun | The type of control server that one tailnet joins: `tailscale` or `headscale`. | provider, backend, flavour, type |
| credential state | noun | The condition where a tailnet holds a complete credential, or holds none. | credential status, auth state |
| write availability | noun | The condition where the daemon can write the policy of one tailnet to its control server. | writable, editable, permission |
| coding agent | noun | The program that reads a skill and runs a command for the operator, such as Claude Code. | AI, assistant, LLM, bot, harness |
| skill | noun | One Markdown file that a coding agent loads, which states how the agent performs one task. | prompt, instruction file, plugin, rule |
| routing form | noun | One command that sends work into the namespace of a named tailnet, such as `hydrascale exec`. | wrapper, prefix, invocation |
| tag | noun | An ownership label that a control server assigns to a device, named `tag:<name>` in a policy document. | label, group (for a tag) |
| group | noun | A named set of users that a policy document's `groups` block defines, named `group:<name>`. | tag (for a group), team |
| host alias | noun | A named alias for a device or a subnet that a policy document's `hosts` block defines. Distinct from `host`, the Linux machine that runs the daemon. | host, alias |
| autogroup | noun | A control-server-defined group that a policy document references by name, such as `autogroup:internet`. | built-in group |
| allow rule | noun | One entry of a policy document's `acls` block: a source, a destination, and a port list. | acl, rule |
| grant | noun | One entry of a policy document's `grants` block: a source, a destination, and an optional application capability. | acl (for a grant) |
| SSH rule | noun | One entry of a policy document's `ssh` block: a source, a destination, an SSH user list, and an action. | ssh acl |
| auto-approver | noun | An entry of a policy document's `autoApprovers` block that lets a route or an exit node skip manual approval. | approver |
| node attribute | noun | An entry of a policy document's `nodeAttrs` block that applies one attribute to a set of targets. | attribute, flag |
| posture | noun | An entry of a policy document's `postures` block that a control server checks against a connecting device. | device posture, check |
| IP set | noun | A named set of network segments that a policy document's `ipsets` block defines. | ipset, CIDR set |
| document model | noun | The in-memory structure that the daemon builds from a policy document's text, used to read and change a policy section without altering a byte it does not touch. | AST, parse tree, document object |
| visual editor | noun | The policy view's region that draws tags, groups, and rules instead of raw text. | drawn editor, GUI editor |
| text editor | noun | The policy view's region that shows the huJSON document as editable text, which `features/08-upstream-policy.md` builds. | raw editor, code editor |

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
| Agent skills | `features/10-agent-skills.md` | Epic 10 | none |
| Policy document model | `features/11-policy-document-model.md` | Epic 11 | none |
| Visual ACL editor | `features/12-visual-acl-editor.md` | Epic 12 | `mockups/06-visual-acl-editor.html` |
| Visual policy editor — SSH and advanced constructs | `features/13-visual-policy-advanced.md` | Epic 13 | `mockups/07-advanced-policy-constructs.html` |

Epic 0 to Epic 9 build version 1.0. Epic 10 follows the release of version 1.0. Epics 11
to 13 build version 1.2, the visual policy editor, and they follow Epic 10. (Version 1.1
already covers the console fixes and the credential-state work between Epic 9 and Epic
10, tagged `v1.1.0` and `v1.1.1`.)

## Architecture & stack

### Components after version 1.0

| Component | Path | Purpose |
|---|---|---|
| Command line interface | `cmd/hydrascale` | `init`, `apply`, `serve`, `install`, `uninstall`, `tui`, `skills`. |
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

### Epic 10: Agent skills

Goal: a coding agent sends a command to the tailnet that the operator names, and it stops
before a command that disconnects the host.
Covers: `features/10-agent-skills.md`.
Depends on: Epic 9, because this epic follows the release of version 1.0.
Exit criteria: `hydrascale switch` states that it changes no state. The help of
`hydrascale env` states no procedure that fails. The repository holds two skills under
`skills/`. `hydrascale skills install` writes them to the skill directory of the
operator. A test fails when a skill names a command that the command tree does not hold.

### Epic 11: Policy document model

Goal: an edit to one part of a policy document changes no byte outside that part.
Covers: `features/11-policy-document-model.md`.
Depends on: Epic 8.
Exit criteria: parsing a document with a comment, then editing one `acls` entry,
produces output where every other line is byte-for-byte identical to the input. A
document holding a key the model does not name round-trips that key unchanged. A parse
failure names its line and column.

### Epic 12: Visual ACL editor

Goal: the operator edits tags, groups, and reachability rules by drawing them, on the
same document the text editor edits.
Covers: `features/12-visual-acl-editor.md`.
Depends on: Epic 11.
Exit criteria: the Policy view carries a Visual and a Text control over one staged
document. The matrix draws every tag and group a rule references. A click stages an
`acls` entry. Push sends the whole document through the existing validate-then-write
path. A section the control server does not support states the reason and stays
editable in Text.

### Epic 13: Visual policy editor — SSH and advanced constructs

Goal: the operator edits SSH access, auto-approvers, node attributes, postures, and
tests by drawing them.
Covers: `features/13-visual-policy-advanced.md`.
Depends on: Epic 12.
Exit criteria: each of the five sections shows and edits its entries. A control server
that does not support `postures` shows every entry read-only and disables Push while the
key remains. Running Tests marks each assertion `pass` or `fail` without blocking Push.

## Milestones

| Milestone | Epics | What is shippable |
|---|---|---|
| M1 — Clean base | 0, 1 | The repository builds, tests, and holds no dead artifact. A patch release is safe. |
| M2 — Correct daemon | 2, 3, 4 | The daemon no longer leaks DNS silently and the known security findings are fixed. This is a releasable `v0.10.0` if version 1.0 slips. |
| M3 — Enforced isolation | 5 | The daemon enforces reachability. The command line interface and the configuration are the only way to change it. |
| M4 — Console | 6, 7 | The operator sees and edits local rules in a browser. This is the headline of version 1.0. |
| M5 — Upstream and release | 8, 9 | Upstream policy control works and version 1.0 ships. |
| M6 — Agent skills | 10 | A coding agent routes a command to the named tailnet. This follows version 1.0. |
| M7 — Visual policy editor | 11, 12, 13 | The operator edits an upstream policy document by drawing tags, groups, rules, SSH access, auto-approvers, node attributes, postures, and tests, without leaving a byte of the document's other content changed. This is version 1.2, and it follows version 1.1 (`v1.1.0`, `v1.1.1`). |

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
| R1 | ~~The Tailscale OAuth scope for a policy write is not confirmed.~~ **Resolved 2026-08-05.** | Epic 8 can document the credential setup. | The scope is `policy_file`. The OpenAPI schema states `OAuth Scope: policy_file.` for `operationId: setPolicyFile`, and `https://tailscale.com/kb/1623/` states that `policy_file` covers `POST /api/v2/tailnet/:tailnet/acl`. That page also states that `devices:posture_attributes` and `devices:core:read` are required with this scope. Retrieved 2026-08-05. |
| R2 | A Headscale `PUT /api/v1/policy` succeeds only when the control server runs `policy.mode: "database"`. `hscontrol/types/config.go:54` at tag `v0.29.3` declares the value. | An operator with a file-mode Headscale server cannot write the policy from the console. | Epic 8 detects the mode from the error and shows the reason. The local-file fallback is out of scope for version 1.0. |
| R3 | The DNS defect may not reproduce on the test host. The test host `/etc/resolv.conf` also carries the immutable attribute, from the earlier investigation of issue #28, so no process can rewrite it. | Epic 4 could produce a fix that is never proven, or a false negative that looks like proof. | Epic 4 clears the immutable attribute first, then attempts the reproduction. If the attempt fails, Epic 4 delivers detection and reporting rather than a targeted fix, and it says so. |
| R4 | The Space Grotesk and Space Mono licences must permit embedding in a binary. | The console must not fetch a font from Google. | Epic 6 checks the SIL Open Font License terms. If embedding is not permitted, the console falls back to `system-ui` and `ui-monospace`. |
| R5 | A local account can drive the root daemon through the console listener. | This is the accepted risk. It is not mitigated, only reduced. | The operator accepted it. Epic 2 records it as a finding so that it is written down, and the release note states it. |
| R6 | The default-deny migration may break an install that relies on a path the operator does not know about. | An upgrade could cut off a working tailnet. | Epic 5 writes the preserving rule set on first start and records it as an event. `access.mode: observe` lets the operator check before enforcement. |
| R7 | The rule engine must not fight a host firewall. | An operator who runs `ufw` or `firewalld` may see rules disappear on reload. | Epic 5 uses a dedicated chain with a single jump, so a reload is detectable and repairable. Epic 5 tests this on the test host. |
| R8 | ~~Whether `docs/specs/` should be tracked in a public repository is unresolved.~~ **Resolved 2026-08-04.** | The build loop reads tracked files only, because a worker runs in a git worktree. | The operator tracks `docs/specs/`, `CLAUDE.md`, and `.claude/`. A GitHub issue on a public repository is public in any case, so an untracked spec produces a dead link rather than privacy. |
| R9 | The operator chose byte-for-byte huJSON round-trip fidelity (Epic 11) over a simpler best-effort reformat. This is a token-level partial rewrite, closer to a small huJSON-aware editor than a parse-and-reprint. | The engineering cost is materially higher than a naive JSON marshal/unmarshal, and a defect here corrupts an operator's real policy document, including comments a colleague wrote. | Epic 11's acceptance criteria require a byte-for-byte diff test on every edit path before Epic 12 or Epic 13 build against it. If the cost proves too high during Epic 11, the operator revisits this decision rather than shipping a silent reformatter. |
| R10 | Headscale does not support `postures`, `srcPosture`, or `ipsets` (confirmed against `docs/ref/policy.md` at tag `v0.29.3`, retrieved 2026-08-23). Its support for `groups`, `hosts`, `tagOwners`, `autoApprovers`, `tests`, and `sshTests` is not explicitly documented at that page. | The visual editor could show a control that a Headscale write silently drops. | Epic 12 and Epic 13 confirm each section's Headscale support against a live Headscale test instance before they ship that section as editable; an unconfirmed section defaults to the read-only treatment FR-vadv-11 defines for `postures`, never to a silent assumption that it works. |

## Changelog

| Date | Round | Change |
|---|---|---|
| 2026-08-04 | 1 | First draft. Written from three interview rounds and a code survey. |
| 2026-08-04 | 1 | Approved. The operator resolved risk R8: `docs/specs/`, `CLAUDE.md` and `.claude/` are all tracked. |
| 2026-08-04 | 1 | Corrected assumption 4a: `vendor/` is untracked on purpose and this is not a defect. Recorded that the test host `/etc/resolv.conf` is immutable, which blocks the Epic 4 reproduction until the attribute is cleared. |
| 2026-08-05 | 1 | Epic 0 built. `internal/routing` now runs `ip route replace` where it ran `ip route add`, because the acceptance criteria of issue #53 name `replace` and because `replace` succeeds for a destination that is already present. |
| 2026-08-05 | 1 | Epic 0 built. **FR-foundation-6** fixes the `Runner` interface to combined output, so `routing.PollStatus` and `namespaces.ListNamespaces` now read the standard output and the standard error stream together, where they read the standard output alone. No command that they run writes to the standard error stream on success, so no defect is known. Epic 5 must keep this in view. |
| 2026-08-05 | 1 | Epic 0 built. The skill `verify-on-phobos` takes the host as `phobos@192.168.1.221` rather than the bare name `phobos`, which does not resolve. The skill gained an explicit `rollback` command block, which **FR-foundation-12** requires and which the first draft did not hold. |
| 2026-08-05 | 1 | **The immutable claim of 2026-08-04 is wrong.** `/etc/resolv.conf` on the test host is a symbolic link to `/run/systemd/resolve/stub-resolv.conf`, and `systemd-resolved` owns the target. No immutable attribute is set. `lsattr` fails on the path because it is a symbolic link. The Epic 4 reproduction is therefore not blocked by an attribute. Measured for issue #75; `docs/dns-investigation.md` holds the evidence. |
| 2026-08-05 | 1 | Epic 2 built. `docs/security-audit.md` holds 49 findings. **Every disposition in it is a proposal and the maintainer has confirmed none of them**, because the audit ran while the operator was not available. `SA-5`, the console threat model, keeps the acceptance that this specification already records. The operator confirms or reverses the other 48. |
| 2026-08-05 | 1 | Epic 2 built. The two high findings that the audit reproduced on the test host, `SA-8` and `SA-9`, are fixed by Epic 5 and not by Epic 3. They need the `HYDRASCALE-FWD` chain and the deny default of `features/05-reachability-model.md`. Epic 5 holds no issue on the board, therefore no scheduled work fixes the two highest reproduced findings. |
| 2026-08-05 | 1 | Epic 2 built. **FR-fix-15** does not cover `SA-1`. That requirement names the secrets file, and the auth key that `GET /api/status` returns comes from the configuration file. Epic 3 widens the scope of issue #71 to redact `StatusResponse.Desired`. |
| 2026-08-05 | 1 | Epic 2 built. `CLAUDE.md` states that the daemon appends its `FORWARD` jump so that an operator rule keeps its position. `internal/namespaces/ns.go:273` inserts at position 1. The test host shows the rules at the foot of the chain, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` each take position 1 after the daemon starts. The position of the rule is therefore not stable. Recorded as `SA-25`. |
| 2026-08-05 | 1 | Epic 4 built. **The `resolv.conf` defect does not reproduce on the test host.** The overlay mount succeeds for both namespaces, and the host file holds the same checksum across the observation and across a host restart. `docs/dns-investigation.md` records the evidence, which **FR-dns-16** requires. |
| 2026-08-05 | 1 | Epic 4 built. The investigation for issue #75 found the defect that explains the report. `internal/daemon/daemon.go:151` sets `cmd.Stderr = nil`, which sends the child standard error to the null device. The message at `cmd/hydrascale/nsdaemon.go:56` therefore reaches no journal, and it never has. On a host where the overlay mount fails, the daemon returns to the behaviour before issue #28 and the operator sees nothing. Issue #76 answers this with a non-zero exit and a protection record, rather than with a message. |
| 2026-08-05 | 1 | Epic 4 built. The configuration gains the key `dns.allow_unprotected`, with the default `false`. When it is `true`, a tailnet starts although the overlay mount failed, and the daemon records the event `dns.unprotected`. The key exists for a host whose kernel cannot mount OverlayFS. |
| 2026-08-05 | 1 | Epic 4 built. The flag `--force` of `hydrascale init` gains a second meaning: it turns the `accept-dns` preflight failure into a warning. The flag already overwrote an existing configuration file. One flag with two stated meanings is chosen over two flags that both mean force. The operator reverses this if they prefer a separate flag. |
| 2026-08-05 | 1 | Epic 4 built. **One acceptance criterion is not verified on hardware.** The sixth criterion of issue #76, "On the test host, two tailnets both report `protected: true`", needs a deploy. No deploy ran, because issue #104 records that the test host restarted eight times with no clean shutdown record, and the daemon on that host serves two real tailnets. The operator runs the `verify-on-phobos` skill and confirms the criterion. |
| 2026-08-05 | 1 | Epic 1 built. The console renders in `system-ui` and `ui-monospace` rather than in `Space Grotesk` and `Space Mono`. The source `tokens/fonts.css` loaded both typefaces from Google Fonts, the console makes no request to another host, and `branding/` holds no font file. Issue #90 holds the decision, which Epic 6 must settle. |
| 2026-08-05 | 1 | Epic 1 built. The compiled daemon leaves the index. The rule `/hydrascale` was already in `.gitignore`; a tracked file overrides an ignore rule, which is why the binary stayed. The 15 MB blob stays in the history, which the non-goals require. |
| 2026-08-05 | 1 | Epic 1 built. Issue #93 records that `.goreleaser.yaml` uses `archives.format`, which GoReleaser version 2 deprecates. The release workflow fails when a later version removes it. |
| 2026-08-05 | 1 | Epic 1 built. The hygiene script omits three rules that `features/01-desktop-client-removal.md` names: `CLAUDE.md`, `.claude/`, and the `Claude Code` text pattern. Risk R8 tracks the first two on purpose, and the third matches the tracked `.claude/` files, so a script holding all three exits 1 on every run. Issue #96 holds the decision. |
| 2026-08-05 | 1 | Epic 7 built. **FR-editor-28** reads the field `active_paths` of `GET /api/access`, which the daemon builds from `ss -H -tna` and `ip -json route get`. The warning reads no property of the console request, because `internal/api/console.go` refuses a console bind address that is not a loopback address. Issue #199 holds the decision of the operator. |
| 2026-08-05 | 1 | Epic 4 built. Issue #76 adds the configuration key `dns.allow_unprotected`, default `false`. **FR-dns-5** and **FR-dns-6** read two ways together, so the build settles the conflict: a failed overlay mount always records the event `dns.unprotected`, and it places the tailnet in an error state only when `dns.allow_unprotected` is `false`. A tailnet that the operator opted out of protection stays out of the error state, because an error state that the operator cannot clear makes the opt-in useless. |
| 2026-08-05 | 1 | Epic 4 built. Issue #76 records the overlay mount failure in the file `/var/lib/hydrascale/state/<id>/dns-unprotected`, because `internal/daemon/daemon.go:151` sets `cmd.Stderr = nil` and the standard error stream of the child reaches no journal. Issue #75 records that finding. |
| 2026-08-05 | 1 | Issue #96 resolved. The operator keeps `CLAUDE.md` and `.claude/` tracked, and `features/01-desktop-client-removal.md` now states the rules that `scripts/check-hygiene.sh` enforces. The script keeps its behaviour. The `Claude Code` content rule goes, because the tracked files contain the string and the rule would fail on every run. The hygiene rules cover tracked files only, so no rule reaches the `Co-Authored-By` line in a commit message. |
| 2026-08-05 | 1 | Issue #90 resolved. The operator decided: "Self-host the font files under `internal/ui/static/brand/fonts/`, with `OFL.txt`. Epic 6 work, not in the current backlog." A request to a font host and a system-font stack as the final state are both rejected. `features/06-console-foundation.md` holds the work that Epic 6 must do, and **FR-console-45** to **FR-console-47** state the result. This round adds no font file, so the console renders `system-ui` and `ui-monospace` until Epic 6 lands. |
| 2026-08-05 | 1 | Epic 3 built, in part. Five issues shipped: #69, #70, #71, #72, and #116. Issue #73 is held. **The batch is partial by decision of the project manager**, and the operator confirms it. |
| 2026-08-05 | 1 | Epic 3 built. **FR-fix-15 is widened.** That requirement names the secrets file, and `SA-1` is an auth key that `GET /api/status` returns from the configuration file. Issue #72 therefore covers every credential in an API response. `config.Tailnet.AuthKey` now carries `json:"-"`, which closes every response at once. Without the widening, the highest control API finding survives the epic. |
| 2026-08-05 | 1 | Epic 3 built. **Issue #116 is new.** The security audit found five findings that no issue of Epic 3 covered: `internal/hostaccess` and `internal/daemon` still called `os/exec` directly, against the rule in `CLAUDE.md`. Epic 0 moved `internal/namespaces` and `internal/routing` and stopped. `git grep -c "exec.Command" -- internal` now returns `internal/execx/execx.go` alone. |
| 2026-08-05 | 1 | Epic 3 built. `execx` gains a second interface, `Starter`, for a long-lived child process. `Runner` runs a command to completion and returns its combined output, and `tailscaled` outlives the call, so it does not fit. The daemon keeps `Setpgid` and `Pdeathsig`, which stop an orphaned `tailscaled`. |
| 2026-08-05 | 1 | Epic 3 built. Two teardown functions had no caller. `hostaccess.Manager.Teardown` and `namespaces.TeardownHostAccess` were dead code, therefore a tailnet the operator removed kept its `/etc/hosts` entries and `host_access: false` left its rules in place. Issue #69 wires both into the reconciler and proves the wiring with a test on the diff. |
| 2026-08-05 | 1 | **Issue #73 is held, and issue #122 records why.** `govulncheck` reports 13 called vulnerabilities in the Go standard library and one in `golang.org/x/net` on `dev`. To merge the step is to fail every pull request until the Go toolchain and the dependencies move. That change alters what the release builds with, therefore the operator decides it. |
| 2026-08-05 | 1 | **The operator confirmed the dispositions of the security audit.** The answer is "Accept all 48 proposals as written." `docs/security-audit.md` records it, and the earlier statement that the maintainer had confirmed none of them is removed. `SA-5` keeps the acceptance that this specification already holds. The routing of `SA-8`, `SA-9`, `SA-33`, and `SA-48` to Epic 5 is settled input to the Epic 5 decomposition. |
| 2026-08-05 | 1 | **Issue #122 resolved. The toolchain moves to Go 1.26.5.** The operator asked for the latest Go 1.26 patch. The measurement gives the same answer: `govulncheck` names `crypto/tls@go1.26.5` as the fix for `GO-2026-5856`, which is the highest fix line of the 13 called standard library advisories, therefore 1.26.5 is also the lowest 1.26 patch that clears them all. `go.mod`, `.github/workflows/ci.yml`, and `.github/workflows/release.yml` now hold the same version, which closes the gap with the test host. `golang.org/x/net` moves from v0.48.0 to v0.53.0, which holds the fix for `GO-2026-4918`. |
| 2026-08-05 | 1 | **Issue #127 built. The test host already captured a crash record, and no package was installed.** The platform holds the `efi_pstore` backend, and `systemd-pstore.service` is enabled by the Ubuntu preset, so step 1 of the issue was a measurement rather than a change. `/var/lib/systemd/pstore` already held five directories from 2026-07-30 and 2026-07-31, and `1785542021` holds `Kernel panic - not syncing: Fatal exception in interrupt` at `blk_mq_end_request+0x3e/0x40`. The deliberate crash of 2026-08-05 added the two directories `1785939881` and `1785939882`, which proves the capture across a restart. One crash writes more than one directory, therefore count the panic lines rather than the directories. |
| 2026-08-05 | 1 | Issue #127. **The archive holds no record for any of the eight restarts that issue #104 measured.** The capture worked on 2026-07-31 and it works now, therefore those eight restarts were not kernel panics. This changelog line records the measurement alone. This issue does not investigate the daemon, which the issue boundaries state. |
| 2026-08-05 | 1 | Issue #127. `kernel.sysrq` equals `176` on the test host, which `/etc/sysctl.d/10-magic-sysrq.conf` sets. That value omits the bit `0x8` that the crash function needs, so `echo c > /proc/sysrq-trigger` does nothing until `sysctl` raises the value. A restart restores `176`, therefore the deliberate crash leaves no change on the host. |
| 2026-08-05 | 1 | Epic 5 built, in part. Issue #134 adds `internal/access` with the rule model, the validation, and the pure compiler. `Compile` takes the chain tail as an argument, so that issue #136 supplies the `observe` tail without a change to the compiler. |
| 2026-08-05 | 1 | Issue #134. **The compiler emits the `internet` destination as `! -o vh+`.** One iptables rule holds one output device match, therefore the compiler cannot name every other namespace device in one rule. The wildcard `vh+` matches every host side veth device that `internal/namespaces/ns.go:220` builds, so the negation excludes every namespace at once. This also satisfies the edge case that two tailnets use overlapping peer address ranges, because the match names a device rather than an address. |
| 2026-08-05 | 1 | Issue #134. **A rule whose `from` is `host` compiles to no iptables rule.** The host originates that traffic on the `OUTPUT` chain, and version 1.0 creates no chain there. The reply from the namespace matches the established-connection rule of `HYDRASCALE-OUT`, so the path works. Validation accepts the rule, which **FR-access-7** requires. The operator reverses this if version 1.0 must filter `OUTPUT`. |
| 2026-08-05 | 1 | Issue #134. **`HYDRASCALE-OUT` closes with one tail rule for each namespace device.** `INPUT` carries the traffic of every host interface, therefore one unmatched `-j DROP` would also stop the SSH session of the operator. `HYDRASCALE-FWD` keeps the single closing rule that `features/05-reachability-model.md` states. |
| 2026-08-05 | 1 | Issue #134. The configuration key `access` is a pointer field, because **FR-access-21** detects the migration by the presence of the key. A nil value means that the file holds no `access` key, and an empty block means that the operator suppressed the migration. A version 0.9 file therefore loads with no change. |
| 2026-08-05 | 1 | Epic 5 built, in part. Issue #136 adds `access.ObserveTail` and `access.TailForMode`, which supply the tail that **FR-access-4** names. The `observe` tail holds a `LOG` rule with the prefix `hydrascale-would-deny: ` and the match `-m limit --limit 60/minute`, then a `RETURN` rule and no `DROP` rule. `TailForMode` rejects a third mode value rather than a return of the enforce tail, because a caller that guesses chooses between a host that filters nothing and a host that drops everything. |
| 2026-08-05 | 1 | Issue #136. **The on-host criterion of issue #136 is not met.** That criterion reads "On the test host, `observe` mode writes `hydrascale-would-deny` lines to the journal and drops nothing". Nothing writes the chain to the host until issue #135, therefore issue #135 carries the criterion. The compiled argument list is the evidence that the rule is correct. |
| 2026-08-05 | 1 | Epic 5 built, in part. Issue #135 adds `access.Writer`, which writes the compiled rule set into `HYDRASCALE-FWD` and `HYDRASCALE-OUT` with `iptables-restore --noflush`, and which keeps one jump rule in `FORWARD` and one in `INPUT`. `internal/namespaces/ns.go` writes no `FORWARD` rule any more, which removes the finding `SA-8` and the finding `SA-9`. |
| 2026-08-05 | 1 | Issue #135. **The jump rule goes to position 1.** `CLAUDE.md` and `.claude/rules/access-invariants.md` state that the daemon appends the jump. The operator decided on 2026-08-05: "insert at position 1". `SA-25` records that the append is not stable, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` each take position 1 after the daemon starts. Issue #139 corrects the two documents; issue #135 changes no document. |
| 2026-08-05 | 1 | Issue #135. **`execx` gains a third interface, `InputRunner`.** `Runner` gives a command an empty standard input, and `iptables-restore` reads its rule file on standard input. `InputRunner` is a separate interface rather than a second method on `Runner`, so that every test double of `Runner` in another package stays valid. `OSRunner` and `Recorder` carry both, and `Recorder.Call` now holds the recorded standard input. |
| 2026-08-05 | 1 | Issue #135. **The chain carries a marker rule that holds the fingerprint of the compiled rule set.** **FR-access-17** asks the daemon to write only when the compiled set and the live chain differ. `iptables -S` returns a rule in a normal form: it orders the options, it appends `/32` to a host address, it adds `-m tcp` to a port match, and it prints `60/min` for `60/minute`. A text comparison against the compiled arguments therefore reports a difference on every tick. The marker rule is `-m comment --comment hydrascale:<fingerprint>` at the head of each chain, it carries no target, and the kernel returns it unchanged. |
| 2026-08-05 | 1 | Issue #135. **The rule file holds an explicit `-F` line for each chain.** `iptables-restore --noflush` keeps the rules of a chain that the file declares, therefore a write without the flush appends a second copy of every rule. The file declares the two chains, flushes them, and writes the rules, all inside one transaction. It names no other chain, so an operator rule keeps its place. |
| 2026-08-05 | 1 | Issue #135. **The daemon removes the version 0.9 `FORWARD` rules after a write in the mode `enforce`, not at start.** The first deploy removed them at start in the mode `observe`, and both namespaces lost every forwarded path within seconds: `sudo ip netns exec ns-havoc curl -sS -m8 https://example.com` returned `curl: (28) Resolving timed out after 8001 milliseconds`. The rules `-i vh<hash> -j ACCEPT` are the only path that accepts the traffic of a namespace until `HYDRASCALE-FWD` holds the rules of the operator, and the mode `observe` writes a chain that accepts nothing and drops nothing. `namespaces.RemoveLegacyForwardRules` matches the two exact forms `-i vh<hash> -j ACCEPT` and `-o vh<hash> -m state --state RELATED,ESTABLISHED -j ACCEPT`, so an operator rule that names the same device keeps its place. |
| 2026-08-05 | 1 | Issue #135. **`SA-48` gets a measurement rather than a rule.** `namespaces.NamespaceForwarding` reads `net.ipv4.ip_forward` inside each namespace on every cycle, and the reconciler records `access.namespace_forwarding` when the value is not `0`. The deny happens in `HYDRASCALE-FWD` on the host before the packet reaches the second namespace, therefore the containment no longer depends on the kernel default. |
| 2026-08-05 | 1 | Issue #135. **A configuration file with no `access` key cuts every namespace off.** `Config.AccessMode` returns `enforce` for such a file, and the file declares no rule, therefore `HYDRASCALE-FWD` holds the closing `-j DROP` rule and no `ACCEPT` rule. Measured on the test host: `sudo ip netns exec ns-havoc curl -sS -m8 https://example.com` returned `curl: (28) Resolving timed out after 8001 milliseconds`, and the same command returned `200` once the file held `from: havoc, to: internet`. **FR-access-21** names a migration that writes the rules of the version 0.9 host; until that migration exists, an operator who installs this version must add an `internet` rule for each tailnet. |
| 2026-08-05 | 1 | Issue #135. **The edge case "a tailnet is removed while a rule names it" is not met end to end.** `internal/config/config.go:175` runs `RuleSet.Validate` inside `LoadConfig`, therefore a file whose rule names a removed tailnet fails to load and the daemon refuses to start. `Reconciler.declaredRules` drops such a rule and records `access.rule_dropped`, but `LoadConfig` rejects the file first, so the filter never runs. Issue #135 does not change `internal/config`, because issue #137 holds that package. The operator decides whether `LoadConfig` drops the rule or keeps the rejection. |
| 2026-08-05 | 1 | Issue #135. **`HYDRASCALE-FWD` opens with a return rule for a packet that touches no namespace device.** `features/05-reachability-model.md` line 158 states that the daemon appends the jump, and the closing `-A HYDRASCALE-FWD -j DROP` rule is safe under that placement, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` accept their traffic first. The operator moved the jump to position 1 on 2026-08-05, therefore every forwarded packet of the host now enters the chain and the closing rule would also stop the container traffic and the subnet route traffic. The rule `-A HYDRASCALE-FWD ! -i vh+ ! -o vh+ -j RETURN` restores that traffic. The daemon denies the paths of a namespace; it owns no other path. The two decisions conflict, and the operator reverses this rule if the daemon must own the whole `FORWARD` chain. |
| 2026-08-05 | 1 | Issue #136. `Config.AccessMode` returns `enforce` both for a file with no `access` key and for an `access` block with no `mode` key, which **FR-access-19** requires. The pointer field of issue #134 makes the two cases distinct in the type, and the mode is the same in both. |
| 2026-08-05 | 1 | Issue #134. **The operator decided that the `internet` destination excludes the private ranges.** The exact words are: "Exclude all RFC1918 + the host's subnets". A `to: internet` rule therefore excludes `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, and `127.0.0.0/8`, together with every namespace device. `features/05-reachability-model.md` now states the five ranges, where it stated "not a private range that another namespace uses". |
| 2026-08-05 | 1 | Issue #134. **The row above about `! -o vh+` is not complete, and this row corrects it.** That match excludes every namespace device and no address range, therefore a namespace with an `internet` rule reached `192.168.1.215`. That is the finding `SA-9`, which the operator accepted for Epic 5. Issue #137 gives every tailnet an `internet` rule at migration, so the gap would have reached every host that upgrades. |
| 2026-08-05 | 1 | Issue #134. **The compiler excludes the five ranges with a repeated `iprange` match in one rule.** The two other forms are wrong: a `RETURN` rule leaves `HYDRASCALE-FWD` and skips its closing `-j DROP`, and one accept rule for each range accepts a packet that is outside any one range. `ipset` is a new host command, which the operator rejected. A read-only check on the test host measured the syntax: `iptables v1.8.10 (nf_tables)` accepts a rule with two `-m iprange ! --dst-range` matches, and it answers `multiple -d flags not allowed` for a rule with two `-d` options. |
| 2026-08-05 | 1 | Issue #122. `go mod tidy` moved `github.com/charmbracelet/bubbles` from the indirect block to the direct block, because `internal/tui/model.go` imports `bubbles/textinput`. The count of direct dependencies that `CLAUDE.md` states goes from six to seven. No dependency is added; the go.mod record was wrong. |
| 2026-08-05 | 1 | **Epics 5 to 9 are on the tracker.** The decomposition produced 28 sub-issues: #134 to #139, #141 to #146, #148 to #152, #154 to #159, and #161 to #165. Every one of the 163 requirements in features 05 to 09 is cited by at least one sub-issue. Two sub-issues hold an open question and carry `status:needs-feedback`: #155 needs the Tailscale OAuth write scope, and #146 needs the operator to confirm the font source and the licence text before a commit. |
| 2026-08-05 | 1 | Issue #137 built. **The migration does not restore one version 0.9 path: a namespace reaching the host local network.** **FR-access-22** states the exact rule set, and it is `internet` only. The `internet` destination now excludes the five private ranges, which the row above records. That path is the finding `SA-9`, therefore the migration closes it rather than preserving it. |
| 2026-08-05 | 1 | Issue #137. **`backupFile` of `cmd/hydrascale/init.go` moves to `config.BackupFile` and both call sites share it.** `SA-24` records that the previous helper wrote the copy at the fixed mode `0640`, and the configuration file can hold an `auth_key`. The copy now takes the mode of the source file, and the helper writes a temporary file and renames it, so a copy that a previous run left at a wider mode does not survive. This narrows the mode of `config.yaml.bak` on a host whose configuration file is `0600`. |
| 2026-08-05 | 1 | Issue #137. **The edge case "a tailnet is removed while a rule names it" cannot arise in the migration**, because the migration runs only when the file holds no `access` block. `LoadConfig` rejects an `access` block that names an undeclared tailnet, which issue #134 added, and that rejection contradicts the stated behaviour "It does not refuse to start". The correction belongs with the chain writer of issue #135, and this issue does not make it. |
| 2026-08-05 | 1 | Epic 5 built, in part. Issue #138 adds `GET /api/access` and `PUT /api/access` on the control socket, which **FR-access-25** to **FR-access-28** name. `GET /api/status` gains the field `access` with the mode and the count of rules. |
| 2026-08-05 | 1 | Issue #138. **The route builds the compiler topology from the configuration file alone.** The veth device name and the veth address are both functions of the tailnet identifier, therefore the route runs no host command and it needs no running namespace. A tailnet that is not running is still a node of the model, which the console must draw. |
| 2026-08-05 | 1 | Issue #138. **`PUT /api/access` runs the compiler before it writes the file.** `RuleSet.Validate` rejects an unknown tailnet, an unknown mode, and an invalid port, and `Compile` rejects the rest. A rule set that the daemon cannot compile therefore never reaches the configuration file, which the behaviour rule "Validate everything, then write" requires. |
| 2026-08-05 | 1 | Issue #138. **`Reconciler.ApplyAccess` records `access.applied` on the route path, not on the tick.** Issue #135 owns the tick that writes the chain, and it was in flight. The route calls `Reconcile` and then records the event with the count of rules, so the operator sees the record of every rule set that the control API applies. Issue #135 must not add a second record on the tick. |
| 2026-08-05 | 1 | Issue #138. The node field `peers` reads the live daemon of each tailnet, and it reports `0` for a tailnet whose daemon answers with an error. `GET /api/access` therefore runs one status call per tailnet. The console of Epic 7 polls this route, so a later change may need a cached count. |
| 2026-08-05 | 1 | Issue #139. **The daemon keeps the insert at position 1, and it reports a displacement.** The operator decided this on 2026-08-05, and the exact words are: "I guess 1, but we need to make sure we catch if this is an issue." `SA-33` records that the position is not stable, because `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD` each take position 1 after the daemon starts. **FR-access-2** now reads as the insert at position 1, and **FR-access-2a** to **FR-access-2c** add the detection. `CLAUDE.md`, `.claude/rules/access-invariants.md`, and `features/05-reachability-model.md` state the same behaviour. |
| 2026-08-05 | 1 | Issue #139. **The reconciler reads the position of the jump rule with `iptables -S FORWARD` on each tick, and the read replaces the check `iptables -C`.** The read costs the same one command per parent chain, and it carries the position as well as the presence. The reconciler records `access.jump_displaced` for a change of position only, therefore a host that keeps its chain adds no event on the next tick. `GET /api/status` carries the position in the field `access.jump_position`. |
| 2026-08-05 | 1 | Issue #139. **The `observe` journal command of `features/05-reachability-model.md` is wrong, and this issue corrects it.** The feature file and issue #136 name `journalctl -u hydrascale | grep hydrascale-would-deny`, which returns nothing. The `LOG` target writes to the kernel log. Measured on the test host: the unit journal held 0 lines and the kernel log held 165. The correct command is `journalctl -k | grep hydrascale-would-deny`. |
| 2026-08-05 | 1 | Issue #139. **The return rule of issue #135 is part of the design, and the feature file now states it.** The operator confirmed it on 2026-08-05: "Keep the RETURN guard". The daemon filters a forwarded packet that involves a namespace device, and it returns every other forwarded packet, therefore the deny default applies to namespace traffic alone. |
| 2026-08-05 | 1 | Issue #139. **Issue #172 stays open.** That issue covers the reconciler failing to repair a missing `FORWARD` rule of an existing namespace after a rollback across the Epic 5 boundary. Issue #139 covers the jump rule alone: its position, and its absence. |
| 2026-08-05 | 1 | Issue #172. **A rollback across the Epic 5 boundary leaves every namespace with no path.** The project manager restored the version 0.9 binary on the test host on 2026-08-05. The chain `HYDRASCALE-FWD` went with the new binary, and version 0.9 writes its two `FORWARD` rules only when it creates the veth pair, therefore the rules never returned. `sudo ip netns exec ns-havoc ping -c1 -W3 1.1.1.1` reported `1 packets transmitted, 0 received, 100% packet loss`. `systemctl restart hydrascale` repaired nothing, for the same reason. The operator restored the two rules by hand. `.claude/skills/verify-on-phobos/SKILL.md` now holds that step. |
| 2026-08-05 | 1 | Issue #172. **The reconciler now repairs the forward path of a namespace that exists.** Issue #135 moved the two `FORWARD` rules into `HYDRASCALE-FWD`, which the chain writer rewrites from its fingerprint on a tick, and issue #139 rewrites the jump rule. The masquerade rule of the nat table stayed at namespace setup alone, therefore it was the last host rule of a namespace that no tick returned. `namespaces.EnsureForwardPath` writes the rule that the host does not hold, the reconciler runs it for each namespace that exists, and it records the event `access.path_repaired`. The repair costs one `iptables -t nat -C` for each namespace on a tick. Risk R7 names the operator firewall reload that this answers. |
| 2026-08-05 | 1 | Issue #172. **`hydrascale status` still reports `healthy` for a namespace with no path, and the operator decides the answer.** `healthy` states that the `tailscaled` process runs, which stays worth reporting on its own. A second field for reachability needs a measurement on every tick, and both the cost and the meaning of that measurement are a product decision. This round adds no such field. |
| 2026-08-05 | 1 | Issue #178. **The operator decided: measured reachability, probed on each tick.** `GET /api/status` now carries `measured_reachability` for each namespace, with the states `reachable`, `unreachable`, and `not_probed`. `healthy` keeps its meaning, which is that the `tailscaled` process runs. The daemon derives the new field from no rule, because a rule that is present is not a packet that arrives. The measurement runs at the head of the tick, before the repair steps. A tick that repairs the forward path and then measures reports no loss, therefore the operator never learns that a rule went away. The field reports the recovery on the tick after the repair. |
| 2026-08-05 | 1 | Issue #178. **The target is `1.1.1.1`, and `probe_target` overrides it.** `reach.Prober` runs `ip netns exec <ns> ping -n -c 1 -W 1 <target>`. The default sends one packet to a third party for each namespace on each tick, which the operator accepts for this reason: the measurement must cross the rule set that the daemon enforces, and the default rule set permits a public destination alone. `HYDRASCALE-FWD` denies every destination in 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, and 127.0.0.0/8. This round first chose the default gateway of the host as the target, and the test host measured `100% packet loss` from a namespace with a working forward path, therefore the choice was wrong. The probe proves that the namespace reaches the internet through the forward path. It does not prove that the tailnet peers, and it does not prove that a name resolves. An operator who accepts no packet to a third party declares `probe_target` with an address inside a tailnet. |
| 2026-08-05 | 1 | Issue #178. **The probe times out at 250 ms, the whole step at 400 ms, and a timeout reports `unreachable`.** The reconciler probes every namespace at the same time, therefore the step costs one budget for one namespace and for eight. The specification bounds the tick at 1 second, and 400 ms holds inside that bound. A path that carries nothing inside the budget carries nothing that the operator can use, therefore the field reports `unreachable`. `ping` waits 1 second for an answer, which is longer than the timeout, therefore the daemon stops the command and `detail` holds `no answer inside 250ms`. |
| 2026-08-05 | 1 | Issue #178. **A tailnet that the operator stopped reports `not_probed`, and the daemon sends no packet from its namespace.** `detail` holds `the operator stopped this tailnet`. A namespace that does not exist reports `not_probed` as well. A broken path reports `unreachable`, therefore the operator reads the two cases apart. A daemon that made no measurement since it started reports `not_probed` with the reason, so the field is present for every namespace. |
| 2026-08-05 | 1 | Issue #175. **The project does not vendor its dependencies, and `CLAUDE.md` no longer claims that it does.** The operator drops the claim rather than the ignore rule, for three reasons: `go.sum` pins the hash of every module and the Go checksum database verifies each one; no build in this project needs the network to be absent; and a tracked `/vendor` tree makes every dependency change a very large diff. The build reaches the Go module proxy. `/vendor/` stays in `.gitignore`, therefore `go mod vendor` writes an untracked directory on a developer machine, and the `gofmt` filter in `.github/workflows/ci.yml` stays for that machine. |
| 2026-08-05 | 1 | Epic 6 built, in part. Issue #141 adds the console listener and the four controls of the section "The console has no authentication". `internal/ui` holds the embedded console, `internal/api/console.go` holds the listener and the controls, and the configuration gains the `console` block with `enabled` and `bind_address`. A file that holds no `console` key serves `127.0.0.1:9443`, so a version 0.9 file loads without an edit. |
| 2026-08-05 | 1 | Issue #141. **`LoadConfig` refuses a non-loopback `console.bind_address`, so a bad value stops every command and not the daemon alone.** **FR-console-2** names the daemon start. The console has no authentication, therefore a listener on a routable address gives every host on the network control of a root daemon, and a value that the daemon must refuse is not a value that `hydrascale apply` should accept. `api.StartConsole` runs the same check again and logs the refusal, which **FR-console-2** requires. |
| 2026-08-05 | 1 | Issue #141. **The `Origin` check accepts the name `localhost`, although `console.bind_address` refuses it.** The two are different questions. A bind address that is a name resolves to an address that the operator can change, so the daemon binds an address. An `Origin` header arrives after the browser already connected to the loopback listener, so `http://localhost:9443` names this console. A page on another host that resolves to a loopback address still sends its own name, and the check refuses it. |
| 2026-08-05 | 1 | Issue #141. **The daemon records the `console.request` event for a mutating request only.** **FR-console-10** names a mutating request, and the console polls `GET /api/status` every 5 seconds, therefore an event for every request would fill the list of 1000 events in about 80 minutes and hide every real action. The message holds the method and the path alone, because a request body carries an auth key and an event reaches the log file and `GET /api/events`. See `SA-1`. |
| 2026-08-05 | 1 | Issue #141. **A console listener that fails to open stops the daemon.** The operator asked for a console; a daemon that runs without one hides the failure until the operator opens a browser. The error names the port and the key `console.bind_address`, which the edge case of `features/06-console-foundation.md` requires. |
| 2026-08-05 | 1 | Issue #142. **The poll layer keeps polling after a failure and it offers a manual retry as well.** **FR-console-16** names the stale marker, and the edge case names the retry after 60 seconds. A console that stops polling needs an operator action after every daemon restart, therefore the layer keeps the 5-second cadence and adds the retry control after 60 seconds of continuous failure. The retry is a second path to the same request. |
| 2026-08-05 | 1 | Issue #142. **A daemon that answers with another `server_version` raises a reload notice rather than a reload.** `go:embed` places the console in the binary, so a restarted daemon serves other console files than the open page holds. The console states the mismatch and the operator reloads. An automatic reload discards an edit that the operator is part way through. |
| 2026-08-05 | 1 | Issue #142. **A custom property that holds `color-mix` takes no fallback from a second declaration.** A custom property accepts any token sequence, so both declarations parse and the later one wins, and the value fails where the console reads it. `internal/ui/static/tokens.css` therefore restates `--lime-soft`, `--scrim`, and `--ring-focus` inside `@supports not (color: color-mix(...))`. The edge case of `features/06-console-foundation.md` names two values; the brand sets three. |
| 2026-08-05 | 1 | Issue #142. **The console JavaScript tests run under Node, and `internal/ui/package.json` declares the module type.** The file holds a name, `private`, and `type`. It names no dependency, it adds no build step, and no lock file exists. `go:embed` reads `internal/ui/static` alone, so the file does not enter the daemon binary. `TestTheConsoleJavaScriptTestsPass` skips where the host holds no `node`. |
| 2026-08-05 | 1 | Issue #143. **One poll reads three routes.** `GET /api/status` holds neither the peer count, nor the local rule set, nor the event log, and **FR-console-18** to **FR-console-24** need all three. The console therefore reads `/api/status`, `/api/access`, and `/api/events` together on one tick, in `fetchConsoleState`. The console keeps one timer and one cadence, which **FR-console-15** requires, and the Go API gains no field. A failure of any one route fails the poll and marks the state stale. |
| 2026-08-05 | 1 | Issue #143. **The accent of the overview belongs to the paths of the selected node.** **FR-console-40** allows the accent for one thing per view and **FR-console-23** names the selected paths. The add action of the populated state is therefore a plain button. The empty state draws no topology, so the add action takes the accent there. |
| 2026-08-05 | 1 | Issue #143. **The topology reports the measured reachability of a tailnet, not the reconciler state.** Issue #178 added `measured_reachability` to each entry of the `actual` field. A namespace that reports no measurement shows `not probed`, and a state that this console does not know shows `unknown`. The console invents no state. The three words are lowercase, which **FR-console-41** requires. |
| 2026-08-05 | 1 | Issue #142. **The shell holds the one `h1` element and the router writes it.** **FR-console-14** allows one heading of the largest size per view. A view section that held its own `h1` would put two on the page while the router hides one, therefore a view holds `h2` and below. |
| 2026-08-05 | 1 | Issue #144. **The daemon states the removal plan and the console repeats no rule.** The removal dialog of **FR-console-29** names the veth device, which `internal/namespaces/ns.go:220` derives from a SHA-256 sum of the namespace name. A console that computed the name again would hold the rule twice, which is the defect that `SA-3` and `SA-14` record. The route `GET /api/tailnet/{id}/removal-plan` therefore states the namespace, the veth device, the state directory, the rule count, and every command. `namespaces.PlanRemoval` and `RealManager.TeardownVeth` read one rule list. |
| 2026-08-05 | 1 | Issue #144. **The removal runs no `tailscale logout`, so the dialog states the command rather than claims it.** `reconciler.Apply` stops the daemon, deletes the namespace and the veth pair, removes the host access state, and removes the state directory. It logs no node out. The mockup `mockups/02-namespace-detail.html` lists `tailscale logout` among the commands that the removal runs, and that is not what the code does. The dialog names the command as the step that the operator runs first, which agrees with **FR-console-30**. |
| 2026-08-05 | 1 | Issue #144. **The detail route gains `backend_state` and `login_url`.** The state "Not authenticated" of `features/06-console-foundation.md` needs the login address, and no route carried it. `tailscale status --json` already reports `BackendState` and `AuthURL`, therefore `daemon.TailscaleStatus` reads both and `GET /api/tailnet/{id}/detail` states both. The daemon runs no new command. |
| 2026-08-05 | 1 | Issue #144. **The accent belongs to the affirmative action, and the selected row shows the selection with a surface.** The mockup paints both the add button and the selected row with the accent, which is two uses in one view. **FR-console-40** allows one. The console keeps the accent on `Add tailnet` and marks the selection with `--surface-raised` and a `--text-tertiary` border. |
| 2026-08-05 | 1 | Issue #144. **The namespace view asks for the detail of each tailnet on the cadence of the poll layer, and it starts no timer.** `GET /api/status` carries no peer count and no tailnet address, and **FR-console-25** requires both in the list. `GET /api/tailnet/{id}/detail` carries them, so the view asks for one body per tailnet once per poll interval and caches the result. The poll layer stays the one clock of the console. |
| 2026-08-05 | 1 | Issue #144. **`POST /api/tailnet/add` accepts `exit_node` and no code reads it.** `SA-49` records the defect. The add flow shows the field, because **FR-console-31** names it, and the daemon stores the value in the configuration file. The value changes nothing on the host until the defect is fixed. |
| 2026-08-05 | 1 | Issue #144. **The add flow clears the auth key whether the daemon accepts the request or refuses it.** **FR-console-33** states that the console never displays the key after the submit. A flow that kept the key for a retry would hold it in a view that the operator can reach, therefore a refused add asks for the key again. |
| 2026-08-05 | 1 | Issue #145. **The DNS regions belong to the settings view.** **FR-console-12** names five navigation entries and `TestTheNavigationHoldsTheFiveEntriesOfTheShell` enforces the count, and `mockups/05-dns-and-settings.html` draws the resolver, the namespace protection, the host file, and the settings on one page. The settings view therefore holds all four regions that the mockup names. The operator reverses this if the console must hold a sixth entry. |
| 2026-08-05 | 1 | Issue #181. **A gate that holds no `node` now fails rather than skips.** `TestTheConsoleJavaScriptTestsPass` runs the console JavaScript tests of `internal/ui/jstest`, and it skipped when `node` was absent, so a gate that lost `node` reported success with none of those tests run. A developer machine keeps the skip. The environment variable `CI` marks a gate, and GitHub Actions sets it. **`~/gate.sh` on the test host sets no environment variable, so the test host keeps the skip until `gate.sh` exports `CI`.** `CLAUDE.md` now names `node` among the tools that the test suite needs. |
| 2026-08-05 | 1 | Issue #145. **`GET /api/status` gains `config_path`, `socket_path`, and `console_address`.** **FR-console-37** names five values that the settings view shows, and no route reported the first three. A constant in the console would state a path that the daemon does not hold, and **FR-console-17** allows no invented data. `console_address` is an empty string for a daemon that opened no console listener, and the view then states the address that the browser reached. |
| 2026-08-05 | 1 | Issue #145. **`GET /api/dns` gains `allow_unprotected` and `host_resolv_path`.** A failed overlay mount is an error state only when `dns.allow_unprotected` is `false`, which the Epic 4 row above records, therefore a reader cannot state the protection state from the `protected` field alone. The DNS view shows a warning dot for an unprotected namespace that the operator opted out of protection, and a critical dot for one that the operator did not. |
| 2026-08-05 | 1 | Issue #145. **One poll now reads four routes.** The poll layer added `GET /api/dns` to the status route, the access route, and the event route. The console keeps one data source and one cadence, which **FR-console-15** requires, and a failure of any one route marks the whole state stale. |
| 2026-08-05 | 1 | Issue #146. **The console self-hosts both typefaces.** `Space Grotesk` version 2.000 comes from `https://github.com/floriankarsten/space-grotesk` at tag `2.0.0`, commit `7220f5d04813fe83babe76d4fd23e02275021280`, file `fonts/woff2/SpaceGrotesk[wght].woff2`. `Space Mono` version 1.003 comes from `https://github.com/googlefonts/spacemono` at commit `329858c2c4dbd3476f972a4ae00624b018cf4b81`, files `fonts/webfonts/SpaceMono-Regular.woff2` and `fonts/webfonts/SpaceMono-Bold.woff2`. Both families carry the SIL Open Font License version 1.1. The licence text of the two projects is identical after the copyright line, so `internal/ui/static/brand/fonts/OFL.txt` holds both copyright lines and that one licence text. The three files take 117 KiB in the binary. |
| 2026-08-05 | 1 | Issue #191. **The gate of the test host marks itself with `HYDRASCALE_GATE`.** The operator chose option B: `~/gate.sh` exports the marker, and `nodeForConsoleTests` reads it beside `CI`. A dedicated marker carries no meaning for another tool, therefore the gate keeps the behaviour of every other command that it runs. The script is not in this repository, so the operator adds the line by hand. |
| 2026-08-05 | 1 | Issue #148. **The staged state model of the access view holds three things.** `internal/ui/static/access.js` holds the rule set that `GET /api/access` returned, the staged rule set, and the difference between them. The staged count is the size of the difference: the added rules, the removed rules, and the changed rules. `setRules` is the one write into the model. The four issues that follow build the matrix, the flow overview, the rule list and the apply on it. A staged edit opens no request, which **FR-editor-23** requires. The mode control is the one mutating request of this view. It sends the rule set of the daemon with the new mode, therefore a staged rule reaches no host before the operator applies it. |
| 2026-08-05 | 1 | Issue #149. **The diagonal is the only inert square, and the mockup differs.** `mockups/03-acl-editor.html` draws the square from `host` to `internet` as inert. **FR-editor-10** names the diagonal alone, and `internal/access/rules.go:147` rejects a rule where `from` equals `to` and no other pair. The matrix follows the requirement, therefore the operator can allow the path from `host` to `internet`. The matrix draws the 28 pixel square from 12 tailnets, and the 34 pixel square below that count, which the edge case of `features/07-console-access-editor.md` states for a dense host. The accent of this view marks the allowed path: the filled square, and the row label and the column label of the square under the pointer. Both marks come from the mockup. |
| 2026-08-05 | 1 | Issue #150. **The flow overview reuses the renderer of the overview topology.** `internal/ui/static/access.js` builds its picture with `buildTopology` and `topologySVGMarkup` of `internal/ui/static/topology.js`, and it gives them the staged rule set rather than the rule set of the daemon. A staged edit therefore draws its curve at once. `topologySVGMarkup` gains the option `bySource`. The overview marks every path of the selected node, because it answers what one namespace touches. The access view marks the paths that start at the selected source, because **FR-editor-5** names the source. Two consequences of the reuse: the right column holds the host and the internet alone, so a path from one tailnet to another tailnet draws as a curve inside the left column rather than to a second column entry, and **FR-editor-2** is therefore satisfied by the curve and not by a node position; and every node of the picture accepts a selection, so the operator can select the host, which the local rule model treats as a source. The picture takes the accent through the class `edge sel`, which the brand allows as the current selection. The apply action keeps the accent of the header. |
| 2026-08-05 | 1 | Issue #151. **The console repeats the port rule of the daemon, and it adds one separator.** The rule list validates a port entry with the pattern of `internal/access/rules.go:51` and it states the three messages of `parsePort` word for word, therefore the console and `PUT /api/access` refuse the same entry with the same words. One port field carries the whole port list of a rule, and **the comma separates the entries**. The daemon holds a list and states no separator, so the console decides this one. The field removes the space around each entry, it takes an empty field as the port list that allows every port, and one bad entry rejects the whole field. The console stages no part of a field that it refuses, which the validation rule of `CLAUDE.md` requires. The staged row takes the word `staged` as a chip rather than the accent colour, because the accent of this view already marks the allowed path and the apply action. |
| 2026-08-05 | 1 | Issue #152. **FR-editor-28 is not built, because no route of `internal/api` reports the path that the operator connection uses.** `internal/api/console.go:113` reads the header `Origin` and the header `X-Hydrascale-Console` and no other property of the request. No handler reads `r.RemoteAddr`, and `internal/api/types.go` declares no field for a client address or a path. The console listener binds a loopback address alone, which `internal/api/console.go:65` enforces, so the address of the console request is always a loopback address and it names no tailnet. The operator decides between three answers: the daemon reports the path of the operator session, which needs a host command and a change to the daemon; the console warns on a wider condition, such as every staged removal of a path that ends at `host`; or **FR-editor-28** goes. Every other acceptance criterion of the issue is built. The apply sends `PUT /api/access?dry_run=true` first and `PUT /api/access` after it, both with the whole rule set, and a refused apply keeps every staged edit and repeats the message of the daemon word for word. The model gains the rebase: it holds the rule set that the operator staged the edits against, therefore the console can write the staged difference onto a rule set that another console changed. |
| 2026-08-05 | 1 | Epic 8 started. **Issue #154 adds the environment variable override alone, because `internal/secrets` already held the file layer.** `secrets.Load`, `secrets.Save`, `secrets.Tailnet`, and the key `secrets_file` were all on `dev` before the issue started, and `cmd/hydrascale/main.go:430` already called `secrets.Load`. Six of the nine acceptance criteria were therefore already met. The first build of the issue declared a second YAML shape, a second reader and a second writer for the same file, and that second reader called `os.Stat` where `secrets.Load` calls `os.Lstat`. A symbolic link at the secrets path passed the mode check on the strength of the mode of the target. **One file on disk takes one decoder.** `internal/policy` now holds `LoadCredential`, `TailscaleClientIDEnvVar`, `TailscaleClientSecretEnvVar`, and `HeadscaleAPIKeyEnvVar`, and it calls `secrets.Load`. It declares no credential type and it writes no file. |
| 2026-08-05 | 1 | Issue #155. **Risk R1 closes: the Tailscale OAuth scope for a policy write is `policy_file`.** The OpenAPI schema states `OAuth Scope: policy_file.` in the description of `operationId: setPolicyFile`, and `https://tailscale.com/kb/1623/` states that the scope covers `POST /api/v2/tailnet/:tailnet/acl`. That page also states that `devices:posture_attributes` and `devices:core:read` are required with the scope, which issue #161 documents. Both sources were retrieved 2026-08-05. Two further points came from the same reading. The token endpoint is `https://api.tailscale.com/api/v2/oauth/token`, it takes the OAuth 2.0 client credentials grant, and an access token expires after one hour. **The schema names `application/json` for the request body of `POST /tailnet/{tailnet}/acl/validate` and it names no other media type for that endpoint**, where it names `application/json` and `application/hujson` for the read and for the write. `features/08-upstream-policy.md` states that validate "Accepts the same content types", which the schema does not confirm. `TailscaleClient.ValidatePolicy` therefore sends `application/json`. A document that holds a comment may fail there, and issue #159 must measure it against a real tailnet before the console offers the validate action. |
| 2026-08-05 | 1 | Epic 8 built, in part. **Issue #156 adds `policy.HeadscaleClient`, and the file policy mode is a message match rather than a status match.** `hscontrol/grpcv1.go:727` of the Headscale source at tag `v0.29.3` returns `types.ErrPolicyUpdateIsDisabled` when `policy.mode` is not `database`, and `hscontrol/types/policy.go:11` declares that error as `errors.New("update is disabled for modes other than 'database'")`. A plain error carries the gRPC code `Unknown`, therefore the REST bridge answers HTTP 500 and not HTTP 403. **The status alone cannot detect the file policy mode**, so the client matches the message text and returns `ErrHeadscaleFileMode`, which names `policy.mode: "database"`. HTTP 404 returns `ErrHeadscaleNoPolicyRoute`, which names v0.29. The client returns the error of the transport unchanged, so a TLS failure reaches the operator word for word, and `TestTheHeadscaleClientAddsNoCertificateOverride` reads the source and fails on `InsecureSkipVerify`, `tls.Config`, and `RootCAs`. |
| 2026-08-05 | 1 | Issue #157. **The control API serves the five policy routes, and three decisions came with them.** First, **the control server kind comes from the control URL alone**: `config.ResolveControlURL` returns an empty value for a tailnet that declares none, and that tailnet is `tailscale`; every other tailnet is `headscale`. The configuration holds no key for the kind, and the credential block cannot state it, because `GET /api/policy` reports the kind of a tailnet that holds no credential. Second, **`PUT /api/policy/{id}` sends the document to the validate route of the control server before it writes**, because **FR-policy-21** binds the daemon and not the console alone. A document that validate rejects returns HTTP 400 with the answer of the control server, and the write route of the control server receives no request. Third, **the write availability of a Headscale tailnet states the credential state alone**. The control server exposes no route that reports its policy mode, so the daemon learns the file policy mode from the answer to a write, which returns HTTP 409 and names `policy.mode: "database"`. The daemon sends the tailnet name `-` to the Tailscale API, which names the tailnet of the access token; the credential is per tailnet, so the dash always names the right tailnet. The path parameter description of the Tailscale OpenAPI schema states the dash, retrieved 2026-08-05. |
| 2026-08-05 | 1 | Issue #158. **The console holds a sixth navigation entry, and the policy view reads two routes outside the poll layer.** The row of issue #145 above states that the operator reverses the five-entry count if the console must hold a sixth entry. **FR-policy-22** to **FR-policy-28** name a view that no other view holds, therefore `policy` follows `access` in the navigation and `consoleViews` holds six identifiers. `GET /api/policy/{id}` reaches the control server and the control server rate-limits, so the poll layer carries neither policy route. The view reads `GET /api/policy` when the set of declared tailnets changes, and it reads one document per selection. It starts no timer. The credential state of a row and the write availability both come from the answer of the daemon; the console computes neither. The accent of this view marks the selected row alone, because the view draws no affirmative action until issue #159 adds the push. |
| 2026-08-05 | 1 | Issue #158. **FR-console-12 now names six navigation entries, and the poll layer carries no policy route.** `features/06-console-foundation.md:69` named five entries: overview, namespaces, access, activity, and settings. The policy view of **FR-policy-22** is the sixth, and it follows access in the navigation. The console and its feature file therefore agree again. **The poll layer carries no policy route, and that is a deliberate exception to FR-console-15.** `GET /api/policy/{id}` reaches the control server, and the control server rate-limits, so a route on the poll cadence would send one request per tailnet every 5 seconds to Tailscale or to Headscale. The policy view reads `GET /api/policy` when the set of declared tailnets changes, and it reads one document per selection. It starts no timer, therefore the poll layer stays the one clock of the console. The row of issue #145 above states that the operator reverses the five-entry count if the console must hold a sixth entry. **That premise is spent: the console holds a sixth entry.** The DNS regions stay in the settings view, which is the decision of the operator. |
| 2026-08-05 | 1 | Issue #159. **One stage per entry drives the two actions, and the accent of the policy view moves to the push.** The stages are `read`, `validating`, `validated`, `validate-failed`, `pushing`, `pushed`, `conflict`, and `push-failed`. Push is enabled in the stage `validated` and in no other stage, therefore **FR-policy-25** reads from one value rather than from two values that disagree. Every edit returns the entry to `read`, and a validate that returns after an edit changes no stage. The row of issue #158 above gave the accent to the selected row while the view drew no affirmative action. **That premise is spent: the push is the affirmative action**, so the push takes the accent and the selected row takes the border colour that the namespace list uses. The console detects a conflict from the status HTTP 409 and never from a message, which **FR-policy-18** and `internal/policy/tailscale.go:43` state. The re-read replaces the document of the read and it keeps the text of the operator, so that the operator compares. The console repeats no request that failed, because an automatic retry against a rate limit of the control server makes that limit worse. `requestJSON` in `internal/ui/static/app.js` now carries the status on the rejected error, which is the value the daemon owns. **Open point:** `writePolicyError` maps `ErrPolicyConflict`, `ErrNoCredential` and `ErrHeadscaleFileMode` all to HTTP 409, therefore a Headscale file policy mode that first appears at a write reads as a conflict. The console states the message of the daemon word for word in that case, and the re-read is a read that changes nothing. A distinct status for the file policy mode needs a change of the daemon, which this issue holds out of scope. |
| 2026-08-05 | 1 | Issue #162. **`docs/DESIGN.md` states the brand, and `internal/ui/static/brand/tokens/` is the one source of every value.** The document names each of the 100 tokens that the seven token files declare, and it repeats the value of each. It states that a token file wins when the document and the token file disagree. `internal/ui/static/tokens.css` is not a token file: it restates `--lime-soft`, `--scrim`, and `--ring-focus` inside an `@supports not` rule, for a browser that reads no `color-mix` function. The document reads the dash values of an allowed path from `internal/ui/static/app.css:247`, which is a 1.4 pixel stroke and a `stroke-dasharray` of `2 6`. **FR-docs-11** names four drawing rules and the document numbers them: one source at a time, no denied path, no arrowhead, and no edge label. The document also names the three marks under `internal/ui/static/brand/` and the 13 icons under `internal/ui/static/brand/icons/`, which **FR-docs-10** does not require. The marks and the icons carry values that a contributor needs, and no other tracked document holds them. |
| 2026-08-05 | 1 | Issue #212. **The toolchain stays at Go 1.26.5, because `govulncheck` names no standard library advisory.** The measurement on this branch reports 0 called vulnerabilities before any change. The 13 called standard library advisories that the row of issue #122 records were measured against Go 1.26.3, and the move to 1.26.5 in that row cleared them. The claim of 13 is therefore stale and no further toolchain move is needed. `go.mod`, `.github/workflows/ci.yml`, and `.github/workflows/release.yml` all state `1.26.5`. |
| 2026-08-05 | 1 | Issue #212. **`golang.org/x/net` moves from v0.53.0 to v0.56.0, and that raises four modules that the report does not name.** The report named 8 uncalled module advisories: 7 in `golang.org/x/net` and 1 in `golang.org/x/sys`. None is called, so the gate passed at v0.53.0. The operator ships version 1.0 with no named advisory, because a dependency scanner reads the module version and not the call graph. The module graph of `golang.org/x/net@v0.56.0` requires `golang.org/x/sys@v0.46.0`, and `go mod tidy` then raised `golang.org/x/mod` to v0.36.0, `golang.org/x/sync` to v0.21.0, `golang.org/x/text` to v0.38.0, and `golang.org/x/tools` to v0.45.0. Those four are the transitive minimum of a named fix, not a wide upgrade. `govulncheck` now reports 0 vulnerabilities at every level. |
| 2026-08-05 | 1 | Issue #212. **The release workflow pins GoReleaser at v2.17.1, and no other item of the issue needed a change.** `version: latest` made a release depend on the newest GoReleaser at the moment the operator pushes the tag. Issue #73 and issue #93 were already closed: commit `6e6a909` merged the `govulncheck` step into `.github/workflows/ci.yml`, and commit `a0ae980` replaced `archives.format` with `formats` in `.goreleaser.yaml`. The pin is the one open item that this issue adds. |
| 2026-08-05 | 1 | Issue #164. **The accent of the terminal interface lives in `cursorStyle` and in no other style.** `cursorStyle` draws the `▸` mark of the current selection, and `selectedRow` takes the raised surface colour rather than the accent. This satisfies **FR-docs-19** and it keeps the rule that a state never tints a whole row. `internal/tui/styles.go` holds the map `styleRegistry`, which names every style, and the test reads that map rather than the source text. A new style therefore belongs in the map. The `▸` mark now renders outside `selectedRow.Render`, because the reset that ends the accent sequence also ends the background of the row. The `DAEMON` column grows from 9 to 10 characters and the `STATE` column from 9 to 11, because each cell now holds a dot, a space, and the longest state word. The state word `ERROR` becomes `error`, which **FR-docs-20** requires. The footer states the local rule mode and the rule count first, and the whole footer truncates to the line width, because the key hints are wider than 80 characters and the operator must read the mode. A status response that holds no `access` field reports `access unknown  rules unknown`. The test that rejects an emoji allows a fixed set of characters outside ASCII: the four of `docs/DESIGN.md`, the horizontal rule, the warning sign of a stale detail response, the two arrows of a key hint, and the em dash and the arrow of a comment. |
| 2026-08-05 | 1 | Issue #163. **The upgrade flow of `features/09-docs-and-release.md` is not buildable in the order it states, and `docs/UPGRADING.md` corrects the order.** That flow sets `access.mode: observe` at step 3 and has the daemon write the `access` block at step 5. `internal/config/migrate.go:72` returns early when `cfg.Access` is not nil, and `internal/access/migrate.go:9` states that the written set names no mode. An `access` block that holds `mode: observe` alone therefore suppresses the migration, and the daemon writes neither the rule set nor the copy at `<config>.pre-v1.backup`. The document states the start first, then the mode `observe` and `systemctl reload`. It also states a second order, in which the operator writes the `access` block and the preserving rules by hand and accepts that the daemon writes no copy. |
| 2026-08-05 | 1 | Issue #163. **`docs/UPGRADING.md` names version 0.10 beside version 0.9.** `v0.10.0` is the last release before version 1.0, and it carries `Hydrascale_v0.10.0_macos.zip`, `Hydrascale_v0.10.0_windows_amd64.exe`, and `Hydrascale_v0.10.0_linux_amd64.tar.gz`, which are the desktop client. **FR-docs-17** names the version 0.9 release alone, and an operator on version 0.10 needs the same statement. `internal/config/config.go:114` at the tag `v0.10.0` calls `yaml.Unmarshal`, which ignores an unknown key, so version 0.10 loads a version 1.0 file and the rollback step keeps the restore of the copy optional. |
| 2026-08-05 | 1 | Issue #215. **The Headscale policy mode value is `database` and not `db`.** `hscontrol/types/config.go:54` at tag `v0.29.3` declares `PolicyModeDB = "database"` and `PolicyModeFile = "file"`, and the installed configuration file comments that the mode is `"file"` or `"database"`. The help text of `cmd/headscale/cli/policy.go` names `acl.policy_mode` and the value `"db"`, and both are wrong, so this specification cites `hscontrol/types/config.go:54` instead. **A read against the database policy mode that holds no stored policy returns HTTP 500 with the message `loading ACL from database: acl policy not found`**, therefore an empty policy is an error status and not an empty document. `HeadscaleClient.Read` returns an empty document and no error for that answer, because no policy is a state of the control server and not a failure. |
| 2026-08-05 | 1 | Issue #221. **A PID whose `/proc` entry is absent is a stop and not a stale PID file.** `validatePID` returned `false` both for a PID that names another process and for a PID that names no process, therefore `Manager.Stop` reported the message `stale PID <pid> for <id> (process is not tailscaled)` at every service stop. `KillMode=control-group` is the systemd default, so systemd stops each `tailscaled` process at the same moment as the daemon. `readPIDState` now separates the two states with `os.IsNotExist`. The exited state removes the PID file and returns no error; the other-process state keeps the earlier message. |
| 2026-08-05 | 1 | Issue #219. **The upgrade order in `features/09-docs-and-release.md` was not buildable.** That file asked the operator to set `access.mode: observe` before the first start of version 1.0. `internal/config/migrate.go:72` returns early when the configuration file holds an `access` block, therefore that order suppresses the migration, the preserving rule set, and the copy at `<config>.pre-v1.backup`. The operator now starts version 1.0 first, then sets the mode and runs `systemctl reload hydrascale`, which `contrib/hydrascale.service:9` declares as `ExecReload=/bin/kill -HUP $MAINPID`. The file also now states that the migrated rule set holds no rule between two tailnets. Measured on the test host on 2026-08-05: `sudo ip netns exec ns-jbones ping -c2 -W2 10.200.0.86` gave 2 transmitted, 0 received. `docs/UPGRADING.md` already held the correct order from issue #163 and it does not change. |
| 2026-08-05 | 1 | Issue #211. **The console screenshots are scrubbed at the API boundary and not in the rendered page.** A browser drives the live console on the test host through a proxy that rewrites every response of `/api/**` before the console reads it. The tailnet identifiers become `alpha` and `beta`, a peer name becomes `peer-01`, a Tailscale address becomes a `100.64.0.0/10` placeholder, and a MagicDNS suffix becomes `tailnet-a.ts.net`. A rewrite of the rendered text nodes was rejected, because it reaches no title attribute, no value that a later poll writes, and no text that a canvas draws. The test host configuration is unchanged. |
| 2026-08-05 | 1 | Issue #211. **`docs/README.md` names `UPGRADING.md`, which issue #163 wrote.** The first build of the index omitted that row, because the file was absent from the branch and the acceptance criteria of issue #211 require that every relative link resolves. Pull request #218 merged the file into `epic/160-docs-and-release`, and this branch merged that. The index now names `DESIGN.md`, `UPGRADING.md`, `dns-investigation.md`, `security-audit.md`, `specs/`, and `images/`. |
| 2026-08-05 | 1 | Issue #211. **The DNS state of the console is the Settings view, therefore the screenshot is `docs/images/console-settings.png`.** The console holds no view named DNS. `GET /api/dns` feeds the `Resolver` card and the `Namespace protection` card of the Settings view, and the image is named for the view that it shows. |
| 2026-08-05 | 1 | Issue #211. **The Access view is the head image of the README, and `docs/images/console-overview.png` enters no document until issue #226 closes.** The capture of the Overview view holds a console defect: `.ev-kind` at `internal/ui/static/app.css:255` carries `width:112px` and `flex:none` with no overflow behaviour, so the event type of a row draws over the message. The row reads `reconcile_completeapplied 4 actions`. The file stays in `docs/images/`, and the Overview view is captured again after issue #226 closes. |
| 2026-08-05 | 1 | Issue #226. `.ev-kind` in `internal/ui/static/app.css` now carries `min-width:112px` where it carried `width:112px`. A fixed width drew an event type wider than 112 pixels outside its box and over the message beside it. The column now grows to hold the whole type, and the message takes the width that remains. The type is the machine value that the operator scans, therefore the column truncates no type. `internal/ui/jstest/overview.test.mjs` holds the two tests. |
| 2026-08-05 | 1 | Issue #231. **`TestTheConsoleJavaScriptTestsPass` now asserts the console test count, and no document records it.** Three files recorded 70 console JavaScript tests where the run reports 218. Each of the three was a comment, therefore the number drifted and nothing failed. The test now reads the summary line `ℹ pass <n>` of `node --test` and fails when the count falls below the constant `minimumConsoleJavaScriptTests`. The constant is a floor and not an exact count, so a new console test does not fail the build. A changed report format that hides the count fails the run as well. `CLAUDE.md` and `.github/workflows/ci.yml` now name no count. |
| 2026-08-05 | 1 | Issue #223. **The reconciler writes the chains before the per-tailnet actions, and `refresh_dns` waits for no deadline inside a tick.** The shutdown removes both chains and the policy of the `FORWARD` chain is `DROP`, therefore a namespace reached no control server for the whole tick. `tailscaled` never reached the `Running` state, and the 60 second deadline of `RefreshDNSConfig` always expired, once for each tailnet. `internal/reconciler/reconciler.go` now runs `applyAccess` and `ensureForwardPath` in front of `Apply`. `daemon.Manager` carries `RefreshDNSConfigIfReady`, which reads the backend state one time and reports whether it refreshed. The reconciler keeps the action planned until the refresh runs, and it records the event `dns.refresh_waits` once for one wait. |
| 2026-08-05 | 1 | Issue #236. The row of the Recent activity panel of the overview now wraps. `internal/ui/static/overview.js` writes the class `ev ev-stack` on the row, and `.ev-stack` in `internal/ui/static/app.css` carries `flex-wrap:wrap` with `flex:1 0 100%` on the message. The panel is 320 pixels wide, and a time of 58 pixels with the type `access.jump_displaced` of 148 pixels left the message 40 pixels. The message then wrapped to one word for each line, and the row measured 220 pixels. The time and the type now take the first line, and the message takes the whole width of the card below them. The message now measures 270 pixels and two lines, and the row measures 85 pixels. The activity view is wider and keeps the one-line row. `internal/ui/jstest/overview.test.mjs` holds the two tests. |
| 2026-08-05 | 1 | Issue #230. **A console screenshot is captured from a static server and a synthetic API body, and no daemon runs.** The daemon does not build on macOS, therefore the capture serves `internal/ui/static` from a local static server and fulfils every `/api/**` request with a value that the capture itself writes. The image holds no value of a real tailnet by construction. Issue #211 rewrote the answer of a live daemon instead, which needs the test host and a tunnel. The Overview view is the head image of the README again, because issue #226 closed the defect that held it back. The event list of the capture holds `access.jump_displaced`, which is the longest type that the daemon emits, so the image is the proof of that fix. |
| 2026-08-05 | 1 | Issue #238. **The tail of the mode `observe` now accepts, where it returned before.** The project manager measured the loss on the test host with `v1.0.0-rc.1`: `sudo ip netns exec ns-jbones ping -c2 -W2 10.200.0.86` returned `2 packets transmitted, 0 received`, the kernel wrote the `hydrascale-would-deny` line for that packet, and the policy counter of `FORWARD` rose from `18877` to `18880` for three packets. `RETURN` gives the packet back to `FORWARD`, whose policy is `DROP` on a host that runs Docker, therefore the host dropped what the daemon promised to keep. **FR-access-4** and **FR-access-20** now name `ACCEPT`, and **FR-access-4a** and **FR-access-20a** state the same for `HYDRASCALE-OUT` and for the policy of `INPUT`. `TailForMode` serves both chains, so both tails accept; an `ACCEPT` on the forward tail alone would leave the same defect on a host whose `INPUT` policy is `DROP`. The `ACCEPT` ends the traversal of `FORWARD`, therefore `ts-forward`, `DOCKER-USER` and `DOCKER-FORWARD` do not see an accepted packet. Version 0.9 wrote `ACCEPT` rules into `FORWARD` and had the same behaviour, so the mode `observe` restores version 0.9 rather than inventing a behaviour. The mode `enforce` is the default and it does not change. |
| 2026-08-05 | 1 | Issue #240. **The operator documents now match the daemon.** `docs/UPGRADING.md` gains a rollback warning and three steps that write the two `FORWARD` rules of each namespace again, because version 0.9 and version 0.10 write those rules at the moment they create the veth pair and a rollback keeps the namespaces. Issue #172 measured the loss, and `hydrascale status` reported `healthy` while no namespace reached anything. Both documents now name about 20 seconds as the wait after a restart, and 60 seconds as the point of a real failure, which issue #223 measured as 17 seconds once and 21 to 22 seconds across eight further restarts. **The rollback section carries no second count**, because a rollback starts version 0.10, which holds the serial `refresh_dns` wait that issue #223 removed, and this project measured version 0.10 before that fix and not after it. Step 6 of the rollback waits for `healthy` and `running` rather than for a fixed count of seconds. The `README.md` no longer tells the operator to write `access.mode: observe` alone, because `internal/config/migrate.go:72` returns early when `cfg.Access != nil` and a hand-written block suppresses the migration. The `README.md` now states the current path to the Tailscale OAuth client, `https://login.tailscale.com/admin/settings/trust-credentials`, and it states that the tail of the mode `observe` accepts. The Docker section of the `README.md` described the `FORWARD` `ACCEPT` rule per namespace that version 1.0 removes, which contradicted `docs/UPGRADING.md`, so it now names the jump into `HYDRASCALE-FWD`. |
| 2026-08-05 | 1 | Issue #243. **The three GitHub Actions move to the majors that declare `using: node24`.** `.github/workflows/ci.yml` and `.github/workflows/release.yml` now hold `actions/checkout@v7`, `actions/setup-go@v7`, and `goreleaser/goreleaser-action@v7`. The earlier majors `actions/checkout@v4`, `actions/setup-go@v5`, and `goreleaser/goreleaser-action@v6` each declare `using: node20` in their own `action.yml`, which is the field that produced the deprecation annotation of the `v1.0.0` release run. The version stays a major tag, because the repository already pinned a major tag and a patch tag adds an update for each patch. The GoReleaser command line tool keeps the pin `version: v2.17.1`, because this change moves the action alone. **The release workflow runs on a tag alone, therefore this change proves `actions/checkout` and `actions/setup-go` through `ci.yml` and it does not prove `goreleaser/goreleaser-action@v7`.** The next release proves that action. The `action.yml` of `goreleaser/goreleaser-action@v7` declares the inputs `distribution`, `version`, and `args`, which the release workflow supplies, and the release notes of `v7.0.0` name the Node.js 24 move as the one breaking change. |
| 2026-08-10 | 1 | Epic 10 built in part. The dependency of issue #249 on issue #248 is removed. FR-skills-10 to FR-skills-15 name no `hydrascale env` behaviour, so the skill `tailnet-exec` needs only the correction of `hydrascale switch` that issue #247 makes. |
| 2026-08-10 | 1 | Epic 10 built in part. **The batch ships five of six issues.** Issue #248 holds at `status:blocked`, because one of its acceptance criteria needs a live tailnet and the test host at `192.168.1.221` does not answer. The five members stand alone, therefore the batch ships without it. The defect that #248 records stays in the product until that issue closes. |
| 2026-08-10 | 1 | Epic 10 built in part. The gate for this batch cross-compiles each test binary to `linux/arm64` and runs it on a Linux host, because `internal/daemon/daemon.go:244` names `Pdeathsig` and six packages therefore fail to build on macOS. `go test -race ./...` does not cross-compile, so the batch pull request on `ubuntu-latest` is the only run of the race detector. |
| 2026-08-10 | 1 | Epic 10 corrected. **FR-skills-6 pinned a form that does not work.** The function `hstn` ran `hydrascale exec <id> -- "$@"`. That command calls `ip netns exec`, which needs root, so the function failed for an account that holds no root permission. The deferred criterion of issue #248 caught it on the test host: "`eval "$(hydrascale env <id>)"` followed by `hstn curl http://<peer>:8080` reaches the peer through the namespace of `<id>`." FR-skills-6 now names `sudo hydrascale exec <id> -- "$@"`, and FR-skills-36 requires the printed comment to state that the command needs root. A test that reads printed bytes cannot observe a permission failure, therefore issue #263 runs the function against a live peer. |
| 2026-08-10 | 1 | Epic 10 corrected. **`hydrascale switch` is the fourth place that omitted the root privilege, and the last one.** The command printed `hydrascale exec <id> -- <command>` and `hydrascale tailscale <id> -- <arguments>`. `runTailscaleInNamespace` at `cmd/hydrascale/main.go:616` calls `runInNamespace`, which runs `ip netns exec`, so both forms need root. Issue #261 corrected the cheat sheet of `hydrascale init`, issue #263 corrected the function `hstn` of `hydrascale env`, and issue #265 corrected the `README.md`. Issue #267 corrects the last one. **FR-skills-2** now names the two `sudo` forms, and the new **FR-skills-37** requires the printed line `Both forms run ip netns exec, which needs root.` The four corrections came one at a time, because each reader found one printed form and no reader read every printed form together. |
| 2026-08-10 | 1 | Epic 10 built. **One omission appeared in four places.** `hydrascale exec` runs `ip netns exec`, which needs root, and only `skills/tailnet-exec/SKILL.md` stated it. Issue #261 corrected the cheat sheet of `hydrascale init`, issue #263 corrected `hydrascale env`, issue #265 corrected `README.md`, and issue #267 corrected `hydrascale switch`. The specification propagated the defect, because FR-skills-2 and FR-skills-6 each pinned a form that holds no `sudo`. A test that reads printed bytes passes on a form that fails on the host, therefore the test host caught this and the test suite did not. |
| 2026-08-13 | 1 | Issue #272. **`SA-48` measures the value that the configuration requires, and no longer the value `0`.** A test on the host found that host access and the unified resolver both failed, because no code writes `net.ipv4.ip_forward` inside a namespace and a new namespace holds `0`. `README.md` promises `host_access: true # the host reaches this tailnet`, and the host reached no peer: `ping -c2 100.83.43.65` from the host returned `100% packet loss` while the same command inside `ns-havoc` returned `0% packet loss`. Every query for a tailnet name timed out. The operator decided on 2026-08-13 that `SetupHostAccess` writes the value `1` and `TeardownHostAccess` writes the value `0`, therefore forwarding follows `host_access` and an isolated tailnet keeps `0`. The reconciler now records `access.namespace_forwarding` when the value differs from the value that the configuration requires. The deny happens in `HYDRASCALE-FWD` on the host, therefore the containment does not depend on this value: with the value `1` in both namespaces, `ns-jbones` reached neither `10.200.0.86`, nor a peer of `havoc`, nor `192.168.1.1`. |
| 2026-08-13 | 1 | Issue #276. **A credential that the control server refuses now holds its own state.** On 2026-08-13 an operator wrote a Tailscale device authentication key, whose prefix is `tskey-auth`, into the field `tailscale_oauth_client_secret`, whose value carries the prefix `tskey-client`. `GET /api/policy` answered `credential_present: true` and `write_available: true`, and the policy view drew `read and write`. The control server answered the token request with HTTP 401 and the message `API token invalid`, which names neither the mistake nor the value, therefore the mistake reached the operator through a failed policy read alone. **FR-policy-5a** and **FR-policy-5b** add a third state. The daemon reads the prefix of the client secret before any request, and it records the answer of a control server that refuses the credential with HTTP 401. A 403 states that the credential is valid and that its scopes do not cover the request, therefore it marks no credential. |
| 2026-08-23 | 1 | Issue #294 resolved. **The toolchain moves to Go 1.26.6.** `govulncheck` named five called standard library advisories on `dev` — GO-2026-6218, GO-2026-6090, GO-2026-6089, GO-2026-5972, and GO-2026-5026 — that a project-review run found while merging an unrelated pull request. Each names `go1.26.6` as its fix. `go.mod`, `.github/workflows/ci.yml`, and `.github/workflows/release.yml` now hold `1.26.6`, which closes the gap and clears every advisory. |
| 2026-08-23 | 1 | **A `/spec-update` round adds version 1.1: the visual policy editor.** `features/08-upstream-policy.md`'s Out of scope explicitly deferred a visual editor for the policy document; the operator asked for one, scoped to a full model of Tailscale's ACL grammar (groups, hosts, tagOwners, ipsets, acls, grants, ssh, autoApprovers, nodeAttrs, postures, tests) with byte-for-byte preservation of every part an edit does not touch, for both Tailscale and Headscale. That scope does not fit one epic under the sizing rule of 3 to 6 sub-issues, so it splits into three: Epic 11 (`features/11-policy-document-model.md`), the document model that makes the round-trip fidelity possible; Epic 12 (`features/12-visual-acl-editor.md`), the matrix-and-rule-list editor for tags, groups, and reachability rules, reusing `features/07-console-access-editor.md`'s visual grammar; Epic 13 (`features/13-visual-policy-advanced.md`), the five remaining constructs. `features/08-upstream-policy.md` itself is unchanged — it still describes the shipped text editor, which the new features extend rather than replace. Confirmed against `https://tailscale.com/kb/1337/acl-syntax` (retrieved 2026-08-23) for the field shapes, and against `docs/ref/policy.md` at Headscale tag `v0.29.3` (retrieved 2026-08-23) for the `postures`/`ipsets` gap that R10 records. R9 records the round-trip fidelity decision as a risk, not a settled fact. |
| 2026-08-23 | 2 | **Approved.** The operator reviewed the first draft's two new mockups and found a layout defect: the reachability matrix, sized to fill a 300px grid column, overflowed its card and bled onto the adjacent Rules card. Both mockups moved to a single stacked column with fixed-size matrix cells in a scrollable wrapper, and the example data's self-contradiction about which tailnet is Headscale (`jbones`, used as Tailscale everywhere else in the package) was corrected to name `corp-prod` instead. The operator approved the revised round. |
| 2026-08-23 | 2 | Issue #309. **The document model adds an eighth direct dependency.** `features/11-policy-document-model.md`'s **FR-model-1** requires the daemon to preserve every comment, every trailing comma, and the exact byte range of every value. A marshal-and-unmarshal round trip through `encoding/json` cannot hold that guarantee, because it discards a comment and it does not track a byte range. `github.com/tailscale/hujson` parses a policy document into an AST that keeps both, and its `Pack` method serializes the AST back to the exact input bytes when no edit changed it. `CLAUDE.md` now states eight direct dependencies. |
| 2026-08-23 | 2 | **`features/12-visual-acl-editor.md`'s Interfaces section was wrong.** It stated
that the visual editor adds no new route. Planning Epic 12 found that browser
JavaScript cannot parse or edit huJSON with the byte-preservation guarantee
`features/11-policy-document-model.md` built — only the daemon's Go document model can.
Two new routes were added: `POST /api/policy/{id}/sections` (parses the staged
document, returns every named section) and `POST /api/policy/{id}/sections/edit`
(applies one add/replace/remove to a section, returns the new document text). Both are
stateless, matching the document model's own Data touched section. Caught before any
Epic 12 code was written. |
| 2026-08-23 | 2 | Issue #322. **`autoApprovers` needed a new addressing scheme on the existing `POST /api/policy/{id}/sections/edit` route.** `internal/policy/document.go`'s `AddMapEntry`/`AddEntry` family operates on a top-level array or a top-level map alone, and `autoApprovers` is neither: it is a top-level object holding a map (`routes`) and a single field (`exitNode`). `Document` gains `AddAutoApproverRoute`, `ReplaceAutoApproverRoute`, `RemoveAutoApproverRoute`, and `SetAutoApproverExitNode`, and the route gains two `section` values, `autoApprovers.routes` (keyed by CIDR) and `autoApprovers.exitNode` (no key; `replace` alone). `features/13-visual-policy-advanced.md`'s Interfaces section records the addressing scheme. |
| 2026-08-23 | 2 | **Version 1.1 is built.** Epics 11, 12, and 13 all merged into `dev`. The
operator edits a policy document's tags, groups, reachability rules, SSH access,
auto-approvers, node attributes, postures, and tests by drawing them, with the same
byte-for-byte preservation guarantee the text editor already held. Three findings
during the build, each closed before it reached a later issue: Epic 12's feature file
wrongly said no new route was needed; Epic 11's edit methods only handled array-shaped
sections, not the map-shaped ones Epic 12 needed; `autoApprovers`' nested shape needed
its own addressing scheme, which Epic 13 added. `features/11-policy-document-model.md`,
`features/12-visual-acl-editor.md`, and `features/13-visual-policy-advanced.md` all
advance to `status: built`. |
| 2026-08-23 | 2 | **Correction: this work is version 1.2, not version 1.1.** The `/spec-update` round and the Epic 11-13 build both called it version 1.1, and the tag names `v1.1.0` and `v1.1.1` already belong to the console fixes and credential-state work that shipped between Epic 9 and Epic 10, before this round started. The Feature map and Milestone M7 now name it version 1.2. |
| 2026-08-24 | 2 | **Design decision: the console caps a list it shares across views, not the daemon's route.** Issue #355 found the Activity view rendering every event the daemon ever recorded, unbounded. `GET /api/events` already caps its own log at 1000 entries (`internal/reconciler/reconciler.go`), and three other views (`namespaces.js`, `overview.js`, `topology.js`) read that same list for their own, smaller purposes. Capping the route itself would take history from every one of them. The fix caps the Activity view's render alone, at 100 rows, and states the count of what it draws against the count the log holds. A future view that reads `GET /api/events` inherits the full list and caps its own render the same way; it does not ask the route for a shorter list. |
| 2026-08-24 | 2 | **Project review 2026-08-23-v1.2 found version 1.2 not ready to tag.** 13 defects filed (issues #345-357). Batch #360 (issues #345, #356, #357) shipped as a ship-partial merge: the fourth member, #353 ("a failing staged test blocks Push"), stays open because the Tailscale control server itself refuses to write a document whose test fails, so **FR-vadv-14 as written ("a failing test does not block Push") cannot be satisfied by the console alone** — the upstream write route decides first. The maintainer has not yet chosen between keeping FR-vadv-14 literally (Push proceeds, the operator reads the control server's own refusal) or correcting it to match the upstream limit (Push stays disabled, the console states the true reason, following the FR-vadv-11 precedent). See issue #353 for the open question. |
| 2026-08-24 | 2 | Issue #371. **`features/12-visual-acl-editor.md`'s FR-vacl-7 described the wrong rule for an inert square.** It stated that "the diagonal accepts no click". The diagonal names the square position, and the square from the wildcard `*` to the wildcard is a diagonal square. Issue #349 established that this square names the path from every source to every destination, therefore the operator adds and removes it like any other rule. `internal/ui/static/policy.js:1458` holds the true rule: a square is inert when both axes name the same value and that value is not the wildcard. FR-vacl-7 now states that a square is inert when it names one identity on both axes. It also states that the wildcard names every identity and not one identity, therefore the wildcard square accepts a click. |
| 2026-08-24 | 2 | Issue #351. **Decision: the matrix draws every named identity, and not the referenced identity alone.** FR-vacl-7 derived the axes from the `src` and the `dst` of an existing rule, so a group, a tag owner, or a host alias that the operator had just staged reached no axis. FR-vacl-8 makes a click on a square the documented way to add a rule, therefore an identity with no rule yet had no path to its first rule, and the operator had to edit the Text view. The operator chose to draw every named identity over the second option, a free-text source and destination picker in the rule list. An `ipsets` key names no axis: a rule names an IP set as `ipset:<name>`, and a Headscale control server supports no `ipsets` section, per FR-vacl-19. Confirmed against `https://tailscale.com/kb/1337/acl-syntax` (retrieved 2026-08-24). |

## Issue map

Every epic of version 1.0 is on the tracker. Epics 5 to 9 were filed on 2026-08-05, after
the security audit closed, so that each decomposition carries the audit findings it must
fix.

| Epic | Issue | Sub-issues | Feature file |
|---|---|---|---|
| Epic 0: Foundation | #48 | #49 #50 #51 #52 #53 #54 | `features/00-foundation.md` |
| Epic 1: Desktop client removal and repository hygiene | #55 | #56 #57 #58 #59 #60 #61 | `features/01-desktop-client-removal.md` |
| Epic 2: Security audit | #62 | #63 #64 #65 #66 #67 | `features/02-security-audit.md` |
| Epic 3: Security fixes | #68 | #69 #70 #71 #72 #73 | `features/03-security-fixes.md` |
| Epic 4: DNS integrity | #74 | #75 #76 #77 #78 #79 | `features/04-dns-integrity.md` |
| Epic 5: Local reachability model | #133 | #134 #135 #136 #137 #138 #139 | `features/05-reachability-model.md` |
| Epic 6: Console foundation | #140 | #141 #142 #143 #144 #145 #146 | `features/06-console-foundation.md` |
| Epic 7: Console access editor | #147 | #148 #149 #150 #151 #152 | `features/07-console-access-editor.md` |
| Epic 8: Upstream policy control | #153 | #154 #155 #156 #157 #158 #159 | `features/08-upstream-policy.md` |
| Epic 9: Documentation and release | #160 | #161 #162 #163 #164 #165 | `features/09-docs-and-release.md` |
| Epic 10: Agent skills | #246 | #247 #248 #249 #250 #251 #252 | `features/10-agent-skills.md` |
| Epic 10 follow-up: the root privilege | none | #261 #263 #265 #267 | `features/10-agent-skills.md` |
| Epic 11: Policy document model | #308 | #309 #310 #311 #312 | `features/11-policy-document-model.md` |
| Epic 12: Visual ACL editor | #313 | #314 #315 #316 #317 #318 #319 | `features/12-visual-acl-editor.md` |
| Epic 13: Visual policy editor — SSH and advanced constructs | #320 | #321 #322 #323 #324 #325 | `features/13-visual-policy-advanced.md` |

Epics 11 to 13 were filed on 2026-08-23, after the `/spec-update` round that added version
1.1 was approved.

### Requirement coverage

| Feature | Requirements | Issues that cover them |
|---|---|---|
| foundation | FR-foundation-1 to 12 | #49 #50 #51 #52 #53 #54 |
| desktop-client-removal | FR-removal-1 to 14 | #56 #57 #58 #59 #60 #61 |
| security-audit | FR-audit-1 to 14 | #63 #64 #65 #66 #67 |
| security-fixes | FR-fix-1 to 19 | #69 #70 #71 #72 #73 |
| dns-integrity | FR-dns-1 to 16 | #75 #76 #77 #78 #79 |
| reachability-model | FR-access-1 to 28 | #134 #135 #136 #137 #138 #139 |
| console-foundation | FR-console-1 to 47 | #141 #142 #143 #144 #145 #146 |
| console-access-editor | FR-editor-1 to 33 | #148 #149 #150 #151 #152 |
| upstream-policy | FR-policy-1 to 28 | #154 #155 #156 #157 #158 #159 |
| docs-and-release | FR-docs-1 to 27 | #161 #162 #163 #164 #165 |
| agent-skills | FR-skills-1 to 4 | #247 |
| agent-skills | FR-skills-5 to 7 | #248 |
| agent-skills | FR-skills-8 to 15, 20 | #249 |
| agent-skills | FR-skills-8, 9, 16 to 20 | #250 |
| agent-skills | FR-skills-21 to 29 | #251 |
| agent-skills | FR-skills-30 to 35 | #252 |
| agent-skills | FR-skills-6, 36 | #263 |
| agent-skills | FR-skills-2, 37 | #267 |
| policy-document-model | FR-model-1 to 4 | #309 |
| policy-document-model | FR-model-5, 6 | #310 |
| policy-document-model | FR-model-7 to 9 | #311 |
| policy-document-model | FR-model-10, 11 | #312 |
| visual-acl-editor | FR-vacl-1 to 3 | #314 |
| visual-acl-editor | FR-vacl-4 to 6 | #315 |
| visual-acl-editor | FR-vacl-7 to 9 | #316 |
| visual-acl-editor | FR-vacl-10 to 12 | #317 |
| visual-acl-editor | FR-vacl-13 to 16 | #318 |
| visual-acl-editor | FR-vacl-17 to 19 | #319 |
| visual-policy-advanced | FR-vadv-1 to 5 | #321 |
| visual-policy-advanced | FR-vadv-6, 7 | #322 |
| visual-policy-advanced | FR-vadv-8, 9 | #323 |
| visual-policy-advanced | FR-vadv-10, 11 | #324 |
| visual-policy-advanced | FR-vadv-12 to 14 | #325 |

Every one of the 239 requirements in these ten features is cited by at least one issue.
Every one of the 44 requirements across `policy-document-model`, `visual-acl-editor`,
and `visual-policy-advanced` is cited by exactly one issue.

**FR-access-2 is covered and reversed.** The requirement states that the daemon appends the
jump rule into `FORWARD`. The operator decided on 2026-08-05 that the daemon keeps the
insert at position 1. Issue #139 edits the requirement text, corrects `CLAUDE.md`, and adds
the detection that the decision needs.

### Requirements already satisfied when the issues were created

| Requirement | State |
|---|---|
| FR-foundation-1 | The `dev` branch exists at `89745a9`. The maintainer cut it by hand. |
| FR-foundation-11, FR-foundation-12 | Written as `.claude/skills/verify-on-phobos/SKILL.md`. Issue #54 proves them on the test host. |
| FR-removal-5, FR-removal-11 | `.gitignore` already ignores `/hydrascale` and `branding/`. The files are still tracked, so the removal work remains. |
