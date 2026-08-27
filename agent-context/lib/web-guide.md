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
  app/                # App Router. globals.css is the stylesheet entry; tokens.css holds app-owned containers.
  components/ui/      # Vendored shadcn
  lib/db/             # Read-only Postgres queries for Server Components
  lib/utils.ts        # cn() — shadcn's clsx + tailwind-merge helper
  .storybook/
```

**`components/ui/` is vendored.** shadcn components are copies that `shadcn add` updates in place. Rewriting one forfeits that — compose instead.

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

The current palette is a neutral semantic base. Use semantic colors such as `background`, `foreground`, `card`, and `muted`; they express role, not a chosen brand. No bespoke brand palette has been selected yet.

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

shadcn owns color and radius through the `@theme inline` block in `app/globals.css`. `app/tokens.css` emits no `--color-*` or `--radius-*`.

CSS requires every `@import` to precede other rules, so `app/tokens.css` imports at the top of `app/globals.css`.

## Storybook

**Stories live under `components/**`.** The glob in `.storybook/main.ts` scans nowhere else — a story placed elsewhere silently never loads.

**The font decorator in `.storybook/preview.ts` is load-bearing.** `app/globals.css` imports the variable fonts and defines the `font-sans` utility. The app layout applies it to the document root. Storybook does not render that layout, so the decorator applies `font-sans` around every story and keeps previews on the app's body font.

Use Storybook to exercise reusable interactive states. The a11y addon checks stories, but does not replace keyboard and focus review in the rendered app. See [`web-testing-guide.md`](./web-testing-guide.md).

## Commands

Run every web command from `apps/web/`. The supported toolchain is Node 26.7.0 and pnpm 11.21.0, pinned in `package.json`. After installing Node 26.7.0, install that pnpm version and then install exactly what the lockfile declares:

```bash
npm install --global pnpm@11.21.0
cd apps/web
pnpm install --frozen-lockfile
```

| Command | Notes |
|---|---|
| `pnpm preflight` | Canonical local and CI gate: typecheck, DB-free tests, Storybook build, production build. Does not need database credentials. Run before handoff or CI. |
| `pnpm dev` | Next dev server. Link `../../.env.local` as `.env.local` first when the route reads Postgres. |
| `pnpm typecheck` | `tsc --noEmit`. Covers `.storybook/` too. Run while iterating on types. |
| `pnpm test` | DB-free Vitest suite. Never connects to Postgres. |
| `pnpm test:db` | Optional view integration suite. Requires `DATABASE_URL` and `DATABASE_URL_RO`; skipped tests are not verification. |
| `pnpm storybook` | Dev server on port 6006. |
| `pnpm build-storybook` | Compiles every story. Included in `pnpm preflight`. |
| `pnpm build` | Production Next build. Included in `pnpm preflight`. |

## Frontend Definition of Done

Before handoff, a frontend change has:

- A loading state, empty state, and actionable error retry wherever its data can be pending, absent, or unavailable.
- Narrow and wide viewport review. Layout changes must preserve readable content and usable controls at both.
- Keyboard and focus review for every interactive control. Labels, semantics, and contrast must remain clear.
- Honest data states. Show missing, partial, unmapped, and unavailable data rather than implying a complete answer.
- Relevant Storybook states and theme review for reusable or themed components. Run the a11y addon where a story exists.

Run `pnpm preflight`. Add `pnpm test:db` when a changed query, view contract, or read-only grant needs integration coverage and the two DSNs are available.
