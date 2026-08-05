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

   If the host restarted, read the crash record before you continue. The section
   [Read the crash record](#read-the-crash-record) holds the steps. Report no restart as
   unexplained until you read that record.

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

## Read the crash record

Issue #104 records that the test host restarts without a clean shutdown record. The
kernel writes a crash record before it restarts, and the record survives the restart.
Read the crash record before you report a restart as unexplained.

The test host holds the `efi_pstore` backend, so it needs no reserved memory and no
`kdump` package. This command names the backend that the kernel registered:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo dmesg | grep "persistent store backend"'
```

The host answers `pstore: Registered efi_pstore as persistent store backend`. A host that
prints nothing holds no `pstore` backend. On such a host, use `kdump` instead.

`systemd-pstore.service` runs at each start, and Ubuntu enables it by default. The service
moves every record out of `/sys/fs/pstore` into `/var/lib/systemd/pstore`. `/sys/fs/pstore`
is therefore empty on a healthy host. Read the archive directory, not the empty one.

1. List the records. The newest directory comes first. One crash writes more than one
   directory, so count the panic lines of step 2 rather than the directories:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo ls -1t /var/lib/systemd/pstore'
   ```

2. Print the panic line of every record:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo grep -h "Kernel panic" /var/lib/systemd/pstore/*/*/dmesg.txt'
   ```

3. Read one whole record. `dmesg.txt` holds the joined log, and the files beside it hold
   the single parts:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo cat /var/lib/systemd/pstore/<dir>/001/dmesg.txt'
   ```

**The record holds its parts in reverse order.** `Part1` carries the panic line and the
backtrace, and it is at the foot of `dmesg.txt`. Search the file with `grep`, because the
first lines of the file are the oldest part.

The archive keeps a record until an operator deletes it. A record that predates the
restart under review is therefore normal. Compare the directory timestamp against the
start time that `last -x reboot` reports.

If the archive holds no record for the restart, then the kernel wrote none. The kernel
writes no record when the host loses power, when the firmware resets the host, or when
the host stops without a panic. Report that absence as the measurement. It excludes a
kernel panic as the cause.

### Confirm that the capture still works

**Warning: the command below stops the host at once, and it stops every tailnet the host
serves.** Ask the operator before you run it, and never run it while another change is
verified on that host.

Run this only after a kernel upgrade, or when the archive holds no record for a restart
that a panic would explain.

`kernel.sysrq` equals `176` on the test host, and `/etc/sysctl.d/10-magic-sysrq.conf` sets
that value. `176` omits the bit `0x8`, which the crash function needs, so `sysctl` must
raise the value first. The restart restores `176` from that file, therefore this step
leaves no change on the host:

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo sysctl -w kernel.sysrq=1 && sync && sudo sh -c "echo c > /proc/sysrq-trigger"'
```

The SSH connection stops without an answer, which is the correct result. Wait for the
host, then read the crash record with the steps above. The record holds the line
`Kernel panic - not syncing: sysrq triggered crash`.

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

### Restore the forward rules after a rollback to version 0.9

Version 0.9 writes two `FORWARD` rules for each namespace, and it writes them only when it
creates the veth pair. Epic 5 removes those rules, because the chain `HYDRASCALE-FWD`
replaces them. A rollback keeps the namespaces, so the setup path does not run and the two
rules never return. A restart of the service repairs nothing, for the same reason. The
project manager measured this on 2026-08-05 and issue #172 records it.

**Warning: a namespace with no `FORWARD` rule reaches nothing, and `hydrascale status`
still reports `healthy` and `running`.** Run these steps after every rollback from a build
that holds `HYDRASCALE-FWD` to a version 0.9 build.

1. Read the chain and the name of each host veth device:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo iptables -S FORWARD; ip -brief addr show type veth'
   ```

2. Write the two rules for each host veth device that step 1 names:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo iptables -A FORWARD -o <veth> -m state --state RELATED,ESTABLISHED -j ACCEPT && sudo iptables -A FORWARD -i <veth> -j ACCEPT'
   ```

3. Confirm the path from inside each namespace:
   ```sh
   ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'sudo ip netns exec ns-<id> ping -c1 -W3 1.1.1.1'
   ```

A rollback to a build that holds `HYDRASCALE-FWD` needs no such step. That daemon writes
the chain and the jump rule on its first tick, and it writes the masquerade rule of a
namespace that lost it.

## Status only

```sh
ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" 'systemctl is-active hydrascale; /usr/local/bin/hydrascale version'
```
