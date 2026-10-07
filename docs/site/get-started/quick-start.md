# Quick start

## The wizard

The interactive wizard is the shortest path. It does these steps:

1. It runs the preflight checks.
2. It writes the configuration file.
3. It asks for an auth key, or for a login in the browser.
4. It starts the first tailnet.
5. It confirms that the tailnet authenticated.

The wizard needs root. Start it:

```bash
sudo hydrascale init
```

## A configuration by hand

To configure the host without the wizard, do these steps.

1. Write a configuration file at `/etc/hydrascale/config.yaml`:

    ```yaml
    version: 2
    tailnets:
      - id: corp-prod
      - id: homelab
        exit_node: exit-us.example.com
    access:
      mode: enforce
      rules:
        - from: corp-prod
          to: internet
        - from: homelab
          to: internet
    resolver:
      mode: unified
    reconciler:
      interval: 10s
    ```

2. Apply the configuration one time:

    ```bash
    sudo hydrascale apply
    ```

3. Or run the daemon, which reconciles on every tick:

    ```bash
    sudo hydrascale serve
    ```

4. Open the console at `http://127.0.0.1:9443`.

5. Read the state:

    ```bash
    sudo hydrascale status
    ```

A tailnet needs about 20 seconds after a start of the daemon before `hydrascale status`
reports `healthy` and `running`. An earlier read reports `down` and `degraded`, which is a
normal state after a start. If a tailnet still reports `down` after 60 seconds, the
tailnet has a real failure.

The `access` block of this file sets the local rules. The mode `enforce` denies each path
that no rule allows. If the host already uses its tailnets, read
[Local rules](../concepts/local-rules.md) before you start the daemon.

## Next step

Read [Get oriented in the console](../guides/first-run.md).
