# Dawn feasibility

> Feasibility review of the Dawn prototype, 2026-10-07. Findings and recommendations; no decisions taken.

Dawn's story ports. Its skill column, its line geometry, and its data read do not port as built. The roles card is self-sufficient: every fact a line shows is also in the card's chips. That property makes the lines an enhancement a production page can scale back, and most fixes below lean on it.

Contract decisions and invariants are treated as fixed. Where a fix would change one, it is listed under owner decisions.

## How this was checked

- Code: `dawn/**`, `_data/query.ts`, Threads for comparison, git history of the skill column.
- Rendered on the owner's dev server, read-only, in an isolated browser context: 1440×900, 1280×680, 1180×800, and 390×844 (mobile, touch). Dark mode by toggling the `.dark` class. Contrast computed in the page from the resolved custom properties.
- Live profile at review time: 5 past titles (Tech Lead is new since the contract), 8 claimed skills, 26 inherited, 12 recommendations. Of the 26 inherited, 7 are reached by some recommendation and 19 sit in the "outside" list.

## Lens 1: skill tiers

The owner's reading holds. The three-tier split is an artifact of the line geometry. It also has a second, worse property: it is defined by two ranking parameters, not by anything about the skill.

### T1. The "outside" tier exists to fit rows on screen

- **Severity:** degrades · **Confidence:** high
- **Breaks for:** every person; the split reads as meaning when it is layout.
- **Evidence:** `skill-column.tsx`, comment on `Outside`: "No line can reach them, so they leave the one-row-per-skill list and wrap below it: the rows a line can land on stay few enough to fit the viewport." `SkillColumn` sets `--dawn-rows` to `claimed.length + linked.length`. `.column` sizes rows as `clamp(1.375rem, (100svh - 3.5rem - 13.5rem) / rows, 1.625rem)`. The split arrived in Threads commit `3078ebb` ("readable type"): "Skill rows that can receive a line stay one per row; the rest wrap into a cloud so every lit skill fits on screen at 14px." Dawn inherited it unchanged.
- **Also:** the split survives on the stacked layout, where no lines draw (seen at 390 and 1180 wide). There the geometric reason is gone and only the tiering remains.
- **Fix:** see the alternatives below. Recommended: tiers by provenance only, reach as a row state.

### T2. The eviction rule is applied to one tier only

- **Severity:** degrades · **Confidence:** high
- **Breaks for:** this profile today. Four of eight claimed skills (UI Design, UX Design, Frontend Development, Frontend Engineering) are in no recommendation's `connects`. They keep full rows, rest at full ink, and never light. Unreached inherited skills are evicted.
- **Evidence:** `SkillColumn` filters `inherited` by `reached`; `claimed` is never filtered. Discovery index table: no recommendation's Connects column names those four.
- **Why it matters:** the column's own premise ("rows a line can land on") is false for claimed skills. The layout cannot evict them, because they are the person's own words. So the viewport-fit guarantee only holds while the claimed list is short (see S1).
- **Fix:** one rule for both tiers. Reach becomes a state every row can be in, not a section.

### T3. "Outside" is a property of the ranking, not of the skill

