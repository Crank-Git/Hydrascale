---
name: verify-on-phobos
description: Deploy the current branch build to the Linux test host and check it, or roll it back. Use when a change touches namespaces, iptables, routing, or DNS, because no unit test covers real host behaviour.
argument-hint: "[deploy|rollback|status] [host]"
allowed-tools: Bash, Read
---

# Verify on the test host

The daemon changes the host network. A unit test replaces the command runner, so it
proves which commands the code runs — not what the kernel does with them. A change to
`internal/access`, `internal/namespaces`, `internal/routing`, `internal/hostaccess`, or
the DNS overlay is not done until it runs on a real Linux host.

The default host is `phobos`. Pass a different host as the second argument. The host
needs passwordless sudo and SSH key access.

**Warning: this replaces the running daemon on a live host.** Confirm with the operator
before you deploy, unless they asked for the deploy in this turn. Read the rollback
section before you start.

## Deploy

1. Build for the host:
   ```sh
   GOOS=linux GOARCH=amd64 go build -o /tmp/hydrascale.test ./cmd/hydrascale
   ```
   If the build fails, stop. Copy nothing.

2. Copy the binary:
   ```sh
   scp /tmp/hydrascale.test "$HOST":/tmp/hydrascale.test
   ```

3. Keep the current binary, then install the new one:
   ```sh
   ssh "$HOST" 'sudo systemctl stop hydrascale \
     && sudo cp /usr/local/bin/hydrascale /usr/local/bin/hydrascale.prev \
     && sudo install -m 0755 /tmp/hydrascale.test /usr/local/bin/hydrascale \
     && sudo systemctl start hydrascale'
   ```

4. Read the result:
   ```sh
   ssh "$HOST" 'systemctl status hydrascale --no-pager; sudo journalctl -u hydrascale -n 40 --no-pager'
   ```

5. If the service did not start, roll back before you debug. A wedged test host helps
   nobody.

## Check host behaviour

Run the checks the change needs. These are read-only.

```sh
ssh "$HOST" 'ip netns list'
ssh "$HOST" 'sudo iptables -S HYDRASCALE-FWD'
ssh "$HOST" 'sudo iptables -S FORWARD'
ssh "$HOST" 'ip -brief addr show type veth'
ssh "$HOST" 'sudo hydrascale status'
```

Reachability, for a change to the rule set. The first must fail and the second must
succeed when a rule allows the path:

```sh
ssh "$HOST" 'sudo ip netns exec ns-<a> ping -c1 -W2 <veth address of b>'
ssh "$HOST" 'sudo ip netns exec ns-<a> curl -sS -m5 -o /dev/null -w "%{http_code}" https://example.com'
```

DNS protection, for a change to the overlay:

```sh
ssh "$HOST" 'sudo cat /proc/$(pgrep -f "tailscaled.*ns-<a>")/mountinfo | grep " /etc "'
ssh "$HOST" 'sha256sum /etc/resolv.conf'
```

Record the command and its output verbatim in the pull request. A claim about host
behaviour needs the output that proves it.

## Rollback

```sh
ssh "$HOST" 'sudo systemctl stop hydrascale \
  && sudo cp /usr/local/bin/hydrascale.prev /usr/local/bin/hydrascale \
  && sudo systemctl start hydrascale'
```

Stopping the service alone returns the host to a working state, because the daemon holds
no lock that survives it. Restore the previous binary as well before you leave the host.

## Status only

```sh
ssh "$HOST" 'systemctl is-active hydrascale; /usr/local/bin/hydrascale version'
```
