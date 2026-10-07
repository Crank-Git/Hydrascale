---
paths:
  - "internal/ui/**"
  - "internal/tui/**"
  - "docs/DESIGN.md"
---

# Console and terminal design rules

The full brand is `docs/DESIGN.md`. The tokens are
`internal/ui/static/brand/tokens/*.css`. These are the rules a change most often breaks.

## Colour

- One accent, acid lime `#c8ff2e`. Use it for **one thing per view**: the affirmative
  action, or an allowed path. Never a second accent.
- A selection takes no accent. It is an inset cell with a `--dim` outline, in every view.
- State is separate from the accent: mint for good, amber for a warning, red for
  critical. Use each as a dot plus a lowercase word.
- Never tint a whole card, a whole row, or a whole edge to show a state.
- The console is dark only. `tokens/colors.css` sets `color-scheme: dark`. Do not add a
  light theme.

## Type

- The sans typeface carries anything a person wrote. The mono typeface carries anything
  the machine owns: an identifier, an address, a port, a timestamp, a CIDR block.
- Never set a value in the sans typeface. Never set a sentence in the mono typeface.
- The sans typeface is Barlow Semi Condensed. The mono typeface is Martian Mono at the
  width `--stretch-mono`.
- The only uppercase is a column head of the board: the sans at `--fs-label` with
  `--ls-label` tracking. Source text stays lowercase.
- A work area sets two sizes: the value size and the label size. Rank comes from weight,
  case, and rule.
- Add no third typeface, no italics, and no other letterspaced text.

## Access control drawing

- Draw one source's paths at a time. Mute the rest. Never draw every path at full
  strength.
- An allowed path is a dotted curve: 2px dash, 6px gap, 1.4px stroke.
- **Denial is the absence of a line.** Draw no red edge, no crossed-out node, and no row
  in the rule list for a denied path.
- Draw no arrowhead, no edge label, no node icon, no minimum map, and no force-directed
  physics. A graph that needs a legend has failed.
- Ports belong in the rule list, where words fit. They never appear on a curve or in a
  matrix square.

## Layout and motion

- One view, one job, one content column, one largest heading.
- Open the contextual panel only when something is selected. Close it with the
  selection.
- Empty is a legitimate state. Say what would fill it. Never show invented data.
- Animate what the operator triggered. One exception: a board cell turns over once,
  character by character, when its value changes between two polls. A poll that changes
  nothing animates nothing, and the first draw animates nothing. The operator ruled this
  on 2026-10-07.
- No entrance animation, no skeleton animation, no spring, no bounce. Honour
  `prefers-reduced-motion`: the turnover does not run under it.

## Copy

- Voice: an operator explaining a system to another operator. Present tense, mechanism
  named, no reassurance.
- Sentence case for prose, headings, and buttons. Lowercase for every machine identifier
  and every reconciler state word.
- Destructive copy names the exact commands the action runs, and states what survives.
- Use no emoji anywhere. The terminal interface uses `●`, `▸`, `┄`, `✓`. The console uses
  stroked SVG.

## Requests

The console makes no request to any host other than its own origin. No font from a
content network, no script from a content network, no image from a content network. The
daemon must work on a host with no internet route, and an operator console for a network
tool must not contact a third party.
