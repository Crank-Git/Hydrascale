# The Hydrascale design

This document states the brand of the Hydrascale console. It is written for a contributor
who has never seen the brand package. Read it before you change a file under
`internal/ui/static/`.

The token files under `internal/ui/static/brand/tokens/` hold every value. This document
names each token and repeats its value. When a value here and a value in a token file
disagree, the token file is correct. Report the difference as a defect.

`.claude/rules/console-brand.md` holds the short rules that a change most often breaks.
This document is the full statement.

## The rules in one place

- The console is dark only, because the brand is dark only.
- Use the accent colour for one thing per view: the affirmative action, or the current
  selection, or an allowed path.
- Show a state as a coloured dot and a lowercase word.
- Render every machine value in the mono typeface and every sentence in the sans typeface.
- A work area sets two type sizes: the value size and the label size.
- The only uppercase text is a column head or a field name, in the sans typeface.
- Every corner is square. The state dot is the one round shape.
- Draw no denied path. Absence is the denial.
- Animate what the operator triggered. The one exception is the turnover of a board cell.
- Use no emoji.
- Make no request to another host.

## The world of the console

The console is a departure board. Each tailnet is one row of fixed cells. The columns
never move. Only the values in the cells change.

The page is a warm black ground. Square cells sit in one frame, with a 1 pixel gutter
between two cells. A cell face is one step lighter than the ground. No view uses a
rounded corner or a shadow. A dialog keeps the one pop shadow.

## The views and their state

Every view uses the same three parts:

- A frame holds one region. A frame is the page colour inside one border. Its first line
  is a header strip that holds the title on the left and a count or a tool on the right.
  `.frame` and `.frame-head` draw it. A `.card` that opens with a `.label` or a
  `.card-head` draws the same strip.
- A board table holds one row per item in fixed columns. `.board` draws it. The Overview
  and the Namespaces view use it.
- A segmented control holds a choice. The current choice reverses its cell and takes no
  accent. `.seg` draws it.

Each view applies them as follows:

| View | Regions |
|---|---|
| Overview | The verdict line, the board, the topology frame, and the events frame. |
| Namespaces | The tailnet table, and the panel frame of the selected tailnet. The add action holds the accent. |
| Access | The toolbar, the allowed paths frame, the reachability frame, and the rules frame. A rule row is a grid with fixed columns for the source, the connector, and the destination. |
| Policy | The tailnet list frame, and the policy document frame. The visual editor shows its ten sections as one grid of cells. |
| Activity | One events frame with two filters: notable or every event, and the tailnet. A reconcile tick is one folded row. The table states each date once. |
| Settings | Titled frames in two columns. A label column of 140 pixels gives each frame one left edge for its values. |

## The token files

The console has no build step. `index.html` loads each token file as a stylesheet, and
`go:embed` puts the whole of `internal/ui/static` in the daemon binary.

| File | Holds |
|---|---|
| `brand/tokens/colors.css` | The palette, the blueprint edge colours, and `--steel`. |
| `brand/tokens/typography.css` | The two typefaces, the size scale, the tracking, the mono width, the line height, and the weights. |
| `brand/tokens/fonts.css` | The `@font-face` rule of each self-hosted font file. |
| `brand/tokens/spacing.css` | The 4 pixel scale, the pad values, the frame widths, and the dot sizes. |
| `brand/tokens/radius.css` | The shape scale. |
| `brand/tokens/elevation.css` | The two shadows and the two rings. |
| `brand/tokens/motion.css` | The durations, the turnover timings, and the one easing curve. |

`internal/ui/static/tokens.css` is not a token file. It restates three values that the
brand builds with the `color-mix` function, for a browser that reads no `color-mix`
function. It declares `--lime-soft`, `--scrim`, and `--ring-focus` inside an `@supports
not` rule. Add no new value to it.

`internal/ui/static/app.css` declares no colour, no spacing step, and no radius of its own.
It reads the tokens. Keep it that way.

## The palette

`brand/tokens/colors.css` sets `color-scheme: dark`. The console has no light theme. Add
none.

