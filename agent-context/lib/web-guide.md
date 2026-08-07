# Web App Guide

> **Read this when:** building or changing anything under `apps/web/` — pages, components, styles, or Storybook.
> **Key invariant:** tailwind-merge reads any bare word in a color namespace as a color. A custom utility named `bg-brand` or `text-body` is dropped by `cn()` silently — no error, no warning.
> **Related:** [Project Overview](./project.md) · [Development Guide](./developer-guide.md) · [Style Guide](./style-guide.md)

---

## What applies from the Development Guide

That guide is Go-first. Working in `apps/web`, read only:

| Section | Why |
|---|---|
| §1 Implementation Quality | Universal — including §1.5's light lane, which is why most UI work skips the plan pipeline. |
| §4.1, §4.3, §4.5 File organization | Universal. §4.2's package advice and §4.4's density rule are Go-specific. |
| §7.1, §7.3, §7.4 Comments | Universal. §7.2 covers Go package doc comments. |

Skip §2, §3, §5, and §6 — Go setup, Go build, Go conventions, Go logging. §5.8 covers sqlc; nothing in `apps/web` is generated.

`testing-guide.md` is Go-only. `apps/web` has no JavaScript test runner. Verification is `tsc --noEmit`, the two builds, and review in Storybook.

## Layout

```
apps/web/
  app/                # App Router. globals.css is the single stylesheet entry.
  components/ui/      # Vendored shadcn
  lib/utils.ts        # cn() — shadcn's clsx + tailwind-merge helper
  tokens.css          # The one @theme block we own
  .storybook/
```

**`components/ui/` is vendored.** shadcn components are copies that `shadcn add` updates in place. Rewriting one forfeits that — compose instead.

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

This is also why `tokens.css` defines no `--text-*`, `--font-weight-*`, `--leading-*`, or `--tracking-*`. The type scale is Tailwind's, wholesale.

Colors are not chosen yet. Revisit this section when they are.

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

`tokens.css` defines `--container-narrow`, `--container-content`, and `--container-wide` — used as `max-w-narrow`, `max-w-content`, `max-w-wide`. Tailwind's own `3xs` through `7xl` remain available alongside them.

shadcn owns color and radius through the `@theme inline` block in `globals.css`. `tokens.css` emits no `--color-*` or `--radius-*`.

CSS requires every `@import` to precede other rules, so `tokens.css` imports at the top of `globals.css`.

## Storybook

**Stories live under `components/**`.** The glob in `.storybook/main.ts` scans nowhere else — a story placed elsewhere silently never loads.

**The font decorator in `.storybook/preview.ts` is load-bearing.** `globals.css` declares `--font-sans: var(--font-sans)` self-referentially, and only `app/layout.tsx` breaks that cycle. Storybook does not render the layout, so without the decorator every story falls back to the browser default serif.

## Commands

From `apps/web/`:

| Command | Notes |
|---|---|
| `pnpm dev` | Next dev server. |
| `pnpm build` | `next build`. Typechecks as part of the build. |
| `pnpm typecheck` | `tsc --noEmit`. Covers `.storybook/` too. |
| `pnpm storybook` | Dev server on port 6006. |
| `pnpm build-storybook` | The only check that every story compiles. |
