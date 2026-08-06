---
id: console-access-editor
feature: Console access editor
epic: "Epic 7: Console access editor"
status: issued
issues: [148, 149, 150, 151, 152, 199]
mockups: [mockups/03-acl-editor.html]
---

## Purpose

Epic 5 gives the daemon a rule set that denies by default. This feature set gives the
operator a way to read and change that rule set by drawing it.

The access view is the reason for the release. It answers one question at a glance: which
namespace can reach which. It answers a second question on demand: on which ports.

The view has three parts. The flow overview shows the shape. The reachability matrix is
the precise editor. The rule list carries the detail that a picture cannot hold.

## User stories

- As the operator, I want to see every allowed path as one picture, so that I understand
  the host without reading iptables output.
- As the operator, I want to click a square to allow a path, so that I do not write a
  rule by hand.
- As the operator, I want to set the ports for a path, so that a rule is as narrow as the
  need.
- As the operator, I want to make several edits and then apply them together, so that the
  host does not change under me while I think.
- As the operator, I want to see what will change before it changes, so that I do not cut
  off my own connection.

## Functional requirements

### The flow overview

- **FR-editor-1** — The flow overview places tailnet nodes on the left and the
  destinations on the right.
- **FR-editor-2** — The destinations are every tailnet, the host, and the internet.
- **FR-editor-3** — The flow overview draws an allowed path as a dotted curve with a 2
  pixel dash, a 6 pixel gap, and a 1.4 pixel stroke.
- **FR-editor-4** — The flow overview draws no line for a denied path.
- **FR-editor-5** — When the operator selects a source, the overview draws that source's
  paths in the accent colour and it draws every other path in the resting edge colour.
- **FR-editor-6** — The flow overview draws no arrowhead, no node icon, and no edge
  label.

### The reachability matrix

- **FR-editor-7** — The matrix has one row per source and one column per destination.
- **FR-editor-8** — A filled square means that at least one rule allows the path.
- **FR-editor-9** — An empty square means that no rule allows the path.
- **FR-editor-10** — The diagonal is inert and it accepts no click.
- **FR-editor-11** — A click on an empty square stages a rule that allows every port.
- **FR-editor-12** — A click on a filled square stages the removal of every rule for
  that path.
- **FR-editor-13** — Hovering a square marks its row label and its column label in the
  accent colour, and it draws no other crosshair.
- **FR-editor-14** — The matrix shows no port detail.
- **FR-editor-15** — The matrix cell uses a 6 pixel corner radius, because a grid must
  read as a grid.

### The rule list

- **FR-editor-16** — The rule list shows one row per rule, with the source, a dotted
  connector, the destination, and the ports as chips.
- **FR-editor-17** — A rule with no port shows the words `all ports`.
- **FR-editor-18** — A staged rule is marked as staged.
- **FR-editor-19** — The operator can edit the ports of a rule in the row.
- **FR-editor-20** — The operator can delete a rule from the row.
- **FR-editor-21** — The rule list shows no row for a denied path.
- **FR-editor-22** — The rule list rejects a port entry that does not match the format
  that `features/05-reachability-model.md` defines, and it states the expected format.

### Staged edits and apply

- **FR-editor-23** — An edit changes the console state only. It does not reach the
  daemon.
- **FR-editor-24** — The console shows the count of staged edits.
- **FR-editor-25** — The console shows a list of the staged edits before the operator
  applies them.
- **FR-editor-26** — **Apply** sends the whole rule set with `PUT /api/access`.
- **FR-editor-27** — **Discard** returns the console to the daemon's rule set.
- **FR-editor-28** — The console warns the operator when a staged edit removes the path
  that the operator's own connection uses. The warning reads the field `active_paths` of
  `GET /api/access`. The warning names each path, and it comes above the staged list and
  above the apply action.
- **FR-editor-29** — After a successful apply, the console clears the staged edits and
  polls the daemon.
- **FR-editor-30** — When apply fails, the console keeps the staged edits and shows the
  daemon's error message.

#### The value that the warning of FR-editor-28 reads

The warning reads the field `active_paths` of `GET /api/access`. The field holds one entry
for each tailnet that carries an active session to this host. `internal/session` builds it
from two commands:

1. `ss -H -tna` reports the sockets of the host, with the state of each one.
2. `ip -json route get <address>` reports the device that carries the traffic to the
   remote address of a session.

A session whose device is the veth device of a tailnet carries the path from that tailnet
to the host.

The warning reads no property of the console request. `internal/api/console.go` refuses a
console bind address that is not a loopback address. A console request therefore always
arrives on the loopback address, and no local rule governs that path. The risk is a second
program, such as a shell session, that reaches this host through a tailnet.

The field holds two limits, and the console warns inside them:

- The field reports an inbound session only. The compiler writes no rule for a source that
  is the host, therefore an edit cuts an inbound session alone.
- The field reports a session that ends on this host only. A session that the host
  forwards between two namespaces holds no socket on the host, and `ss` reports none.

### Mode

- **FR-editor-31** — The access view shows the current mode, `enforce` or `observe`.
- **FR-editor-32** — In `observe` mode, the view states that the daemon denies nothing
  and it names the log command that shows what it would deny.