### Surfaces

The surfaces are warm and brown-grey. There are four steps and no more.

| Token | Value | Use |
|---|---|---|
| `--ink` | `#0d0c0b` | The page, and the gutter between two board cells. |
| `--s1` | `#141312` | A board cell face, the rail, and a frame head. |
| `--s2` | `#1c1a18` | A hover state, the selected row, and the current navigation entry. |
| `--s3` | `#24211e` | An input, an inset, and the lower line of the flap split. |

### Lines

| Token | Value | Use |
|---|---|---|
| `--line` | `#262320` | A structural edge. |
| `--line-soft` | `#1a1816` | A row divider, and a muted path in the topology. |
| `--steel` | `#45413c` | The one frame strip around the verdict line and the board. |

Use `--steel` for the board frame only. Every other frame uses `--line`.

### Text

There are three text steps.

| Token | Value | Use |
|---|---|---|
| `--tx` | `#eceae6` | Body text and every board value. |
| `--mu` | `#a39b90` | Secondary text. |
| `--dim` | `#8a8277` | Tertiary text, a column head, and a value that the daemon does not report. |

### The accent

There is one accent, acid lime. Use it for an action and for a selection.

| Token | Value | Use |
|---|---|---|
| `--lime` | `#c8ff2e` | The accent. |
| `--lime-ink` | `#101208` | Text on an accent fill. |
| `--lime-soft` | `color-mix(in srgb, var(--lime) 14%, transparent)` | An accent wash. |

Use the accent for one thing per view. Never add a second accent. A disabled affirmative
button loses the accent, so the accent always marks a thing that the operator can do now.

### The states

A state colour never marks an action.

| Token | Value | Use |
|---|---|---|
| `--ok` | `#7ddc8f` | Good. |
| `--warn` | `#f0a63c` | A warning. |
| `--crit` | `#ff5f52` | Critical. |

Show a state as a dot plus a lowercase word. The dot carries the state colour and the word
stays in the body colour. Never tint a whole card, a whole row, or a whole edge to show a
state. `app.css` holds the class `.dot` and the three modifiers `.dot.ok`, `.dot.warn`, and
`.dot.crit`.

A dot with no modifier is a quiet state. It draws in `--dim`. Use it for a condition that
is not a fault, such as a paused tailnet. A red dot on the board always means a fault.

### The semantic aliases

A view reads the alias rather than the raw token.

`--surface-page`, `--surface-card`, `--surface-raised`, `--surface-inset`,
`--border-default`, `--border-soft`, `--text-body`, `--text-secondary`, `--text-tertiary`,
`--accent`, `--accent-ink`, `--status-ok`, `--status-warn`, `--status-crit`.

### The blueprint edges

These four tokens carry the dotted-edge language of the topology and the access views.

| Token | Value | Use |
|---|---|---|
| `--edge` | `#6a6359` | An allowed path at rest. Its contrast on the page is 3.3:1. |
| `--edge-active` | `var(--lime)` | The paths of the selected source. |
| `--edge-deny` | `#332b28` | A muted structural line. |
| `--scrim` | `color-mix(in srgb, #060605 72%, transparent)` | The layer behind a dialog. |

## The typography

There are two typefaces and no third. The console self-hosts both, because the console
makes no request to another host. `brand/fonts/OFL.txt` holds the licence of both
families, which is the SIL Open Font License version 1.1.

| Token | Value |
|---|---|
| `--sans` | `'Barlow Semi Condensed', 'Arial Narrow', system-ui, sans-serif` |
| `--mono` | `'Martian Mono', ui-monospace, Menlo, monospace` |
| `--font-display` | `var(--sans)` |
| `--font-body` | `var(--sans)` |
| `--font-data` | `var(--mono)` |
| `--stretch-mono` | `75%` |

The sans typeface carries anything a person wrote. The mono typeface carries anything the
machine owns: an identifier, an address, a port, a timestamp, a count, and a CIDR block.
Never set a machine value in the sans typeface. Never set a sentence in the mono typeface.

