# Uninstall

`hydrascale uninstall` removes Hydrascale from the host:

```bash
sudo hydrascale uninstall
```

The command does these steps in this order:

1. It logs each tailnet node out while its `tailscaled` still runs, so no node stays in the
   admin console of the tailnet. `--keep-nodes` skips this step.
2. It stops and disables the systemd service. The shutdown of the daemon stops each
   `tailscaled` and removes the local rule chains.
3. It deletes the namespaces, the veth pairs, the iptables and ip6tables rules, the host
   routes, and the DNS entries.
4. It removes `/var/lib/hydrascale`, `/var/log/hydrascale`, and the unit file
   `/etc/systemd/system/hydrascale.service`.

The command finds the tailnets from the namespaces on the host, and it does not change the
configuration file.

A plain `uninstall` keeps the binary and `/etc/hydrascale`, so the command can run again.

| Flag | Effect |
|---|---|
| `--keep-nodes` | Keeps each tailnet node. The command logs no node out. |
| `--purge` | Also removes `/usr/local/bin/hydrascale` and `/etc/hydrascale`. |
| `--yes` | Skips the confirmation. |

If a teardown step fails, `uninstall` lists each failure and exits non-zero. It then keeps
`/var/lib/hydrascale` and the service unit. Correct the failure and run the command again.
