---
id: foundation
feature: Foundation
epic: "Epic 0: Foundation"
status: issued
issues: [48, 49, 50, 51, 52, 53, 54]
mockups: []
---

## Purpose

Hydrascale is an existing project with a working test suite and a continuous integration
workflow. It does not need a full foundation epic. It needs four things that the work
after it depends on.

First, the branch model that the operator chose. Second, a continuous integration
workflow that checks formatting, because a public repository gains contributors. Third, a
command runner interface, because the rule engine in Epic 5 writes iptables rules and no
test can verify that today. Fourth, a repeatable way to verify a change on the test host,
because every change in this release alters host behaviour.

## What already exists

| Item | State |
|---|---|
| Test harness | `go test ./...`, 11 packages, race detector in continuous integration. Keep. |
| Continuous integration | `.github/workflows/ci.yml` runs build, vet, and test. Extend. |
| Release | `.goreleaser.yaml` and `.github/workflows/release.yml`. Epic 1 changes it. |
| Dependency management | 6 direct dependencies. `vendor/` is untracked on purpose; the maintainer rsyncs it to the test host. Keep. |
| Branch model | Only `main` exists. Add `dev`. |
| Command runner | `internal/namespaces` and `internal/routing` call `exec.Command` directly. Replace. |
| Test host procedure | None. Add. |

## User stories

- As a contributor, I want the continuous integration workflow to tell me that my
  formatting is wrong, so that a reviewer does not have to.
- As a contributor, I want to run the tests without root, so that I can work on a laptop.
- As the operator, I want one command that puts a branch build on the test host, so that
  I verify a change the same way every time.
- As the operator, I want one command that returns the test host to the released daemon,
  so that a failed test does not leave the host broken.

## Functional requirements

- **FR-foundation-1** — The repository has a `dev` branch that starts at the current
  `main` commit.
- **FR-foundation-2** — The continuous integration workflow runs on a pull request into
  `dev` and on a pull request into `main`.
- **FR-foundation-3** — The continuous integration workflow fails when `gofmt -l .`
  prints a file name.
- **FR-foundation-4** — The continuous integration workflow runs `go vet ./...`.
- **FR-foundation-5** — The continuous integration workflow runs `go test -race ./...`.
- **FR-foundation-6** — The package `internal/execx` provides a `Runner` interface with
  one method that runs a command and returns its combined output and its error.
- **FR-foundation-7** — `internal/namespaces` calls a `Runner` rather than
  `exec.Command`.
- **FR-foundation-8** — `internal/routing` calls a `Runner` rather than `exec.Command`.
- **FR-foundation-9** — `internal/execx` provides a recording `Runner` for tests. The
  recording `Runner` returns a scripted result for a command and it records every command
  that a test ran.
- **FR-foundation-10** — A test can assert the exact argument list of every command that
  the code under test ran.
- **FR-foundation-11** — The skill `.claude/skills/verify-on-phobos` builds the current
  branch for Linux, copies the binary to the test host, restarts the service, and prints
  the daemon status.
- **FR-foundation-12** — The skill `.claude/skills/verify-on-phobos` accepts a `rollback`
  argument that stops the service and restores the released binary.

## User flows

### A contributor opens a pull request

1. The contributor pushes a branch.
2. The contributor opens a pull request into `dev`.
3. The continuous integration workflow runs format, vet, and test.
4. The workflow reports one status on the pull request.

### The operator verifies a change on the test host

1. The operator runs the `verify-on-phobos` skill.
2. The skill builds the branch with `GOOS=linux GOARCH=amd64 go build ./cmd/hydrascale`.
3. The skill copies the binary to `/usr/local/bin/hydrascale.test` on the test host.
4. The skill stops the service, moves the test binary into place, and starts the service.
5. The skill prints `systemctl status hydrascale` and the last 40 log lines.
6. The operator checks the result.
7. If the change is wrong, the operator runs the skill with `rollback`.

## Screens & states

This feature set has no screen.

## Behaviour rules

- The `Runner` interface takes a context. A command that outlives its context is killed.
- The recording `Runner` fails a test when the code under test runs a command that the
  test did not script. A silent default hides a defect.
- The `verify-on-phobos` skill never runs against a host other than the configured test
  host. The host address is a parameter with a default, not a hard-coded value.
- The skill keeps the previous binary at `/usr/local/bin/hydrascale.prev` so that
  rollback needs no network.

## Data touched

No entity changes. `internal/execx` is a new package with no persistent state.

## Interfaces

```go
// Package execx runs external commands through an interface that a test can replace.
package execx

// Runner runs one external command.
type Runner interface {
    // Run executes name with args and returns the combined output.
    // Run returns an error when the command fails to start or exits non-zero.
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}
```

The existing behaviour uses `exec.CommandContext(ctx, name, args...).CombinedOutput()`.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The test host is unreachable. | The skill stops and prints the SSH error. It changes nothing. |
| The build fails. | The skill stops before it copies anything. |
| The service does not start after the copy. | The skill prints the log and tells the operator to run `rollback`. |
| `gofmt` reports a vendored file. | The workflow excludes `/vendor` from the format check. |

## Acceptance criteria

- [ ] The `dev` branch exists on the remote.
- [ ] A pull request into `dev` triggers the continuous integration workflow.
- [ ] The workflow fails when a source file is not formatted.
- [ ] `go test ./...` passes after `internal/namespaces` uses the `Runner`.
- [ ] A test asserts that `SetupVeth` runs `ip link add` with the expected arguments.
- [ ] A test fails when the code runs an unscripted command.
- [ ] The `verify-on-phobos` skill puts a branch build on the test host and prints the
      service status.
- [ ] The `verify-on-phobos` skill with `rollback` restores the previous binary and the
      daemon starts.

## Out of scope

- A new test framework. The standard library is enough.
- A coverage gate. The project has no coverage baseline and a gate would block work.
- A continuous integration job that runs on the test host. A self-hosted runner is more
  surface than this release needs.

## Open questions

None.