- **FR-editor-33** — The operator can change the mode from the view, through a dialog
  that states what the change does.

## User flows

### The operator allows one path

1. The operator opens the access view.
2. The operator selects the source `jbones` in the flow overview.
3. The overview draws the two paths that `jbones` already has.
4. The operator clicks the empty square at row `jbones`, column `homelab`.
5. The console stages a rule that allows every port.
6. The overview draws a new dotted curve for the staged path.
7. The staged count reads `1`.
8. The operator selects **Apply**.
9. The console sends the whole rule set.
10. The daemon applies it and the console clears the staged count.

### The operator narrows a rule to two ports

1. The operator finds the rule row for `jbones` to `homelab`.
2. The operator selects the ports field.
3. The operator enters `tcp/22` and `tcp/443`.
4. The row shows two chips and it is marked as staged.
5. The operator selects **Apply**.

### The operator would cut off their own connection

1. The operator reaches the host with a shell session through the tailnet `jbones`.
2. The operator stages the removal of the rule from `jbones` to `host`.
3. The daemon reports the path `jbones` to `host` in the field `active_paths`.
4. The console shows a warning above the staged list and above the apply action, and the
   warning names the path.
5. The operator can still apply, because the console does not block a decision the
   operator made.

## Screens & states

### Access — `mockups/03-acl-editor.html`

| Region | Content |
|---|---|
| Header | The heading, the mode, the staged count, and the apply and discard actions. |
| Flow overview | Sources on the left, destinations on the right, dotted curves for allowed paths. |
| Matrix | The grid of squares. |
| Rule list | One row per rule, with ports. |

| State | What it shows |
|---|---|
| Empty | No rule exists. The view states that nothing can reach anything, and it names the matrix as the way to start. |
| Populated, nothing selected | Every allowed path at the resting edge weight. |
| Source selected | That source's paths in the accent colour, every other path muted. |
| Staged | The staged count, the staged list, and each staged rule marked in the rule list. |
| Apply in progress | The apply action is disabled and it states that the daemon is applying. |
| Apply failed | The error message from the daemon, above the staged list, with the edits kept. |
| Observe mode | A statement that the daemon denies nothing, and the log command. |

## Behaviour rules

- Denial is the absence of a line and the absence of a row. The console never draws a red
  edge, never crosses out a node, and never writes the word `denied` as a state.
- One source at a time. The console never draws the full set of paths at full strength.
- Ports live in the rule list. They never appear on a curve or in a square.
- The console stages. The daemon applies. The console never writes a rule that the daemon
  has not accepted.
- Apply sends the whole rule set rather than a change list, so that two consoles cannot
  interleave partial writes.
- The console applies no edit automatically, including after a timeout.

## Data touched

The console reads and writes the `access` block through `GET /api/access` and
`PUT /api/access`. It creates no new entity.

## Interfaces

`features/05-reachability-model.md` defines both routes. The console uses
`PUT /api/access?dry_run=true` to compute the effect of the staged rule set before the
operator applies it.

## Edge cases & failures

| Case | Behaviour |
|---|---|
| Another console applies a change while edits are staged. | The poll shows a rule set that differs from the base. The console states that the daemon's rule set changed and it offers to rebase the edits or to discard them. |
| The operator stages 40 edits. | The staged list scrolls. The count is exact. |
| A tailnet is removed while a staged rule names it. | The console drops the staged rule and it states which one it dropped. |
| The host has 12 tailnets. | The matrix uses the 28 pixel cell rather than the 34 pixel cell, as the brand reference specifies for a dense host. |
| A port entry is `22`. | The console rejects it and it states that the format is `tcp/22`. |
| The daemon returns HTTP 400 on apply. | The console shows the daemon's message verbatim and it keeps every staged edit. |

## Acceptance criteria

- [ ] The access view draws one dotted curve per allowed path and no line for a denied
      path.
- [ ] Selecting a source draws its paths in the accent colour and mutes the rest.
- [ ] Clicking an empty matrix square stages a rule and the staged count reads `1`.
- [ ] Clicking a filled square stages the removal of the path's rules.
- [ ] The diagonal square accepts no click.
- [ ] Hovering a square marks its row label and its column label and draws no other
      crosshair.
- [ ] A rule row shows `tcp/22` and `tcp/443` as two chips.
- [ ] A rule row with no port shows `all ports`.
- [ ] Entering the port `22` shows an error that names the expected format.
- [ ] **Apply** sends one `PUT /api/access` with the whole rule set.
- [ ] **Discard** returns the view to the daemon's rule set.
- [ ] A failed apply keeps the staged edits and shows the daemon's message.
- [ ] The view states the mode, and `observe` mode names the log command.
- [ ] No screen in the view contains the word `denied` as a state label.
- [ ] Every control in the view reaches focus by keyboard.

## Out of scope

- Per-peer rules. A rule names a tailnet.
- Drag to connect. A click on a square is the editor; a drag on the curve adds a second
  interaction model for the same action.
- A rule history or an undo stack beyond **Discard**.
- Upstream policy editing. `features/08-upstream-policy.md` owns it.

## Open questions

None.
