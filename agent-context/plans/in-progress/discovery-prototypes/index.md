# Discovery prototypes

> Build-session contract — decisions, invariants, track ownership.

## Goal

Five page-scale sketches of the Discovery page, under `apps/web/app/prototypes/discovery/`. Discovery comes after profile setup and before the dashboard. The person has just described their past and is unsure where they fit in a fast-changing market. This is the lowest point of their journey, and the page has to lift them: it recommends roles drawn from their profile and the harvested postings, and lets them pin the ones worth watching. The five differ in feel — tone, layout, typography, motion — and never in data. The owner is a design engineer; visual polish and purposeful motion are the bar, not extras.

## Decisions

| Decision | Consequence |
|---|---|
| Recommend **roles**, display them by **title**. Every card is a canonical role, headlined by `headline`; `titles` supplies the "also called" spellings. | Pins stay canonical roles (settled architecture). An unfamiliar title under a familiar role is the discovery moment. Never headline with `roleName`. |
| **The score is never rendered** — no percentage, no meter, no "match". | The top score is ~0.25 because seven claimed skills cover a slice of any role; "25% match" would deflate the person and claim hireability. Rank, `strength`, `bring`, `grow`, and `closestPast` carry the reasoning. |
| **Pins are local state**, seeded from `recommendation.pinned`. | Testing five pages never writes to the real profile. A reload resets. The real write path (`pinRoleAction`) is settled and out of scope. |
| **Mood palettes are prototype-local.** A variant may define expressive colours as CSS custom properties, prefixed with its name (`--airy-glow`), scoped to its own root. | Lift needs warmth the achromatic accent cannot give. Keeping them local means a winner becomes a token proposal, not a silent token. |
| **`motion` is installed** (`motion/react`, v14). | Layout and shared-element animation (a card moving into a pinned tray), springs, and scroll-linked effects are available. No other new dependency. |
| **The shared read writes its own SQL**, a stated deviation from web-guide §Prototypes. | The ranking compares skill signatures, and the measure grammar has no similarity measure or title grouping. That gap is a finding for the spec. Both surviving safety rules hold: pooled clients only, and "open" and "latest classification" come from the views. |
| **"New this month" is omitted.** | `first_seen_at` is the fetcher's first sighting; a company onboarded last week makes every posting look new. Momentum would overstate. |

## Invariants

**Hard constraints** — breaking one corrupts the experiment or the honesty of the page:

1. **Data comes only from `getDiscoveryData()`** in `_data/query.ts`, called from the variant's Server Component `page.tsx` and passed to Client Components as props. No SQL, no other DB import, no edit to `_data/`. If a variant needs a field that isn't there, report it — don't route around it.
2. **Never render `score`**, and never phrase fit as hireability or a percentage match ("you qualify", "92% match", "you'd get hired"). Coverage language only: what you bring, what it also asks for.
3. **`grow[].share` is P(skill | role)** — the share of this role's postings that ask for the skill. Render it as such ("asked for in 61% of postings"), never as the person's gap or deficiency.
4. **Show coverage somewhere on the page**, even small: ranked from `coverage.classifiedPostings` classified of `coverage.openPostings` open postings. A recommendation from a third of the corpus must not read as the whole market.
5. **Colour.** Real tokens (`apps/web/app/theme.css`) for ink, surfaces, and data marks. Mood colours only through the variant's own `--<variant>-*` custom properties, used via Tailwind's `bg-(--airy-glow)` syntax or in the variant's own CSS. Never edit `theme.css`, `tokens.css`, `globals.css`, or `scripts/theme/`. Never wear `success-*`/`danger-*`/`warning-*` (status) or `trend-*` for fit or strength: a close fit is not a success state, and a stretch is not a failure.
6. **Encouragement is evidence.** Every uplifting line cites something real from the data: a past title, a claimed skill, a posting or company count. No generic affirmations.
7. **Motion directs the eye.** Each animation answers "where should I look next?" — staged reveals in reading order, a pin landing where pins collect. No decorative loops; the one exception is a variant whose concept *is* motion (Marquee's cycling titles). Wrap the variant in `<MotionConfig reducedMotion="user">`.
8. **Pinning.** Toggle per recommendation, keyed by `roleSlug`. The pinned count is visible. A "Continue to your dashboard" call to action becomes active once at least one role is pinned; activating it does nothing beyond acknowledging visually (the dashboard is not built).

**Conventions:**

- The page renders inside the app layout: sidebar plus a 56px sticky header. A full-height section is `min-h-[calc(100svh-3.5rem)]`, not `100vh`.
- Fonts: Funnel Display (`font-display`) and Schibsted Grotesk (`font-sans`), both variable. No new fonts.
- Light mode is the review target. Dark mode must not break, but needn't be tuned.
- `page.tsx` opens with the prototype header comment: the question it answers, and the date asked (2026-10-05).
- Split by component. No file over ~400 lines (`developer-guide.md` §4.1).
- `strength` is relative to the top recommendation: `close` ≥ 0.6× the top score, `adjacent` ≥ 0.3×, `stretch` below that.
- `bring` can be empty, and is for most roles below the top four. `closestPast` can be null. Design for both.

## Data shape

`DiscoveryData` in `_data/query.ts` is the contract. Live values for the current profile (2026-10-05): four past titles (Visual Designer, Webmaster + UI Specialist, Frontend Engineer, Product Owner), seven claimed skills, twelve recommendations from Product Engineer down to Solutions Architect, two already pinned (Solutions Engineer, Design Engineer — only the first is a recommendation). `/prototypes/discovery` prints the full table.

Known data texture: some title spellings are noisy ("Engineering", "AI") and the tail of the ranking drifts into sales roles via generic skills. Present it honestly; don't filter in the variant.

## Tracks

All five run concurrently on this branch in the main checkout — no worktrees, because folders are disjoint. The owner's dev server on `localhost:3000` serves every route. Tracks do not commit; the coordinator does.

| Track | Owns | Direction |
|---|---|---|
| Airy | `discovery/airy/**` | Light, generous space, soft tonal depth, slow staggered drift. Gentle reassurance. |
| Ledger | `discovery/ledger/**` | Cut and dry. Editorial Swiss grid, a precise ranked table with reasons in columns; rows and bars reveal in order. Confidence through clarity. |
| Marquee | `discovery/marquee/**` | Display typography carries everything. Roles set as enormous type, alternate titles cycling beneath, a poster sequence. |
| Constellation | `discovery/constellation/**` | Spatial. Past roles at the centre, recommendations at distances set by rank; paths draw from where you've been to where you could go. |
| Letter | `discovery/letter/**` | Paced narrative, one idea per screen, ending in the roles to pin. |

Coordinator owns `_data/**`, `discovery/page.tsx`, `package.json`, and this file.

## Acceptance (every track)

Run from `apps/web/`. Other tracks edit concurrently, so filter to your own folder.

| Check | Expected |
|---|---|
| `pnpm typecheck 2>&1 \| grep 'prototypes/discovery/<track>/'` | no output |
| `pnpm theme:check` | no line naming a file under your folder |
| `curl -s -o /dev/null -w '%{http_code}' localhost:3000/prototypes/discovery/<track>` | `200` |
| `grep -rn '\.score' app/prototypes/discovery/<track>` | no output |
| `git status --porcelain` | nothing of yours outside `app/prototypes/discovery/<track>/` |
| Headless screenshots at 1440 wide, settled and mid-animation | looked at, and polished |

## Open questions

- Which direction, or blend, becomes the spec. Settled by the owner after review.
- Whether a winning mood palette becomes a token family. Follows from the above.

## Result

All five variants are built, reviewed in one pass, and pass `pnpm preflight` (267 tests; production build lists all six discovery routes).

| Acceptance row | Airy | Ledger | Marquee | Constellation | Letter |
|---|---|---|---|---|---|
| Typecheck clean | ✓ | ✓ | ✓ | ✓ | ✓ |
| `theme:check` | ✓ | ✓ | ✓ | ✓ | ✓ |
| Route 200 | ✓ | ✓ | ✓ | ✓ | ✓ |
| No `.score` | ✓ | ✓ | ✓ | ✓ | ✓ |
| Changes confined to own folder | ✓ | ✓ | ✓ | ✓ | ✓ |
| Headless screenshots reviewed | ✓ | ✓ | ✓ | ✓ | ✓ |

The review fixed a title-display bug in `_data/query.ts` (a head absent from its raw title recovered a truncated prefix), guarded three divisions, and corrected copy that claimed more than the data supports: pluralities phrased as "most", a capped title count phrased as a total, broken grammar at zero or one claimed skill, and headlines that implied lineage when no role has any. Client number formatting now pins `en-US` to avoid hydration mismatches.

**Outstanding manual proof** — none of this is visible headless:

- Real-time motion feel: spring tension, stagger pacing, and Letter's ~2.5s first beat.
- Trackpad momentum in Letter: one flick should move one beat.
- Keyboard focus rings and screen-reader output.
- Marquee's headline sizing on a cold load (sized in JS after first paint).

**Left for the owner:**

- Marquee's kicker "Because you were a {title}" claims causation; `closestPast` is only the most similar past role. Other variants say "builds on" or "nearest".
- Marquee's palette has five plates; a sixth past title repeats a hue.

**Findings for the spec:**

- No variant can show a pin that is not a recommendation; `DiscoveryData` needs the profile's pins with headlines.
- Every variant wanted a reason when `bring` is empty (kinship versus fit), and Letter wanted the share of postings asking for skills the person already brings.
- Distinct postings and companies across a set of roles cannot be summed from per-role counts; a set-level count needs its own read.
- The measure grammar has no similarity measure or title grouping; the shared read writes its own SQL.
- Title spellings are noisy enough that a typography-led direction amplifies them; a per-spelling posting floor is worth deciding.
- Classified counts drift during enrichment, so the ranking shifts between loads.
