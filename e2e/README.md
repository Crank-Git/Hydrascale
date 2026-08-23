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
- `tests/policy-editor.spec.js` — opening the policy document of a tailnet with a
  working credential, editing and reverting it, and confirming Validate enables Push
  and a further edit disables it again.

`tests/policy-editor.spec.js` never clicks Push: the sandbox's tailnets are real
tailnets, and a push changes what every device in the tailnet reaches. The suite also
adds no test for the line-number gutter's scroll-sync with the document text, because
this review could not confirm that behaviour either way. See
`docs/manual/policy-editor.md`.
