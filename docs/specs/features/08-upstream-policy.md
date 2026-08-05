---
id: upstream-policy
feature: Upstream policy control
epic: "Epic 8: Upstream policy control"
status: issued
issues: [154, 155, 156, 157, 158, 159]
mockups: [mockups/04-upstream-policy.html]
---

## Purpose

A local rule controls what the host forwards. It does not control what a control server
tells a peer. Those are two different access-control systems and an operator needs both.

This feature set reads the policy of each tailnet from its control server, shows it, lets
the operator change it, and writes it back. It supports Tailscale and Headscale. It
writes the policy document. It does not manage devices, users, tags, or keys.

The two control servers differ. Tailscale supports read, validate, preview, and write
through its public API. Headscale supports read and validate always, and write only when
the control server runs its database policy mode. The console states the difference
rather than hiding it.

## Verified against

| Interface | Source | Retrieved |
|---|---|---|
| Tailscale API v2 policy endpoints | The Tailscale OpenAPI schema at `https://api.tailscale.com/api/v2?outputOpenapiSchema=true`, `openapi: 3.1.0`, server `https://api.tailscale.com/api/v2` | 2026-08-04 |
| Headscale policy endpoints | `https://github.com/juanfont/headscale/blob/v0.29.3/proto/headscale/v1/headscale.proto` lines 194-213, and `gen/openapiv2/headscale/v1/headscale.swagger.json` at the same tag | 2026-08-04 |
| Headscale policy modes | `https://github.com/juanfont/headscale/blob/v0.29.3/docs/ref/policy.md` and `cmd/headscale/cli/policy.go:114` | 2026-08-04 |

## User stories

- As the operator, I want to read the policy of a tailnet without opening the Tailscale
  admin console, so that I manage the host from one place.
- As the operator, I want to check a policy change before I push it, so that I do not
  lock myself out of a tailnet.
- As the operator, I want to know why a push is unavailable, so that I do not think the
  console is broken.
- As the operator, I want my credentials in a root-only file, so that a non-root account
  cannot read them.

## Functional requirements

### Credentials

- **FR-policy-1** — The daemon reads credentials from the file that `secrets_file`
  names, which defaults to `/etc/hydrascale/secrets.yaml`.
- **FR-policy-2** — An environment variable overrides the matching file value.
- **FR-policy-3** — The variable names are `HYDRASCALE_TS_CLIENT_ID_<ID>`,
  `HYDRASCALE_TS_CLIENT_SECRET_<ID>`, and `HYDRASCALE_HS_API_KEY_<ID>`.
- **FR-policy-4** — The control API never returns a credential value.
- **FR-policy-5** — The control API returns, per tailnet, whether a credential is
  present and which control server kind it targets.
- **FR-policy-6** — The console can write a credential through `PUT /api/policy/{id}/credentials`.
- **FR-policy-7** — The daemon writes the secrets file with mode `0600` and owner root.
- **FR-policy-8** — The daemon never logs a credential value.

### Read

- **FR-policy-9** — `GET /api/policy/{id}` returns the policy document of tailnet `{id}`
  as text.
- **FR-policy-10** — The response states the control server kind, `tailscale` or
  `headscale`.
- **FR-policy-11** — The response states whether a write is available.
- **FR-policy-12** — For a Tailscale tailnet, the response carries the `ETag` value that
  the control server returned.
- **FR-policy-13** — When no credential is configured, the route returns HTTP 409 and a
  message that names the credential the tailnet needs.

### Validate

- **FR-policy-14** — `POST /api/policy/{id}/validate` sends the edited document to the
  control server's validate endpoint and returns the result.
- **FR-policy-15** — The console runs validate before it offers to push.

### Write

- **FR-policy-16** — `PUT /api/policy/{id}` writes the edited document to the control
  server.
- **FR-policy-17** — For a Tailscale tailnet, the daemon sends the `If-Match` header with
  the `ETag` value from the read.
- **FR-policy-18** — When the control server returns HTTP 412, the daemon returns HTTP
  409 with a message that states that the policy changed since the read.
- **FR-policy-19** — For a Headscale tailnet whose control server runs the file policy
  mode, the write fails and the daemon returns the control server's error and the reason.
- **FR-policy-20** — The daemon records the event `policy.pushed` with the tailnet
  identifier and the control server kind.
- **FR-policy-21** — The daemon never writes a policy document that validate rejected.

