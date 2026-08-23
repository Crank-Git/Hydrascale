# Hydrascale console E2E suite

This suite drives a real browser against a running console with Playwright. It covers
the happy paths that [`docs/manual`](../docs/manual) documents.

## Setup

```sh
cd e2e
npm install
npx playwright install chromium
```

## Run

```sh
npm run test:e2e
```

The suite reads the base URL from `HYDRASCALE_E2E_BASE_URL`. It defaults to
`http://127.0.0.1:9443`, the address an SSH tunnel to a test host commonly uses.

```sh
HYDRASCALE_E2E_BASE_URL=http://127.0.0.1:9443 npm run test:e2e
```

## Scope

- `tests/overview-and-navigation.spec.js` — the console lands on Overview, lists every
  tailnet with its reconciler state, and every view in the main navigation bar opens.
- `tests/access-editor.spec.js` — selecting a source tailnet in the Access view, staging
  a local rule from the reachability matrix, and discarding it back to zero staged
  edits.

The suite adds no test for the Policy document editor (line numbers, Validate, Push).
The review that produced this suite reached the Policy tailnet list and the read-only
state of a tailnet with no credential, but its access token to the tailnet with a usable
credential was rejected by the control server, an environmental condition of the review
session. See `docs/manual/policy-editor.md`.
