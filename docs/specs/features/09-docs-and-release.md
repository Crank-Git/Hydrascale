---
id: docs-and-release
feature: Documentation, terminal interface restyle, and the release
epic: "Epic 9: Documentation, terminal interface restyle, and the release"
status: issued
issues: [161, 162, 163, 164, 165]
mockups: []
---

## Purpose

Version 1.0 changes what the product is. The documentation must change with it. The
terminal interface must match the brand, because it is one of the three surfaces that the
operator keeps.

This feature set also runs the release. A release of a project with existing users needs
an upgrade note that states what changed and what an operator must do.

## User stories

- As a new reader, I want the first screen of the README to tell me what Hydrascale does
  and to show the mark, so that I decide in ten seconds.
- As an existing operator, I want an upgrade note that states what changed, so that an
  upgrade does not surprise me.
- As a contributor, I want a design document, so that my pull request matches the brand.
- As the operator, I want the terminal interface to use the same palette as the console,
  so that the product looks like one product.

## Functional requirements

### The README

- **FR-docs-1** — `README.md` shows the new mark rather than `assets/logo.png`.
- **FR-docs-2** — `README.md` describes the console, the local rules, and the upstream
  policy feature.
- **FR-docs-3** — `README.md` contains no desktop client section.
- **FR-docs-4** — `README.md` states that the console has no authentication and that it
  binds a loopback address.
- **FR-docs-5** — `README.md` states that `socket_group` membership gives full control of
  the daemon.
- **FR-docs-6** — `README.md` documents every new configuration key: `console`, `access`,
  `secrets_file`, and `dns.allow_unprotected`.
- **FR-docs-7** — `README.md` documents the credential setup for Tailscale and for
  Headscale.
- **FR-docs-8** — `README.md` states that a Headscale control server needs
  `policy.mode: "db"` for a policy write.
- **FR-docs-9** — The table of contents matches the sections.

### The design document

- **FR-docs-10** — `docs/DESIGN.md` states the palette, the typography, the shape scale,
  the motion rules, and the blueprint edge rules.
- **FR-docs-11** — `docs/DESIGN.md` states the access-control drawing rules: one source
  at a time, no denied path, no arrowhead, no edge label.
- **FR-docs-12** — `docs/DESIGN.md` names no tool that generated it and no private
  document.

### The upgrade note

- **FR-docs-13** — `docs/UPGRADING.md` states what version 1.0 removes.
- **FR-docs-14** — `docs/UPGRADING.md` states that a version 0.9 configuration file loads
  without an edit.
- **FR-docs-15** — `docs/UPGRADING.md` states that the daemon writes an `access` block on
  first start and keeps a backup of the previous configuration file.
- **FR-docs-16** — `docs/UPGRADING.md` states how to check the new rules before
  enforcement, with `access.mode: observe`.
- **FR-docs-17** — `docs/UPGRADING.md` states that the desktop client is gone and that
  the version 0.9 release still carries it.

### The terminal interface

- **FR-docs-18** — `internal/tui/styles.go` uses the brand palette.
- **FR-docs-19** — The terminal interface uses the accent colour for the current
  selection only.
- **FR-docs-20** — The terminal interface shows a state as a dot and a lowercase word.
- **FR-docs-21** — The terminal interface shows the local rule mode and the rule count.
- **FR-docs-22** — The terminal interface contains no emoji.

### The release

- **FR-docs-23** — The tag `v1.0.0` produces a Linux binary on the GitHub Releases page.
- **FR-docs-24** — The release note lists the breaking changes.
- **FR-docs-25** — The release note states the accepted console risk.
- **FR-docs-26** — The repository hygiene script passes before the tag.
- **FR-docs-27** — The released binary runs on the test host and serves the console.

## User flows

### The maintainer cuts the release

1. The maintainer merges `dev` into `main`.
2. The maintainer runs the repository hygiene script.
3. The maintainer runs the `verify-on-phobos` skill against `main`.
4. The maintainer confirms the console, the local rules, and the DNS state on the test
   host.
5. The maintainer tags `v1.0.0`.
6. GoReleaser builds the binary and attaches it to the release.
7. The maintainer writes the release note from `docs/UPGRADING.md`.

### An existing operator upgrades

1. The operator reads the release note.
2. The operator downloads the binary and installs it.
3. The operator sets `access.mode: observe` in the configuration file.
4. The operator restarts the service.
5. The daemon writes the `access` block and the backup file.
6. The operator uses the host for a day and reads the would-deny log lines.
7. The operator adds the rules the log shows.
8. The operator sets `access.mode: enforce` and restarts the service.

## Screens & states

The terminal interface keeps its current screens. Only the palette and two new fields
change.

| State | What changes |
|---|---|
| Tailnet list | The state dot uses the brand state colours. The selected row uses the accent colour. |
| Footer | Shows the local rule mode and the rule count. |
| Detail | The machine values keep the mono rendering that a terminal already gives them. |

## Behaviour rules

- The README opens with what the product does and who it is for. It does not open with a
  feature list.
- The upgrade note states a fact per line. It does not reassure.
- The design document is written for a contributor who has never seen the brand package.
- The terminal interface changes colour only. Version 1.0 adds no new terminal view.

## Data touched

No entity changes.

## Interfaces

None.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| The terminal does not support 24-bit colour. | Lip Gloss degrades the colour. The state dot and the word still read, because the word carries the meaning. |
| The hygiene script fails before the tag. | The maintainer fixes the violation. The tag waits. |
| GoReleaser fails on the tag. | The maintainer deletes the tag, fixes the workflow, and tags again. No release is published in a broken state. |
| An operator upgrades without reading the note. | The daemon writes the preserving rule set, so the host keeps working. The event log states what the daemon wrote. |

## Acceptance criteria

- [ ] `README.md` shows the new mark and describes the console.
- [ ] `README.md` contains no occurrence of `Wails` or `Desktop GUI`.
- [ ] `README.md` documents `console`, `access`, `secrets_file`, and
      `dns.allow_unprotected`.
- [ ] `README.md` states the console authentication position and the `socket_group` root
      equivalence.
- [ ] The table of contents matches the sections.
- [ ] `docs/DESIGN.md` exists and states the palette, the type, and the edge rules.
- [ ] `docs/DESIGN.md` names no private document and no generating tool.
- [ ] `docs/UPGRADING.md` states the removals, the configuration compatibility, and the
      observe-mode procedure.
- [ ] The terminal interface uses the brand palette and shows the rule mode.
- [ ] The repository hygiene script passes on `main`.
- [ ] The `v1.0.0` release page carries one Linux binary and no desktop client artifact.
- [ ] The released binary runs on the test host and serves the console at
      `127.0.0.1:9443`.

## Out of scope

- A documentation site. The README and two documents are enough.
- A new terminal interface view.
- A migration tool. The daemon performs the migration.

## Open questions

None.
