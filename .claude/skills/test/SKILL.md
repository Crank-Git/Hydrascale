---
name: test
description: Run the Hydrascale test suites. Use when asked to test, to check a change, or to find out whether the build is green.
allowed-tools: Bash, Read
---

# Test Hydrascale

Run these from the repository root. The tests never touch the real host network.

## Linux and macOS

The daemon is Linux-only, because `internal/daemon` uses `Pdeathsig`. On macOS, the
packages below do not build, so their tests do not run:

- `cmd/hydrascale`
- `internal/api`
- `internal/daemon`
- `internal/hostaccess`
- `internal/reconciler`
- `internal/tui`

On macOS, build and vet for Linux instead:

```sh
GOOS=linux go build ./...
GOOS=linux go vet ./...
```

Run the tests of those packages on a Linux host, such as the test host of the
`verify-on-phobos` skill. The other packages build and pass on macOS.

## The full gate — what continuous integration runs

`.github/workflows/ci.yml` runs these steps in this order. Run all of them before you open
a pull request.

1. **Repository hygiene** — the `check-hygiene` skill states each check:
   ```sh
   scripts/check-hygiene.sh
   ```
2. **Build**:
   ```sh
   go build ./...
   ```
3. **Format** — the command must print nothing. A developer machine can hold an untracked
   `vendor/`, so the filter removes it:
   ```sh
   gofmt -l . | grep -v '^vendor/' || true
   ```
4. **Vet**:
   ```sh
   go vet ./...
   ```
5. **Install the documentation site tools** — CI uses Python 3.13:
   ```sh
   pip install -r docs/site/requirements.txt
   ```
6. **Build the documentation site** — `mkdocs build --strict` fails on a warning, such as
   a link to a missing page:
   ```sh
   scripts/docs-build.sh
   ```
7. **Test with race detector**:
   ```sh
   node --version
   python3 --version
   go test -race -v ./...
   ```
8. **Vulnerability check** — the version is pinned:
   ```sh
   go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
   ```

The documentation site builds before the Go tests, because the tests of
`scripts/docs_build_test.go` need `mkdocs` on the path.

## The tools that the suite needs

The Go tests run three other tools:

- `node` — `TestTheConsoleJavaScriptTestsPass` in `internal/ui` runs the console
  JavaScript tests with `node --test`. The console has no build step, so `node` alone is
  enough.
- `python3` — `TestTheSpecificationRendererTestsPass` in `docs/specs` runs the tests of
  the specification renderer.
- `mkdocs` — the tests of `scripts/docs_build_test.go` build the documentation site. Step 5
  installs it.

If a developer machine holds no such tool, the tests that need it skip. If a gate holds no
such tool, those tests fail. The environment variable `CI` or `HYDRASCALE_GATE` marks a
gate. A skipped test reads like a pass, so confirm each tool before you trust a green
result.

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
