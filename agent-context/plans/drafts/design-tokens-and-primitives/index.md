# Design Tokens and Layout Primitives

## Goal

Give `apps/web` a token-driven primitive layer — `Box`, `Text`, `Stack`, `Grid`, `Container` — so page and feature code composes layout from typed props instead of Tailwind class strings. A TypeScript token module becomes the single source for both the Tailwind `@theme` block and the primitives' prop types, so the style scale and the API cannot drift apart.

This is the foundation the seven views in `research/ui-view-catalog.md` get built on. Establishing it before product screens exist is the cheap moment; retrofitting it after is not.

## Scope

### In scope

- Token source modules under `apps/web/tokens/` — spacing scale, typography variants, breakpoint names, container widths.
- A codegen step emitting three artifacts from those tokens: the `@theme` CSS block, static Tailwind class maps, and prop union types.
- Five primitives with closed prop sets: `Box`, `Text`, `Stack`, `Grid` (with `GridItem`), `Container`.
- Responsive prop support via a per-breakpoint object form.
- Storybook stories for every primitive and a token-reference story.
- Rewriting `app/page.tsx` to use primitives, as the first consumer proving the API.

### Out of scope

- **Semantic color and radius tokens.** shadcn already owns these through the `@theme inline` block in `app/globals.css` and the `--primary`/`--background` custom properties. Replacing them breaks every future `shadcn add`. The token layer adds spacing and typography only, and reads shadcn's color vars where primitives need color.
- **Replacing Tailwind's numeric spacing.** `--spacing: 0.25rem` stays. shadcn's `tailwind.css` computes against it in ~40 places; removing it breaks their utilities.
- **A JavaScript unit-test runner.** No test runner exists in `apps/web` today and adding one is its own decision. Verification for this plan is `tsc --noEmit`, `pnpm build`, the codegen drift check, and Storybook's a11y addon. See Open questions.
- **`Inline`, `Spacer`, `Divider`, `Center`.** Follow-on once the prop conventions have been exercised by real screens.
- **Migrating `components/ui/button.tsx`.** shadcn components are vendored copies; rewriting them forfeits upstream updates. They are component files, so their Tailwind classes are already where classes belong.
- **Arbitrary/escape values** (`p="17px"`). The closed prop set is the point.

## Acceptance criteria

- [ ] `pnpm tokens` regenerates the theme CSS, class maps, and union types from `apps/web/tokens/`; running it a second time produces no file diff.
- [ ] `pnpm tokens --check` exits non-zero when any generated file is stale, and `pnpm build` runs the check before compiling.
- [ ] Every generated file carries a `DO NOT EDIT` header naming `pnpm tokens` as its regenerator, matching the convention in `agent-context/lib/developer-guide.md` §5.8.
- [ ] Changing a spacing token's value in the TypeScript source and rerunning codegen changes the emitted CSS custom property and every utility derived from it, with no hand edit to any `.css` file.
- [ ] Adding a step to the spacing scale in TypeScript makes that step available as a value on every spacing prop, with editor autocomplete, without editing any component file.
- [ ] A spacing step not present in the token source is a TypeScript error at the prop call site, rather than compiling to a Tailwind multiplier value.
- [ ] Numeric Tailwind utilities (`p-4`, `p-2.5`, `h-8`, `size-6`) still compile after the token layer lands, and `components/ui/button.tsx` renders identically to its pre-change appearance.
- [ ] Passing `className` or `style` to any of the five primitives is a TypeScript error.
- [ ] `Box` renders the HTML element named by its `as` prop and defaults to `div`.
- [ ] `Box` applies background, radius, border, shadow, and overflow from its props, and those surfaces track the active light or dark theme without a per-theme prop.
- [ ] `Text` renders the default element for its variant, and `as` overrides that element without changing the visual style — `<Text variant="h1" as="h2">` renders an `h2` styled as `h1`.
- [ ] `Text` colors resolve from shadcn's semantic foreground variables, so the same `color` value stays legible in both themes.
- [ ] Every responsive prop accepts either a bare token or a per-breakpoint object, and the object form emits the base class plus one breakpoint-prefixed class per named breakpoint.
- [ ] `Stack` changes flex direction at breakpoints when given the object form of `direction`, and positions children along both axes via `align` and `justify`.
- [ ] `Grid` sets column count responsively, and `GridItem` spans columns responsively.
- [ ] `Container` constrains width to the named `maxWidth` token, centers itself horizontally, and applies its responsive gutter as inline padding.
- [ ] `app/page.tsx` contains no Tailwind class string and renders the same visual result as before the change.
- [ ] Storybook builds, every primitive has at least one story, and the a11y addon reports no violations on primitive stories.
- [ ] `pnpm build` and `tsc --noEmit` both pass.
- [ ] The generated class-map module is at most 12 KB gzipped.

