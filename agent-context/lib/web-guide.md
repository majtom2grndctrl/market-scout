# Web App Guide

> **Read this when:** building or changing anything under `apps/web/` — routes, components, styles, data access, or Storybook.
> **Key invariant:** raw location text is not analytical geography. Use curated `market`; compensation remains outside the vocabulary.
> **Related:** [Project Overview](./project.md) · [Development Guide](./developer-guide.md) · [Web Testing Guide](./web-testing-guide.md) · [Style Guide](./style-guide.md)

---

## What applies from the Development Guide

That guide is Go-first. Working in `apps/web`, read only:

| Section | Why |
|---|---|
| §1 Implementation Quality | Universal — including §1.5's light lane, which is why most UI work skips the plan pipeline. |
| §2 Development Setup | Follow the `apps/web/.env.local` symlink setup so Next reads the canonical root env file. |
| §4.1, §4.3, §4.5 File organization | Universal. §4.2's package advice and §4.4's density rule are Go-specific. |
| §7.1, §7.3, §7.4 Comments | Universal. §7.2 covers Go package doc comments. |

Skip the rest of §2, plus §3, §5, and §6 — Go setup, Go build, Go conventions, Go logging. §5.8 covers sqlc; nothing in `apps/web` is generated.

Read [`web-testing-guide.md`](./web-testing-guide.md) before adding or changing web tests. `pnpm test` is DB-free. `pnpm test:db` runs only the Postgres view suite. Vitest transpiles without typechecking, so `pnpm typecheck` remains the type gate.

## Layout

```
apps/web/
  app/                # App Router. globals.css is the stylesheet entry; theme.css holds colour, tokens.css containers.
  components/ui/      # shadcn, forked onto the semantic colour tokens
  lib/db/             # Read-only Postgres queries for Server Components
  lib/utils.ts        # cn() — shadcn's clsx + tailwind-merge helper
  .storybook/
```

**`components/ui/` is forked, not vendored.** These began as shadcn copies, then moved onto the semantic colour tokens. `shadcn add` overwrites a file wholesale, so re-adding a component reintroduces `bg-card`/`text-muted-foreground` — names that no longer resolve, and Tailwind emits nothing for a colour it cannot find, so the component renders unstyled rather than erroring. After `shadcn add`, migrate the new file with the table in § Colour before using it. Structure and behaviour still come from upstream: compose around a component rather than rewriting its markup.

## Data access

Server Components query Postgres directly with the read-only `DATABASE_URL_RO`. `lib/db/client.ts` owns the pooled `postgres` client and marks reads dynamic before opening it. Do not import it from Client Components.

Keep read queries in `lib/db/`. A query accepts a SQL client so DB tests can run the same read through the read-only role; the Server Component wrapper obtains that client. SQL views define derived state such as open postings. Consumers do not recreate those rules.

Route handlers are reserved for streaming responses. A normal request-response read belongs in a Server Component. First app-owned writes use Server Actions and a dedicated role; do not broaden the read-only connection.

Raw location text is excluded from analytical groupings and filters. Curated `market` is the supported location dimension. It maps observed values through the read model and preserves `unmapped` as an explicit result. Compensation is excluded from the analytical vocabulary.

## Styling

Two layers. Tailwind's scales are the primitive layer — `text-lg`, `p-4`, `rounded-sm` name a size, not a purpose. Custom utilities in `globals.css` are the semantic layer, naming what a thing is and composing out of primitives.

**Prefer a semantic utility where one exists.** A class pile you have assembled twice is the signal to add a third. Reach for primitives directly when the case is one-off, or when size genuinely is the point.

A semantic name earns its place even when it duplicates a primitive's value. `--container-narrow` is 32rem, the same as Tailwind's `max-w-lg`; `narrow` states intent, `lg` states size.

Two rules govern what a new utility can be called and which Tailwind classes it may use.

### Color namespaces

`cn()` runs tailwind-merge, which resolves conflicts by parsing class names. It reads a bare word in a color namespace as a color — including words it has never seen. Two classes it reads as the same property collide, and the later one wins outright. Verified against tailwind-merge 3.6.0:

| Input | Result |
|---|---|
| `twMerge("bg-brand bg-card")` | `bg-card` — `bg-brand` read as a color, dropped |
| `twMerge("text-body text-muted-foreground")` | `text-muted-foreground` — same failure |
| `twMerge("card-surface bg-card")` | both survive — no Tailwind prefix to collide with |
| `twMerge("rounded-card rounded-lg")` | both survive — `card` is not a valid radius, so it passes through |
| `twMerge("text-brand text-lg")` | both survive — different groups, color and size |

