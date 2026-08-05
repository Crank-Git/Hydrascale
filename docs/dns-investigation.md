# DNS investigation — the host `resolv.conf` defect

Issue #75. Epic #74. The attempt ran on 2026-08-05 against the test host
`phobos@192.168.1.221` (hostname `mars`).

Issue #28 reported that a namespace `tailscaled` replaced the host `/etc/resolv.conf`.
Pull request #30 added a per-namespace overlay on `/etc`. The maintainer believes the
host file still changes. This document records the attempt to reproduce that change, the
commands, and the outcome.

## Verdict

**The defect does not reproduce.** The overlay on `/etc` is mounted for both namespaces.
The host `/etc/resolv.conf` holds the same content and the same checksum before the
observation, across a host restart, and after the observation. Each namespace
`tailscaled` wrote its `resolv.conf` into its own overlay upper directory.

None of the three candidate causes in the issue body matches. Section
[Candidate causes](#candidate-causes) states the evidence for each.

The observation found one true defect that is not a `resolv.conf` change: the daemon
sends the child standard error to `/dev/null`, so the diagnostic message at
`cmd/hydrascale/nsdaemon.go:56` reaches no log. Section
[A second defect](#a-second-defect-the-diagnostic-message-reaches-no-log) records it.

## The issue body is wrong about the immutable attribute

The `## Warning` section of issue #75 says the test host `/etc/resolv.conf` is immutable.
It instructs the worker to run `sudo chattr -i /etc/resolv.conf`. That instruction is
wrong.

`/etc/resolv.conf` is a symbolic link to `/run/systemd/resolve/stub-resolv.conf`.
`systemd-resolved` owns the target. `lsattr` fails on the path because the path is a
symbolic link, not because a flag is set. The target carries no attribute.

```
$ ssh phobos@192.168.1.221 'ls -l /etc/resolv.conf'
lrwxrwxrwx 1 root root 37 Jul  8 17:23 /etc/resolv.conf -> /run/systemd/resolve/stub-resolv.conf

$ ssh phobos@192.168.1.221 'lsattr /etc/resolv.conf; echo "lsattr_rc=$?"'
lsattr: Operation not supported While reading flags on /etc/resolv.conf
lsattr_rc=1

$ ssh phobos@192.168.1.221 'lsattr -d /run/systemd/resolve/stub-resolv.conf'
---------------------- /run/systemd/resolve/stub-resolv.conf
```

The attribute field is `----------------------`. No attribute is set, and the immutable
flag `i` is absent.

**No `chattr` command ran. No attribute was cleared. No attribute was restored.** The
worker changed no attribute, because there was none to change.

The `## Changelog` table of `docs/specs/spec.md` still records the immutable claim. That
record is wrong and it needs a correction.

## The host and its resolver

```
$ ssh phobos@192.168.1.221 'uname -a; lsb_release -d'
Linux mars 6.17.0-35-generic #35~24.04.1-Ubuntu SMP PREEMPT_DYNAMIC Tue May 26 19:30:42 UTC 2 x86_64 x86_64 x86_64 GNU/Linux
Description:	Ubuntu 24.04.4 LTS

$ ssh phobos@192.168.1.221 'systemctl is-active systemd-resolved'
active

$ ssh phobos@192.168.1.221 'resolvectl status | head -3'
Global
         Protocols: -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
  resolv.conf mode: stub
```

`resolv.conf mode: stub` is the healthy state. The operator boundary of 2026-08-05
forbids a change to `systemd-resolved`. The worker read its state and it changed nothing.

## Start state

The worker recorded the host state before the observation.

```
$ ssh phobos@192.168.1.221 'systemctl is-active hydrascale; sha256sum /usr/local/bin/hydrascale; ip netns list; readlink -f /etc/resolv.conf'
active
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale
ns-havoc (id: 1)
ns-jbones (id: 0)
/run/systemd/resolve/stub-resolv.conf
```

No build was deployed. The daemon already ran with two tailnets, so step 2 of the issue
body was already true.

## The host `/etc/resolv.conf`

```
$ ssh phobos@192.168.1.221 'ls -l /run/systemd/resolve/stub-resolv.conf; sha256sum /etc/resolv.conf; cat /etc/resolv.conf'
-rw-r--r-- 1 systemd-resolve systemd-resolve 920 Aug  4 22:01 /run/systemd/resolve/stub-resolv.conf
ebdf560272a77357195c39e98340b77e18c8a8ce2025ee950e9e0c7b01467ab8  /etc/resolv.conf
# This is /run/systemd/resolve/stub-resolv.conf managed by man:systemd-resolved(8).
# Do not edit.
#
# This file might be symlinked as /etc/resolv.conf. If you're looking at
# /etc/resolv.conf and seeing this text, you have followed the symlink.
#
# This is a dynamic resolv.conf file for connecting local clients to the
# internal DNS stub resolver of systemd-resolved. This file lists all
# configured search domains.
#
# Run "resolvectl status" to see details about the uplink DNS servers
# currently in use.
#
# Third party programs should typically not access this file directly, but only
# through the symlink at /etc/resolv.conf. To manage man:resolv.conf(5) in a
# different way, replace this symlink by a static file or a different symlink.
#
# See man:systemd-resolved.service(8) for details about the supported modes of
# operation for /etc/resolv.conf.

nameserver 127.0.0.53
options edns0 trust-ad
search .
```

`nameserver 127.0.0.53` is the `systemd-resolved` stub address. It is not
`100.100.100.100`. A namespace `tailscaled` did not replace this file.

## The host restarted during the observation

At 00:56 the test host restarted. The worker did not command the restart. Every command
the worker ran was a read: `ls`, `readlink`, `cat`, `head`, `sha256sum`, `lsattr`,
`ip netns exec ... cat`, `grep` on `/proc/<pid>/mountinfo`, `ps`, `systemctl is-active`,
and `journalctl`. The worker names no cause for the restart.

```
$ ssh phobos@192.168.1.221 'uptime; who -b; last -x reboot | head -4'
 00:58:49 up 2 min,  3 users,  load average: 1.06, 0.66, 0.28
         system boot  2026-08-05 00:56
reboot   system boot  6.17.0-35-generi Wed Aug  5 00:56   still running
reboot   system boot  6.17.0-35-generi Tue Aug  4 22:01   still running
reboot   system boot  6.17.0-35-generi Tue Aug  4 15:35   still running
reboot   system boot  6.17.0-35-generi Mon Aug  3 06:01   still running
```

`last -x reboot` holds no matching `shutdown` record for any boot. The restart was
therefore not a clean shutdown. This matches an earlier unexplained lock of the same host
class during a multi-tailnet start.

The restart improved the evidence. It gave a clean start of the daemon and of both
namespaces, and the worker measured the host file across that start. The host file
checksum is the same value before the restart and after it.

## The overlay is mounted for each namespace

The daemon started the two namespace daemons at 00:56:09.

```
$ ssh phobos@192.168.1.221 'sudo journalctl -u hydrascale -b --no-pager | head -20'
Aug 05 00:56:09 mars systemd[1]: Started hydrascale.service - Hydrascale - Multi-Tailnet Manager.
Aug 05 00:56:09 mars hydrascale[2135]: Hydrascale daemon starting (reconcile every 10s)...
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [reconcile_start]
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 api: removed stale socket /var/lib/hydrascale/api.sock
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 api: socket group access enabled for "hydrascale"
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 api: listening on /var/lib/hydrascale/api.sock
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [reconcile_apply] 4 actions
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Created namespace: ns-jbones
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Set up veth pair for namespace ns-jbones with IPs from 10.200.0.0/16
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Set up host access rules for namespace ns-jbones
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [action_ok] jbones: create_namespace
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 tailscaled started in namespace "ns-jbones" (PID 2214)
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [action_ok] jbones: start_daemon
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Created namespace: ns-havoc
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Set up veth pair for namespace ns-havoc with IPs from 10.200.0.0/16
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 Set up host access rules for namespace ns-havoc
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [action_ok] havoc: create_namespace
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 tailscaled started in namespace "ns-havoc" (PID 2280)
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [action_ok] havoc: start_daemon
Aug 05 00:56:09 mars hydrascale[2135]: 2026/08/05 00:56:09 [reconcile_complete] applied 4 actions
```

```
$ ssh phobos@192.168.1.221 'ps -eo pid,lstart,args | grep [t]ailscaled'
   1074 Wed Aug  5 00:56:03 2026 /usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/run/tailscale/tailscaled.sock --port=41641
   2214 Wed Aug  5 00:56:08 2026 tailscaled --state=/var/lib/hydrascale/state/jbones/tailscaled.state --socket=/var/lib/hydrascale/state/jbones/tailscaled.sock --statedir=/var/lib/hydrascale/state/jbones
   2280 Wed Aug  5 00:56:09 2026 tailscaled --state=/var/lib/hydrascale/state/havoc/tailscaled.state --socket=/var/lib/hydrascale/state/havoc/tailscaled.sock --statedir=/var/lib/hydrascale/state/havoc
```

PID 1074 is the host `tailscaled`. PID 2214 and PID 2280 are the namespace daemons.

```
$ ssh phobos@192.168.1.221 'for p in 2214 2280; do echo "--- pid $p ---"; sudo grep -w "/etc" /proc/$p/mountinfo; done'
--- pid 2214 ---
3709 3699 259:2 /etc/netns/ns-jbones/resolv.conf /run/systemd/resolve/stub-resolv.conf rw,relatime master:1 - ext4 /dev/nvme0n1p2 rw
4045 3693 0:74 / /etc rw,relatime - overlay overlay rw,lowerdir=/etc,upperdir=/var/lib/hydrascale/state/jbones/etc-upper,workdir=/var/lib/hydrascale/state/jbones/etc-work,uuid=on,nouserxattr
--- pid 2280 ---
4065 4053 259:2 /etc/netns/ns-havoc/resolv.conf /run/systemd/resolve/stub-resolv.conf rw,relatime master:1 - ext4 /dev/nvme0n1p2 rw
4068 4047 0:77 / /etc rw,relatime - overlay overlay rw,lowerdir=/etc,upperdir=/var/lib/hydrascale/state/havoc/etc-upper,workdir=/var/lib/hydrascale/state/havoc/etc-work,uuid=on,nouserxattr
```

Each namespace daemon holds an `overlay` mount on `/etc`. The upper directory is
namespace-local. The overlay mount succeeded for both namespaces.

The same two mounts were present before the restart, under the earlier process
identifiers 95840 and 95986.

```
$ ssh phobos@192.168.1.221 'for p in 95840 95986; do echo "--- pid $p ---"; sudo grep -w "/etc" /proc/$p/mountinfo; done'
--- pid 95840 ---
373 277 259:2 /etc/netns/ns-jbones/resolv.conf /run/systemd/resolve/stub-resolv.conf rw,relatime master:1 - ext4 /dev/nvme0n1p2 rw
379 85 0:42 / /etc rw,relatime - overlay overlay rw,lowerdir=/etc,upperdir=/var/lib/hydrascale/state/jbones/etc-upper,workdir=/var/lib/hydrascale/state/jbones/etc-work,uuid=on,nouserxattr
--- pid 95986 ---
458 435 259:2 /etc/netns/ns-havoc/resolv.conf /run/systemd/resolve/stub-resolv.conf rw,relatime master:1 - ext4 /dev/nvme0n1p2 rw
461 382 0:73 / /etc rw,relatime - overlay overlay rw,lowerdir=/etc,upperdir=/var/lib/hydrascale/state/havoc/etc-upper,workdir=/var/lib/hydrascale/state/havoc/etc-work,uuid=on,nouserxattr
```

## The namespace write lands in the upper directory

Each namespace `tailscaled` replaced `/etc/resolv.conf` inside its own mount namespace at
00:56:16. The result is in the overlay upper directory and it is not on the host.

```
$ ssh phobos@192.168.1.221 'sudo ls -la --time-style=full-iso /var/lib/hydrascale/state/jbones/etc-upper'
total 12
drwxr-xr-x 2 root root 4096 2026-08-05 00:56:16.246771683 -0400 .
drwx------ 6 root root 4096 2026-08-05 00:56:10.099458128 -0400 ..
-rw-r--r-- 1 root root  257 2026-08-05 00:56:16.246771683 -0400 resolv.conf
lrwxrwxrwx 1 root root   37 2026-07-08 17:23:46.457919546 -0400 resolv.pre-tailscale-backup.conf -> /run/systemd/resolve/stub-resolv.conf

$ ssh phobos@192.168.1.221 'sudo cat /var/lib/hydrascale/state/jbones/etc-upper/resolv.conf'
# resolv.conf(5) file generated by tailscale
# For more info, see https://tailscale.com/s/resolvconf-overwrite
# DO NOT EDIT THIS FILE BY HAND -- CHANGES WILL BE OVERWRITTEN

nameserver 100.100.100.100
nameserver fd7a:115c:a1e0::53
search tailfe323c.ts.net

$ ssh phobos@192.168.1.221 'sudo cat /var/lib/hydrascale/state/havoc/etc-upper/resolv.conf'
# resolv.conf(5) file generated by tailscale
# For more info, see https://tailscale.com/s/resolvconf-overwrite
# DO NOT EDIT THIS FILE BY HAND -- CHANGES WILL BE OVERWRITTEN

nameserver 100.100.100.100
nameserver fd7a:115c:a1e0::53
search taildf854a.ts.net

$ ssh phobos@192.168.1.221 'sudo sha256sum /var/lib/hydrascale/state/jbones/etc-upper/resolv.conf /var/lib/hydrascale/state/havoc/etc-upper/resolv.conf'
2a0e26779e6cbb78e81a8a290a28844c0a6514449109a0ca986fa70bb061e3d9  /var/lib/hydrascale/state/jbones/etc-upper/resolv.conf
e2fc170f96e6ad14362ec062d6481b241d13ff38407bccb46af292ee906a0958  /var/lib/hydrascale/state/havoc/etc-upper/resolv.conf
```

Three facts confirm that the overlay contains the write:

- The search domain differs per namespace, `tailfe323c.ts.net` and `taildf854a.ts.net`.
  Each file therefore comes from a different tailnet.
- `resolv.pre-tailscale-backup.conf` is the original symbolic link. `tailscaled` read the
  host symbolic link through the overlay lower directory and it moved the link into the
  upper directory.
- Both files carry `nameserver 100.100.100.100`, and the host file does not.

## The checksum of the host file before and after each namespace start

`systemd-resolved` wrote the stub file at 00:56:02, which is 7 seconds before the daemon
started the first namespace daemon at 00:56:09.

```
$ ssh phobos@192.168.1.221 'ls -l --time-style=full-iso /run/systemd/resolve/stub-resolv.conf; sha256sum /etc/resolv.conf; tail -4 /etc/resolv.conf'
-rw-r--r-- 1 systemd-resolve systemd-resolve 920 2026-08-05 00:56:02.111959439 -0400 /run/systemd/resolve/stub-resolv.conf

ebdf560272a77357195c39e98340b77e18c8a8ce2025ee950e9e0c7b01467ab8  /etc/resolv.conf
nameserver 127.0.0.53
options edns0 trust-ad
search .
```

The modification time did not advance past 00:56:02. Both namespace daemons started
after that time and both wrote their `resolv.conf` at 00:56:16. The host file therefore
did not change when a namespace started.

The checksum `ebdf560272a77357195c39e98340b77e18c8a8ce2025ee950e9e0c7b01467ab8` is the
same value at three points: before the restart, after the restart, and at the end of the
observation.

## `/etc/netns/<ns>/resolv.conf` changes, and that change is correct

The modification time of `/etc/netns/<ns>/resolv.conf` advances often. The content does
not change.

```
$ ssh phobos@192.168.1.221 'sudo ls -la --time-style=full-iso /etc/netns/ns-havoc /etc/netns/ns-jbones; sudo sha256sum /etc/netns/ns-havoc/resolv.conf /etc/netns/ns-jbones/resolv.conf; sudo cat /etc/netns/ns-havoc/resolv.conf'
/etc/netns/ns-havoc:
total 12
drwxr-xr-x 2 root root 4096 2026-07-08 09:02:32.140497640 -0400 .
drwxr-xr-x 5 root root 4096 2026-07-09 16:47:22.357190817 -0400 ..
-rw-r--r-- 1 root root   19 2026-08-05 01:03:10.180179760 -0400 resolv.conf

/etc/netns/ns-jbones:
total 12
drwxr-xr-x 2 root root 4096 2026-07-08 16:05:31.411500865 -0400 .
drwxr-xr-x 5 root root 4096 2026-07-09 16:47:22.357190817 -0400 ..
-rw-r--r-- 1 root root   19 2026-08-05 01:03:10.078179590 -0400 resolv.conf

d9fcf7783d7b0b51869ef66ed4febbf50f514057108278e330b1e84d8aeb33e0  /etc/netns/ns-havoc/resolv.conf
d9fcf7783d7b0b51869ef66ed4febbf50f514057108278e330b1e84d8aeb33e0  /etc/netns/ns-jbones/resolv.conf
nameserver 1.1.1.1
```

`internal/reconciler/reconciler.go:299` calls `namespaces.WriteNamespaceResolvConf` on
every pass. `internal/namespaces/ns.go:400` writes the file each time. The write is
idempotent and the content is stable. This is the intended behaviour of issue #22 and it
is not the defect of issue #28.

## A caution about `ip netns exec ... cat /etc/resolv.conf`

The issue brief proposes `ip netns exec <ns> cat /etc/resolv.conf` as a test of the
overlay. That command does not read what `tailscaled` reads.

```
$ ssh phobos@192.168.1.221 'sudo ip netns exec ns-havoc cat /etc/resolv.conf; sudo ip netns exec ns-havoc sha256sum /etc/resolv.conf'
nameserver 1.1.1.1
d9fcf7783d7b0b51869ef66ed4febbf50f514057108278e330b1e84d8aeb33e0  /etc/resolv.conf
```

`ip netns exec` creates a new mount namespace and it bind-mounts
`/etc/netns/<ns>/resolv.conf`. The overlay belongs to the mount namespace of the running
`tailscaled` process, so the new mount namespace does not hold it. The output above is
`/etc/netns/ns-havoc/resolv.conf`, not the overlay file.

Read `/proc/<pid>/mountinfo` to test the overlay. That is the reliable test, and this
document uses it.

The value still supports the verdict. The result differs from the host file, so the
namespace does not read the host file. The checksum `d9fcf778...` is not the host
checksum `ebdf5602...`.

## Candidate causes

The issue body lists three candidates. The evidence matches none of them.

### 1. The silent failure path at `cmd/hydrascale/nsdaemon.go:56` — no match

`cmd/hydrascale/nsdaemon.go:53-57` holds the path:

```go
if err := syscall.Mount("overlay", "/etc", "overlay", 0, opts); err != nil {
	// Don't fail the daemon over the shield; log and continue so the
	// tailnet still comes up (falls back to pre-#28 behaviour).
	fmt.Fprintf(os.Stderr, "hydrascale __nsdaemon: overlay /etc failed: %v (continuing without host-DNS shield)\n", err)
}
```

The mount succeeded for both namespaces, so this branch did not run. The `mountinfo`
evidence above proves the mount.

### 2. A host on which the overlay mount cannot succeed — no match

The test host mounts the overlay. The kernel is `6.17.0-35-generic` and the file system
is `ext4` on `/dev/nvme0n1p2`. The mount options record `uuid=on,nouserxattr`, which the
kernel selected without an error.

### 3. A second cause — no match for a change to the host `/etc/resolv.conf`

The host file is unchanged. No third mechanism changed it during the observation.

**None of the three candidates matches.** The overlay of pull request #30 works on this
host.

## A second defect: the diagnostic message reaches no log

The observation found a real defect, but it is a defect of visibility and not of DNS.

`internal/daemon/daemon.go:149-151` launches the helper and it discards the child output:

```go
cmd := exec.Command("ip", args...)
cmd.Stdout = nil
cmd.Stderr = nil
```

In `os/exec`, a nil `Stderr` connects the child standard error to the null device. The
message that `cmd/hydrascale/nsdaemon.go:56` writes therefore goes to `/dev/null`. It
reaches no journal and no operator.

The journal confirms this. The daemon log holds no occurrence of the message, across
every retained boot:

```
$ ssh phobos@192.168.1.221 'sudo journalctl -u hydrascale --no-pager | grep -c -i -E "overlay|shield|__nsdaemon"'
0
```

That count of `0` is not evidence that the mount succeeded. The message cannot appear
even when the mount fails. The `mountinfo` evidence, and only that evidence, proves the
mount.

The consequence: on a host where the overlay mount fails, the daemon falls back to the
behaviour before issue #28, the host `/etc/resolv.conf` is at risk, and the operator sees
nothing. That is the shape of the defect the maintainer reports, and this host does not
show it because the mount succeeds here.

This document does not fix the defect. It records it, so that Epic #74 can carry a fix.

## The defect does not reproduce — what a reproduction needs

FR-dns-16 requires this statement. **The defect did not reproduce on the test host.**

A reproduction needs a host, or a condition, on which `syscall.Mount("overlay", "/etc",
...)` fails. Each of the following produces that condition. Each one needs the operator
to decide, because the operator boundary of 2026-08-05 forbids a change to the resolver
and to the symbolic link.

- **A file system that cannot serve as an overlay upper directory.** Place
  `/var/lib/hydrascale/state` on `overlayfs`, on `ZFS`, or on `tmpfs` with the wrong
  options. The kernel rejects a nested or unsupported upper directory, and the mount
  fails.
- **A kernel without `CONFIG_OVERLAY_FS`.** Some appliance kernels and some container
  hosts omit it. `syscall.Mount` then returns `ENODEV`.
- **A restricted user namespace.** Run the daemon where `CAP_SYS_ADMIN` is absent in the
  mount namespace. The mount returns `EPERM`.
- **A fault injected into the helper.** Add a test hook that forces the mount to fail,
  then start a namespace and measure the host file. This needs no second host, and it
  tests the failure path directly.

The fourth option is the cheapest and it is the most controlled. It reproduces the
failure path in a test rather than on a live host.

A second, separate condition also needs the operator to decide. The maintainer reports a
change on a host that this observation did not see. That host may differ from `mars` in
one of these:

- `/etc/resolv.conf` is a real file, not a symbolic link into `/run`.
- `systemd-resolved` is absent and another program owns `/etc/resolv.conf`.
- The state directory is on a file system that refuses the overlay.

Ask the maintainer for `readlink -f /etc/resolv.conf`, for `findmnt -T
/var/lib/hydrascale/state`, and for `grep -w /etc /proc/<pid>/mountinfo` on the affected
host. Those three values identify which condition applies.

## End state

The worker recorded the host state at the end of the observation.

```
$ ssh phobos@192.168.1.221 'systemctl is-active hydrascale; sha256sum /usr/local/bin/hydrascale; ip netns list; readlink -f /etc/resolv.conf; sha256sum /etc/resolv.conf'
active
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale
ns-havoc (id: 1)
ns-jbones (id: 0)
/run/systemd/resolve/stub-resolv.conf
ebdf560272a77357195c39e98340b77e18c8a8ce2025ee950e9e0c7b01467ab8  /etc/resolv.conf
```

The four values match the start state. The host `/etc/resolv.conf` checksum also matches. The host restarted at 00:56 for a cause the worker
did not command and does not know. The worker deployed no build, wrote no file on the
host, stopped no unit, and mounted nothing.
