# Security audit — the forwarding rules

This file is a fragment of the security audit. It answers issue #66. It covers
**FR-audit-6**. Issue #67 collects every fragment into `docs/security-audit.md`.

The identifiers run from `SA-60` to `SA-69`. Issue #67 renumbers the complete set.

This audit changes no code. Epic 3 and Epic 5 make the corrections.

## Method

The audit did **not** deploy a build. The test host already ran the daemon with two
tailnets, and the condition that this fragment examines was present. A deploy therefore
adds risk and it adds no information.

The audit observed the running host and it changed no host state. Every command is a read
of state, an ICMP echo, or one TCP connection attempt.

The test host is `phobos` at `192.168.1.221`. Its host name is `mars`. It holds two
namespaces, `ns-havoc` and `ns-jbones`, and each namespace holds one `tailscaled` that
joins a different tailnet.

### The state before the work

```
$ systemctl is-active hydrascale
active

$ sha256sum /usr/local/bin/hydrascale
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale

$ ip netns list
ns-havoc (id: 1)
ns-jbones (id: 0)
```

### The state after the work

```
$ systemctl is-active hydrascale
active

$ sha256sum /usr/local/bin/hydrascale
01e178851b522240db72796b8785310a2610820c758791b5d88cc5953ecea1cb  /usr/local/bin/hydrascale

$ ip netns list
ns-havoc (id: 1)
ns-jbones (id: 0)
```

The three values are the same before and after. The audit left the host as it found it.
The `FORWARD` chain is also the same before and after; the chain is quoted below.

## The addresses

```
$ ip -o addr show | grep -E "vh|veth"
6: vh02a1edb1c461    inet 10.200.0.165/30 scope global vh02a1edb1c461\       valid_lft forever preferred_lft forever
6: vh02a1edb1c461    inet6 fe80::607b:46ff:fef5:1172/64 scope link \       valid_lft forever preferred_lft forever
8: vh5cde1b791fe1    inet 10.200.0.85/30 scope global vh5cde1b791fe1\       valid_lft forever preferred_lft forever
8: vh5cde1b791fe1    inet6 fe80::64b9:71ff:feac:205/64 scope link \       valid_lft forever preferred_lft forever
```

```
$ sudo ip netns exec ns-havoc ip -o addr show
4: tailscale0    inet 100.121.171.43/32 scope global tailscale0\       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fd7a:115c:a1e0::fd35:ab2c/128 scope global \       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fe80::608d:bf55:5c76:88ac/64 scope link stable-privacy \       valid_lft forever preferred_lft forever
7: vn5cde1b791fe1    inet 10.200.0.86/30 scope global vn5cde1b791fe1\       valid_lft forever preferred_lft forever
7: vn5cde1b791fe1    inet6 fe80::644d:80ff:fe20:278b/64 scope link \       valid_lft forever preferred_lft forever

$ sudo ip netns exec ns-jbones ip -o addr show
4: tailscale0    inet 100.94.158.62/32 scope global tailscale0\       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fd7a:115c:a1e0::b736:9e40/128 scope global \       valid_lft forever preferred_lft forever
4: tailscale0    inet6 fe80::a8d9:914f:9014:5f82/64 scope link stable-privacy \       valid_lft forever preferred_lft forever
5: vn02a1edb1c461    inet 10.200.0.166/30 scope global vn02a1edb1c461\       valid_lft forever preferred_lft forever
5: vn02a1edb1c461    inet6 fe80::b84e:84ff:fe9d:d3ee/64 scope link \       valid_lft forever preferred_lft forever
```

| Namespace | Host side | Host address | Namespace side | Namespace address |
|---|---|---|---|---|
| `ns-havoc` | `vh5cde1b791fe1` | `10.200.0.85` | `vn5cde1b791fe1` | `10.200.0.86` |
| `ns-jbones` | `vh02a1edb1c461` | `10.200.0.165` | `vn02a1edb1c461` | `10.200.0.166` |

The addresses agree with the values that the project manager recorded before the work.

## The chain

```
$ sudo iptables -S FORWARD
-P FORWARD DROP
-A FORWARD -j ts-forward
-A FORWARD -j DOCKER-USER
-A FORWARD -j DOCKER-FORWARD
-A FORWARD -o vh5cde1b791fe1 -m state --state RELATED,ESTABLISHED -j ACCEPT
-A FORWARD -i vh5cde1b791fe1 -j ACCEPT
-A FORWARD -o vh02a1edb1c461 -m state --state RELATED,ESTABLISHED -j ACCEPT
-A FORWARD -i vh02a1edb1c461 -j ACCEPT
```

