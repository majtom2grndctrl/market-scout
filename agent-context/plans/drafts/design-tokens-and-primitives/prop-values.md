# Prop Value Tables

> **Read this when:** implementing any task in `index.md`. Every enumerated prop value and the exact class it emits.
> **Key invariant:** these tables are the closed prop set. A value absent here is a TypeScript error, by design.
> **Related:** `index.md` (the plan; spacing and type scales live there)

---

## Type size

Numeric, 100–900. Each step carries a paired default line height, emitted as `--text-<step>--line-height`.

| Step | font-size | px | Paired line height | Typical use |
|---|---|---|---|---|
| 100 | `0.75rem` | 12 | 1.4 | Eyebrow, caption, legal |
| 200 | `0.8125rem` | 13 | 1.4 | Dense table cells, metadata |
| 300 | `0.875rem` | 14 | 1.5 | Secondary body, labels |
| 400 | `1rem` | 16 | 1.6 | Body |
| 500 | `1.25rem` | 20 | 1.5 | Lead paragraph, h6 |
| 600 | `1.5rem` | 24 | 1.4 | h5, h4 |
| 700 | `2rem` | 32 | 1.3 | h3 |
| 800 | `2.5rem` | 40 | 1.2 | h2 |
| 900 | `3.5rem` | 56 | 1.1 | h1, display |

Tailwind puts the paired line height behind `var(--tw-leading, …)`, so a `leading` prop overrides it with no specificity conflict. Verified against Tailwind 4.3.3.

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
| `strong` | *(inherit)* | 700 |
| `em` | *(inherit)* | *(inherit)* |
| `blockquote` | 500 | 400 |
| `small` | 300 | 400 |
| `figcaption`, `caption` | 200 | 400 |
| `li`, `dt`, `dd`, `cite`, `abbr`, `time` | 400 | 400 |

`dt` takes weight 600. Defaults are the ramp; overriding `size` on a heading is how visual level decouples from document structure.

## Text weight

| Value | Emitted class | CSS |
|---|---|---|
| 400 | `font-400` | 400 |
| 500 | `font-500` | 500 |
| 600 | `font-600` | 600 |
| 700 | `font-700` | 700 |

## Text leading

Overrides the size's paired default.

| Value | Emitted class | CSS |
|---|---|---|
| `tight` | `leading-tight` | 1.1 |
| `snug` | `leading-snug` | 1.3 |
| `normal` | `leading-normal` | 1.5 |
| `relaxed` | `leading-relaxed` | 1.7 |

## Text tracking

| Value | Emitted class | CSS |
|---|---|---|
| `tight` | `tracking-tight` | -0.02em |
| `normal` | `tracking-normal` | 0 |
| `wide` | `tracking-wide` | 0.04em |
| `wider` | `tracking-wider` | 0.08em |

## Text color

Every value resolves to a shadcn semantic variable, so it survives a theme switch.

| Value | Emitted class | shadcn property |
|---|---|---|
| `default` | `text-foreground` | `--foreground` |
| `muted` | `text-muted-foreground` | `--muted-foreground` |
| `primary` | `text-primary` | `--primary` |
| `accent` | `text-accent-foreground` | `--accent-foreground` |
| `destructive` | `text-destructive` | `--destructive` |
| `inverse` | `text-primary-foreground` | `--primary-foreground` |

`inverse` is for text sitting on a `primary` surface.

## Text — remaining props

| Prop | Values | Emitted class |
|---|---|---|
| `family` | `sans`, `mono` | `font-sans`, `font-mono` |
| `align` | `start`, `center`, `end` | `text-start`, `text-center`, `text-end` |
| `transform` | `none`, `uppercase`, `lowercase`, `capitalize` | `normal-case`, `uppercase`, `lowercase`, `capitalize` |
| `truncate` | `true` | `truncate` |
| `lineClamp` | `1`–`6` | `line-clamp-<n>` |
| `wrap` | `normal`, `balance`, `pretty`, `nowrap` | `text-wrap`, `text-balance`, `text-pretty`, `text-nowrap` |

## Box — background

