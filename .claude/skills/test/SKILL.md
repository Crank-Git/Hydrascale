---
name: test
description: Run the Hydrascale test suites. Use when asked to test, to check a change, or to find out whether the build is green.
allowed-tools: Bash, Read
---

# Test Hydrascale

Run these from the repository root. They all work on macOS and on Linux, because the
tests never touch the real host network.

## The full gate — what continuous integration runs

```sh
gofmt -l . | grep -v '^vendor/' || true   # must print nothing
go vet ./...
go test -race ./...
```

Run the whole gate before you open a pull request.

## While you work

```sh
go test ./internal/access/...    # one package
go test -run TestCompile ./...   # one test
go test ./internal/ui/...        # the console JavaScript, through the Node harness
```

## When a host-behaviour test fails

The tests replace the command runner, so a failure names the exact command the code ran.
Read the recorded argument list in the failure output before you change the code. A test
that fails with "unscripted command" means the code ran a command the test did not
expect; that is usually the defect, not the test.

## What the tests do not cover

No test writes a real iptables rule, creates a real namespace, or mounts a real overlay.
Verify a change to host behaviour on the test host with the `verify-on-phobos` skill. A
change to `internal/access`, `internal/namespaces`, `internal/routing`,
`internal/hostaccess`, or the DNS overlay is not done until it runs there.