## Token naming inventory

One name crosses three representations. Pin the transform once; every generated artifact follows it.

| Concept | TS token key | CSS custom property | Tailwind utility | Prop value |
|---|---|---|---|---|
| Spacing step | `spacing[400]` | `--spacing-400` | `p-400`, `gap-400`, `mx-400` | `p={400}` |
| Spacing zero | *(none — Tailwind built-in)* | *(none)* | `p-0` | `p={0}` |
| Type variant | `text.eyebrow` | `--text-eyebrow` (+ `--text-eyebrow--line-height`, `--letter-spacing`, `--font-weight`) | `text-eyebrow` | `variant="eyebrow"` |
| Type variant (multi-word) | `text['body-sm']` | `--text-body-sm` | `text-body-sm` | `variant="body-sm"` |
| Container width | `container.prose` | `--container-prose` | `max-w-prose` | `maxWidth="prose"` |
| Breakpoint | `breakpoints.md` | *(TW default, not re-emitted)* | `md:` prefix | `{ md: ... }` key |

Rule: token keys are already in their final form, so the CSS property is `--<namespace>-<key>` verbatim. No camelCase-to-kebab conversion anywhere — that conversion is the drift risk this table exists to remove.

Spacing props take numbers, not strings. Type variants and container widths stay named, because those values are semantic rather than positions on a scale.

Verified against Tailwind 4.3.3: named `--spacing-*` tokens generate `p-<name>`-style utilities and their responsive variants, coexisting with the numeric `--spacing` multiplier. A `--text-<name>` token with `--line-height`, `--letter-spacing`, and `--font-weight` sub-properties compiles to a single composite class setting all four properties.

## Spacing scale

Steps run 100–900, mirroring `font-weight`. Both scales put their neutral value at 400: `font-weight: 400` is normal text, `--spacing-400` is the 1rem base.

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

The gaps are the point. A step between 400 and 500 lands at 450, a finer one at 425 or 475, and a sub-minimal step at 50 — all without renumbering anything downstream. A t-shirt scale forces a rename cascade or a `2xs`/`3xs` prefix pile-up at exactly that moment.

`0` is not a token. Tailwind ships `p-0` natively, so the class map points step `0` at the built-in utility and no `--spacing-0` property is emitted.

Verified against Tailwind 4.3.3: a defined `--spacing-100` shadows Tailwind's dynamic numeric utility, so `p-100` compiles to `var(--spacing-100)` (4px) rather than `calc(var(--spacing) * 100)` (25rem). Small numeric utilities (`p-4`, `p-2.5`, `h-8`, `size-6`) are unaffected, and neither shadcn's bundled CSS nor `apps/web` source uses any three-digit spacing utility, so nothing existing is shadowed.

## Tasks

### Task 1: Token modules and codegen

Build the token source and the generator every other task consumes.