Name a custom utility outside `text-`, `bg-`, `border-`, and `fill-`. `card-surface` is safe; `bg-card-surface` is not. Everywhere else an unrecognized name passes through untouched.

This is also why `app/tokens.css` defines no `--text-*`, `--font-weight-*`, `--leading-*`, or `--tracking-*`. The type scale is Tailwind's, wholesale.

Colour has its own rules and its own file. See § Colour below.

### Colour

`app/theme.css` defines every colour token: raw OKLCH values under `:root` and
`.dark`, then an `@theme inline` block that turns each into Tailwind utilities.
`inline` is load-bearing — without it `bg-surface-raised` would bake in the light
value and the `.dark` block would never take effect.

Nine families. Reach for the one that names what the colour *means*, never the
one that happens to look right.

| Family | Tokens | Use for |
|---|---|---|
| Surface | `surface-page` `-raised` `-sunken` `-overlay` `-row-hover` `-row-focus` `-row-selected` | Anything content sits on |
| Content | `content-primary` `-secondary` `-muted` `-disabled` `-inverse` `-link` | Ink |
| Edge | `edge-hairline` `edge` `edge-strong` `focus` | Boundaries and the focus ring |
| Accent | `accent-solid` `-hover` `-active` `-subtle`, `on-solid` | Primary action, selection chrome |
| Status | `success-` `warning-` `danger-` `info-` × `solid` `subtle` `edge` `ink` | State of something the system did |
| Trend | `trend-up-` `trend-down-` `trend-flat-` × `solid` `subtle` `edge` `ink` | Direction of a measure |
| Freshness | `fresh-` `aging-` `stale-` `unavailable-` × `ink` `subtle` `edge` | Age of the system's own knowledge |
| Provenance | `observed` `derived` `derived-subtle`, `hatch-derived` | Read from the source vs. inferred by a model |
| Series | `series-1`…`series-8`, `series-other`, `ramp-1`…`ramp-7`, `delta-down-3`…`delta-up-3` | Identity, magnitude, signed change inside charts |

**Status and trend are different families and mixing them is a correctness bug.**
A rising posting count is good news for an employer and bad news for a candidate;
the tool has no standing to decide which. Status answers *did this work*. Trend
answers *which way is it moving*. Trend therefore runs blue/orange rather than
green/red, which also keeps it colourblind-safe and visually distant from status.
A signed delta wears `trend-*`. A failed fetch run wears `danger-*`.

**Four slots per family, and they compose the same way every time.** Given any
status or trend family:

- `bg-*-solid` with `text-on-solid` — a filled badge or a chart mark.
- `bg-*-subtle` + `border-*-edge` + `text-*-ink` — a quiet badge.
- `text-*-ink` alone — inline text.

Two invariants make that safe, and `pnpm theme:check` fails the build on either:

1. `on-solid` clears 4.5:1 on *every* `*-solid`. Light-mode solids therefore sit
   at or below OKLCH L 0.52, dark-mode solids at or above L 0.68. A new solid
   outside that band silently breaks white-on-colour text.
2. Every `*-ink` clears 4.5:1 on both its own `*-subtle` and on `surface-raised`,
   so the same ink works inside a badge and bare on a card.

**Accent is achromatic.** Near-black in light, near-white in dark. In a dense
analytical tool hue is a scarce channel, and spending it on buttons means every
reading competes with chrome. It also leaves trend, status, and freshness
mutually distinguishable, which they would not be with a sixth brand hue in play.
`content-link` is the one near-exception: a chroma so low it reads as ink, with
the underline carrying the affordance.

**Freshness decays in chroma, not in contrast.** `fresh → aging → stale` hold the
same hue and lose saturation; all three stay above 4.5:1 because a stale label
still has to be readable. `unavailable` drops to neutral and adds
`hatch-unavailable` — the state has to survive greyscale, print, and
forced-colors, so it cannot rest on hue.

**Provenance leans on pattern.** `hatch-derived` over `bg-derived-subtle` marks a
field a model inferred rather than observed. The hatch is deliberately faint; the
dimmer `text-derived` ink is the primary signal, and the hatch is the channel that
survives when colour does not. `--hatch-angle`, `--hatch-gap`, and `--hatch-line`
are exposed so an SVG `<pattern>` in a chart matches the DOM utility exactly.

