# Hydrascale

Hydrascale lets one Linux host join several Tailscale tailnets at the same time. The
daemon creates a network namespace per tailnet and runs a separate `tailscaled` inside
each. A reconciler loop drives the live host toward a declared YAML state. A DNS
forwarder resolves names across every tailnet.

The project is building **version 1.0**. Version 1.0 removes the desktop client, adds a
console that the daemon serves on the loopback address, and gives the operator control
over reachability — both the local rules the host enforces and the upstream policy each
control server holds.

Design lives in `docs/specs/`. Read `docs/specs/spec.md` and the relevant
`docs/specs/features/*.md` before you build a feature.

## Stack

- Go 1.26.5, standard library first. Seven direct dependencies, vendored in `/vendor`.
- Bubble Tea and Lip Gloss for the terminal interface.
- The console is a static single-page application with no build step. `go:embed` places
  `internal/ui/static` in the binary.
- The daemon is Linux-only. It does not build on macOS, because it uses `Pdeathsig`.

## Layout

| Path | Holds |
|---|---|
| `cmd/hydrascale` | The command line interface: `init`, `apply`, `serve`, `install`, `tui`. |
| `internal/reconciler` | The control loop. |
| `internal/namespaces` | Namespaces, veth pairs, and the iptables rules that setup writes. |
| `internal/access` | The local rule model and the chain that enforces it. |
| `internal/dns` | The unified resolver. |
| `internal/hostaccess` | Route and hosts-file propagation to the host. |
| `internal/api` | The HTTP server on the control socket and on loopback. |
| `internal/ui` | The console: `static/` plus its Go handlers and tests. |
| `internal/policy` | The Tailscale and Headscale policy clients. |
| `internal/tui` | The terminal interface. |
| `internal/execx` | The command runner interface that tests replace. |

## Commands

```sh
go build ./...                 # build
go test ./...                  # unit and integration tests
go test -race ./...            # what continuous integration runs
go vet ./...                   # vet
gofmt -l .                     # must print nothing
govulncheck ./...              # dependency check
node --version                 # the test suite needs node
```

The test suite needs `node` as well as the Go tools.
`TestTheConsoleJavaScriptTestsPass` in `internal/ui/shell_test.go` runs the 70 console
JavaScript tests of `internal/ui/jstest` with `node --test`. The console has no build step
and `internal/ui/package.json` names no dependency, so `node` alone is enough.

A developer machine that holds no `node` skips these tests. A gate that holds no `node`
fails, because a silent loss of the 70 tests reads exactly like success. The environment
variable `CI` marks a gate.

Run the daemon on the test host rather than on a developer machine. Use the
`verify-on-phobos` skill; it builds, deploys, and rolls back.

## Conventions

### Commands to the host

Never call `exec.Command` directly in `internal/`. Call the `execx.Runner` that the
package holds. A test replaces the runner and asserts the exact argument list. A direct
call is untestable and it is rejected in review.

### iptables

The daemon owns the chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT`, and one jump rule into
each of `FORWARD` and `INPUT`. It writes no other rule. It inserts the jump at position 1,
which the operator decided on 2026-08-05. The position is not stable, because
`ts-forward`, `DOCKER-USER` and `DOCKER-FORWARD` each take position 1 after the daemon
starts. The reconciler therefore reads the position on each tick, records the event
`access.jump_displaced` when the position changes, and writes the jump rule again when the
parent chain holds none. It moves no rule of the operator.

### Errors

Return an error. A best-effort operation that fails silently is a defect. When a cleanup
step fails, continue the remaining steps, collect the errors, and return them together.

### Validation

Every mutating route validates its whole request body before it changes anything. A
failure returns HTTP 400 and the body `{"error": "<message>"}`. Reject a bad value; never
correct it silently.

### Secrets

An auth key, an OAuth client secret, and a Headscale API key never reach the log, never
reach a control API response, and never enter the configuration file. They live in
`/etc/hydrascale/secrets.yaml` at mode `0600`, or in an environment variable.

### The console

The console is dark only, because the brand is dark only. Use the accent colour for one
thing per view: the affirmative action, or the current selection, or an allowed path.
Show a state as a coloured dot and a lowercase word. Render every machine value in the
mono typeface and every sentence in the sans typeface. Draw no denied path — absence is
the denial. Use no emoji. Make no request to another host.

### Commits and branches

Branch from `dev` and open a pull request into `dev`. Never commit to `main`; `main`
equals the last release. Write a commit subject in the imperative, 72 characters or
fewer.

### Writing

Every document, issue, comment, and code comment uses Simplified Technical English.
`.claude/rules/ste.md` holds the standard. The controlled vocabulary is the `## Terms`
table in `docs/specs/spec.md`. Read it before you write a domain word.

### This repository is public

Commit no secret, no private development note, and no planning artifact that is not part
of `docs/specs/`. `scripts/check-hygiene.sh` enforces this and continuous integration
runs it.
