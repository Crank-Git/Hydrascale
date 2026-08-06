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
- Draw no denied path. Absence is the denial.
- Use no emoji.
- Make no request to another host.

## The token files

The console has no build step. `index.html` loads each token file as a stylesheet, and
`go:embed` puts the whole of `internal/ui/static` in the daemon binary.

| File | Holds |
|---|---|
| `brand/tokens/colors.css` | The palette and the blueprint edge colours. |
| `brand/tokens/typography.css` | The two typefaces, the size scale, the tracking, the line height, and the weights. |
| `brand/tokens/fonts.css` | The `@font-face` rule of each self-hosted font file. |
| `brand/tokens/spacing.css` | The 4 pixel scale, the pad values, the frame widths, and the dot sizes. |
| `brand/tokens/radius.css` | The shape scale. |
| `brand/tokens/elevation.css` | The two shadows and the two rings. |
| `brand/tokens/motion.css` | The three durations and the one easing curve. |

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
| `--ink` | `#0d0c0b` | The page. |
| `--s1` | `#141312` | A card. |
| `--s2` | `#1c1a18` | A raised element and a hover state. |
| `--s3` | `#24211e` | An input, an inset, and a track. |

### Lines

| Token | Value | Use |
|---|---|---|
| `--line` | `#262320` | A structural edge. |
| `--line-soft` | `#1a1816` | A row divider. |

### Text

There are three text steps.

| Token | Value | Use |
|---|---|---|
| `--tx` | `#eceae6` | Body text. |
| `--mu` | `#8d867d` | Secondary text. |
| `--dim` | `#5b544c` | Tertiary text and a label. |

### The accent

There is one accent, acid lime. Use it for an action and for a selection.

| Token | Value | Use |
|---|---|---|
| `--lime` | `#c8ff2e` | The accent. |
| `--lime-ink` | `#101208` | Text on an accent fill. |
| `--lime-soft` | `color-mix(in srgb, var(--lime) 14%, transparent)` | An accent wash. |

Use the accent for one thing per view. Never add a second accent.

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

### The semantic aliases

A view reads the alias rather than the raw token.

`--surface-page`, `--surface-card`, `--surface-raised`, `--surface-inset`,
`--border-default`, `--border-soft`, `--text-body`, `--text-secondary`, `--text-tertiary`,
`--accent`, `--accent-ink`, `--status-ok`, `--status-warn`, `--status-crit`.

### The blueprint edges

These four tokens carry the dotted-edge language of the access views.

| Token | Value | Use |
|---|---|---|
| `--edge` | `#3a352f` | An allowed path at rest. |
| `--edge-active` | `var(--lime)` | The paths of the selected source. |
| `--edge-deny` | `#332b28` | A muted structural line. |
| `--scrim` | `color-mix(in srgb, #060605 72%, transparent)` | The layer behind a dialog. |

## The typography

There are two typefaces and no third. The console self-hosts both, because the console
makes no request to another host. `brand/fonts/OFL.txt` holds the licence of both
families, which is the SIL Open Font License version 1.1.

| Token | Value |
|---|---|
| `--sans` | `'Space Grotesk', system-ui, sans-serif` |
| `--mono` | `'Space Mono', ui-monospace, Menlo, monospace` |
| `--font-display` | `var(--sans)` |
| `--font-body` | `var(--sans)` |
| `--font-data` | `var(--mono)` |

The sans typeface carries anything a person wrote. The mono typeface carries anything the
machine owns: an identifier, an address, a port, a timestamp, and a CIDR block. Never set a
machine value in the sans typeface. Never set a sentence in the mono typeface.

`brand/fonts/` holds three files. `SpaceGrotesk[wght].woff2` is a variable font that
carries every weight from 300 to 700 in one file. `SpaceMono-Regular.woff2` and
`SpaceMono-Bold.woff2` carry the two mono weights, because Space Mono publishes no variable
font. The console sets no italic, so no italic file ships.

### The size scale

| Token | Value |
|---|---|
| `--fs-display` | `44px` |
| `--fs-h1` | `30px` |
| `--fs-h2` | `21px` |
| `--fs-h3` | `17px` |
| `--fs-lead` | `15px` |
| `--fs-body` | `14px` |
| `--fs-sm` | `13px` |
| `--fs-data` | `12.5px` |
| `--fs-micro` | `11.5px` |
| `--fs-label` | `11px` |

### The tracking

