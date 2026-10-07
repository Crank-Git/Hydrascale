# Concepts

This section states how the daemon works: the local rules and their two modes, the
upstream policy, DNS, and networking.

| Page | States |
|---|---|
| [Local rules](local-rules.md) | The rule set that the host enforces with iptables, and the modes `enforce` and `observe`. |
| [Upstream policy](upstream-policy.md) | The policy that each control server holds, and how the daemon reads, validates, and writes it. |
| [DNS](dns.md) | The two host DNS modes, split DNS, the short names, and the overlay mount on `/etc`. |
| [Networking](networking.md) | IP forwarding, the veth pairs, NAT, the listen port of each namespace, IPv6, and Docker. |
