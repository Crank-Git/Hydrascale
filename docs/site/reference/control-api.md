# The control API

The daemon serves the JSON API on the control socket `/var/lib/hydrascale/api.sock`, and on
the console listener at `127.0.0.1:9443`. The `tui` command and the `status` command use the
control socket. Every mutating route on the console listener requires the header
`X-Hydrascale-Console: 1`.

The console is the one supported client of the API. The JSON shape of a route can change
in a release, so a script that reads a route can break.

| Route | Method | Description |
|---|---|---|
| `/api/status` | GET | The declared state and the live state of every tailnet |
| `/api/events` | GET | The recent events of the reconciler. See [Events](events.md). |
| `/api/reconcile` | POST | Run one reconcile now |
| `/api/tailnet/add` | POST | Add a tailnet to the config and reconcile |
| `/api/tailnet/remove` | POST | Remove a tailnet from the config and reconcile |
| `/api/tailnet/connect` | POST | Clear the error state and reconnect a tailnet |
| `/api/tailnet/disconnect` | POST | Stop the process of a tailnet and keep the config |
| `/api/tailnet/{id}/detail` | GET | The peers, the routes, and the addresses of one tailnet |
| `/api/tailnet/{id}/removal-plan` | GET | What a removal of one tailnet deletes |
| `/api/config` | GET | The current configuration, with every credential removed |
| `/api/config/dns` | POST | Change the resolver configuration |
| `/api/dns` | GET | The resolver state, the DNS protection state, and the split DNS domains and conflict per namespace |
| `/api/access` | GET, PUT | Read and write the local rule set |
| `/api/policy` | GET | The control server kind and the write availability per tailnet |
| `/api/policy/{id}` | GET, PUT | Read and write the policy of one tailnet |
| `/api/policy/{id}/validate` | POST | Validate a policy document at the control server |
| `/api/policy/{id}/credentials` | PUT | Write the credential of one tailnet |
| `/api/policy/{id}/sections` | POST | Parse the policy document in the body and return its sections. It changes no state. |
| `/api/policy/{id}/sections/edit` | POST | Apply one edit to the policy document in the body and return the result. It sends nothing to the control server. |

Every mutating route validates the whole request body before it changes anything. A failure
returns HTTP 400 and the body `{"error": "<message>"}`.

A route answers HTTP 405 for a method that the table does not name.

## An example

```bash
sudo curl --unix-socket /var/lib/hydrascale/api.sock http://unix/api/status
```

To reach the socket without root, or from another machine, read
[Remote access](../operations/remote-access.md).
