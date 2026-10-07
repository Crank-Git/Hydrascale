# Headscale and a custom control server

The key `control_url` points a tailnet at [Headscale](https://github.com/juanfont/headscale),
or at another control server that Tailscale clients accept. A tailnet on a self-hosted
control server therefore runs next to a tailnet on the Tailscale coordination server.

## Set the control URL

Set the key per tailnet, or set a global default:

```yaml
version: 2
control_url: "https://headscale.example.com"   # the default for every tailnet

tailnets:
  - id: homelab
    # takes the global control_url

  - id: corp-infra
    control_url: "https://headscale.corp.internal"
    # takes its own Headscale instance

  - id: personal
    # no control_url: the Tailscale coordination server
```

A `control_url` of a tailnet overrides the global default. A tailnet that declares
neither joins the Tailscale coordination server.

The URL needs the scheme `https`. The scheme `http` is accepted only when the host is a
loopback IP address. The daemon refuses the name `localhost`.

## Log in without an auth key

If you have no pre-authentication key, log in after the daemon creates the namespace.
Start the daemon, or run `apply`. Then run the login for each tailnet:

```bash
# Start the daemon, which creates the namespaces and starts tailscaled
sudo hydrascale serve &

# Log in to a Headscale tailnet
sudo hydrascale tailscale corp-prod -- up --login-server https://headscale.example.com

# Log in to the Tailscale coordination server
sudo hydrascale tailscale personal -- up
```

The daemon prints the authentication URL. Open it in a browser and approve the device.
The namespace stays up, and the daemon manages it from that point.

## What to check with Headscale

- **The auth key format.** A Headscale auth key has a different form from a Tailscale
  `tskey-auth-*` key. The daemon checks no form. It writes the key to a file of mode
  `0600` and runs `tailscale up --auth-key=file:<path>`. It removes the file after the
  login. Use the key type of the control server.
- **MagicDNS.** The DNS names of host access depend on the DNS configuration of the
  control server. Headscale serves MagicDNS, and its suffix and its behaviour can differ
  from Tailscale. The daemon also exports the split DNS domains of each tailnet to the
  resolver of the host. A Headscale control server can serve no split DNS. If a name
  does not resolve, read the Headscale DNS configuration.
- **DERP relays.** Tailscale runs its own global DERP relay network. Headscale uses the
  same relays, its own relays, or both. If two peers cannot connect, read the DERP map of
  the Headscale instance. A direct connection through STUN works without the control
  server.
- **The policy.** A policy write needs Headscale v0.29 or later, and
  `policy.mode: "database"`. See [Upstream policy](../concepts/upstream-policy.md#headscale)
  and [Credentials](credentials.md#a-headscale-credential).