- Author tokens as `as const` TypeScript modules under `apps/web/tokens/`, one file per namespace. TypeScript is the source of truth, so the scale and the prop types come from one place.
- Emit the `@theme` block to a generated CSS file that `app/globals.css` imports. Keeping it separate from `globals.css` means codegen never has to parse or rewrite a hand-edited file.
- Emit static class maps as nested objects keyed by breakpoint, then by prop value. Tailwind scans source as plain text and cannot resolve a runtime-built class name, so every class string must appear complete and literal in the generated file.
- Emit prop union types derived from the same token keys.
- Write the shared `Responsive<T>` type and a `resolveResponsive` helper by hand next to the generated maps. Primitives call it rather than reimplementing breakpoint walking five times.
- Write the shared DOM-passthrough base prop type here too, not inside `Box`. All five primitives depend on it, and owning it in Task 1 is what keeps Phase 2 concurrent.
- Type that base as `Omit<React.HTMLAttributes<HTMLElement>, 'className' | 'style' | 'color'>` plus `ref`. `color` must be omitted because it collides with the prop name `Text` uses.
- Declare `className?: never` and `style?: never` explicitly rather than relying on `Omit` alone. `Omit` silently permits a spread object; `never` produces a direct, readable error at the call site.
- Run the generator with plain `node` — Node v26 strips TypeScript types natively, so no new dependency is needed.
- Support a `--check` mode that regenerates into memory and exits non-zero on any difference. This is what makes the drift check enforceable in `pnpm build`.
- Commit generated files. Builds must not require the generator to have been run first.

Generate responsive tiers only for props that need them. Full responsive coverage on all fourteen spacing props would produce roughly 900 redundant entries against a 12 KB budget.

| Responsive | Base tier only |
|---|---|
| `p`, `px`, `py`, `m`, `mx`, `my` | `pt`, `pr`, `pb`, `pl`, `mt`, `mr`, `mb`, `ml` |
| `gap`, `gapX`, `gapY`, `direction`, `align`, `justify` | `wrap` |
| `columns`, `colSpan`, `gutter` | — |

- Derive the spacing prop's TypeScript union from the token keys plus a literal `0`. Only defined steps are safe: an undefined three-digit value silently falls back to Tailwind's multiplier, so `p-450` would compile to 112.5rem rather than failing. The union is what makes that unreachable from prop call sites.

Do not:
- Emit `--spacing` itself, or a `--spacing-0`, or any `--color-*` or `--radius-*` token.
- Use `--*: initial` to reset Tailwind's default theme.
- Hand-write any file the generator owns.

### Task 2: Box

The spacing and surface primitive. Renders one element, no layout opinion of its own.

- Constrain `as` to a union of literal tag names — `div`, `section`, `article`, `header`, `footer`, `main`, `aside`, `nav`, `figure`, `ul`, `li`. A fixed union avoids generic polymorphic-component typing, which is where this kind of component usually stalls.
- Build on the shared base prop type from Task 1 rather than declaring a second one, so the `className` ban is defined in exactly one place.
- Carry spacing, plus `bg`, `radius`, `border`, `shadow`, and `overflow` drawn from shadcn's existing custom properties. A closed prop set blocks a page the moment it can't express something, so `Box` needs real surface coverage on day one.

### Task 3: Text

Typography, with visual style decoupled from document structure.

- Map each variant to the composite `text-<variant>` class from Task 1. One token yields one class covering size, line height, tracking, and weight.
- Default `as` per variant: variants named after a tag (`h1`–`h6`) render that tag; `label` renders `label`; `code` renders `code`; every other variant renders `p`. `display` renders `p`, not `h1` — a visual scale must never silently create document structure.
- Let `as` override the default element without affecting the variant's styling. This is the whole point of the split, and the accessibility case for it.
- Take `color` from shadcn's semantic foreground vars, not raw palette values, so light and dark themes keep working.

### Task 4: Stack

One-dimensional flex layout. Supersedes the `HStack`/`VStack` pair.

- Make `direction` responsive, so one component covers what Chakra splits into two.
- Accept the shared spacing props alongside `gap`, `align`, `justify`, and `wrap`.

### Task 5: Grid and GridItem

Two-dimensional layout for scorecards and dashboards.