```
$ sudo iptables -L FORWARD -v -n --line-numbers
Chain FORWARD (policy DROP 0 packets, 0 bytes)
num   pkts bytes target     prot opt in     out     source               destination
1     6142  679K ts-forward  0    --  *      *       0.0.0.0/0            0.0.0.0/0
2     6142  679K DOCKER-USER  0    --  *      *       0.0.0.0/0            0.0.0.0/0
3     6144  679K DOCKER-FORWARD  0    --  *      *       0.0.0.0/0            0.0.0.0/0
4     1527  206K ACCEPT     0    --  *      vh5cde1b791fe1  0.0.0.0/0            0.0.0.0/0            state RELATED,ESTABLISHED
5     1559  142K ACCEPT     0    --  vh5cde1b791fe1 *       0.0.0.0/0            0.0.0.0/0
6     1514  183K ACCEPT     0    --  *      vh02a1edb1c461  0.0.0.0/0            0.0.0.0/0            state RELATED,ESTABLISHED
7     1550  149K ACCEPT     0    --  vh02a1edb1c461 *       0.0.0.0/0            0.0.0.0/0
```

The supporting host state is:

```
$ sysctl net.ipv4.ip_forward net.ipv4.conf.vh5cde1b791fe1.forwarding net.ipv4.conf.vh02a1edb1c461.forwarding
net.ipv4.ip_forward = 1
net.ipv4.conf.vh5cde1b791fe1.forwarding = 1
net.ipv4.conf.vh02a1edb1c461.forwarding = 1

$ sudo iptables -t nat -S POSTROUTING
-P POSTROUTING ACCEPT
-A POSTROUTING -j ts-postrouting
-A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE
-A POSTROUTING -s 10.200.0.164/30 -j MASQUERADE
-A POSTROUTING -s 10.200.0.84/30 -j MASQUERADE
```

```
$ sudo ip netns exec ns-havoc ip route show
default via 10.200.0.85 dev vn5cde1b791fe1
10.200.0.84/30 dev vn5cde1b791fe1 proto kernel scope link src 10.200.0.86
```

The namespace holds a default route to the host, the host forwards, and the host
translates the namespace source address. The two rules at positions 5 and 7 accept on the
input interface alone. They hold no destination match and no output interface match.

## The findings

### SA-60 — A namespace reaches another namespace

**Severity: high. The audit reproduced this finding on the test host.**

`internal/namespaces/ns.go:273` writes one rule for each namespace:

```
iptables -I FORWARD 1 -i vh<hash> -j ACCEPT
```

The rule accepts every packet that enters the host on that interface, to every
destination. The default route at `internal/namespaces/ns.go:261` sends every packet from
the namespace to the host, and `internal/namespaces/ns.go:266` enables forwarding on the
host side of the pair. The host route table holds a route to each other namespace.

The audit ran, in both directions:

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 10.200.0.166
PING 10.200.0.166 (10.200.0.166) 56(84) bytes of data.
64 bytes from 10.200.0.166: icmp_seq=1 ttl=63 time=0.113 ms

--- 10.200.0.166 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.113/0.113/0.113/0.000 ms
exit=0
```

```
$ sudo ip netns exec ns-jbones ping -c1 -W2 10.200.0.86
PING 10.200.0.86 (10.200.0.86) 56(84) bytes of data.
64 bytes from 10.200.0.86: icmp_seq=1 ttl=63 time=0.099 ms

--- 10.200.0.86 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.099/0.099/0.099/0.000 ms
exit=0
```

The audit also reached the host side of the other pair:

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 10.200.0.165
PING 10.200.0.165 (10.200.0.165) 56(84) bytes of data.
64 bytes from 10.200.0.165: icmp_seq=1 ttl=64 time=0.052 ms

--- 10.200.0.165 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.052/0.052/0.052/0.000 ms
exit=0
```

The first two replies carry `ttl=63`. The initial value is 64, therefore the packet
crossed one router: the host forwarded it from one veth pair to the other. The third reply
carries `ttl=64`, because `10.200.0.165` is an address of the host itself and the packet
did not cross the `FORWARD` chain.

**The mechanism.** The chain policy is `DROP`, and the packet from `ns-havoc` to
`10.200.0.166` does not match a rule that a different service owns. It matches rule 5,
`-i vh5cde1b791fe1 -j ACCEPT`, because the rule tests the input interface alone. The reply
matches rule 6, the `RELATED,ESTABLISHED` rule of the other pair. The `DROP` policy
therefore stops nothing here. The policy only stops a packet that enters on an interface
that holds no daemon rule.

