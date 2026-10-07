# Remote access

The daemon serves the control API on the control socket `/var/lib/hydrascale/api.sock`. The
socket is root only by default: the mode is `0600`, and no other account traverses
`/var/lib/hydrascale`. A client that is not root needs group access to the socket. A client
on another machine reaches the socket over an SSH forward, and the SSH account needs the
same group access.

## Give a group access to the socket

**Warning — membership of `socket_group` is equivalent to root access.** A member of the
group sends a command to the daemon, and the daemon runs as root. The member creates a
namespace, writes a host route, and runs a command as root. Name a group that holds only
trusted operators.

The daemon refuses the root group. When `socket_group` names a group with the group
identifier 0, the daemon opens no control socket and writes the error to the log.

Create the group, add the account to it, and name the group in the configuration file:

```bash
sudo groupadd hydrascale
sudo usermod -aG hydrascale "$USER"    # the change applies at the next login
# set `socket_group: hydrascale` in the configuration file, then restart the daemon
sudo systemctl restart hydrascale
```

`hydrascale init` offers this as a step. With `socket_group` set, the daemon gives the
socket the owner `root:hydrascale` and the mode `0660`. It gives the directory
`/var/lib/hydrascale` the same owner and the mode `0750`. A member of the group then reaches
the control API without root.

## Reach the socket from another machine

Forward the socket over SSH, then send the request to the forwarded path. The SSH account
must be a member of the socket group on the host:

```bash
ssh -L /tmp/hydrascale.sock:/var/lib/hydrascale/api.sock user@linux-host
curl --unix-socket /tmp/hydrascale.sock http://unix/api/status
```

## Reach the console from another machine

The console has no authentication, so the SSH forward is the only control on this path.
Read [The console has no authentication](../security/console.md) first.

Forward the console port:

```bash
ssh -L 9443:127.0.0.1:9443 user@linux-host
```

The local port can be any free port. The daemon accepts an `Origin` header that names a
loopback host on any port.
