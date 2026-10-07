# Credentials

A credential authenticates the daemon to a control server. A policy read and a policy
write need one. The daemon reads a credential at the moment it needs one, and it holds
none between requests.

## The secrets file

A credential never enters the configuration file, never reaches the log, and never
reaches a control API answer. It lives in the secrets file, which the key `secrets_file`
names. The default path is `/etc/hydrascale/secrets.yaml`.

The daemon refuses a secrets file that grants group access or other access. It also
refuses a secrets file that is a symbolic link, or that is not a regular file. Give the
file the mode `0600` and the owner root.

```yaml
# /etc/hydrascale/secrets.yaml, mode 0600, owner root
tailnets:
  corp-prod:
    tailscale_oauth_client_id: "..."
    tailscale_oauth_client_secret: "..."
  homelab:
    headscale_api_key: "..."
    headscale_address: "https://headscale.example.com"
```

Write the file with the mode in place:

```bash
sudo install -m 0600 /dev/null /etc/hydrascale/secrets.yaml
sudo "$EDITOR" /etc/hydrascale/secrets.yaml
```

If the file does not exist, the daemon reads a credential from the environment variables
alone. See [An environment variable overrides a file value](#an-environment-variable-overrides-a-file-value).
The console writes the same values through `PUT /api/policy/{id}/credentials`.

## A Tailscale credential

The daemon authenticates to the Tailscale API with an OAuth client. Create the client in
the Tailscale admin console:

1. Open `https://login.tailscale.com/admin/settings/trust-credentials`.
2. Select the button **Credential**.
3. Select **OAuth**.
4. Select the scopes.
5. Select **Generate credential**.

**Warning — the admin console shows the client secret one time only.** Copy the secret
into the secrets file before you leave the page.

**A policy write needs three scopes, not one.** Give the client these scopes:

- `policy_file`
- `devices:posture_attributes`
- `devices:core:read`

When the operator selects the policy file scope at write, the admin console grants
`devices:core:read` and `devices:posture_attributes` on its own. The source of the path
is `https://tailscale.com/kb/1215/oauth-clients`, retrieved on 2026-08-05.

`https://tailscale.com/kb/1623/`, "Trust credentials", section `Scopes`, states verbatim:

> policy_file The credential has access to read, validate, and modify the tailnet policy
> file. devices:posture_attributes and devices:core:read are required when using this
> scope. Endpoints from policy_file:read POST /api/v2/tailnet/:tailnet/acl

The Tailscale OpenAPI schema states `OAuth Scope: policy_file.` in the description of
`operationId: setPolicyFile`. Both sources were retrieved on 2026-08-05. A client that
holds `policy_file` alone fails at the write. The permission error names no cause.

Write the client identifier and the client secret into the secrets file as
`tailscale_oauth_client_id` and `tailscale_oauth_client_secret`. The daemon exchanges
them for an access token at `https://api.tailscale.com/api/v2/oauth/token`. The access
token expires after one hour.

## A Headscale credential

Create an API key on the Headscale host:

```bash
headscale apikeys create --expiration 90d
```

Write the key into the secrets file as `headscale_api_key`. Write the base address of the
control server as `headscale_address`. The daemon sends the key as a bearer token.

Before a policy write, set `policy.mode: "database"` in the Headscale configuration. A
server in the file policy mode serves a policy read, and it refuses a policy write. See
[Upstream policy](../concepts/upstream-policy.md#headscale).

## An environment variable overrides a file value

An environment variable overrides the matching file value, and it overrides that value
alone. An empty variable overrides nothing. `<ID>` is the tailnet identifier in upper
case, with each dash replaced by an underscore.

| Variable | Overrides |
|---|---|
| `HYDRASCALE_TS_CLIENT_ID_<ID>` | `tailscale_oauth_client_id` |
| `HYDRASCALE_TS_CLIENT_SECRET_<ID>` | `tailscale_oauth_client_secret` |
| `HYDRASCALE_HS_API_KEY_<ID>` | `headscale_api_key` |

No environment variable overrides `headscale_address`. The secrets file holds it.
