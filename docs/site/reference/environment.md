# Environment variables

An environment variable overrides one credential of one tailnet. In each name, `<ID>` is
the tailnet identifier in upper case, with each dash replaced by an underscore. For the
tailnet `corp-prod`, `<ID>` is `CORP_PROD`.

| Variable | Description |
|---|---|
| `HYDRASCALE_AUTHKEY_<ID>` | Overrides the `auth_key` of the tailnet `<ID>`. For the tailnet `corp-prod`, set `HYDRASCALE_AUTHKEY_CORP_PROD=tskey-auth-xxxxx`. |
| `HYDRASCALE_TS_CLIENT_ID_<ID>` | Overrides `tailscale_oauth_client_id` in the secrets file. |
| `HYDRASCALE_TS_CLIENT_SECRET_<ID>` | Overrides `tailscale_oauth_client_secret` in the secrets file. |
| `HYDRASCALE_HS_API_KEY_<ID>` | Overrides `headscale_api_key` in the secrets file. |

An empty variable overrides nothing. The daemon then reads the value from the file.
