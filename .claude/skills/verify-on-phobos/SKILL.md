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

## The arguments

The first argument selects the action: `deploy`, `rollback`, or `status`. The default
action is `deploy`.

The second argument is the host. The host is a parameter, never a fixed value inside a
command. Set it once, then run every block below without a change:

```sh
HOST="${2:-phobos@192.168.1.221}"
```

The default host answers at the address `192.168.1.221` as the user `phobos`. The
default host reports its own name as `mars`. No SSH alias named `phobos` exists, so the
bare host name `phobos` does not resolve. Pass a second argument to select another host.

The host needs passwordless sudo and SSH key access.

Every remote command in this document carries two options:

- `-o BatchMode=yes` stops SSH from a password prompt that waits for an operator.
- `-o ConnectTimeout=10` stops SSH from a wait of several minutes on a host that is down.

**Warning: the deploy action replaces the running daemon on a live host.** Confirm with
the operator before you deploy, unless they asked for the deploy in this turn. Read the
rollback section before you start.

## Deploy

1. Confirm the host answers. If this step fails, stop. The host keeps its current state,
   because no later step ran:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'systemctl is-active hydrascale; sha256sum /usr/local/bin/hydrascale'
   ```

2. Build for the host. If the build fails, stop. Copy nothing:
   ```sh
   GOOS=linux GOARCH=amd64 go build -o /tmp/hydrascale.test ./cmd/hydrascale
   ```

3. Copy the binary:
   ```sh
   scp -o BatchMode=yes -o ConnectTimeout=10 /tmp/hydrascale.test "$HOST":/tmp/hydrascale.test
   ```

4. Keep the current binary at `/usr/local/bin/hydrascale.prev`, then install the new one:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo systemctl stop hydrascale \
     && sudo cp /usr/local/bin/hydrascale /usr/local/bin/hydrascale.prev \
     && sudo install -m 0755 /tmp/hydrascale.test /usr/local/bin/hydrascale \
     && sudo systemctl start hydrascale'
   ```

5. Print the service status and the log:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'systemctl status hydrascale --no-pager; sudo journalctl -u hydrascale -n 40 --no-pager'
   ```

6. If step 5 reports a state other than `active (running)`, print that log to the
   operator. Tell the operator to run the rollback action. Roll back before you debug. A
   host that holds a stopped daemon helps nobody.

## Check host behaviour

Run the checks the change needs. These are read-only.

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'ip netns list'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo iptables -S HYDRASCALE-FWD'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo iptables -S FORWARD'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'ip -brief addr show type veth'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo hydrascale status'
```

The daemon creates the chain `HYDRASCALE-FWD` only on a branch that holds the local rule
set. On an earlier branch, `sudo iptables -S HYDRASCALE-FWD` prints
`iptables: No chain/target/match by that name.` and returns 1. That result is correct for
such a branch.

Reachability, for a change to the rule set. The first must fail and the second must
succeed when a rule allows the path:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo ip netns exec ns-<a> ping -c1 -W2 <veth address of b>'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo ip netns exec ns-<a> curl -sS -m5 -o /dev/null -w "%{http_code}" https://example.com'
```

DNS protection, for a change to the overlay:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo cat /proc/$(pgrep -f "tailscaled.*ns-<a>")/mountinfo | grep " /etc "'
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sha256sum /etc/resolv.conf'
```

Record the command and its output verbatim in the pull request. A claim about host
behaviour needs the output that proves it.

## Rollback

The `rollback` argument runs this one block. It stops the service, it restores the
previous binary, and it starts the service again:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo systemctl stop hydrascale \
  && sudo cp /usr/local/bin/hydrascale.prev /usr/local/bin/hydrascale \
  && sudo systemctl start hydrascale'
```

Confirm the result. The state must equal `active`. The checksum must equal the checksum
that step 1 of the deploy action recorded:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'systemctl is-active hydrascale; sha256sum /usr/local/bin/hydrascale; ip netns list'
```

Step 4 of the deploy action writes `/usr/local/bin/hydrascale.prev`. A host that never
ran the deploy action holds no such file. On such a host the `cp` command fails and the
service stays stopped. Start the service again by hand in that case.

Stopping the service alone returns the host to a working state, because the daemon holds
no lock that survives it. Restore the previous binary as well before you leave the host.

## Status only

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'systemctl is-active hydrascale; /usr/local/bin/hydrascale version'
```