The mono typeface always sets `font-stretch` to `--stretch-mono`. The narrow width lets a
fixed cell hold a long identifier. The class `.mono` also sets tabular figures, so the
digits of a column align.

### The font files

`brand/fonts/` holds four files. Each file holds the Latin subset only. The files come
from the Fontsource npm packages at version 5.3.0.

| File | Source | Holds |
|---|---|---|
| `BarlowSemiCondensed-Regular.woff2` | `@fontsource/barlow-semi-condensed` | Weight 400. |
| `BarlowSemiCondensed-Medium.woff2` | `@fontsource/barlow-semi-condensed` | Weight 500. |
| `BarlowSemiCondensed-SemiBold.woff2` | `@fontsource/barlow-semi-condensed` | Weight 600. |
| `MartianMono[wdth,wght].woff2` | `@fontsource-variable/martian-mono` | A variable font: weight 100 to 800, width 75% to 112.5%. |

Barlow Semi Condensed publishes no variable font, so the console loads the three weights
it uses. No file holds the sans at weight 700. Set no sans text at `--fw-bold`. The
console sets no italic, so no italic file ships.

### The size scale

| Token | Value |
|---|---|
| `--fs-display` | `40px` |
| `--fs-h1` | `28px` |
| `--fs-h2` | `20px` |
| `--fs-h3` | `17px` |
| `--fs-lead` | `16px` |
| `--fs-body` | `15px` |
| `--fs-sm` | `14px` |
| `--fs-data` | `13px` |
| `--fs-micro` | `12px` |
| `--fs-label` | `12px` |

A work area sets two sizes: the value size and the label size. The board sets its values
at `--fs-data` and its column heads at `--fs-label`. Rank comes from weight, case, and
rule, not from a third size.

### The tracking

| Token | Value | Use |
|---|---|---|
| `--ls-display` | `-.01em` | The display size. |
| `--ls-h1` | `-.005em` | The first heading. |
| `--ls-h2` | `0em` | The second heading. |
| `--ls-body` | `0em` | Body text. |
| `--ls-label` | `.08em` | An uppercase column head or field name. |
| `--ls-mono` | `0em` | A machine value. |

The only uppercase text in the console is a column head or a field name. It uses the sans
typeface at `--fs-label`, `--fw-medium`, and `--ls-label`, in `--dim`. The class `.label`,
the board head, and the title block all use this form. Source text stays lowercase. Add no
other letterspaced text.

### The line height

| Token | Value |
|---|---|
| `--lh-display` | `1.02` |
| `--lh-head` | `1.12` |
| `--lh-body` | `1.5` |
| `--lh-data` | `1.35` |

### The weights

| Token | Value |
|---|---|
| `--fw-regular` | `400` |
| `--fw-medium` | `500` |
| `--fw-semibold` | `600` |
| `--fw-bold` | `700` |

## The shape scale

Every corner is square. A board cell, a button, an input, a frame, and a dialog have no
corner radius. The state dot is the one round shape.

| Token | Value | Use |
|---|---|---|
| `--r-xs` | `0px` | A matrix cell. |
| `--r-sm` | `0px` | A button, a chip, and an input. |
| `--r` | `0px` | A row and a cell. |
| `--r-lg` | `0px` | A section. |
| `--r-xl` | `0px` | A panel and a dialog. |
| `--r-pill` | `999px` | The state dot. |

Read a radius token even where its value is `0px`. Use `--r-pill` for the state dot only.

## The space scale

One 4 pixel scale carries every gap.

| Token | Value |
|---|---|
| `--sp-1` | `4px` |
| `--sp-2` | `8px` |
| `--sp-3` | `12px` |
| `--sp-4` | `16px` |
| `--sp-5` | `20px` |
| `--sp-6` | `24px` |
| `--sp-7` | `32px` |
| `--sp-8` | `40px` |
| `--sp-9` | `56px` |
| `--sp-10` | `80px` |

### The pad values