| Token | Value | Use |
|---|---|---|
| `--ls-display` | `-.04em` | The display size. |
| `--ls-h1` | `-.035em` | The first heading. |
| `--ls-h2` | `-.025em` | The second heading. |
| `--ls-body` | `-.01em` | Body text. |
| `--ls-label` | `.14em` | The uppercase mono label. |
| `--ls-mono` | `0em` | A machine value. |

A display size and a heading always carry negative tracking. The only uppercase text in the
console is the mono label at `--fs-label` with `--ls-label`. Source text stays lowercase.
Add no letterspaced sans.

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

Every corner is soft and consistent. The matrix grid is the one square element.

| Token | Value | Use |
|---|---|---|
| `--r-xs` | `6px` | The matrix cell. |
| `--r-sm` | `9px` | A button, a chip, and an input. |
| `--r` | `12px` | A row and a small card. |
| `--r-lg` | `16px` | A card. |
| `--r-xl` | `20px` | A panel and a dialog. |
| `--r-pill` | `999px` | A pill and a dot. |

## The spacing

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
| `--pad-row` | `12px 16px` | A list row. |
| `--pad-btn` | `10px 16px` | A button. |
| `--pad-btn-sm` | `7px 12px` | A small button. |
| `--pad-input` | `11px 13px` | An input. |
| `--pad-view` | `28px 32px` | The view frame. |

### The frame

| Token | Value | Use |
|---|---|---|
| `--w-nav` | `212px` | The left navigation. |
| `--w-side` | `320px` | The contextual right panel. |
| `--h-topbar` | `60px` | The top bar. |
| `--maxw-read` | `640px` | The widest prose column. |
| `--maxw-app` | `1440px` | The widest application frame. |
| `--dot` | `8px` | The state dot. |
| `--dot-sm` | `6px` | The small state dot. |

## The elevation

Depth comes from the surface value and not from a shadow. Exactly two shadows exist.

| Token | Value | Use |
|---|---|---|
| `--shadow-pop` | `0 24px 60px -20px rgba(0,0,0,.7)` | A dialog and a menu. |
| `--shadow-lift` | `0 2px 0 rgba(0,0,0,.25)` | A card that reads as pressed into the page. |
| `--ring-accent` | `0 0 0 1px var(--lime)` | A selected element. |
| `--ring-focus` | `0 0 0 2px color-mix(in srgb, var(--lime) 45%, transparent)` | The keyboard focus. |

## The motion

Motion is fast and small. Animate only what the operator triggered.

| Token | Value | Use |
|---|---|---|
| `--dur-1` | `90ms` | A hover and a colour change. |
| `--dur-2` | `160ms` | A state change. |
| `--dur-3` | `280ms` | A path draw and a panel reveal. |
| `--ease` | `cubic-bezier(.2,.7,.3,1)` | Every transition. |

`brand/tokens/motion.css` holds a `@media (prefers-reduced-motion: reduce)` rule that sets
`--dur-1`, `--dur-2`, and `--dur-3` to `0ms`. Read a duration from a token, so that the
rule reaches your transition.

Add no entrance animation, no skeleton animation, no spring, and no bounce.

## The access-control drawing rules

`docs/specs/features/07-console-access-editor.md` states these rules as requirements. Four
of them govern every drawing in the access views.

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

Draw no node icon, no minimum map, and no force-directed physics. Ports belong in the rule
list, where words fit. A port never appears on a curve and never appears in a matrix
square.

## The marks

`internal/ui/static/brand/` holds three marks. Each mark is a stroked SVG on a 64 by 64
view box with a round line cap. The mark draws six paths that rise from one trunk.

| File | Stroke | Stroke width | Use |
|---|---|---|---|
| `brand/logo.svg` | `currentColor` | `3.4` | The mark that takes the colour of its parent. |
| `brand/logo-lime.svg` | `#c8ff2e` | `3.4` | The navigation mark and the page icon. |
| `brand/logo-compact.svg` | `currentColor` | `4.8` | The mark at a small size. |

`index.html` sets `brand/logo-lime.svg` as the `rel="icon"` of the page, and it draws the
same file at 22 by 22 pixels beside the wordmark. The wordmark reads `hydrascale`, with
`scale` in a second style.

## The icon set

`internal/ui/static/brand/icons/` holds 13 icons: `access.svg`, `activity.svg`,
`back.svg`, `dns.svg`, `namespaces.svg`, `overview.svg`, `peers.svg`, `plus.svg`,
`policy.svg`, `refresh.svg`, `route.svg`, `settings.svg`, and `trash.svg`.

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
