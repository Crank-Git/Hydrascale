---
id: security-fixes
feature: Security fixes
epic: "Epic 3: Security fixes"
status: issued
issues: [68, 69, 70, 71, 72, 73]
mockups: []
---

## Purpose

Epic 2 produces a list of findings. This feature set fixes the ones the operator
accepted. It also fixes three defects that the survey already confirmed, because they do
not depend on the audit result.

The reachability defect is the largest of them. Epic 5 fixes it, because the fix is the
rule engine. This feature set fixes everything else.

## User stories

- As the operator, I want a removed namespace to leave no rule behind, so that the host
  firewall does not accumulate dead rules.
- As the operator, I want to be told that `socket_group` membership equals root, so that
  I choose the group carefully.
- As the operator, I want a failing operation to report a failure, so that I do not
  believe a broken state is a working one.
- As a contributor, I want a test for each fix, so that the defect cannot return.

## Functional requirements

### Teardown

- **FR-fix-1** — `TeardownVeth` returns an error when it cannot remove a rule that it
  created.
- **FR-fix-2** — The reconciler records an event when teardown fails.
- **FR-fix-3** — The daemon removes every iptables rule at start that names a veth device
  that no longer exists.
- **FR-fix-4** — A test proves that teardown removes every rule that setup created.

### The control socket and the socket group

- **FR-fix-5** — `hydrascale init` states, at the `socket_group` prompt, that membership
  of the group gives full control of the daemon.
- **FR-fix-6** — `README.md` states the same in the `socket_group` section.
- **FR-fix-7** — The daemon logs the group name and the resulting socket mode at start.
- **FR-fix-8** — The daemon refuses to start when `socket_group` names a group with
  group id 0, and it logs the reason.

### Input validation

- **FR-fix-9** — Every mutating route validates its request body before it acts on it.
- **FR-fix-10** — A route returns HTTP 400 and the body `{"error": "<message>"}` when
  validation fails.
- **FR-fix-11** — A tailnet identifier matches `^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`.
- **FR-fix-12** — The daemon rejects a control URL that does not use the `https` scheme,
  unless the host is a loopback address.
- **FR-fix-13** — The daemon rejects a path derived from a request that leaves the
  configured state directory.

### Secrets

- **FR-fix-14** — The daemon never writes an auth key, an OAuth secret, or a Headscale
  API key to the log.
- **FR-fix-15** — The control API never returns the contents of the secrets file.
- **FR-fix-16** — The daemon creates the secrets file with mode `0600` and owner root.
- **FR-fix-17** — The daemon refuses to read a secrets file whose mode grants access to
  a group or to other accounts, and it logs the reason.

### Continuous integration

- **FR-fix-18** — The continuous integration workflow runs `govulncheck ./...`.
- **FR-fix-19** — The workflow fails when `govulncheck` reports a vulnerability that
  affects a called function.

## User flows

### A namespace is removed

1. The operator removes a tailnet from the configuration.
2. The reconciler calls the namespace teardown.
3. Teardown removes the iptables rules, the veth pair, and the namespace, in that order.
4. If a step fails, teardown continues with the remaining steps and it collects the
   errors.
5. Teardown returns the collected errors.
6. The reconciler records an event that names each failed step.

### The daemon starts after an unclean stop

1. The daemon reads the configuration.
2. The daemon lists the iptables rules in its own chain.
3. The daemon lists the veth devices that exist.
4. The daemon removes each rule whose veth device does not exist.
5. The daemon logs the count of rules it removed.

## Screens & states

This feature set has no screen. The console shows the resulting events in the activity
view, which `features/06-console-foundation.md` defines.

## Behaviour rules

- A cleanup step that fails does not stop the remaining cleanup steps. Collect the
  errors and return them together.
- A validation rule rejects rather than corrects. The daemon does not silently normalize
  a bad tailnet identifier.
- The daemon logs a refusal with the reason and the offending value, unless the value is
  a secret.
- A change to a mode or an owner is not applied when the target is a symbolic link.

## Data touched

| Entity | Change |
|---|---|
| Event | Two new kinds: `teardown.failed` and `rules.reaped`. |
| Configuration | The new `secrets_file` key, with the default `/etc/hydrascale/secrets.yaml`. |

## Interfaces

No route is added. Every mutating route gains validation. The error body shape is:

```json
{ "error": "tailnet id \"My Net\" is not valid" }
```

## Edge cases & failures

| Case | Behaviour |
|---|---|
| A rule was already removed by an operator. | `iptables -D` returns an error. Teardown treats "rule does not exist" as success. |
| The secrets file does not exist. | The daemon starts. Upstream policy control is unavailable and the console says why. |
| The secrets file has mode `0644`. | The daemon logs a refusal and does not read the file. Upstream policy control is unavailable. |
| `govulncheck` reports a vulnerability in a vendored dependency that no code calls. | The workflow passes. `govulncheck` reports reachability. |
| A group named in `socket_group` does not exist. | The current behaviour already fails at start. Keep it. |

## Acceptance criteria

- [ ] A test proves that `TeardownVeth` returns an error when a rule delete fails.
- [ ] A test proves that setup and teardown leave the recorded command list balanced.
- [ ] A test proves that the daemon removes a rule that names a missing veth device.
- [ ] `hydrascale init` prints the root-equivalence warning before the `socket_group`
      prompt.
- [ ] `README.md` states the root equivalence in the `socket_group` section.
- [ ] The daemon refuses to start with `socket_group: root`.
- [ ] A request with the tailnet identifier `My Net` returns HTTP 400 and a JSON error
      body.
- [ ] A request with the control URL `http://example.com` returns HTTP 400.
- [ ] A request with the control URL `http://127.0.0.1:8080` is accepted.
- [ ] The daemon refuses a secrets file with mode `0644` and logs the reason.
- [ ] No log line in a full start, add, and remove cycle contains a value that begins
      with `tskey-`.
- [ ] The continuous integration workflow runs `govulncheck`.

## Out of scope

- The reachability fix. Epic 5 owns it.
- Console authentication. The operator accepted the risk and the release note states it.
- A rewrite of the control API route shapes. Epic 6 changes what it needs.

## Open questions

- Which findings from `docs/security-audit.md` are in this feature set. The audit answers
  this, and `spec-to-issues` reads the answer from the audit rather than from this file.