| Value | Emitted class |
|---|---|
| `none` | *(no class)* |
| `background` | `bg-background` |
| `card` | `bg-card` |
| `popover` | `bg-popover` |
| `primary` | `bg-primary` |
| `secondary` | `bg-secondary` |
| `muted` | `bg-muted` |
| `accent` | `bg-accent` |
| `destructive` | `bg-destructive` |

## Box — radius

Drawn from shadcn's `--radius-*` ladder in `app/globals.css`.

| Value | Emitted class |
|---|---|
| `none` | `rounded-none` |
| `sm`, `md`, `lg`, `xl`, `2xl`, `3xl`, `4xl` | `rounded-<value>` |
| `full` | `rounded-full` |

## Box — border

Border color comes from the `*` base rule in `app/globals.css`, which already applies `border-border`. These props set width only.

| Value | Emitted class |
|---|---|
| `none` | *(no class)* |
| `all` | `border` |
| `top`, `right`, `bottom`, `left` | `border-t`, `border-r`, `border-b`, `border-l` |
| `x`, `y` | `border-x`, `border-y` |

## Box — shadow

Tailwind built-ins. shadcn defines no `--shadow-*` property.

| Value | Emitted class |
|---|---|
| `none` | `shadow-none` |
| `sm`, `md`, `lg`, `xl` | `shadow-<value>` |

## Box — overflow

| Value | Emitted class |
|---|---|
| `visible` | `overflow-visible` |
| `hidden` | `overflow-hidden` |
| `clip` | `overflow-clip` |
| `auto` | `overflow-auto` |
| `x-auto`, `y-auto` | `overflow-x-auto`, `overflow-y-auto` |

## Box and Stack — sizing

Viewport sizing is why `app/page.tsx` could not be expressed without these.

| Prop | Values | Emitted class |
|---|---|---|
| `width` | `auto`, `full`, `fit`, `min`, `max` | `w-<value>` |
| `height` | `auto`, `full`, `screen`, `svh`, `dvh` | `h-<value>` |
| `minHeight` | `none`, `full`, `screen`, `svh`, `dvh` | `min-h-<value>` |

`svh` and `dvh` are Tailwind built-ins. `min-h-none` emits no class.

## Stack — direction, align, justify, wrap

| Prop | Values | Emitted class |
|---|---|---|
| `direction` | `row`, `row-reverse`, `col`, `col-reverse` | `flex-<value>` |
| `align` | `start`, `center`, `end`, `stretch`, `baseline` | `items-<value>` |
| `justify` | `start`, `center`, `end`, `between`, `around`, `evenly` | `justify-<value>` |
| `wrap` | `nowrap`, `wrap`, `wrap-reverse` | `flex-<value>` |

## Grid — columns and spans

| Prop | Values | Emitted class |
|---|---|---|
| `columns` | `1`–`12` | `grid-cols-<n>` |
| `columns` | `auto` | the auto-fit utility for the given `minItemWidth` |
| `colSpan` | `1`–`12`, `full` | `col-span-<n>`, `col-span-full` |
| `rowSpan` | `1`–`6`, `full` | `row-span-<n>`, `row-span-full` |

## Grid — minItemWidth

Only meaningful with `columns="auto"`. Each value needs a generated `@utility` block, because `repeat(auto-fit, minmax(…, 1fr))` has no theme-token equivalent.

| Value | Track minimum | Emitted utility |
|---|---|---|
| `xs` | `12rem` | `grid-auto-xs` |
| `sm` | `16rem` | `grid-auto-sm` |
| `md` | `20rem` | `grid-auto-md` |
| `lg` | `24rem` | `grid-auto-lg` |

## Container — maxWidth

Names avoid Tailwind's `--container-*` defaults (`xs` through `7xl`) and its deprecated `--max-width-*` scale, so nothing existing is shadowed.

| Value | Width | Emitted class |
|---|---|---|
| `narrow` | `32rem` | `max-w-narrow` |
| `content` | `45rem` | `max-w-content` |
| `wide` | `72rem` | `max-w-wide` |
| `full` | *(none)* | `max-w-full` |

`full` uses Tailwind's built-in and emits no token.
