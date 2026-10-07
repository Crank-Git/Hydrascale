# Requirements

The host needs these things:

- **Linux.** A network namespace is a feature of the Linux kernel.
- **Root.** The daemon creates namespaces, mounts an overlay on `/etc`, and writes
  iptables rules. `hydrascale init` refuses to run as any other account.
- **Tailscale.** The commands `tailscaled` and `tailscale` must be in `$PATH`.
- **iproute2.** The daemon runs `ip` to manage a namespace.
- **iptables and ip6tables.** The daemon writes the NAT rules and the forward rules of
  both address families. The `iptables` package of each distribution holds both
  commands.
- **Kernel support for network namespaces** (`CONFIG_NET_NS`). Every current kernel
  holds it.
- **Kernel policy routing** (`CONFIG_IP_MULTIPLE_TABLES`, `CONFIG_IPV6_MULTIPLE_TABLES`).
  [Host access](../guides/host-access.md) needs it to propagate an accepted subnet route
  to the host. Most distribution kernels hold it. Some single-board kernels omit it, and
  route propagation then changes nothing.
- **IP forwarding.** Run `sudo sysctl -w net.ipv4.ip_forward=1`. The preflight checks of
  `hydrascale init` read this value and offer to set it. See
  [IP forwarding](../concepts/networking.md#ip-forwarding).
- **IPv6, if you want it.** The host needs an IPv6 default route, and Linux 6.17 or
  later. An older kernel needs the key `ipv6: true`. See
  [IPv6](../concepts/networking.md#ipv6).
- **Go 1.26 or later**, for a build from source only.

## Next step

Read [Install](install.md).