- Make `columns` and `GridItem`'s `colSpan` responsive; a fixed column count is unusable on the composite views.
- Support `columns="auto"` paired with a `minItemWidth` token, emitting a pregenerated `repeat(auto-fit, minmax(…, 1fr))` class per token value. Card grids need intrinsic wrapping, and it cannot be expressed as a column count.

### Task 6: Container

Page-width shell.

- Center horizontally by default and constrain to a named `maxWidth` token.
- Make `gutter` responsive, since page margins are the most breakpoint-sensitive value in any layout.

### Task 7: Stories and first consumer

- Add a story per primitive, plus one token-reference story rendering the spacing scale and every type variant.
- Rewrite `app/page.tsx` using only primitives. This is the plan's real acceptance test: anything it cannot express is a prop-set gap to fix now, not after other screens depend on it.

Do not add `className` to a primitive to unblock the rewrite. Extend the prop set instead — that pressure is the mechanism this plan is buying.

## Sequencing

**Phase 1 (sequential):** Task 1 — every other task imports its generated maps and types.
**Phase 2 (concurrent):** Tasks 2–6 — one file each, no composition between them; all five share only Task 1's helpers.
**Phase 3 (sequential):** Task 7 — consumes all five primitives.

Primitives must not compose each other. `Stack` rendering a `Box` internally would serialize Phase 2 and make class precedence depend on nesting order.

## Rough sketch

```
apps/web/
  tokens/
    spacing.ts            # canonical
    typography.ts         # canonical
    breakpoints.ts        # canonical — names only; values stay Tailwind defaults
    generated/
      theme.css           # @theme block, imported by app/globals.css
      class-maps.ts       # static Tailwind class strings
      types.ts            # Space, TextVariant, Breakpoint unions
  components/primitives/
    shared.ts             # Responsive<T>, resolveResponsive(), base prop type — hand-written
    box.tsx  text.tsx  stack.tsx  grid.tsx  container.tsx
  scripts/build-tokens.ts
```

```ts
// Proposed design
type Responsive<T> = T | { base?: T; sm?: T; md?: T; lg?: T; xl?: T; '2xl'?: T }

<Stack direction={{ base: 'col', md: 'row' }} gap={500} align="center">
  <Text variant="eyebrow" color="muted">Demand trend</Text>
  <Text variant="h1" as="h2">Stripe</Text>
</Stack>
```

`resolveResponsive` walks the breakpoint keys in order and indexes the generated map per tier, so `direction={{ base: 'col', md: 'row' }}` yields `flex-col md:flex-row`. Primitives keep using `cn` internally for conflict resolution even though no external `className` can arrive.

## Design principle: friction is the feedback

The closed prop set is not a constraint to work around. These primitives are atomic, so their props should cover ordinary layout completely. A page that cannot express itself in them signals one of two things, and both are worth hearing:

- **A one-off need** — build a real component for it. That is the intended outcome, and it keeps the exception named and reviewable instead of scattered as inline classes.
- **A recurring need** — the system is wrong. Repeated reaching for the same missing value means the spacing scale has a gap, the color tokens are too rigid, or a prop is missing. Fix the system, not the call site.

An escape hatch would convert both signals into silence. That is the reason there isn't one.

Practical consequence for Task 7: log what `app/page.tsx` cannot express rather than routing around it. The list is the first real measurement of whether the scale is right.

## Open questions

- **Test runner.** Planned as the immediate follow-up, not a someday item. The primitives are pure value-to-class functions and `resolveResponsive` is the one piece with real branching, so the highest-value tests are cheap. Vitest is the likely fit, but it is a new dependency and the first JavaScript testing convention in a repo whose `testing-guide.md` is Go-only — which is why it is its own decision rather than a bullet inside this plan.
- **`agent-context/lib/` capture at promotion.** Three durable facts belong in the library, not in this plan: the token pipeline's direction (TypeScript source, CSS generated, never the reverse) and the closed-prop-set principle above, both in `project.md`; and the generated-file rows for `pnpm tokens` in `developer-guide.md` §5.8.