| Token | Value | Use |
|---|---|---|
| `--pad-card` | `18px 20px` | A card. |
| `--pad-card-lg` | `22px 24px` | A large card. |
| `--pad-row` | `12px 16px` | A list row and the poll banner. |
| `--pad-btn` | `10px 16px` | A button. |
| `--pad-btn-sm` | `7px 12px` | A small button. |
| `--pad-input` | `11px 13px` | An input. |
| `--pad-view` | `28px 32px` | The view frame. |

### The frame

| Token | Value | Use |
|---|---|---|
| `--w-nav` | `212px` | The left rail. |
| `--w-side` | `320px` | The contextual right panel, and the events frame of the Overview view. |
| `--h-topbar` | `60px` | The top bar. |
| `--maxw-read` | `640px` | The widest prose column. |
| `--maxw-app` | `1440px` | The widest application frame. |
| `--dot` | `8px` | The state dot. |
| `--dot-sm` | `6px` | The small state dot. |

## The elevation

Depth comes from the surface value and from a 1 pixel line, not from a shadow. The shell
and the board use no shadow. Two shadow tokens exist.

| Token | Value | Use |
|---|---|---|
| `--shadow-pop` | `0 24px 60px -20px rgba(0,0,0,.7)` | A dialog and a menu. |
| `--shadow-lift` | `0 2px 0 rgba(0,0,0,.25)` | A card that reads as pressed into the page. |
| `--ring-accent` | `0 0 0 1px var(--lime)` | A selected element. |
| `--ring-focus` | `0 0 0 2px color-mix(in srgb, var(--lime) 45%, transparent)` | The keyboard focus. |

Every control reaches focus by keyboard, and it draws `--ring-focus`. An SVG group draws
no `box-shadow`, so a topology node draws a 2 pixel outline in `--tx`.

## The motion

Motion is fast and small. Animate what the operator triggered. The turnover of a board
cell is the one exception.

| Token | Value | Use |
|---|---|---|
| `--dur-1` | `90ms` | A hover and a colour change. |
| `--dur-2` | `160ms` | A state change. |
| `--dur-3` | `280ms` | A path draw and a panel reveal. |
| `--dur-flip` | `180ms` | One character of a turnover. |
| `--stagger-flip` | `14ms` | The delay between two characters of a turnover. |
| `--ease` | `cubic-bezier(.2,.7,.3,1)` | Every transition. |

`brand/tokens/motion.css` holds a `@media (prefers-reduced-motion: reduce)` rule. The rule
sets every duration token to `0ms`, `--dur-flip` and `--stagger-flip` included. Read a
duration from a token, so that the rule reaches your transition.

### The turnover

The operator ruled on 2026-10-07 that a board cell can turn over. These are the
conditions:

- A cell turns over once, one character after the next, when its value changes between
  two polls.
- A cell does not turn over on the first draw of the page.
- A cell does not turn over when a poll changes nothing.
- A new row does not turn over.
- The turnover does not run under `prefers-reduced-motion`.

Each character rotates from `rotateX(-90deg)` to rest in `--dur-flip`. Its delay is its
index times `--stagger-flip`. When the last character stops, the cell becomes plain text
again. `board.js` compares the text of each cell with the text of the last draw, and
`overview.js` marks the cells that changed.

Add no entrance animation, no skeleton animation, no spring, and no bounce.

## The shell

The shell is the left rail and the view frame. It takes no accent, so each view keeps the
accent for its own one thing.

### The rail

The rail is `--w-nav` wide on `--s1`, with a `--line` edge on the right. It holds three
parts, from the top:

1. The mark and the wordmark.
2. The numbered index of the six views.
3. The title block, at the foot of the rail.

### The numbered index

Each entry of the index is a number cell and a sentence-case view name. The number is the
key that opens the view: `1` opens Overview and `6` opens Settings. `app.js` binds the
keys. A key does nothing while the focus is in a control that takes text.

The number cell is a 24 pixel square with a `--line` edge on `--ink`, with the number in
the mono typeface. The current entry reverses its number cell: a `--tx` fill with the
number in `--ink`. The current entry also takes a `--s2` row. The current entry uses no
accent.

### The title block