**The harm condition.** A process in one namespace reaches the veth address of every other
namespace. The harm becomes a cross-tailnet harm when a service in a namespace listens on
its veth address, or when the second namespace forwards. See SA-62 for what the audit
could not demonstrate.

**Fix.** Epic 5, not Epic 3. `docs/specs/features/05-reachability-model.md`
**FR-access-1** to **FR-access-4** replace the unrestricted `ACCEPT` with the
`HYDRASCALE-FWD` chain, and the default in that chain is deny. The Epic 5 exit criterion
in `docs/specs/spec.md:464` states that "the test host proves that a namespace cannot
reach another namespace without a rule".

### SA-61 — A namespace reaches the host local network

**Severity: high. The audit reproduced this finding on the test host.**

The same rule at `internal/namespaces/ns.go:273` accepts a packet to any destination, and
`internal/namespaces/ns.go:285` translates the namespace source address to the host
address. A namespace therefore reaches the host local network as though it were the host.

The audit made one connection attempt to the local network gateway, and one to a second
known host:

```
$ sudo ip netns exec ns-havoc nc -vz -w2 192.168.1.1 22
nc: connect to 192.168.1.1 port 22 (tcp) failed: Connection refused
exit=1

$ sudo ip netns exec ns-havoc nc -vz -w2 192.168.1.215 22
Connection to 192.168.1.215 22 port [tcp/ssh] succeeded!
exit=0
```

Both results prove reachability. The gateway answered with a TCP reset, which the tool
reports as "Connection refused"; the packet therefore reached the gateway and the gateway
answered. The second host completed the TCP handshake.

**The harm condition.** A tailnet peer that gains code execution inside one namespace
reaches every host on the operator local network, with the source address of the host. The
local network sees the traffic as host traffic, therefore a local network firewall that
trusts the host trusts the namespace.

**Fix.** Epic 5, as for SA-60. `docs/specs/features/05-reachability-model.md` makes the
default deny, therefore a packet to the local network needs an explicit rule.

### SA-62 — Delivery into the second tailnet is not demonstrated

**Severity: low, because the audit did not reproduce it. The audit records the mechanism
and the uncertainty, not a result.**

SA-60 proves that the host forwards a packet from one namespace into the other. It does
not prove that the second namespace passes the packet on to its own tailnet. The audit
tried and the result is inconclusive.

```
$ sudo ip netns exec ns-havoc ping -c1 -W2 100.91.107.38
PING 100.91.107.38 (100.91.107.38) 56(84) bytes of data.

--- 100.91.107.38 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1

$ sudo ip netns exec ns-havoc ping -c1 -W2 100.114.149.115
PING 100.114.149.115 (100.114.149.115) 56(84) bytes of data.

--- 100.114.149.115 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1
```

Both addresses are peers of the `ns-jbones` tailnet, and the host route table sends both
through `ns-jbones`. Neither peer answered `ns-havoc`. **Neither peer answered `ns-jbones`
either**, therefore the negative result says nothing about containment:

```
$ sudo ip netns exec ns-jbones ping -c1 -W2 100.91.107.38
PING 100.91.107.38 (100.91.107.38) 56(84) bytes of data.

--- 100.91.107.38 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1

$ sudo ip netns exec ns-jbones ping -c1 -W2 100.114.149.115
PING 100.114.149.115 (100.114.149.115) 56(84) bytes of data.

--- 100.114.149.115 ping statistics ---
1 packets transmitted, 0 received, 100% packet loss, time 0ms
exit=1
```

A peer that does not answer its own namespace cannot show whether a different namespace
reaches it. **The audit therefore does not claim a cross-tailnet reproduction.**

The audit did establish where the packet stops. The host sends the packet into
`ns-jbones`, and `ns-jbones` does not forward:

```
$ sudo ip netns exec ns-havoc ip route get 100.91.107.38
100.91.107.38 via 10.200.0.85 dev vn5cde1b791fe1 src 10.200.0.86 uid 0
    cache

$ sudo ip netns exec ns-jbones sysctl net.ipv4.ip_forward
net.ipv4.ip_forward = 0

$ sudo ip netns exec ns-jbones iptables -S FORWARD
-P FORWARD ACCEPT
-A FORWARD -j ts-forward
```

`net.ipv4.ip_forward = 0` inside `ns-jbones` stops a forwarded packet in that namespace.
The daemon sets forwarding on the host side of the pair at
`internal/namespaces/ns.go:266`; it sets no equivalent value inside the namespace.

