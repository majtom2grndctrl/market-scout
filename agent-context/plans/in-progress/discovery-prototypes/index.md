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
| **A sixth direction, Threads** (owner request after reviewing the five). Skills on the left, recommended roles on the right; focusing a role lights the skills it draws on and draws rounded connector lines to them. Card style borrows Airy's. | The story is "the skills are already there; you hadn't seen how they come together." It needs skills the person never typed, so the read gained `personSkills` and `connects`. |
| **Inherited skills read a past role through the person's own flavour of it** — its postings that also ask for a claimed skill. | Design Engineer spans UI and CAD work; read whole, a UI designer inherited REVIT and GD&T. Below three such postings the whole role stands in. The ranking itself still reads whole roles. |
| **"New this month" is omitted.** | `first_seen_at` is the fetcher's first sighting; a company onboarded last week makes every posting look new. Momentum would overstate. |

## Invariants

**Hard constraints** — breaking one corrupts the experiment or the honesty of the page:

1. **Data comes only from `getDiscoveryData()`** in `_data/query.ts`, called from the variant's Server Component `page.tsx` and passed to Client Components as props. No SQL, no other DB import, no edit to `_data/`. If a variant needs a field that isn't there, report it — don't route around it.
2. **Never render `score`**, and never phrase fit as hireability or a percentage match ("you qualify", "92% match", "you'd get hired"). Coverage language only: what you bring, what it also asks for.
3. **`grow[].share` is P(skill | role)** — the share of this role's postings that ask for the skill. Render it as such ("asked for in 61% of postings"), never as the person's gap or deficiency.
4. **Show coverage somewhere on the page**, even small: ranked from `coverage.classifiedPostings` classified of `coverage.openPostings` open postings. A recommendation from a third of the corpus must not read as the whole market.
5. **Colour.** Real tokens (`apps/web/app/theme.css`) for ink, surfaces, and data marks. Mood colours only through the variant's own `--<variant>-*` custom properties, used via Tailwind's `bg-(--airy-glow)` syntax or in the variant's own CSS. Never edit `theme.css`, `tokens.css`, `globals.css`, or `scripts/theme/`. Never wear `success-*`/`danger-*`/`warning-*` (status) or `trend-*` for fit or strength: a close fit is not a success state, and a stretch is not a failure.
6. **Inherited is not claimed.** A `personSkills` entry with `claimed: false` was inferred from the market, not stated by the person. It reads that way in copy ("comes with roles you've held") and looks distinct from a claimed skill. Never present it as something the person said they have.
7. **Encouragement is evidence.** Every uplifting line cites something real from the data: a past title, a claimed skill, a posting or company count. No generic affirmations.
8. **Motion directs the eye.** Each animation answers "where should I look next?" — staged reveals in reading order, a pin landing where pins collect. No decorative loops; the one exception is a variant whose concept *is* motion (Marquee's cycling titles). Wrap the variant in `<MotionConfig reducedMotion="user">`.
9. **Readable type.** Every piece of reading text clears 4.5:1 against its actual background in every state — dimmed, receded, lit, on a mood wash — or 3:1 at 24px and up (18.66px bold). Nothing a person reads is under 14px. Recede by stepping ink tokens (`content-primary` → `-secondary` → `-muted`), never to `content-disabled` (≈2.4:1, reserved for disabled controls) and never by opacity on text. A genuinely disabled control is the one exemption, and must be named as such. Owner feedback after reviewing the six.
10. **Pinning.** Toggle per recommendation, keyed by `roleSlug`. The pinned count is visible. A "Continue to your dashboard" call to action becomes active once at least one role is pinned; activating it does nothing beyond acknowledging visually (the dashboard is not built).

**Conventions:**

- The page renders inside the app layout: sidebar plus a 56px sticky header. A full-height section is `min-h-[calc(100svh-3.5rem)]`, not `100vh`.
- No eyebrow text — the small, often uppercase, tracked label above a heading. The owner asked for none unless requested; position and type scale carry hierarchy. Applies to all six.
- When small type grows to meet Invariant 9, re-step the section's whole scale so its hierarchy still reads proportionally. Never bump one size alone: a 13px label raised to 14px beside 14px body text is a flattened hierarchy, not a fix.
- Fonts: Funnel Display (`font-display`) and Schibsted Grotesk (`font-sans`), both variable. No new fonts.
- Light mode is the review target. Dark mode must not break, but needn't be tuned.
- `page.tsx` opens with the prototype header comment: the question it answers, and the date asked (2026-10-05).
- Split by component. No file over ~400 lines (`developer-guide.md` §4.1).
- `strength` is relative to the top recommendation: `close` ≥ 0.6× the top score, `adjacent` ≥ 0.3×, `stretch` below that.
- `bring` can be empty, and is for most roles below the top four. `closestPast` can be null. Design for both.

## Data shape

`DiscoveryData` in `_data/query.ts` is the contract. `personSkills` lists matched claimed skills first, then inherited ones; every slug in a recommendation's `connects` is in `personSkills`. `connects` can be empty. Live values for the current profile (2026-10-05): four past titles (Visual Designer, Webmaster + UI Specialist, Frontend Engineer, Product Owner), seven claimed skills, twelve recommendations from Product Engineer down to Solutions Architect, two already pinned (Solutions Engineer, Design Engineer — only the first is a recommendation). `/prototypes/discovery` prints the full table.

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
| Threads | `discovery/threads/**` | Skills left, roles right; a role lights the skills it draws on, with rounded connector lines. Airy's card style. |
| Dawn | `discovery/dawn/**` | Threads duplicated, then re-lit with Airy's dawn palette and iterated on as "a new dawn". Same data, layout, and line modes. |

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
| `node <scratchpad>/shared/audit-type.mjs <url>` in every reviewable state | exit 0, or only named disabled-control exemptions |

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

**Readable-type pass** (Invariant 9, after the owner's review of all six). Every variant now passes the type audit in each reviewable state; the only remaining failure anywhere is a disabled "Continue" control before the first pin, named as the exemption in code. Ledger kept none: its disabled label explains what pinning unlocks, so it reads. Eyebrows are gone from all six. Starting counts at 1440 wide: Threads 215, Airy 157, Constellation 60, Ledger 45, Marquee 41, Letter 27 across beats. Two audit gaps surfaced: a gradient painted on a sibling layer is invisible to an ancestor-walking contrast check, so Airy and Constellation measured against screenshot pixels instead; and states behind a click (pinned, expanded, hovered) need a driver that clicks first.

**Threads connector modes.** `?lines=compositor` (default), `fade`, `js`. JS-routed lines lag the compositor's scroll by a frame, because the markers are sticky and the SVG scrolls with the page: measured drift 20.5 px median, 40.5 max. Compositor mode cuts each line into pieces that each sit still in one frame — port end on a page `ScrollTimeline`, skill end with the sticky column, the vertical run as a bar from one clipped by a box from the other — and measures 0.5 px, the resting offset. Direction flips, lanes, and radii stay in JS, timed for when corners have shrunk to nothing. Browsers without scroll-driven animations fall back to `fade`. A production version would carry this construction, not the JS router.

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
- **Skills match only by exact term, so a role can stay buried behind synonyms.** Solutions Engineer asks for `communication`, `technical-communication`, `technical-demos`, `api-integration`; the profile holds `communication-skills`, `presentation-skills`, `public-speaking`, `rest-api-design-integration`. Similarity from shared roles was tested and rejected: it finds context, not meaning (`communication-skills` sits nearest `prospecting`; `ui-design` nearest `cad-software`), which is also why sales roles rise. Meaning-level matching needs skill embeddings (pgvector is enabled; no skill has one) — a schema change and a write path, so it goes through `/draft-session`. Two companions: repair the near-duplicate terms through the taxonomy repair functions, and pin a fixture profile whose expected top roles include Solutions Engineer, so the mechanism is tested rather than the result tilted.
- Threads (added after the first five) passes the same gates. `grow` excludes only claimed skills, so an inherited skill can sit in both `connects` and `grow`; Threads filters it. Its agent wished for the past title each connection comes through, per role, and P(skill | role) per connection for line weight.

## Dawn refinement, round 1 (2026-10-07)

Dawn is the chosen direction. The session's end goal is a problem brief for a production Discovery page, distilled from Dawn once it reads right. This round acts on `dawn-feasibility.md`. Everything above still binds.

**Decisions**

| Decision | Consequence |
|---|---|
| **Desktop only.** The app runs against a local Postgres; no phone form is planned. | The stacked layout below 60rem stays as built. No new narrow-viewport work. |
| **Two skill sections, by where a skill came from:** skills the person added, and skills common in their past roles. The "outside" tier is removed. | One rule for both sections. Whether a role draws on a skill is a row state, never a section. |
| **Every skill gets a row**, reached or not. A row shows how many of the listed roles draw on it. A skill no listed role draws on rests muted, in its section. | The design skills stay with the person instead of being greyed out under a cutoff. |
| **The column scrolls; rows are not sized to the viewport.** `--dawn-rows` and its chrome term go. | Scales to any skill count. Lines to scrolled-out skills need edge markers. |
| **Edge markers.** When the lit role draws on skills scrolled out of the column's visible band, their lines end at a marker pinned to the column's top or bottom edge, carrying the count beyond it. Activating the marker scrolls those skills into view. | Every lit connection keeps a visible line. No lit skill disappears without a trace. |
| **`compositor` stays the default line mode.** `fade` stays as the automatic fallback. | Edge markers must work in both. `js` is a comparison baseline only and needs no edge markers. |
| **Unmatched claimed skills are shown** in "skills you added", after the matched ones, muted, with a short plain note that they don't match a skill in postings yet. | Nothing the person typed vanishes. They never connect, so they take no reach count. |
| **Ranking is out of scope.** The owner will work with the data directly after prototyping. | No change to `_data/query.ts` ordering, cutoffs, or recommendation set. |
| **Proper design tokens come first, in their own brief.** | Dawn keeps its `--dawn-*` properties this round. |

**Invariants added this round**

11. **Order within a section is stable for a page load.** Within each section, rows sort by reach count, most first, ties in data order. The lit role never reorders rows.
12. **Nothing sits between a skill's marker and the gutter.** The reach count must not lie in a line's path.
13. **Never scroll the skill column while the pointer is in it.** Auto-scrolling to bring lit skills into view is allowed otherwise. The edge marker is the reader's own way to get there.
14. **Graphic marks clear 3:1** against what they sit on, in light and dark, including the gold port (2.03:1 in light at review).

**Copy, this round** (plain product copy; Invariants 6 and 7):

- Inherited section heading: "Common in your past roles". Its note became "From postings for titles you've held." so "Common" does not repeat.
- No copy names an internal cutoff ("top 10"). The role card's empty-bring line becomes "Closest to your {closestPast} role. Its most-asked skills aren't on your list.", with the second sentence alone when `closestPast` is null.
- Page headline "Roles that use your skills" becomes "Roles near what you've done".

**Track: Dawn round 1.** One agent. Owns `apps/web/app/prototypes/discovery/dawn/**`. Reads `_data/query.ts`, never edits it.

**Acceptance**

- `pnpm typecheck` and `pnpm theme:check` pass from `apps/web/`.
- `grep -rn "top 10\|outside every\|--dawn-rows" app/prototypes/discovery/dawn` returns nothing.
- Rendered on `/prototypes/discovery/dawn` at 1440×900 and 1280×680, light and dark, with screenshots looked at: two sections, every skill present, reach counts visible, unmatched claimed skills present, and at 1280×680 a lit role with a skill scrolled out shows an edge marker that scrolls to it when clicked. Checked in `compositor` and `?lines=fade`.
- No file under `dawn/` over ~400 lines.

**Result, round 1.** Every acceptance row passed: typecheck, `theme:check`, the grep, file sizes (largest 394 lines), and screenshots at both sizes in both modes and themes. Compositor mode needed no per-frame script. Each line picks its end (skill or edge marker) once per layout pass. Lines to a skill leaving the column run off its clipped edge until the scroll settles, then move to the marker. The port is now 3.73:1 on the card. Reach counts sit inside the pill, between the name and the marker. Section headings dropped their counts.

The review's four "unmatched" claimed skills are matched but reached by no listed role. The live profile has no unmatched claims, so that path was checked against injected local data only.

Outstanding manual proof: trackpad momentum in headed Chrome and Safari, especially the snap when the settle moves a line from a clipped skill to the marker; a "below" marker against live data, since reach-first ordering keeps reached skills near the top; a dark-mode type audit.
