<p align="center">
  <img src="../internal/ui/static/brand/logo-lime.svg" alt="The Hydrascale mark" width="96">
</p>

<h1 align="center">The Hydrascale documents</h1>

<p align="center">Run multiple Tailscale tailnets simultaneously on a single Linux machine.</p>

<p align="center">
  <a href="https://github.com/Crank-Git/Hydrascale/actions/workflows/ci.yml"><img src="https://github.com/Crank-Git/Hydrascale/actions/workflows/ci.yml/badge.svg?branch=dev" alt="The state of continuous integration"></a>
  <img src="https://img.shields.io/badge/go-1.26-8d867d" alt="The Go version">
  <img src="https://img.shields.io/badge/platform-linux-8d867d" alt="The platform">
  <a href="../LICENSE"><img src="https://img.shields.io/badge/license-MIT-8d867d" alt="The license"></a>
</p>

The [README](../README.md) states what Hydrascale does and how to run it. This directory
holds the documents that go further than the README.

## The documents

| Document | Holds |
|---|---|
| [DESIGN.md](DESIGN.md) | The brand of the console: the tokens, the palette, the typography, the marks, and the drawing rules. |
| [UPGRADING.md](UPGRADING.md) | The upgrade to version 1.0 for an operator who runs version 0.9 or version 0.10. |
| [dns-investigation.md](dns-investigation.md) | The measurement of the host `resolv.conf` defect, and the reason for the overlay mount on `/etc`. |
| [security-audit.md](security-audit.md) | The audit of the daemon, with one entry per finding and the state of each fix. |
| [specs/](specs/) | The specification of version 1.0. |
| [images/](images/) | The screenshots of the console and the architecture diagram that the README draws. |

## The specification

[specs/spec.md](specs/spec.md) is the whole specification. It states the terms, the
requirements, and the changelog of every scope decision.
[specs/spec.html](specs/spec.html) is the same text as one page that a browser reads.

[specs/features/](specs/features/) holds one file per feature. Read the file of a feature
before you build it.

| File | Feature |
|---|---|
| [00-foundation.md](specs/features/00-foundation.md) | The reconciler, the namespaces, and the configuration file. |
| [01-desktop-client-removal.md](specs/features/01-desktop-client-removal.md) | The removal of the desktop client. |
| [02-security-audit.md](specs/features/02-security-audit.md) | The audit that finds the defects. |
| [03-security-fixes.md](specs/features/03-security-fixes.md) | The fix of each audit finding. |
| [04-dns-integrity.md](specs/features/04-dns-integrity.md) | The overlay mount and the unified resolver. |
| [05-reachability-model.md](specs/features/05-reachability-model.md) | The local rule model and the iptables chains. |
| [06-console-foundation.md](specs/features/06-console-foundation.md) | The console shell, the views, and the loopback listener. |
| [07-console-access-editor.md](specs/features/07-console-access-editor.md) | The Access view and its drawing rules. |
| [08-upstream-policy.md](specs/features/08-upstream-policy.md) | The Tailscale and Headscale policy clients. |
| [09-docs-and-release.md](specs/features/09-docs-and-release.md) | The documents and the release. |

[specs/mockups/](specs/mockups/) holds the mockups that the specification refers to.
