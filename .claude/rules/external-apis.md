---
paths:
  - "internal/policy/**/*.go"
  - "internal/api/**/*.go"
  - "docs/specs/**/*.md"
---

# External APIs — read the documentation, never assume

Before you write a call against any service below, confirm the operation name, the
required parameters, the response shape, the error cases, and the permissions the call
needs. Read the vendor's own documentation or its machine-readable schema. Do not
describe an interface from memory, and do not infer it from the name.

Cite the documentation URL and the version in the pull request body.

| Service | Version pinned | Documentation |
|---|---|---|
| Tailscale API | v2, OpenAPI 3.1.0 | `https://api.tailscale.com/api/v2?outputOpenapiSchema=true` (machine-readable), `https://tailscale.com/api` (rendered) |
| Headscale API | v0.29.3 | `https://github.com/juanfont/headscale/blob/v0.29.3/proto/headscale/v1/headscale.proto`, `docs/ref/policy.md` at the same tag |

The rendered Tailscale documentation page renders with JavaScript, so `WebFetch` returns
navigation rather than content. Fetch the OpenAPI schema instead; it is YAML despite the
query parameter name.

## What the project already confirmed

Retrieved 2026-08-04. Re-confirm before you write the call.

### Tailscale — the policy document

Base URL `https://api.tailscale.com/api/v2`. Authentication is a bearer token from an
OAuth client.

| Method and path | Notes |
|---|---|
| `GET /tailnet/{tailnet}/acl` | Returns `application/json` or `application/hujson`. The response carries an `ETag` header. The scope `policy_file:read` covers it. |
| `POST /tailnet/{tailnet}/acl` | Accepts the same content types. The optional `If-Match` header carries the `ETag` value from the read. A mismatch returns HTTP 412. |
| `POST /tailnet/{tailnet}/acl/validate` | Validates and tests a document. |
| `POST /tailnet/{tailnet}/acl/preview` | Previews rule matches. Version 1.0 does not use it. |

**Unconfirmed:** the OAuth scope that a policy write needs. The schema names
`policy_file:read` for the read and names no scope for the write. Confirm it from the
Tailscale OAuth documentation before you document the credential setup. Risk R1 in
`docs/specs/spec.md` tracks this.

### Headscale — the policy document

Headscale exposes its gRPC service over a grpc-gateway REST bridge, so this project uses
`net/http` and adds no gRPC dependency.

| Method and path | Notes |
|---|---|
| `GET /api/v1/policy` | Works in both policy modes. |
| `PUT /api/v1/policy` | Succeeds only when the control server runs `policy.mode: "db"`. `cmd/headscale/cli/policy.go:114` states the constraint. |
| `POST /api/v1/policy/check` | Checks a document for errors. Used as the validate step. |

Authentication is a bearer token holding a Headscale API key.

Confirmed again 2026-08-05 for issue #156, from
`https://raw.githubusercontent.com/juanfont/headscale/v0.29.3/proto/headscale/v1/headscale.proto`
lines 194-213 and
`https://raw.githubusercontent.com/juanfont/headscale/v0.29.3/proto/headscale/v1/policy.proto`:

- The three routes above are the `google.api.http` annotations of `GetPolicy`,
  `SetPolicy`, and `CheckPolicy`. `SetPolicy` and `CheckPolicy` both carry `body: "*"`.
- The request body of `SetPolicy` and of `CheckPolicy` is `{"policy": "<document>"}`.
- The response body of `GetPolicy` and of `SetPolicy` is
  `{"policy": "<document>", "updatedAt": "<timestamp>"}`. `CheckPolicyResponse` is empty.
- `hscontrol/grpcv1.go:727` returns `types.ErrPolicyUpdateIsDisabled` when `policy.mode`
  is not `db`. `hscontrol/types/policy.go:11` declares it as
  `errors.New("update is disabled for modes other than 'database'")`. That error carries
  the gRPC code `Unknown`, therefore **the REST bridge answers HTTP 500 and not HTTP
  403**. A client detects the file policy mode from the `message` field of the answer,
  not from the status.
- An error answer of the REST bridge has the shape
  `{"code": <number>, "message": "<text>", "details": []}`.

## Rules

- A capability you could not confirm goes into `Risks & open questions` in the spec. It
  never enters a requirement as though it were settled.
- Documentation that contradicts the plan is a reason to stop and ask. It is not
  something to reconcile by guessing.
- A call that changes a resource on a control server affects every device in that
  tailnet. Confirm with the operator before you run one against a real tailnet.
- A read-only call is the safe way to learn the shape of a real resource. Prefer it.