The title block names the daemon that serves the page. It is a column of three fields
with a `--line` edge, and a `--line` rule between two fields:

| Field | Value |
|---|---|
| `Daemon` | The daemon version. |
| `Access` | The access mode and the rule count. |
| `Last tick` | The time of the last reconcile tick. |

Each field name uses the uppercase label form. Each value uses the mono typeface. The
block writes only a value that a poll returned. Before the first poll, each value is a
lowercase sentence, such as `no version yet`.

### The poll banner

The poll banner states a failed poll. It is hidden while every poll succeeds. It holds a
dot, a sentence, and a retry button, and it takes no accent.

### A narrow screen

At 900 pixels and less, the rail becomes one row at the top of the page. The row scrolls
sideways. The title block is hidden, and the verdict line shows the time of the last
tick in its place.

## The board

The board is the main component of the Overview view. It answers one question first: is
every tailnet healthy and reachable, and if not, which one is not.

### The verdict line

The verdict line is the largest text of the Overview view. The view heading stays in the
page for a screen reader only.

The line sits on `--s1` inside the `--steel` frame, directly above the board. It holds
three parts:

- A dot in the tone of the worst row.
- One sentence in the sans typeface at `--fs-h2` and `--fw-semibold`. A fault adds the
  identifier of each faulted tailnet in the mono typeface.
- The counts, in the mono typeface in `--mu`, at the right end of the line.

Each count is the sum of a board column: tailnets, peers, allowed paths, and the
reconciler state word. A `·` in `--dim` separates two counts. A wrapped line never starts
with a separator.

### The table

The board is a table of fixed columns in one `--steel` frame on `--ink`. A 1 pixel gutter
of `--ink` separates two cells. The columns, in order, are:

| Column head | Value | Alignment |
|---|---|---|
| `Tailnet` | The tailnet identifier, at `--fw-semibold`. It is the row header. | Left |
| `State` | The reconciler state, as a dot and a word. | Left |
| `Reachability` | The measured reachability, as a dot and a word. | Left |
| `Probe` | The probe target. | Left |
| `Peers` | The peer count. | Right |
| `Paths` | The count of allowed paths. | Right |
| `Exit node` | The exit node. | Left |
| `Host access` | `on`, `off`, or `default`. | Left |
| `Policy` | The credential state, as a dot and a word. | Left |

A column head uses the uppercase label form. Every value uses the mono typeface at
`--fs-data`. A number aligns to the right edge of its cell. A value that the daemon does
not report reads `none` in `--dim`. The board shows no value that the API does not report.

### The flap split

Each value cell is 44 pixels high, on `--s1`. A horizontal split crosses the middle of the
cell: a 1 pixel line of `--ink` over a 1 pixel line of `--s3`. The split shows the two
leaves of a split-flap display. It is a background image, so it stays behind the
characters. A hover and the selection change the face to `--s2` and keep the split.

### The row order

A row with a fault sorts above every row without one. A row takes the rank of its worst
cell: `crit` above `warn`. Rows of equal rank keep the order of the topology model. A row
moves only when its rank changes.

### The policy credential

The `Policy` column states the credential state of each tailnet:

| Word | Dot | Reason |
|---|---|---|
| `usable` | `--ok` | The control server accepts the credential. |
| `no credential` | Quiet, in `--dim` | A credential is optional. Only the Policy view needs one. |
| `not read yet` | Quiet, in `--dim` | The daemon has not read the policy state. |
| `credential rejected` | `--crit` | The control server rejected the credential. This is a fault. |

An absent credential is never a fault. A rejected credential is always a fault.

### The selection

A row is a control. A pointer, the Enter key, or the space bar selects the row. The arrow
keys move between rows. A second selection of the same row clears the selection.

The selection is the one accent use of the Overview view. The selected row names its
tailnet in `--lime`, on a `--s2` face. The topology draws the paths of that tailnet in
`--edge-active`. When no tailnet exists, no selection exists. The add action then takes
the accent.

### A narrow screen

