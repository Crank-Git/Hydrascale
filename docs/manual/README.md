# Hydrascale console manual

Hydrascale lets one host join several tailnets at the same time. The daemon keeps a
separate namespace for each tailnet and enforces reachability between them. The console
is the web interface that the daemon serves on the loopback address. The operator uses
the console to read the state of each tailnet, to change the local rules that the host
enforces, and to read and change the upstream policy that each control server holds.

## Contents

| Page | Covers |
|---|---|
| [Get oriented in the console](first-run.md) | The Overview, Namespaces, Access, Policy, Activity, and Settings views, on first arrival. |
| [Change a local rule in Access](access-editor.md) | Stage a local rule, enter ports, and discard the staged edit. |
| [Read the upstream policy in Policy](policy-editor.md) | The Policy tailnet list, the credential state of each tailnet, and the read-only state of a tailnet with no credential. |

## Getting started

Start with [Get oriented in the console](first-run.md). It shows the Overview view, the
view the console opens on, and it names every other view in the main navigation bar.