### The console view

- **FR-policy-22** — The policy view lists every tailnet with its control server kind and
  its credential state.
- **FR-policy-23** — Selecting a tailnet shows its policy document in a text editor.
- **FR-policy-24** — The editor marks the document as edited when it differs from the
  document that the console read.
- **FR-policy-25** — The view has a validate action and a push action, and push is
  disabled until validate succeeds.
- **FR-policy-26** — The view shows the validate result, including each error with its
  line number when the control server supplies one.
- **FR-policy-27** — For a tailnet that cannot be written, the view shows the document as
  read-only and it states the reason.
- **FR-policy-28** — The view states that a policy change affects every device in the
  tailnet, not only this host.

## User flows

### The operator reads and changes a Tailscale policy

1. The operator opens the policy view and selects a Tailscale tailnet.
2. The console sends `GET /api/policy/{id}`.
3. The daemon requests an OAuth access token from the control server with the client
   credentials.
4. The daemon sends `GET /api/v2/tailnet/{tailnet}/acl` with the bearer token.
5. The daemon returns the document and the `ETag` value.
6. The console shows the document.
7. The operator edits the document.
8. The operator selects **Validate**.
9. The daemon sends `POST /api/v2/tailnet/{tailnet}/acl/validate`.
10. The control server accepts the document.
11. The operator selects **Push**.
12. The daemon sends `POST /api/v2/tailnet/{tailnet}/acl` with `If-Match` set to the
    `ETag` value.
13. The control server accepts the write and the daemon records `policy.pushed`.

### The operator reads a Headscale policy that cannot be written

1. The operator selects a Headscale tailnet.
2. The daemon sends `GET /api/v1/policy` to the Headscale address with the API key as a
   bearer token.
3. The daemon returns the document and it marks the write as unavailable.
4. The console shows the document read-only.
5. The console states that the control server runs the file policy mode, and that a write
   needs `policy.mode: "db"`.

### The policy changed since the read

1. The operator pushes a document.
2. The control server returns HTTP 412.
3. The daemon returns HTTP 409 with the reason.
4. The console states that another person changed the policy.
5. The console offers to re-read the document, and it keeps the operator's edited text so
   that the operator can compare.

## Screens & states

### Policy — `mockups/04-upstream-policy.html`

| Region | Content |
|---|---|
| Tailnet list | One row per tailnet: identifier, control server kind, credential state, write availability. |
| Editor | The policy document in the mono typeface, with line numbers. |
| Result | The validate result, or the push result. |
| Actions | Validate, push, re-read, and discard. |

| State | What it shows |
|---|---|
| No credential | The row states that the tailnet needs a credential, and it names which one. The editor region states the same. |
| Read-only | The document, with a statement of why a write is unavailable. |
| Loaded | The document, unedited, with push disabled. |
| Edited | The document, marked edited, with validate enabled and push disabled. |
| Validated | The validate result, with push enabled. |
| Validate failed | Each error with its line number, and push disabled. |
| Push conflict | The statement that the policy changed, and the re-read action. |
| Unreachable | The control server did not answer. The view names the address and the error. |

## Behaviour rules

- The daemon reads a credential at the moment it needs it. It does not hold a credential
  in memory between requests.
- The daemon caches an OAuth access token until it expires, because a token request per
  call is wasteful and the control server rate-limits.
- Push is disabled until validate succeeds on the current text. An edit after a
  successful validate disables push again.
- The console never edits the document for the operator. It does not reformat, it does
  not sort, and it does not add a field.
- The document is huJSON. The console does not parse it as JSON, because huJSON allows a
  comment and a trailing comma.
- A read failure never changes the local rule set. The two systems are independent.

## Data touched

| Entity | Change |
|---|---|
| Secrets file | New file, with a per-tailnet credential block. |
| Configuration | New key `secrets_file`. |
| Event | New kind `policy.pushed`. |

## Interfaces

### Tailscale

Base URL `https://api.tailscale.com/api/v2`. Authentication is a bearer token. The token
comes from an OAuth client. Verified against the Tailscale OpenAPI schema, retrieved
2026-08-04.