At 600 pixels and less, the board keeps the `Tailnet`, `State`, `Reachability`, and
`Peers` columns and hides the rest. The topology shows its text equivalent in place of the
picture.

### Below the board

Below the board, the topology frame and the events frame sit side by side. The events
frame is `--w-side` wide. Each frame has a `--line` edge on `--ink`, and a head on `--s1`.
The events frame lists the last five events that are not a routine reconcile tick. At
1040 pixels and less, the two frames stack.

## The access-control picture rules

`docs/specs/features/07-console-access-editor.md` states these rules as requirements. Four
of them govern every picture in the topology and the access views.

1. **One source at a time.** Draw the paths of one source. Mute the rest. Never draw the
   full set of paths at full strength.
2. **No denied path.** Denial is the absence of a line and the absence of a row. Draw no
   red edge, no crossed-out node, and no rule row for a denied path. Write the word
   `denied` as no state.
3. **No arrowhead.** Draw no arrowhead on a curve.
4. **No edge label.** Draw no label on a curve. A graph that needs a legend has failed.

An allowed path is a dotted curve. `app.css` sets the class `.edge` to a 1.4 pixel stroke
and a `stroke-dasharray` of `2 6`, which is a 2 pixel dash and a 6 pixel gap. The stroke
reads `--edge` at rest. The class `.edge.sel` reads `--edge-active`, which is the accent.
The class `.edge.muted` reads `--line-soft`.

A topology node is a square box on `--s1` with a `--line` edge. Its name and its state
line use the mono typeface. The selected node draws its edge in the accent.

Draw no node icon, no minimum map, and no force-directed physics. Ports belong in the rule
list, where words fit. A port never appears on a curve and never appears in a matrix
square.

## The marks

`internal/ui/static/brand/` holds three marks. Each mark is a stroked SVG on a 64 by 64
view box with a round line cap. The mark draws six paths that rise from one trunk.

| File | Stroke | Stroke width | Use |
|---|---|---|---|
| `brand/logo.svg` | `currentColor` | `3.4` | The mark that takes the colour of its parent. |
| `brand/logo-lime.svg` | `#c8ff2e` | `3.4` | The rail mark and the page icon. |
| `brand/logo-compact.svg` | `currentColor` | `4.8` | The mark at a small size. |

`index.html` sets `brand/logo-lime.svg` as the `rel="icon"` of the page. It draws the same
file at 22 by 22 pixels beside the wordmark. The wordmark reads `hydrascale` in the sans
typeface at `--fs-h3` and `--fw-semibold`, with `scale` in `--lime`.

## The icon set

`internal/ui/static/brand/icons/` holds 13 icons: `access.svg`, `activity.svg`,
`back.svg`, `dns.svg`, `namespaces.svg`, `overview.svg`, `peers.svg`, `plus.svg`,
`policy.svg`, `refresh.svg`, `route.svg`, `settings.svg`, and `trash.svg`. The rail shows
a number cell in place of an icon. No view draws an icon now.

Every icon shares one construction:

- a 24 by 24 view box;
- `fill="none"`;
- `stroke="currentColor"`, so the icon takes the colour of its parent;
- `stroke-width="1.6"`;
- `stroke-linecap="round"` and `stroke-linejoin="round"`.

A new icon matches this construction. The console draws a stroked SVG and it uses no emoji.
The terminal interface uses the characters `●`, `▸`, `┄`, and `✓`.

## The layout and the copy

- One view, one job, one content column, one largest heading.
- Open the contextual panel only when something is selected. Close the panel with the
  selection.
- Empty is a legitimate state. State what fills it. Show no invented data.
- The voice is an operator who explains a system to another operator. Use the present
  tense. Name the mechanism. Add no reassurance.
- Use sentence case for prose, for a heading, and for a button. Use lowercase for every
  machine identifier and every reconciler state word.
- Destructive copy names the exact commands that the action runs, and it states what
  survives.

## The request rule

The console makes a request to its own origin alone. It loads no font, no script, and no
image from another host. The daemon must work on a host with no internet route. An
operator console for a network tool must not contact a third party.
