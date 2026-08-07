# Prop Value Tables

> **Read this when:** implementing any task in `index.md`. Every enumerated prop value and the exact class it emits.
> **Key invariant:** these tables are the complete prop surface. Every prop of every primitive appears here with its value set, emitted class, and responsive tier. A value absent here is a TypeScript error, by design.
> **Related:** `index.md` (the plan; spacing and type scales live there)

---

## Spacing props

Values for every prop below: `0`, `100`, `200`, `300`, `400`, `500`, `600`, `700`, `800`, `900`.

| Step | Value | px |
|---|---|---|
| 0 | `0` | 0 |
| 100 | `0.25rem` | 4 |
| 200 | `0.5rem` | 8 |
| 300 | `0.75rem` | 12 |
| 400 | `1rem` | 16 |
| 500 | `1.5rem` | 24 |
| 600 | `2rem` | 32 |
| 700 | `3rem` | 48 |
| 800 | `4rem` | 64 |
| 900 | `6rem` | 96 |

| Prop | Emitted class | Tier |
|---|---|---|
| `p` | `p-<step>` | responsive |
| `px` | `px-<step>` | responsive |
| `py` | `py-<step>` | responsive |
| `pt`, `pr`, `pb`, `pl` | `pt-<step>` etc | base |
| `m` | `m-<step>` | responsive |
| `mx` | `mx-<step>`, plus `mx-auto` for `auto` | responsive |
| `my` | `my-<step>` | responsive |
| `mt`, `mr`, `mb`, `ml` | `mt-<step>` etc | base |
| `gap` | `gap-<step>` | responsive |
| `gapX` | `gap-x-<step>` | responsive |
| `gapY` | `gap-y-<step>` | responsive |

`auto` is valid only on `mx`, and only `Container` uses it — it is how Container centers.

Step `0` maps to Tailwind's built-in `p-0`, `gap-0`, and so on; no `--spacing-0` property is emitted.

`Box`, `Stack`, `Grid`, `GridItem`, and `Container` all take the padding and margin props. Only `Stack` and `Grid` take the gap props.

## Type size

Numeric, 100–900. Each step carries a paired default line height, emitted as `--text-<step>--line-height`.

| Step | font-size | px | Paired line height | Typical use | Tier |
|---|---|---|---|---|---|
| 100 | `0.75rem` | 12 | 1.4 | Eyebrow, legal | responsive |
| 200 | `0.8125rem` | 13 | 1.4 | Dense table cells, metadata | responsive |
| 300 | `0.875rem` | 14 | 1.5 | Secondary body | responsive |
| 400 | `1rem` | 16 | 1.6 | Body | responsive |
| 500 | `1.25rem` | 20 | 1.5 | Lead paragraph | responsive |
| 600 | `1.5rem` | 24 | 1.4 | Section heading | responsive |
| 700 | `2rem` | 32 | 1.3 | Page heading | responsive |
| 800 | `2.5rem` | 40 | 1.2 | Page heading | responsive |
| 900 | `3.5rem` | 56 | 1.1 | Page heading | responsive |

Tailwind puts the paired line height behind `var(--tw-leading, …)`, so a `leading` prop overrides it with no specificity conflict. Verified against Tailwind 4.3.3.

Element defaults are the authoritative mapping; this column is orientation only.

## Element defaults

`as` alone renders correctly styled text. `size`, `weight`, and `leading` override per call site.

| `as` | Default size | Default weight |
|---|---|---|
| `h1` | 900 | 700 |
| `h2` | 800 | 700 |
| `h3` | 700 | 600 |
| `h4` | 600 | 600 |
| `h5` | 500 | 600 |
| `h6` | 400 | 600 |
| `p` | 400 | 400 |
| `span`, `div` | 400 | 400 |
| `label` | 300 | 500 |
| `code`, `kbd`, `samp` | 300 | 400 |
| `strong` | *(no class)* | 700 |
| `em` | *(no class)* | *(no class)* |
| `blockquote` | 500 | 400 |
| `small` | 300 | 400 |
| `figcaption`, `caption` | 200 | 400 |
| `li`, `dd`, `cite`, `abbr`, `time` | 400 | 400 |
| `dt` | 400 | 600 |

Defaults are the ramp; overriding `size` on a heading is how visual level decouples from document structure. Cells marked (no class) emit nothing for that axis, so the element inherits it from its parent.

## Element unions

