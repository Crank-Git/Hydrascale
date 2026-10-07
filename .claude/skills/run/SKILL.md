---
name: run
description: Start the daemon and open the console to see a change working. Use when asked to run the app, launch Hydrascale, screenshot the console, or confirm a change works outside the tests.
argument-hint: "[port]"
allowed-tools: Bash, Read
---

# Run Hydrascale

The daemon needs Linux and root, because it creates network namespaces. There are two
ways to see a change working, and the right one depends on what changed.

## The console alone, on a developer machine

Use this for a console change. It needs no root and no Linux.

The console is static files under `internal/ui/static`. Serve them:

```sh
cd internal/ui/static && python3 -m http.server 9443
```

Open `http://127.0.0.1:9443`. The console shows its error state, because no daemon
answers `/api/status`. A populated console needs a running daemon. Use the test host for
that, as the next section states.

## The whole daemon, on the test host

Use this for anything that touches the host network. Deploy with the `verify-on-phobos`
skill, then forward the console port:

```sh
ssh -L 9443:127.0.0.1:9443 phobos@192.168.1.221
```

Open `http://127.0.0.1:9443` on your own machine. The console binds the loopback address
on the test host only; the SSH forward is the supported way to reach it. No SSH alias
named `phobos` exists, so name the user and the address.

## The end-to-end suite

`e2e/` holds a Playwright suite that drives a real browser against a running console.
Keep the SSH forward of the previous section open, then run the suite:

```sh
cd e2e && npm install && npx playwright install chromium && npm run test:e2e
```

The suite reads the console address from `HYDRASCALE_E2E_BASE_URL`. The default value is
`http://127.0.0.1:9443`, which is the address of the SSH forward. `e2e/README.md` states
what each test covers.

## Configuration for a local run

`contrib/dev-config.yaml` declares two tailnets with no auth key. It drives the console
against a daemon that cannot connect, which is the state that most needs a designed
empty view. Use it when you change an empty state.

```sh
sudo hydrascale serve --config contrib/dev-config.yaml
```

## How to tell it is up

```sh
curl -s --unix-socket /var/lib/hydrascale/api.sock http://local/api/status | head -20
curl -s -H 'X-Hydrascale-Console: 1' http://127.0.0.1:9443/api/status | head -20
```

The daemon logs the console address at start, with the statement that the console has no
authentication. If that line is missing, the console listener did not start; check
`console.enabled` and `console.bind_address` in the configuration.
