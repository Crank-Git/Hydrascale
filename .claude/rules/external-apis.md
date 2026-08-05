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
| `POST /oauth/token` | Returns an access token from the client credentials. Retrieved 2026-08-05. |
| `GET /tailnet/{tailnet}/acl` | Returns `application/json` or `application/hujson`. The response carries an `ETag` header. The scope `policy_file:read` covers it. |
| `POST /tailnet/{tailnet}/acl` | Accepts `application/json` or `application/hujson`. The optional `If-Match` header carries the `ETag` value from the read. A mismatch returns HTTP 412. The scope `policy_file` covers it. |
| `POST /tailnet/{tailnet}/acl/validate` | Validates and tests a document. The schema names `application/json` for the request body of this endpoint alone. The scope `policy_file:read` covers it. |
| `POST /tailnet/{tailnet}/acl/preview` | Previews rule matches. Version 1.0 does not use it. |

**Confirmed 2026-08-05:** the OAuth scope that a policy write needs is `policy_file`.

- The OpenAPI schema states `OAuth Scope: policy_file.` in the description of
  `operationId: setPolicyFile`, which is `POST /tailnet/{tailnet}/acl`. Retrieved from
  `https://api.tailscale.com/api/v2?outputOpenapiSchema=true` on 2026-08-05.
- `https://tailscale.com/kb/1623/`, "Trust credentials", `Scopes`, states verbatim:
  "policy_file The credential has access to read, validate, and modify the tailnet policy
  file. devices:posture_attributes and devices:core:read are required when using this
  scope. Endpoints from policy_file:read POST /api/v2/tailnet/:tailnet/acl". The page
  states "Last validated: Jan 30, 2026". Retrieved 2026-08-05.
- A credential with the `policy_file` scope therefore also needs the scopes
  `devices:posture_attributes` and `devices:core:read`. Issue #161 documents this.

The token endpoint is `https://api.tailscale.com/api/v2/oauth/token`, which
`https://tailscale.com/kb/1215/oauth-clients` states. It accepts the OAuth 2.0 client
credentials grant request format, and it answers with the client credentials grant
response format: `access_token`, `token_type`, `expires_in`, and `scope`. An access token
expires after one hour. Retrieved 2026-08-05.

### Headscale — the policy document

Headscale exposes its gRPC service over a grpc-gateway REST bridge, so this project uses
`net/http` and adds no gRPC dependency.

| Method and path | Notes |
|---|---|
| `GET /api/v1/policy` | Works in both policy modes. |
| `PUT /api/v1/policy` | Succeeds only when the control server runs `policy.mode: "db"`. `cmd/headscale/cli/policy.go:114` states the constraint. |
| `POST /api/v1/policy/check` | Checks a document for errors. Used as the validate step. |

Authentication is a bearer token holding a Headscale API key.

## Rules

- A capability you could not confirm goes into `Risks & open questions` in the spec. It
  never enters a requirement as though it were settled.
- Documentation that contradicts the plan is a reason to stop and ask. It is not
  something to reconcile by guessing.
- A call that changes a resource on a control server affects every device in that
  tailnet. Confirm with the operator before you run one against a real tailnet.
- A read-only call is the safe way to learn the shape of a real resource. Prefer it.