- `Box`, `Stack`, `Grid`, `GridItem`, `Container`: `div`, `section`, `article`, `header`, `footer`, `main`, `aside`, `nav`, `figure`, `ul`, `li`. Default `div`.
- `Text`: `p`, `span`, `div`, `h1`, `h2`, `h3`, `h4`, `h5`, `h6`, `label`, `code`, `kbd`, `samp`, `strong`, `em`, `blockquote`, `small`, `figcaption`, `caption`, `li`, `dt`, `dd`, `cite`, `abbr`, `time`. Default `p`.

`Text` additionally accepts `htmlFor`, `dateTime`, and `cite` as optional props, because `React.HTMLAttributes<HTMLElement>` carries no element-specific attributes and `label`, `time`, and `blockquote` are unusable without them.

## Breakpoints

| Name | Min width |
|---|---|
| `sm` | 40rem |
| `md` | 48rem |
| `lg` | 64rem |
| `xl` | 80rem |
| `2xl` | 96rem |

Values are Tailwind's defaults and are not re-emitted. The generated class map carries a `base` key holding the unprefixed class, plus one key per name above.

## Text weight

| Value | Emitted class | CSS | Tier |
|---|---|---|---|
| 400 | `font-400` | 400 | base |
| 500 | `font-500` | 500 | base |
| 600 | `font-600` | 600 | base |
| 700 | `font-700` | 700 | base |

## Text leading

Overrides the size's paired default.

| Value | Emitted class | CSS | Tier |
|---|---|---|---|
| `tight` | `leading-tight` | 1.1 | base |
| `snug` | `leading-snug` | 1.3 | base |
| `normal` | `leading-normal` | 1.5 | base |
| `relaxed` | `leading-relaxed` | 1.7 | base |

## Text tracking

| Value | Emitted class | CSS | Tier |
|---|---|---|---|
| `tight` | `tracking-tight` | -0.02em | base |
| `normal` | `tracking-normal` | 0 | base |
| `wide` | `tracking-wide` | 0.04em | base |
| `wider` | `tracking-wider` | 0.08em | base |

## Text color

Every value resolves to a shadcn semantic variable, so it survives a theme switch.

| Value | Emitted class | shadcn property | Tier |
|---|---|---|---|
| `default` | `text-foreground` | `--foreground` | base |
| `muted` | `text-muted-foreground` | `--muted-foreground` | base |
| `primary` | `text-primary` | `--primary` | base |
| `accent` | `text-accent-foreground` | `--accent-foreground` | base |
| `destructive` | `text-destructive` | `--destructive` | base |
| `inverse` | `text-primary-foreground` | `--primary-foreground` | base |

`inverse` is for text sitting on a `primary` surface.

## Text — remaining props

| Prop | Values | Emitted class | Tier |
|---|---|---|---|
| `family` | `sans`, `mono` | `font-sans`, `font-mono` | base |
| `align` | `start`, `center`, `end` | `text-start`, `text-center`, `text-end` | base |
| `transform` | `none`, `uppercase`, `lowercase`, `capitalize` | `normal-case`, `uppercase`, `lowercase`, `capitalize` | base |
| `truncate` | `true`, `false` | `truncate` / *(no class)* | base |
| `lineClamp` | `1`–`6` | `line-clamp-<n>` | base |
| `wrap` | `normal`, `balance`, `pretty`, `nowrap` | `text-wrap`, `text-balance`, `text-pretty`, `text-nowrap` | base |

## Box — background

| Value | Emitted class | Tier |
|---|---|---|
| `none` | *(no class)* | base |
| `background` | `bg-background` | base |
| `card` | `bg-card` | base |
| `popover` | `bg-popover` | base |
| `primary` | `bg-primary` | base |
| `secondary` | `bg-secondary` | base |
| `muted` | `bg-muted` | base |
| `accent` | `bg-accent` | base |
| `destructive` | `bg-destructive` | base |

## Box — radius

Drawn from shadcn's `--radius-*` ladder in `app/globals.css`.

| Value | Emitted class | Tier |
|---|---|---|
| `none` | `rounded-none` | base |
| `sm`, `md`, `lg`, `xl`, `2xl`, `3xl`, `4xl` | `rounded-<value>` | base |
| `full` | `rounded-full` | base |

## Box — border

Border color comes from the `*` base rule in `app/globals.css`, which already applies `border-border`. These props set width only.