**The harm condition.** The containment that stops the packet is a kernel default inside
the namespace, not a rule that the daemon writes and not a rule that a test asserts. A
later change that enables forwarding inside a namespace, for a subnet router or for an
exit node, removes the containment silently and turns SA-60 into a cross-tailnet leak.

**Fix.** Epic 5 removes the dependence on the default, because the deny happens in
`HYDRASCALE-FWD` on the host before the packet reaches the second namespace. A test that
asserts the value inside the namespace is worth adding with that work.

### SA-63 — The daemon inserts at position 1, and the convention states that it appends

**Severity: medium. The audit reproduced the divergence in the code and on the host.**

`CLAUDE.md` states, under `### iptables`:

> The daemon owns the chains `HYDRASCALE-FWD` and `HYDRASCALE-OUT`, and one jump rule into
> each of `FORWARD` and `INPUT`. It writes no other rule. It appends the jump rather than
> inserting it at position 1, so an operator firewall rule keeps its position.

The code does the opposite. `internal/namespaces/ns.go:273` and
`internal/namespaces/ns.go:278` both insert:

```
iptables -I FORWARD 1 -i vh<hash> -j ACCEPT
iptables -I FORWARD 1 -o vh<hash> -m state --state RELATED,ESTABLISHED -j ACCEPT
```

There is no `HYDRASCALE-FWD` chain and no `HYDRASCALE-OUT` chain in the code. A search of
`cmd/` and `internal/` for either name returns nothing. The chains that `CLAUDE.md`
describes are the Epic 5 design, and the file states them as though they are the current
behaviour.

**The position is not stable.** The rules are at positions 4 to 7 on the test host, at the
bottom of the chain, although the daemon inserted each one at position 1. `ts-forward`,
`DOCKER-USER`, and `DOCKER-FORWARD` start after the daemon, and each one takes position 1
in turn. The position of a daemon rule therefore depends on the order in which the
services start, and it changes when a service restarts.

**The harm condition.** Two harms follow, and they are opposite. When the daemon starts
last, its unrestricted `ACCEPT` sits above an operator firewall rule and it defeats that
rule for every packet from a namespace. When the daemon starts first, a later service can
place a `DROP` above the daemon rules and break every namespace. Neither outcome is
declared, and no test covers either one.

**Fix.** Epic 5 **FR-access-2** appends the jump rule. `CLAUDE.md` describes the state
after Epic 5 lands and it needs a note that says so until then.

### SA-64 — No rule covers IPv6, and each namespace holds an IPv6 tailnet address

**Severity: medium. The audit reproduced the missing rules on the test host.**

SA-32 in `docs/security-audit/02-exec-call-sites.md` records that the daemon writes no
IPv6 firewall rule, and SA-33 records that the daemon does not log the gap at start. This
fragment adds the host evidence, and it does not repeat the analysis.

Each namespace holds an IPv6 tailnet address, quoted in **The addresses** above:
`fd7a:115c:a1e0::fd35:ab2c` in `ns-havoc` and `fd7a:115c:a1e0::b736:9e40` in `ns-jbones`.
Each veth pair holds an IPv6 link-local address. Every rule that this fragment quotes is
an `iptables` rule, therefore every rule applies to IPv4 alone.

**The harm condition.** The `HYDRASCALE-FWD` design of Epic 5 denies by default. A design
that writes `iptables` alone leaves IPv6 at the `ip6tables` policy of the host, therefore
an operator who reads the console sees a deny that IPv6 does not carry out.

**Fix.** Epic 5 for the rule set, together with SA-32 and SA-33. The audit records the
pair together so that the Epic 5 work does not repeat the IPv4 shape in `ip6tables` terms
alone.

## The answers to the acceptance criteria

| Question | Answer |
|---|---|
| Did a namespace reach another namespace? | Yes. Both directions, `ttl=63`. See SA-60. |
| Did a namespace reach the host local network? | Yes. Two hosts answered. See SA-61. |
| Did a namespace reach the second tailnet? | Not demonstrated. See SA-62. |
| Where do the daemon rules sit in `FORWARD`? | Positions 4 to 7 of 7, below `ts-forward`, `DOCKER-USER`, and `DOCKER-FORWARD`, although the code inserts each rule at position 1. See SA-63. |

The README claim that traffic from one tailnet never leaks into another holds for the
route table and for the network stack of each namespace. It does not hold for forwarding
on the host.