| Method and path | Purpose | Notes |
|---|---|---|
| `GET /tailnet/{tailnet}/acl` | Read the policy. | Returns `application/json` or `application/hujson`. The scope `policy_file:read` covers it. The response carries an `ETag` header. |
| `POST /tailnet/{tailnet}/acl` | Write the policy. | Accepts `application/json` or `application/hujson`. The optional `If-Match` header carries the `ETag` value from the read. A mismatch returns HTTP 412. |
| `POST /tailnet/{tailnet}/acl/validate` | Validate and test the policy. | Accepts the same content types. |
| `POST /tailnet/{tailnet}/acl/preview` | Preview which rules match. | Version 1.0 does not use it. |

The `If-Match` header also accepts the literal `ts-default`, which writes only when the
policy is still the untouched default. Version 1.0 does not use that form.

### Headscale

Base URL is the address in the credential block. Authentication is a bearer token that
holds a Headscale API key. Verified against `headscale.proto` at tag `v0.29.3`, retrieved
2026-08-04.

| Method and path | Purpose | Notes |
|---|---|---|
| `GET /api/v1/policy` | Read the policy. | Works in both policy modes. |
| `PUT /api/v1/policy` | Write the policy. | Succeeds only when the control server runs `policy.mode: "db"`. `cmd/headscale/cli/policy.go:114` states the constraint. |
| `POST /api/v1/policy/check` | Check the policy for errors. | Used as the validate step. |

Headscale exposes these over a grpc-gateway REST bridge, so the daemon uses `net/http`
and adds no gRPC dependency.

### The daemon's own routes

| Method and path | Purpose |
|---|---|
| `GET /api/policy` | List every tailnet with its control server kind, its credential state, and its write availability. |
| `GET /api/policy/{id}` | Read one policy document. |
| `POST /api/policy/{id}/validate` | Validate an edited document. |
| `PUT /api/policy/{id}` | Write an edited document. |
| `PUT /api/policy/{id}/credentials` | Write the credential for one tailnet. |

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The OAuth client is invalid. | The token request fails. The daemon returns HTTP 502 with the control server's message, and it does not log the secret. |
| The tailnet name is unknown to Tailscale. | The control server returns HTTP 404. The console states that the tailnet name in the credential block does not match a tailnet on the account. |
| The control server rate-limits the daemon. | The daemon returns the control server's status and its message. The console shows it and it does not retry automatically. |
| The Headscale address uses a self-signed certificate. | The request fails. Version 1.0 does not add a certificate override. The console shows the TLS error. |
| The document is 400 kilobytes. | The editor loads it. The daemon limits a request body to 1 megabyte. |
| A Headscale control server older than v0.29 does not expose `/api/v1/policy`. | The request returns HTTP 404. The console states that the control server does not support policy access and it names the version that does. |
| The operator pushes a policy that locks this host out. | The push succeeds. The console warned that a policy change affects every device. The daemon does not second-guess the operator. |

## Acceptance criteria

- [ ] `GET /api/policy` lists every tailnet with its control server kind and its
      credential state, and it returns no credential value.
- [ ] A tailnet with no credential returns HTTP 409 from `GET /api/policy/{id}`, with a
      message that names the credential.
- [ ] The daemon reads a Tailscale policy document with an OAuth client and returns the
      `ETag` value.
- [ ] `HYDRASCALE_TS_CLIENT_ID_<ID>` overrides the value in the secrets file.
- [ ] The daemon sends `If-Match` on a Tailscale write, and a stale value produces HTTP
      409 from the daemon.
- [ ] The daemon reads a Headscale policy document with an API key.
- [ ] A Headscale control server in file policy mode produces a read-only view with the
      stated reason.
- [ ] A Headscale control server in database policy mode accepts a write.
- [ ] Push is disabled until validate succeeds, and an edit after validate disables it
      again.
- [ ] A validate failure shows each error with its line number.
- [ ] No log line contains an OAuth secret or a Headscale API key.
- [ ] The secrets file is created with mode `0600`.
- [ ] The policy view states that a change affects every device in the tailnet.

## Out of scope

- Device, user, tag, and key management on the control server.
- A visual editor for the policy document. Version 1.0 shows text.
- The Tailscale preview endpoint.
- The Headscale local-file fallback for a control server in file policy mode.
- A certificate override for a self-signed Headscale control server.

## Open questions

- The OAuth scope name that a Tailscale policy write needs. The OpenAPI schema names
  `policy_file:read` for the read and it names no scope for the write. The epic confirms
  the write scope from the Tailscale OAuth documentation before it writes the setup
  guide. `spec.md` records this as risk R1.
