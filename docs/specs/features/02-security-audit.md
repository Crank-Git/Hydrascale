---
id: security-audit
feature: Security audit
epic: "Epic 2: Security audit"
status: built
issues: [62, 63, 64, 65, 66, 67]
mockups: []
---

## Purpose

The operator believes the daemon has several small authorization defects. The planning
survey found four candidates. A guess is not a plan. This feature set produces a written
list of findings, so that Epic 3 fixes facts.

The audit reads code. It does not change code. It ends with the operator choosing which
findings Epic 3 fixes.

## What the survey already found

These four are inputs to the audit, not its output. The audit confirms each one and
assigns a severity.

| Candidate | Evidence | Why it matters |
|---|---|---|
| The host `FORWARD` chain accepts every packet from a namespace. | `internal/namespaces/ns.go:263` inserts `iptables -I FORWARD 1 -i <hostVeth> -j ACCEPT`. There is no destination match and no output interface match. | A process in one namespace reaches another namespace and the host network. The rule sits above an operator firewall rule because it is inserted at position 1. |
| Teardown is best effort. | `internal/namespaces/ns.go:297` ignores the result of every delete. | A removed namespace can leave an `ACCEPT` rule and a `MASQUERADE` rule on the host. |
| The overlay mount failure is silent. | `cmd/hydrascale/nsdaemon.go:56` prints to standard error and continues. | The host `/etc/resolv.conf` file loses its protection and nothing reports it. |
| The control API has no authentication. | `internal/api/server.go` registers eight routes and none checks an identity. | Membership of `socket_group` is equivalent to root, and the documentation does not say so. |

## User stories

- As the operator, I want a written list of what is wrong, so that I fix the real
  defects rather than the ones I remember.
- As the operator, I want each finding to have a `file:line` reference, so that a fix is
  unambiguous.
- As the operator, I want to choose which findings get fixed in this release, so that the
  release ships.

## Functional requirements

- **FR-audit-1** — The audit produces `docs/security-audit.md`.
- **FR-audit-2** — Each finding has an identifier of the form `SA-<n>`.
- **FR-audit-3** — Each finding has a severity of `high`, `medium`, or `low`.
- **FR-audit-4** — Each finding has at least one `file:line` reference.
- **FR-audit-5** — Each finding states the condition under which it causes harm.
- **FR-audit-6** — Each finding states whether the audit reproduced it on the test host.
- **FR-audit-7** — The audit covers every route that `internal/api/server.go` registers.
- **FR-audit-8** — The audit covers every `os/exec` call site in `cmd/` and `internal/`.
- **FR-audit-9** — The audit covers every file mode and every socket mode that the daemon
  sets.
- **FR-audit-10** — The audit covers every path that the daemon builds from configuration
  input or from request input.
- **FR-audit-11** — The audit covers the teardown path of every resource that the daemon
  creates.
- **FR-audit-12** — The audit records the console threat model as a finding, so that the
  accepted risk is written down.
- **FR-audit-13** — The audit records whether an IPv6 gap exists in the firewall rules.
- **FR-audit-14** — The audit states, for each finding, whether Epic 3 fixes it.

## User flows

### The audit runs

1. The engineer lists every control API route and its handler.
2. The engineer reads each handler and records what it validates.
3. The engineer lists every `os/exec` call and records where each argument comes from.
4. The engineer lists every `os.Chmod`, `os.Chown`, `os.WriteFile`, and `syscall.Umask`
   call and records the resulting mode.
5. The engineer lists every resource that setup creates and checks that teardown removes
   it.
6. The engineer reproduces each candidate finding on the test host where reproduction is
   possible.
7. The engineer writes `docs/security-audit.md`.
8. The operator reads the file and marks each finding as accepted or deferred.

### The reachability finding is reproduced

1. The engineer starts the daemon on the test host with two tailnets.
2. The engineer records the veth address of each namespace from `ip addr`.
3. The engineer runs `ip netns exec ns-<a> ping -c1 <veth address of b>`.
4. The engineer runs `ip netns exec ns-<a> nc -vz <a host on the host local network> 22`.
5. The engineer records the result of each command verbatim in the finding.

## Screens & states

This feature set has no screen.

## Behaviour rules

- The audit reads code and runs read-only commands. It changes no source file other than
  `docs/security-audit.md`.
- A finding that the audit cannot reproduce says so. It does not claim a reproduction it
  did not perform.
- The audit quotes command output verbatim. It does not paraphrase evidence.
- The audit does not rank a finding by how easy the fix is. Severity describes harm.

## Data touched

No entity changes. The audit adds one document.

## Interfaces

The audit inspects these routes, which `internal/api/server.go:47-59` registers:

| Method and path | Handler | Mutating |
|---|---|---|
| `/api/status` | `handleStatus` | no |
| `/api/events` | `handleEvents` | no |
| `/api/reconcile` | `handleReconcile` | yes |
| `/api/tailnet/add` | `handleTailnetAdd` | yes |
| `/api/tailnet/remove` | `handleTailnetRemove` | yes |
| `/api/tailnet/connect` | `handleTailnetConnect` | yes |
| `/api/tailnet/disconnect` | `handleTailnetDisconnect` | yes |
| `/api/config/dns` | `handleConfigDNS` | yes |
| `/api/config` | `handleConfig` | yes |
| `GET /api/tailnet/{id}/detail` | `handleTailnetDetail` | no |

## Edge cases & failures

| Case | Behaviour |
|---|---|
| A candidate finding does not reproduce. | The finding records the attempt, the command, and the output, and it lowers its severity to reflect the uncertainty. |
| The audit finds a defect that is not a security defect. | The audit records it in a separate `Other defects` section. Epic 3 does not have to fix it. |
| The audit finds a high-severity defect that needs a large change. | The audit records it. The operator decides whether Epic 3 fixes it or whether it becomes its own epic. |

## Acceptance criteria

- [ ] `docs/security-audit.md` exists and is tracked.
- [ ] Every finding has an identifier, a severity, and a `file:line` reference.
- [ ] The audit lists all ten control API routes and states what each validates.
- [ ] The audit lists every `os/exec` call site in `cmd/` and `internal/`.
- [ ] The audit states, for the reachability finding, the exact commands it ran on the
      test host and their output.
- [ ] The audit records the console threat model as a finding.
- [ ] The audit states whether an IPv6 firewall gap exists.
- [ ] The operator has marked every finding as accepted or deferred.

## Out of scope

- Any code fix. Epic 3 fixes.
- A dependency vulnerability scan. The project has six direct dependencies and a
  vendored tree; `govulncheck` belongs in Epic 3 as a continuous integration step rather
  than in a manual audit.
- A review of the Tailscale client itself.

## Open questions

None.