#### Series colours

Slot order is a safety mechanism, not a preference. Two constraints shaped it:

- **Trend hues sit in slots 7 and 8.** A chart with six or fewer series never
  puts a series colour in the same hue family as a trend chip.
- **Adjacent slots stay separable** under protanopia and deuteranopia — worst
  adjacent pair ΔE 8.4 (target ≥ 8) and 19.3 unsimulated (floor ≥ 15), in both
  modes.

Assign slots in fixed order and never cycle. A ninth series is not a new hue: it
takes `series-other`, which is achromatic precisely so it reads as "everything
else" rather than as another identity. `composition-chart.tsx` does this in
`seriesPalette()`; d3's ordinal scale would otherwise wrap a short range and paint
series 9 as series 1.

That order costs something, and the cost is documented rather than hidden:
scatter, bubble, choropleth, and small-multiple charts — where any two marks can
end up adjacent — carry a **two-slot cap**. Past two, facet or fold into
`series-other`. No such form exists in the app today; every current chart is a
bar, line, area, stack, or histogram, where only neighbouring slots touch.

Series colours never wear status or trend meaning, and status colours are never
"series 4". Where a series genuinely means good/bad, it wears status tokens
instead — never both in one chart. Several series and status colours share a hue
family (there are more roles than distinguishable hues), so anything carrying
state ships with an icon and a label, never colour alone.

`ramp-1…7` is sequential — magnitude only, `ramp-1` receding toward the surface,
and the anchor flips in dark mode. `delta-down-3 … delta-mid … delta-up-3` is
diverging, built on the trend poles so a heatmap of change and a delta chip agree
about which way is up.

#### Migrating shadcn's names

`shadcn add` writes the upstream palette. Translate it before use:

| shadcn | Market Scout |
|---|---|
| `background` | `surface-page` (`surface-raised` for a chart mark's separator stroke) |
| `foreground`, `card-foreground`, `popover-foreground`, `accent-foreground`, `secondary-foreground` | `content-primary` |
| `muted-foreground` | `content-muted` |
| `card` | `surface-raised` |
| `popover` | `surface-overlay` |
| `muted` | `surface-sunken` |
| `accent` (a hover fill upstream) | `surface-row-hover` |
| `primary` / `primary-foreground` | `accent-solid` / `on-solid` |
| `secondary` | `accent-subtle` |
| `destructive` | `danger-solid`, or `danger-ink` for text |
| `border` | `edge` |
| `input` | `border-edge-strong` for the border, `bg-surface-sunken` for the field |
| `ring` | `focus` |
| `chart-1`…`chart-5` | `series-1`…`series-5` |

Upstream leans on opacity where a real token now exists — `hover:bg-primary/80`
becomes `hover:bg-accent-hover`, `bg-destructive/10 text-destructive` becomes
`bg-danger-subtle text-danger-ink`. Prefer the token; the alpha fudge does not
survive a surface change.

`pnpm theme:check` scans source for retired names and for any colour utility
naming a token that does not exist. That check exists because the failure is
otherwise invisible: Tailwind emits no rule for a colour it cannot resolve and
raises no error, so the component renders unstyled and the build stays green.

One naming collision to expect: `accent-` is both a family here and Tailwind's
`accent-color` utility, so styling a native checkbox reads `accent-accent-solid`.
It is correct, and it compiles.

`--sidebar-*` is the one upstream namespace still standing, because
`components/ui/sidebar.tsx` reads those names directly. They are aliases of the
tokens above, never independent values, so the sidebar cannot drift.

#### Changing a value

`app/theme.css` is generated. Do not hand-edit it — the families are calibrated
against each other, and a single tuned-by-eye value breaks the two invariants
above silently, because nothing in the Next build checks contrast.

```bash
# edit scripts/theme/tokens.mjs, then
pnpm theme:build && pnpm theme:check
```

`theme:check` reports every contrast pair, every chroma value clamped to fit
sRGB, and whether `theme.css` still matches the spec; it exits non-zero on any
failure and runs first in `pnpm preflight`. Review the result by eye in the
`Theme/Colour tokens` Storybook story, which renders every family in both modes.

Adding a family is the same loop plus two edits the generator cannot infer: a row
in `GROUPS` in `scripts/theme/generate.mjs` (which fixes the emitted order and the
section comment) and the checks that family needs in `check.mjs`. A token absent
from `GROUPS` is silently never emitted.

### Deprecated utilities

Tailwind 4.3.3 keeps these for backward compatibility. This project is new — never use them.

| Deprecated | Use |
|---|---|
| `rounded` | `rounded-sm` |
| `shadow` | `shadow-sm` |
| `shadow-inner` | `inset-shadow-sm` |
| `drop-shadow` | `drop-shadow-sm` |
| `blur` | `blur-sm` |
| `max-w-prose` | a container token, or `max-w-[65ch]` |

`max-w-prose` also traps in the other direction. Tailwind's deprecated `--max-width-prose: 65ch` outranks any `--container-prose` you define, so the token is ignored with no error. Don't define one.

### Container widths

`app/tokens.css` defines `--container-narrow`, `--container-content`, and `--container-wide` — used as `max-w-narrow`, `max-w-content`, `max-w-wide`. Tailwind's own `3xs` through `7xl` remain available alongside them.

`app/globals.css` owns type and radius. `app/theme.css` owns every `--color-*`. `app/tokens.css` emits neither — containers only.

CSS requires every `@import` to precede other rules, so `app/tokens.css` imports at the top of `app/globals.css`.

## Storybook

**Stories live under `components/**`.** The glob in `.storybook/main.ts` scans nowhere else — a story placed elsewhere silently never loads.

**The font decorator in `.storybook/preview.ts` is load-bearing.** `app/globals.css` imports the variable fonts and defines the `font-sans` utility. The app layout applies it to the document root. Storybook does not render that layout, so the decorator applies `font-sans` around every story and keeps previews on the app's body font.

Use Storybook to exercise reusable interactive states. The a11y addon checks stories, but does not replace keyboard and focus review in the rendered app. See [`web-testing-guide.md`](./web-testing-guide.md).

## Commands

Run every web command from `apps/web/`. The supported toolchain is Node 26.7.0 and pnpm 11.21.0 or newer, declared as ranges in `package.json`. Install pnpm, then install exactly what the lockfile declares:

```bash
npm install --global pnpm
cd apps/web
pnpm install --frozen-lockfile
```

`packageManager` names the pnpm version in use rather than pinning an older one. When it names a version below the one installed, pnpm self-installs that version as a native binary — and pnpm 11 ships no macOS x64 build, so on an Intel Mac every script fails before it runs. Raise `packageManager` when you upgrade pnpm.

| Command | Notes |
|---|---|
| `pnpm preflight` | Canonical local and CI gate: palette checks, typecheck, DB-free tests, Storybook build, production build. Does not need database credentials. Run before handoff or CI. |
| `pnpm dev` | Next dev server. Link `../../.env.local` as `.env.local` first when the route reads Postgres. |
| `pnpm typecheck` | `tsc --noEmit`. Covers `.storybook/` too. Run while iterating on types. |
| `pnpm test` | DB-free Vitest suite. Never connects to Postgres. |
| `pnpm test:db` | Optional view integration suite. Requires `DATABASE_URL` and `DATABASE_URL_RO`; skipped tests are not verification. |
| `pnpm storybook` | Dev server on port 6006. |
| `pnpm build-storybook` | Compiles every story. Included in `pnpm preflight`. |
| `pnpm build` | Production Next build. Included in `pnpm preflight`. |
| `pnpm theme:check` | Three guards: contrast and ordering over the palette, `app/theme.css` still matching `scripts/theme/tokens.mjs`, and no source file naming a colour token that does not exist. No dependencies. Runs first in `pnpm preflight`. |
| `pnpm theme:build` | Regenerates `app/theme.css` from `scripts/theme/tokens.mjs`. Run after any palette edit. |

## Frontend Definition of Done

Before handoff, a frontend change has:

- A loading state, empty state, and actionable error retry wherever its data can be pending, absent, or unavailable.
- Narrow and wide viewport review. Layout changes must preserve readable content and usable controls at both.
- Keyboard and focus review for every interactive control. Labels, semantics, and contrast must remain clear.
- Honest data states. Show missing, partial, unmapped, and unavailable data rather than implying a complete answer.
- Relevant Storybook states and theme review for reusable or themed components. Run the a11y addon where a story exists.

Run `pnpm preflight`. Add `pnpm test:db` when a changed query, view contract, or read-only grant needs integration coverage and the two DSNs are available.