| Value | Emitted class | Tier |
|---|---|---|
| `none` | *(no class)* | base |
| `all` | `border` | base |
| `top`, `right`, `bottom`, `left` | `border-t`, `border-r`, `border-b`, `border-l` | base |
| `x`, `y` | `border-x`, `border-y` | base |

## Box — shadow

Tailwind built-ins. shadcn defines no `--shadow-*` property.

| Value | Emitted class | Tier |
|---|---|---|
| `none` | `shadow-none` | base |
| `sm`, `md`, `lg`, `xl` | `shadow-<value>` | base |

## Box — overflow

| Value | Emitted class | Tier |
|---|---|---|
| `visible` | `overflow-visible` | base |
| `hidden` | `overflow-hidden` | base |
| `clip` | `overflow-clip` | base |
| `auto` | `overflow-auto` | base |
| `x-auto`, `y-auto` | `overflow-x-auto`, `overflow-y-auto` | base |

## Box and Stack — sizing

Viewport sizing is why `app/page.tsx` could not be expressed without these.

| Prop | Values | Emitted class | Tier |
|---|---|---|---|
| `width` | `auto`, `full`, `fit`, `min`, `max` | `w-<value>` | responsive |
| `height` | `auto`, `full`, `screen`, `svh`, `dvh` | `h-<value>` | responsive |
| `minHeight` | `none`, `full`, `screen`, `svh`, `dvh` | `min-h-<value>`, except `none` → `min-h-auto` | responsive |

`svh` and `dvh` are Tailwind built-ins. `none` emits `min-h-auto` rather than no class. `auto` is CSS's initial value, so the base tier behaves as an absent prop does — but an empty class cannot reset a min-height inherited from a lower breakpoint, which is what `none` now makes expressible.

## Display classes

Two primitives emit an unconditional base class. Without it the other layout props do nothing — `flex-col` sets flex-direction on a block element and silently no-ops.

| Component | Always emits |
|---|---|
| `Stack` | `flex` |
| `Grid` | `grid` |

`Container` centers by defaulting `mx` to `auto`, not by emitting a literal class. A literal sits last in the stylesheet and outranks every base-tier `mx` a caller passes.

## Stack — direction, align, justify, wrap

| Prop | Values | Emitted class | Tier |
|---|---|---|---|
| `direction` | `row`, `row-reverse`, `col`, `col-reverse` | `flex-<value>` | responsive |
| `align` | `start`, `center`, `end`, `stretch`, `baseline` | `items-<value>` | responsive |
| `justify` | `start`, `center`, `end`, `between`, `around`, `evenly` | `justify-<value>` | responsive |
| `wrap` | `nowrap`, `wrap`, `wrap-reverse` | `flex-<value>` | base |

`direction` defaults to `col` when unset. A Stack with no direction is a vertical stack.

## Grid — columns and spans

| Prop | Values | Emitted class | Tier |
|---|---|---|---|
| `columns` | `1`–`12` | `grid-cols-<n>` | responsive |
| `columns` | `auto` | the auto-fit utility for the given `minItemWidth` | base (bare value only) |
| `colSpan` | `1`–`12`, `full` | `col-span-<n>`, `col-span-full` | responsive |
| `rowSpan` | `1`–`6`, `full` | `row-span-<n>`, `row-span-full` | responsive |

## Grid — minItemWidth

Only meaningful with `columns="auto"`. Each value needs a generated `@utility` block, because `repeat(auto-fit, minmax(…, 1fr))` has no theme-token equivalent.

| Value | Track minimum | Emitted utility | Tier |
|---|---|---|---|
| `xs` | `12rem` | `grid-auto-xs` | base |
| `sm` | `16rem` | `grid-auto-sm` | base |
| `md` | `20rem` | `grid-auto-md` | base |
| `lg` | `24rem` | `grid-auto-lg` | base |

## Container — maxWidth

Names avoid Tailwind's `--container-*` defaults (`3xs` through `7xl`) and its deprecated `--max-width-*` scale, so nothing existing is shadowed.

| Value | Width | Emitted class | Tier |
|---|---|---|---|
| `narrow` | `32rem` | `max-w-narrow` | responsive |
| `content` | `45rem` | `max-w-content` | responsive |
| `wide` | `72rem` | `max-w-wide` | responsive |
| `full` | *(none)* | `max-w-full` | responsive |

`full` uses Tailwind's built-in and emits no token.
