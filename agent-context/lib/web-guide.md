# Web App Guide

> **Read this when:** building or changing anything under `apps/web/` — pages, components, tokens, or Storybook.
> **Key invariant:** TypeScript tokens are the source; CSS, class maps, and prop types are generated from them. Never the reverse.
> **Related:** [Project Overview](./project.md) · [Style Guide](./style-guide.md) · [Development Guide](./developer-guide.md) §5.8

---

## What applies from the Development Guide

`developer-guide.md` is Go-first. Working in `apps/web`, read only:

| Section | Why |
|---|---|
| §1 Implementation Quality | Universal. Deliver the impact; surface deviations. |
| §4.1, §4.3, §4.5 File organization | Universal. §4.2's package advice and §4.4's density rule are Go-specific. |
| §5.8 Generated files | Owns the `pnpm tokens` row. The never-hand-edit rule is absolute here too. |
| §7.1, §7.3, §7.4 Comments | Universal. §7.2 is Go package doc comments. |

Skip §2, §3, §5.1–5.7, and §6 — Go setup, Go build pipeline, Go conventions, and Go logging. Nothing in them governs this app.

`testing-guide.md` is Go-only. `apps/web` has no JavaScript test runner yet; verification is `tsc --noEmit`, the builds, and review in Storybook.

## Layout

```
apps/web/
  app/                     # App Router. globals.css imports the generated theme.
  components/
    primitives/            # Hand-written token-driven layout layer
    style-guide/           # Storybook-only reference stories
    ui/                    # Vendored shadcn — treat as third-party
  lib/utils.ts             # cn() — shadcn's tailwind-merge helper
  tokens/                  # Canonical: spacing.ts, typography.ts, layout.ts
    generated/             # Never hand-edit. See developer-guide §5.8.
  scripts/build-tokens.ts  # The generator
  .storybook/
```

**`ui/` is vendored, `primitives/` is ours.** shadcn components are copies that `shadcn add` can update; rewriting one forfeits that. Compose them into primitive layout instead of replacing them. Their Tailwind classes are already inside a component file, which is where classes belong.

## Primitives

`Box`, `Text`, `Stack`, `Grid`/`GridItem`, `Container`. Each is a named export from its own file — no barrel — and renders exactly one element.

**Never import `cn` inside `components/primitives/`. Join with `clsx`.** `cn` runs tailwind-merge, which doesn't know the custom scales and drops classes silently: `twMerge('text-400 text-muted-foreground')` returns only the color, `twMerge('font-600 font-sans')` only the family. Any prefix carrying two meanings is at risk. `components/ui/` and `app/layout.tsx` keep using `cn` — the ban is scoped to primitives.

**Prop sets are closed.** `className` and `style` are typed `never`, not merely omitted — `Omit` alone still admits a spread object. Each primitive has a `.type-test.tsx` sibling asserting the bans with `@ts-expect-error`; an unused one is itself a type error, so `tsc --noEmit` fails the moment a ban stops working.

**No escape hatch, deliberately.** A page that can't express itself signals either a one-off (build a real component) or a gap in the scale (fix the scale). An escape hatch converts both signals into silence. This is also why `Text` has no `variant` prop: a named bundle hides that a recurring pattern exists, and grows one entry at a time until the variant list is the design system.

Overlapping props like `p` and `px` resolve by Tailwind's stylesheet order, exactly as the raw utilities would. Primitives don't deduplicate.

## Tokens

Edit a module under `tokens/`, run `pnpm tokens`. The generator emits the `@theme` block, `@utility` blocks, static class maps, and prop union types — one source, so the scale and the API can't drift.

Numeric scales run 100–900 with neutral at 400, mirroring `font-weight`. The gaps are load-bearing: a step can land at 450 later without a rename cascade.

Traps, all of which fail silently:

- **Class strings must appear complete and literal** in generated output. Tailwind scans source as plain text and can't resolve a runtime-built name.
- **No case conversion in property names.** The custom property is `--<namespace>-<key>` from the token key verbatim. camelCase-to-kebab is the drift this rule exists to prevent.
- **Never emit `--container-prose`.** Tailwind's deprecated `--max-width-prose: 65ch` wins over it, so the token is ignored with no error.
- **A token name matching a Tailwind built-in** either shadows it or is dropped, depending on namespace. The generator's probe checks both directions against a deliberate-override list; keep it passing.
- **Never emit a centering class as a literal** (`mx-auto` in `Container`). A literal sits last in the stylesheet and outranks every caller-passed `mx`, making the prop unreachable.

shadcn owns color and radius through its `@theme inline` block. The token layer emits no `--color-*` or `--radius-*`, and Tailwind's numeric `--spacing` multiplier stays — shadcn's bundled CSS computes against it in 34 places.

## Commands

Run from `apps/web/`:

| Command | Notes |
|---|---|
| `pnpm tokens` | Regenerate. `--check` exits non-zero on drift. |
| `pnpm typecheck` | `tsc --noEmit`. Consumes the type tests. |
| `pnpm dev` | Regenerates first. |
| `pnpm build` | Runs `pnpm tokens --check` before `next build`. |
| `pnpm storybook` / `pnpm build-storybook` | The only check that every story compiles. |

Stories must live under `components/**` — the Storybook glob in `.storybook/main.ts` scans nowhere else, so a story beside its tokens silently never loads.
