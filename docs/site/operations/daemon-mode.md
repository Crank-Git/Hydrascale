# Daemon mode

## The systemd unit

`hydrascale install` writes the unit and creates the directories that the daemon needs. To
install the unit by hand:

```bash
sudo mkdir -p /var/lib/hydrascale
sudo cp contrib/hydrascale.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hydrascale
```

The unit at `contrib/hydrascale.service` runs `hydrascale serve` as root. `hydrascale install`
writes the same unit, and a test keeps the two copies equal. The unit sets no systemd
sandbox, because a sandbox stops the daemon from reaching iptables, the network
namespaces, and the mount propagation to the host. The unit restarts the daemon 5 seconds
after a failure.

## SIGHUP

SIGHUP makes the daemon read the configuration file and reconcile at once, without a wait
for the next tick. Under systemd, `systemctl reload hydrascale` sends the signal.

## A graceful shutdown

SIGINT and SIGTERM cancel the control loop. The daemon stops every tailnet process
concurrently, with a timeout of 30 seconds, and exits. The daemon records
`shutdown_complete`, or `shutdown_timeout` when the timeout ends first. See
[Events](../reference/events.md).

## Monitoring

```bash
sudo systemctl status hydrascale
sudo journalctl -u hydrascale -f
```
