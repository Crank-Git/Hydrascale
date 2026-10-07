# Hydrascale

Hydrascale lets one Linux host join more than one tailnet at the same time. The daemon
creates one namespace for each tailnet and runs a separate `tailscaled` in each
namespace. The reconciler compares the live host with the configuration file and acts on
the difference. The DNS forwarder resolves the names of every tailnet.

To install the daemon, read [Get started](get-started/index.md).