- **Severity:** degrades · **Confidence:** high
- **Breaks for:** the person at their low point. The outside list holds Prototyping, Design Systems, Design Tokens, CSS, HTML/CSS, Animation, User Research, Inclusive Design & Accessibility. That is this person's design identity, greyed and wrapped under a label that says no role wants it.
- **Evidence:** a skill is outside when it is absent from every listed role's top `SIGNATURE_SIZE` (10) skills, across `RECOMMENDATION_LIMIT` (12) roles (`reachedSlugs` in `copy.ts`; `signature` CTE in `query.ts`). Change either constant and membership changes. Classified counts drift during enrichment (4,180 → 4,185 during this review), so membership can change between loads.
- **Information design verdict:** the split carries a real signal: none of these twelve roles leans on your design skills. But that signal is a finding about the ranking (exact-term skill matching, an engineering-heavy corpus; see the contract's synonym finding). The page presents it as a finding about the person. A job seeker cannot act on "outside every role's top 10". They can act on "used by 4 of these roles".
- **Fix:** show reach as a count per skill, computed client-side from `connects`. Skills with zero reach stay in their provenance tier, muted, in place.

### T4. As a scaling constraint, eviction only postpones overflow

- **Severity:** blocks shipping (for realistic profiles) · **Confidence:** high, from the formula; not rendered at scale
- **Breaks for:** anyone with more than about 18 reachable rows on a 680px-tall viewport.
- **Evidence:** at the 22px row floor, rows that fit ≈ (viewport height − 272px) ÷ 22px: 18 at 680, 24 at 800, 28 at 900. Today's 15 rows fit. A 25–40-skill profile overflows on every common laptop height from claimed skills alone. Then the column scrolls; `shownMarks` and `seen` drop lines to rows outside the visible band, with no indication. The layout effect in `useLines` re-centres lit skills only when they fit in the band.
- **Fix:** stop sizing rows to the viewport. Accept that the column scrolls, and give off-screen lit skills an edge marker (alternative B).

### T5. The inherited count invites a possession reading

- **Severity:** polish · **Confidence:** medium
- **Evidence:** heading "From your past roles 26". The screen reader reads the same string.
- **Fix:** see K3 under Copy.

### Alternatives for the column

| Option | What changes | Gains | Costs |
|---|---|---|---|
| **A. Provenance tiers; reach as state** | Two sections: skills you added, common in your past roles. Every skill gets a row. Reached rows sort first within each section, or every row shows a reach count ("4 roles"). Unreached rows rest muted. | One rule for both tiers. The split means something to a job seeker. Reach becomes evidence (Invariant 7). No new data. | Longer column; needs B, C, or D for overflow. |
| **B. Edge markers for off-screen skills** | Column scrolls freely. A line whose skill is scrolled out ends at a pinned marker at the column's top or bottom edge ("2 more above"); activating it scrolls there. `--dawn-rows` sizing goes. | Scales to any skill count. Keeps one row per skill and the line metaphor. | More geometry states. A line no longer always lands on a word. |
| **C. Focus column** | Column shows the lit role's connects (at most 10, bounded by the signature size). The full inventory collapses to a summary or moves to the profile. Skills move between states with a layout animation. | Lines always fit; geometry simplifies a lot. | Loses the whole inventory beside the roles, which is half of Threads' story ("the skills are already there"). |
| **D. Gather, no lines** | Lit skills rise to the top of their section; the card's chips carry the connection. | Works on every viewport, keyboard, and screen reader. Deletes about 1,400 lines of line machinery. | Loses Dawn's signature visual. The "skills come together" metaphor weakens. |
| **E. One moving frame** | Make the skill column static inside a viewport-high stage and let only the role list scroll. Only the port end of a line moves. | Lines need one scroll timeline, or none in fade mode. | A page-inside-a-page scroll region. Awkward with the app's sticky header, and still no phone form. |

**Recommendation:** A for the information model, B for overflow on desktop, lines in fade mode (see I2). If B's states prove fiddly in practice, fall back to C before D.

## Lens 2: data scale

### S1. An empty profile gets twelve arbitrary roles

- **Severity:** blocks shipping · **Confidence:** high, from code; not run
- **Breaks for:** a person with no matched past titles and no matched claimed skills.
- **Evidence:** `query.ts`: `person` is empty, so `person_norm` is null, every `kinship` is null and every `fit` is 0. `scored` coalesces both to 0 and orders `score DESC, rn.role_id`, `LIMIT 12`. The page then renders twelve roles, by role id, under "Roles that use your skills" and "12 roles, closest first".
- **Fix:** a minimum-evidence gate in the read. Return no recommendations when the person signature is empty. Give the page an empty state that routes to the profile.
- **Tradeoff:** none worth weighing.

### S2. Zero recommendations also empties the inherited skills

- **Severity:** degrades · **Confidence:** high, from code
- **Evidence:** `const inherited = ranked[0]?.inherited ?? []`. Inherited skills ride on the first ranked row. No rows, no inherited skills, even when past titles match.
- **Fix:** read inherited skills as their own query, independent of the ranking.

### S3. Inherited skills grow linearly with past titles

- **Severity:** degrades · **Confidence:** medium
- **Evidence:** `INHERITED_PER_ROLE = 8` per distinct past role, deduplicated across roles. Five past roles yield 26 today. Eight could approach 60.
- **Fix:** cap and rank inherited skills globally, or show only reached ones, with a count and a disclosure for the rest. This interacts with T3. A cap is a ranking decision, not a layout one.

### S4. Few or many recommendations

- **Severity:** degrades (3) · polish (20) · **Confidence:** medium
- **3 recommendations:** fewer `connects`, so almost every inherited skill falls outside. The column becomes mostly a grey list. T3's artifact gets worse as the ranking gets thinner, which is exactly when the person most needs lift.
- **20 recommendations:** `RECOMMENDATION_LIMIT` is fixed at 12. The card would need paging or a "show more". Entrance stagger (`ENTRANCE.roles + index × roleStep`) puts row 20 at 2.14s, after the 1.75s auto-light; harmless, because those rows are off-screen.

### S5. A role with empty `connects` gets no feedback in the column

- **Severity:** degrades · **Confidence:** high, rendered
- **Breaks for:** Backend Engineer in the live data. Lighting it dims the whole column to `content-muted`, draws nothing, and lights nothing. Only the card's sentence explains.
- **Evidence:** `Rows` sets `state = anyLit ? "dim" : "rest"` whenever a role holds the light, even one with no connects (`hasActive`).
- **Fix:** do not dim the column when the lit set is empty. Or show a one-line note at the column's top.

### S6. Long skill names

- **Severity:** polish · **Confidence:** medium, from code
- **Evidence:** column pills `truncate`, with no full-name affordance for claimed skills; inherited ones have the hover note. Card chips are `whitespace-nowrap`; a 40-character name can overflow a 390px card. Today's longest names sit in the outside list, so this did not show.
- **Fix:** let chips wrap, or cap with a full-name title on focus and hover.

### S7. Line count is bounded by the signature size

- **Severity:** polish · **Confidence:** high
- **Evidence:** `connects` is at most 10 (`SIGNATURE_SIZE`). At 6px lane spacing, the 104px gutter holds about 13 lanes. A larger signature would push lanes into the column.
- **Fix:** derive lane spacing from gutter width ÷ line count, or keep the signature size as a stated contract.

### S8. Synonyms spend rows

- **Severity:** polish · **Confidence:** high
- **Evidence:** "Frontend Development" and "Frontend Engineering" are both claimed, and both unreached. This is the contract's existing synonym finding showing up as layout cost.

## Lens 3: interaction

### I1. There is no phone form for the lines, and the column buries the roles

- **Severity:** blocks shipping (phone) · **Confidence:** high, rendered
- **Breaks for:** phones, and any desktop window under about 1,216px with the sidebar open. At 1180 wide the container measured 909px, under the 60rem query, so the page stacks with no lines.
- **Evidence:** at 390×844, the first role's headline sits at 1,087px. The skill cloud fills the first screen and a half. Lighting a role lights pills far above, out of view. The card's chips already say what each role draws on, so the page works, but the column is dead weight there.
- **Fix:** on the stacked layout, put roles first. Collapse skills into a short summary, such as a count of skills and a disclosure, or a sticky strip that shows the lit role's skills. Treat lines as a wide-layout enhancement.
- **Tradeoff:** phone users lose the inventory-beside-roles view. They keep every fact.

### I2. Line modes: fade is the shippable baseline

- **Severity:** blocks shipping (as built) · **Confidence:** medium-high
- **`js`:** not shippable. The contract measured 20.5px median drift.
- **`compositor`:** works in the tested browser. Each line becomes about 10 DOM nodes, plus page and column scroll-timeline animations. Port positions come from piecewise-linear tracks predicted over the whole document's scroll range (`portTrack`). They stay right only while every layout change is observed: `ResizeObserver` on four boxes, `resize`, `fonts.ready`, and a re-read on scroll settle. Anything else desyncs the pieces until the next settle: late images, content that expands, the sidebar toggling, mobile URL-bar `svh` changes, pinch zoom. It needs `ScrollTimeline`. Chromium has it. Safari has it from 26 (medium confidence). Firefox stable falls back to fade.
- **`fade`:** every browser, no prediction, no drift. The cost is a draw-in replay at every scroll stop, which may feel busy.
- **Recommendation:** ship fade, with a quieter return: an opacity fade-in of the settled paths, with no `pathLength` replay. Keep the full draw-in for a new light only. Revisit compositor only if alternative E leaves a single moving frame. Delete `js`.
- **Tradeoff:** lines are absent mid-scroll. With the card self-sufficient, nothing is lost but the effect.

### I3. Provenance is hover-only

- **Severity:** degrades · **Confidence:** high
- **Breaks for:** keyboard and touch users. They cannot see which past title an inherited skill comes from.
- **Evidence:** `Provenance` shows on `group-hover/skill`. Skill rows are not focusable. A screen reader gets the `sr-only` copy; sighted keyboard users get nothing.
- **Fix:** move provenance to the card's chips, per connection. That needs the per-connection past title (C3). Or make skill rows focusable with a toggletip.

### I4. Focus side effects

- **Severity:** polish · **Confidence:** medium, from code
- **Evidence:** `onFocus` sits on the role `li`, so tabbing to a role's Pin button also lights it. When the column overflows, each light can scroll the column (`useLines` layout effect). Tabbing down the list will move the column under the reader.
- **Fix:** light on the headline button's focus only. Never auto-scroll the column in response to focus.

### I5. `aria-pressed` on a single-select

- **Severity:** polish · **Confidence:** high
- **Evidence:** the headline button has `aria-pressed={lit}`. Pressing it again does not unpress, so it does not behave as a toggle.
- **Fix:** `aria-current`, or a radio-group pattern, or no state at all, since the lit role changes nothing a screen reader needs.

### I6. Screen readers: the card is the accessible equivalent

- **Severity:** none; a rule worth keeping · **Confidence:** high
- **Evidence:** lines and port are `aria-hidden`. Lit skills are not announced. The card's `dl` ("You added", "From past roles") names every connected skill. The page loses no fact without lines.
- **Recommendation:** make this a stated invariant of the production design. Lines decorate a fact the card already states.

### I7. Reduced motion

- **Severity:** polish · **Confidence:** high
- **Evidence:** `MotionConfig reducedMotion="user"` is handled well. Lines appear whole; the sky fades in with the card. CSS colour transitions (500ms on pills) still run. Acceptable: they are colour, not movement.

### I8. Loading, empty, and error states, and read cost

- **Severity:** blocks shipping · **Confidence:** medium (dev-server timings)
- **Evidence:** Dawn and Threads took 1.8–2.1s per load; `/status` took 0.23s on the same server. No suspense boundary, empty state, or error retry exists (suspended by the prototype rules).
- **Fix:** a streamed skeleton for the card and column, an error boundary with retry, and the S1 empty state. The read-cost fix is C1.

### I9. Auto-light ignores what the reader is doing

- **Severity:** polish · **Confidence:** medium, from code
- **Evidence:** the top role lights 1.75s after load unless a role was already chosen. Scrolling does not count as a choice. On a short viewport with an overflowing column, the light can scroll the column the reader is in.
- **Fix:** treat any scroll or pointer movement before the timer as a choice. Skip the auto-light.

### I10. The pin bar covers card content

- **Severity:** polish · **Confidence:** high, rendered at 1280×680
- **Evidence:** the sticky pin bar overlays the first role's chips at rest.
- **Fix:** reserve bottom padding equal to the bar's height on the card's list.

## Lens 4: data contract

### C1. The ranking is prototype SQL, outside the read model

- **Severity:** blocks shipping · **Confidence:** high
- **Evidence:** `query.ts` defines signatures (P(skill | role)), rarity weights, kinship, fit, closest past role, the flavoured past-role reading, and inherited skills, all in one CTE. project.md: derived state lives in SQL views; the measure grammar has no similarity measure. The contract already names this deviation.
- **Fix:** split along the privacy boundary.
  - Profile-independent, as views: role signatures, meaning share, rarity weight, and top-N rank per role. Likely materialized, given I8's timing.
  - Profile-dependent, as a `lib/db` read: scoring, closest past role, and inherited skills, taking profile ids as input. The profile cannot be joined in SQL, so this cannot be a view.
- **Tradeoff:** a materialized view needs a refresh story tied to enrichment waves. That also answers the drift finding: the ranking would change per refresh, not per load.

### C2. The score reaches the client

- **Severity:** degrades · **Confidence:** high, verified in the page HTML
- **Evidence:** `DawnDiscovery` receives the whole `DiscoveryData`. `score` values (0.2334…) appear in the serialized RSC payload. Invariant 2 governs rendering, but a value in page source is one devtools tab from rendered.
- **Fix:** map to a client-safe shape at the server boundary: no `score`, no ids the client does not use.

### C3. Fields a real page needs

| Missing | Used for | Notes |
|---|---|---|
| P(skill \| role) per connection | Line weight; "asked for in 61% of postings" on lit chips | `grow` has `share`; `connects` does not. |
| Past title per connection, per role | Provenance on the card's chips (I3); "TypeScript, from your Tech Lead role" | `fromPast` exists per skill, not per role connection. |
| Reach per skill | Alternative A | Computable client-side from `connects`. No read change. |
| Pins that are not recommendations | Pin bar, returning visitors | Already in the contract's findings. |
| A reason when `connects` is empty | Backend Engineer's card | Already in the contract's findings. |
| Unmatched claimed skills | The column | `claimedSkills` carries them; `personSkills` drops them; Dawn never renders them. A person who typed a skill sees it vanish. |

### C4. Generic skills pull in sales roles under a fit headline

- **Severity:** degrades · **Confidence:** high
- **Evidence:** Business Development Representative ranks third and Field Sales Representative eleventh, through Communication Skills and Presentation Skills. Under "Roles that use your skills" this reads as an endorsement.
- **Fix:** an owner decision on a floor (see below). The contract already notes the drift into sales roles.

### C5. `query.ts` exceeds the file-size convention

- **Severity:** polish · **Confidence:** high
- **Evidence:** 451 lines against the ~400 convention. C1's split resolves it.

## Lens 5: implementation architecture

### A1. What ports and what is rebuilt

The line machinery is 1,393 of Dawn's 2,851 lines: `use-lines`, `geometry`, `compose`, `track`, `layout`, `drive`, `store`, `light`, `modes`, `connectors`, `pieces`, `thread-paint`.

| Ports with light edits | Rebuilt or reshaped |
|---|---|
| `copy.ts` helpers (`plural`, `listJoin`, `alsoPostedAs`, `alsoAsksFor`) | `skill-column.tsx`: new tier model |
| `geometry.ts` `routeBundle`: pure, unit-testable | `use-lines.ts`: fade-only is far smaller |
| `motion.ts` vocabulary (`DRIFT`, `DRAW`, `SETTLE`) | `compose.ts`, `track.ts`, `drive.ts`, `pieces.tsx`: only if compositor survives |
| `pin-button.tsx`, `skill-mark.tsx` | `dawn.module.css` glow and palette: becomes tokens (V1) |
| `role-item.tsx` markup, after I4 and I5 | `_data/query.ts`: becomes views plus a `lib/db` read (C1) |
| `sky.tsx` structure | Page shell: loading, empty, and error states (I8) |

### A2. Layout constants are duplicated between CSS and TypeScript

- **Severity:** degrades · **Confidence:** high
- **Evidence:** `HEADER = 56` versus CSS `top: 3.5rem`. `FADE = 24` versus the 3rem mask. `MIN_GUTTER = 48`. The 13.5rem column chrome term counts heading and note heights by hand, so a wrapped note or a copy change mis-sizes rows. A change on one side silently skews the geometry.
- **Fix:** read the constants from CSS custom properties at layout time, or keep one constants module both sides import. Drop the chrome term with T4's fix.

### A3. DOM measurement cost

- **Severity:** polish · **Confidence:** medium
- **Evidence:** `readLayout` makes about five `getBoundingClientRect` calls plus one per connected skill, and three `getComputedStyle` calls. It runs per relayout: resize, observed box resize, font load, each scroll settle. Scroll frames in compositor mode read only scroll offsets. The cost is fine as built. The risk is correctness (I2), not speed.

### A4. Client boundary and hydration

- **Severity:** polish · **Confidence:** medium
- **Evidence:** everything under `page.tsx` is a Client Component. The light state spans both columns, so one client root is reasonable. Hydration risk is low: numbers pin `en-US`; `Sky` renders the same first frame on server and client; the line mode switch happens after mount, before any line draws.
- **Fix:** render the intro and the card's static text on the server, with the light as a small client island. Optional.

### A5. Motion is timed in absolute seconds

- **Severity:** polish · **Confidence:** high
- **Evidence:** `ENTRANCE.light = 1.75` and `SUNRISE` are fixed. Stagger scales with counts. The two only drift apart at large counts (S4).

### A6. No tests

- **Severity:** degrades (for production) · **Confidence:** high
- **Evidence:** prototype exemption. `routeBundle`, `piecesOf`, and `portTrack` are pure and are the first candidates.

## Lens 6: visual system

### V1. The mood palette needs to become tokens, with checks

- **Severity:** blocks shipping (per the colour rules) · **Confidence:** high
- **Evidence:** `.root` defines 15 `--dawn-*` colours and 4 shadows, light and dark, in a CSS module. Production colour comes from `scripts/theme/tokens.mjs` through `pnpm theme:check`. The contract says a winner becomes a token proposal.
- **Measured contrast (light / dark):**

| Pair | Light | Dark | Bar |
|---|---|---|---|
| ink on lit claimed pill (`content-primary` / `--dawn-glow`) | 17.1 | 12.3 | 4.5 ✓ |
| ink on lit inherited pill (`content-primary` / `--dawn-horizon`) | 17.6 | 14.4 | 4.5 ✓ |
| lit rank number (`--dawn-ember` / glow, horizon, raised) | 5.9 / 6.1 / 6.7 | 8.0 / 9.4 / 10.0 | 4.5 ✓ |
| dim row (`content-muted` / page) | 5.0 | 6.4 | 4.5 ✓ |
| rest inherited (`content-secondary` / page) | 8.1 | 9.2 | 4.5 ✓ |
| `content-muted` / `--dawn-morning` | 4.58 | 5.18 | 4.5 ✓, 0.08 of margin |
| `content-muted` / `--dawn-deep` | 4.19 | 4.69 | 4.5 ✗ light. No text sits there today, by design comment only. |
| thread rose / morning | 3.07 | 6.62 | 3.0 non-text ✓, barely |
| thread lilac / morning | 3.30 | 5.82 | 3.0 ✓ |
| port (`--dawn-port`) / raised | 2.03 | 10.15 | 3.0 ✗ light, if the port is meaningful |
| dashed inherited chip edge (`edge` / raised) | 1.37 | 1.53 | carries meaning with the glyph |
| disabled Continue (`content-disabled` / sunken) | 2.19 | 2.97 | named exemption |

- **Fix:** propose a narrow family (sky, sun, glow, ember, thread) with `theme:check` invariants. Ember clears 4.5 on glow, horizon, and raised. Muted ink clears 4.5 on every sky stop text can reach. Thread ends clear 3:1 on morning. Fold deep into morning, or encode "no text on deep" as a layout rule a check can see.

### V2. Hue on chrome breaks the achromatic-accent rule

- **Severity:** owner decision · **Confidence:** high
- **Evidence:** web-guide §Colour: accent is achromatic, so hue stays reserved for data. Dawn spends blue, gold, rose, and lilac on atmosphere. Defensible on a page with no data marks. A token family of atmosphere colours is still a new category the system currently rules out.

### V3. Claimed versus inherited rests on thin cues in the column

- **Severity:** polish · **Confidence:** medium
- **Evidence:** at rest the column separates them by section, ink step, weight, and a 7px dot or ring. In the dim state both step to `content-muted`, leaving weight and the 7px glyph. The card's text labels carry the distinction properly.
- **Fix:** keep the ink step in the dim state (`content-secondary` for claimed), or enlarge the glyph.

### V4. Alpha on tokens

- **Severity:** polish · **Confidence:** high
- **Evidence:** `bg-surface-row-hover/60`, `bg-surface-raised/85` with backdrop blur, `ring-edge-hairline/70`. web-guide prefers a real token over an alpha fudge.

### V5. Dark mode reads as night

- **Severity:** polish · **Confidence:** medium, rendered with the class toggled
- **Evidence:** dark morning is a navy glow; lit pills turn brown with ember marks. Contrast passes. The dawn metaphor does not carry. The contract allows untuned dark mode for prototypes; a shipped page needs a decision.

### V6. Type floor holds

- **Severity:** none · **Confidence:** high, from code
- **Evidence:** all reading text is 14px or larger. Counts, labels, and rank numbers use `text-sm`. The headline scale steps cleanly.

## Lens 7: copy

Checked against the contract's copy invariants and plain product copy. No "X, not Y" constructions, colon reveals, or triplets in product copy. "Also posted as" lists three titles by construction (`alsoPostedAs(rec, 3)`). It reads as data, but four or two would avoid the cadence.

### K1. "19 more outside every role's top 10"

- **Severity:** degrades · **Confidence:** high
- **Problem:** explains an internal cutoff (meta-explanation). "Every role" means the twelve listed. It frames the person's design skills as outside.
- **Fix:** goes away with alternative A. If a heading is still needed: "Not asked for by these roles yet". Better, no heading: muted rows with a "0 roles" reach count.

### K2. "None of its top 10 skills are on your list."

- **Severity:** degrades · **Confidence:** high
- **Problem:** leads with the negative and names the cutoff, at the page's lowest-confidence moment.
- **Fix:** "Closest to your Tech Lead role. Its most-asked skills aren't on your list." When `closestPast` is null, the second sentence stands alone.

### K3. "From your past roles 26" (column) and "From past roles" (card)

- **Severity:** degrades · **Confidence:** medium
- **Problem:** Invariant 6. "From your past roles" with a count reads as "26 skills you have". Threads said "Come with roles you've held" and added "Read from the market, not something you said." Dawn's plainer pass kept the attribution only in the note.
- **Fix:** put the market in the heading: "Common in your past roles". Keep the note.

### K4. "Roles that use your skills."

- **Severity:** degrades · **Confidence:** high
- **Problem:** false for Backend Engineer (empty `connects`). It overclaims the sales roles (C4). Invariant 7 asks encouragement to cite evidence; the headline is a blanket claim.
- **Fix:** "Roles near what you've done", or filter out roles with empty `connects`, which is an owner decision.

### K5. "Ranked from 4,180 classified of 11,422 open postings, across 122 roles."

- **Severity:** polish · **Confidence:** high
- **Problem:** "classified" is pipeline vocabulary.
- **Fix:** "Ranked from 4,180 of 11,422 open postings read so far, across 122 roles." Invariant 4 still holds.

### K6. Passing

"Skills you added", "You added", "12 roles, closest first", "Also asks for … (48% of postings)", "closest to your Tech Lead role", "Pin a role to watch it.", and "Continue to your dashboard" all meet the invariants. Invariant 3's share phrasing is right.

## What unblocks a real build, in order

1. **Settle the column model and the phone form together.** A plus B on wide layouts, roles-first with collapsed skills when stacked. This decides how much of the 1,393-line line machinery survives. Do it before porting anything.
2. **Move the read into the read model.** Signature views, a `lib/db` scoring read taking profile ids, the empty-profile gate (S1), inherited skills independent of the ranking (S2), no `score` on the client (C2), and per-connection share and past title (C3).
3. **Ship lines in fade mode** with a quiet return after scroll. Keep the card as the accessible equivalent (I6). Revisit compositor only if the column model leaves one moving frame.

Then the token proposal for the palette (V1), and states plus tests (I8, A6).

## Owner decisions

| Decision | Options | Recommendation |
|---|---|---|
| Skill column model | A, B, C, D, E (Lens 1) | A plus B. Fall back to C before D. |
| Unreached skills | Hide; separate section; in place, muted | In place, muted, with a reach count. Hiding them hides the ranking's blind spot. |
| Phone | Lines on phone; no lines, roles first | No lines. Roles first, skills collapsed. |
| Line mode | compositor, fade | Fade, with a quieter return after scroll. |
| Atmosphere hue as tokens (V2) | New family; keep page-local; drop | A narrow family with `theme:check` invariants. It is the page's whole lift. |
| Headline and empty-`connects` roles (K4) | Change the headline; filter the roles; both | Change the headline. Filtering changes the ranking, which belongs in C1's work. |
| Generic-skill sales roles (C4) | Rarity floor; per-role minimum on distinctive overlap; leave | A floor on distinctive overlap, decided in the ranking spec, not on this page. |
| Unmatched claimed skills (C3) | Show as unmatched; omit | Show, muted, as unmatched. It matches how the profile treats them. |
| Dark mode (V5) | Tune a dark dawn; accept night | Tune it before ship, or ship light-only styling for the glow. |

## Not verified

| Gap | Where to look first |
|---|---|
| Real-time motion and compositor sync under trackpad momentum | Headed Chrome and Safari on the owner's machine. Scroll the card fast past the pin bar and header. |
| Safari and Firefox behaviour of `effectiveLineMode` | Safari 26 for compositor; Firefox stable for the fade fallback. |
| Screen reader output and real keyboard flow | VoiceOver on the card's `dl` and on the pin bar's live region. Tab through all twelve roles with an overflowing column. |
| Layout at 25–40 skills, 0–2 claimed, 3 or 20 recommendations | A fixture profile on the test database through a second dev server (web-guide §Verifying UI). Everything in Lens 2 is derived from code and formulas. |
| Read cost in a production build | `pnpm build && pnpm start` on a second port, then time the page. The dev timings include compile overhead. |
| Dark mode through the app's real toggle | Toggled the `.dark` class directly; the app's theme switch may differ. |
| GPU cost of the large blurred glows on low-end hardware | Chrome performance trace with 4× CPU throttle, scrolling the card. |
