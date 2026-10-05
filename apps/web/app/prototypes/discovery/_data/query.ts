// Prototype data access — asked 2026-10-05. Not production.
//
// Question: which roles should a person look at next, given what they have
// done and what they claim? Five Discovery sketches share this one read so
// they differ in feel, never in data.
//
// DEVIATION FROM § Prototypes, stated rather than hidden. This file writes SQL,
// because the ranking is outside the measure grammar: it compares each role's
// skill signature against the person's, and the vocabulary has no similarity
// measure and no title grouping. That gap is the finding. Both safety rules
// hold: reads go through the app's pooled clients (`getSql`, `getProfile`),
// and "open" and "latest classification" come from the read-model views.
//
// The profile is read on the app client and passed into the market read as
// ids. The read-only role cannot reach the `app` schema, and a measure that
// depends on the profile takes it as input rather than joining it in SQL.
//
// THE RANKING. A role's signature is P(skill | role) over open, classified
// postings, weighted by how few roles want that skill strongly. Without the
// weight, communication and collaboration dominate every comparison and a
// designer's nearest role came out as mechanical engineer. The person's
// signature is the mean of their past roles' signatures plus every claimed
// skill at the strongest weight any role gives it. Score blends two readings:
//   kinship — cosine of the person's signature against the role's
//   fit     — the weighted share of the role's top ten skills the person claims
// Both are 0..1 and small: seven claimed skills cover a slice of any role.
// That is why the score is never rendered. Rank and reasons carry it.

import { getSql } from "@/lib/db/client";
import { getProfile } from "@/lib/db/profile";

export interface SkillRef {
  readonly slug: string;
  readonly name: string;
}

export interface TitleVariant {
  /** As employers write it, recovered from the raw title's own casing. */
  readonly title: string;
  readonly postings: number;
}

/** Relative to the top recommendation's score, never an absolute claim. */
export type Strength = "close" | "adjacent" | "stretch";

export interface Recommendation {
  /** 1-based, best first. */
  readonly rank: number;
  readonly roleId: string;
  readonly roleSlug: string;
  readonly roleName: string;
  /**
   * The title to display, not roleName. The role's own name as employers write
   * it when any employer does; otherwise the most common title. Most-common
   * alone headlined three different roles "Software Engineer".
   */
  readonly headline: string;
  /** Five titles, most postings first, plus the role's own name when it falls outside the five. May include the headline; filter it out for an "also called" list. */
  readonly titles: readonly TitleVariant[];
  /** The past title whose role is most like this one. Null when the profile has no matched past title. */
  readonly closestPast: { readonly titleText: string; readonly roleName: string } | null;
  /** Claimed skills among this role's top ten, strongest first. May be empty. */
  readonly bring: readonly SkillRef[];
  /** This role's top-ten skills that appear in personSkills, claimed or inherited, most distinctive first. Every slug here is in personSkills. */
  readonly connects: readonly SkillRef[];
  /** Up to four of this role's top skills the person has not claimed, most distinctive first (not by share). share is P(skill | role), 0..1. */
  readonly grow: readonly (SkillRef & { readonly share: number })[];
  /** Open, classified postings assigned this role. */
  readonly openPostings: number;
  readonly companies: number;
  readonly strength: Strength;
  /** Internal ordering value, 0..1. Never render it. */
  readonly score: number;
  /** Pinned in the real profile when the page rendered. */
  readonly pinned: boolean;
}

/**
 * A skill the person has, either claimed outright or inherited from a past
 * role's signature. At least one of `claimed` and `fromPast` is set. An
 * inherited skill is inferred from the market, not stated by the person, and
 * must read that way: "common in roles you've held", never "your skill".
 */
export interface PersonSkill {
  readonly slug: string;
  readonly name: string;
  readonly claimed: boolean;
  /** Inherited only: the past titles it comes with. Always empty for a claimed skill, which the person named themselves. */
  readonly fromPast: readonly string[];
}

export interface PastRole {
  readonly titleText: string;
  readonly roleSlug: string | null;
  readonly roleName: string | null;
  readonly seniority: string | null;
  /** Open, classified postings for the matched role. Null: unmatched, or the role is below the posting floor. */
  readonly openPostings: number | null;
}

export interface DiscoveryData {
  readonly pastRoles: readonly PastRole[];
  /** Every claimed skill, matched or not, as the person wrote it. */
  readonly claimedSkills: readonly { readonly text: string; readonly skill: SkillRef | null }[];
  /** Matched claimed skills first, in profile order, then inherited skills, most distinctive first. Unmatched claims are absent; they cannot connect. */
  readonly personSkills: readonly PersonSkill[];
  /** Best first. Excludes roles the person has held. At most RECOMMENDATION_LIMIT. */
  readonly recommendations: readonly Recommendation[];
  readonly coverage: {
    readonly openPostings: number;
    readonly classifiedPostings: number;
    /** Roles above the posting floor — the pool the ranking chose from. */
    readonly rolesConsidered: number;
  };
}

// A role needs this many open classified postings before its signature means
// anything. Six keeps design-side roles (graphic designer, design manager) in
// a corpus that skews engineering.
const MIN_POSTINGS = 6;
const RECOMMENDATION_LIMIT = 12;
const SIGNATURE_SIZE = 10;
const GROW_LIMIT = 4;
const TITLE_LIMIT = 5;
// How many of each past role's most distinctive skills the person inherits.
// Eight keeps a four-role profile near twenty inherited skills.
const INHERITED_PER_ROLE = 8;
// A past role needs this many postings asking for a claimed skill before its
// flavoured reading replaces the whole role.
const FLAVOUR_FLOOR = 3;

interface RankedRow {
  role_id: string;
  slug: string;
  name: string;
  n: number;
  companies: number;
  score: number;
  closest_past_role_id: string | null;
  bring: { slug: string; name: string }[];
  grow: { slug: string; name: string; share: number }[];
  connects: { slug: string; name: string }[];
  inherited: { slug: string; name: string; past_role_ids: string[] }[];
}

export async function getDiscoveryData(): Promise<DiscoveryData> {
  const [profile, sql] = await Promise.all([getProfile(), getSql()]);

  const pastRoleIds = [...new Set(profile.pastTitles.flatMap((t) => (t.role ? [t.role.id] : [])))];
  const claimedSkillIds = [...new Set(profile.claimedSkills.flatMap((c) => (c.skill ? [c.skill.id] : [])))];
  const pinnedRoleIds = new Set(profile.pins.flatMap((p) => (p.role ? [p.role.id] : [])));

  const ranked = await sql<RankedRow[]>`
    WITH classified AS (
      SELECT op.job_posting_id, lc.classification_id, jp.company_id
      FROM open_postings op
      JOIN latest_classifications lc USING (job_posting_id)
      JOIN job_postings jp ON jp.id = op.job_posting_id
    ),
    posting_role AS (
      SELECT c.job_posting_id, c.company_id, jpr.role_id
      FROM classified c JOIN job_posting_roles jpr USING (classification_id)
    ),
    posting_skill AS (
      SELECT c.job_posting_id, jps.skill_id
      FROM classified c JOIN job_posting_skills jps USING (classification_id)
    ),
    role_n AS (
      SELECT role_id, count(*)::float8 AS n, count(DISTINCT company_id)::int AS companies
      FROM posting_role GROUP BY role_id HAVING count(*) >= ${MIN_POSTINGS}
    ),
    role_skill AS (
      SELECT pr.role_id, ps.skill_id, count(*)::float8 / rn.n AS share
      FROM posting_role pr JOIN posting_skill ps USING (job_posting_id) JOIN role_n rn USING (role_id)
      GROUP BY pr.role_id, ps.skill_id, rn.n
    ),
    -- Rarity: a skill at least 10% of many roles want is ambient, not signal.
    -- A skill no role wants at 10% gets the full weight.
    rarity AS (
      SELECT skill_id, ln((SELECT count(*) FROM role_n)::float8 / count(*)) AS w
      FROM role_skill WHERE share >= 0.1 GROUP BY skill_id
    ),
    weighted AS (
      SELECT rs.role_id, rs.skill_id, rs.share,
             rs.share * coalesce(r.w, ln((SELECT count(*) FROM role_n)::float8)) AS w
      FROM role_skill rs LEFT JOIN rarity r USING (skill_id)
    ),
    signature AS (
      SELECT * FROM (
        SELECT weighted.*, row_number() OVER (PARTITION BY role_id ORDER BY w DESC, skill_id) AS rk
        FROM weighted
      ) ranked WHERE rk <= ${SIGNATURE_SIZE}
    ),
    norm AS (SELECT role_id, sqrt(sum(w * w)) AS l FROM weighted GROUP BY role_id),
    person AS (
      SELECT skill_id, sum(w) AS w FROM (
        -- The mean runs over past roles that have a signature. A past role
        -- below the posting floor contributes nothing, so counting it would
        -- shrink the past relative to the claimed skills.
        SELECT skill_id, w / greatest(1, (SELECT count(*) FROM role_n WHERE role_id = ANY(${pastRoleIds}::bigint[]))) AS w
        FROM weighted WHERE role_id = ANY(${pastRoleIds}::bigint[])
        UNION ALL
        SELECT m.skill_id, coalesce((SELECT max(w) FROM weighted WHERE skill_id = m.skill_id), 0)
        FROM unnest(${claimedSkillIds}::bigint[]) AS m(skill_id)
      ) parts GROUP BY skill_id
    ),
    person_norm AS (SELECT nullif(sqrt(sum(w * w)), 0) AS l FROM person),
    kinship AS (
      SELECT r.role_id, sum(r.w * p.w) / (nullif(n.l, 0) * (SELECT l FROM person_norm)) AS k
      FROM weighted r JOIN person p USING (skill_id) JOIN norm n USING (role_id)
      GROUP BY r.role_id, n.l
    ),
    fit AS (
      SELECT role_id,
             coalesce(sum(w) FILTER (WHERE skill_id = ANY(${claimedSkillIds}::bigint[])), 0) / nullif(sum(w), 0) AS f
      FROM signature GROUP BY role_id
    ),
    closest AS (
      SELECT DISTINCT ON (a.role_id) a.role_id, b.role_id AS past_role_id
      FROM weighted a
      JOIN weighted b USING (skill_id)
      JOIN norm na ON na.role_id = a.role_id
      JOIN norm nb ON nb.role_id = b.role_id
      WHERE b.role_id = ANY(${pastRoleIds}::bigint[])
      GROUP BY a.role_id, b.role_id, na.l, nb.l
      ORDER BY a.role_id, sum(a.w * b.w) / nullif(na.l * nb.l, 0) DESC NULLS LAST, b.role_id
    ),
    -- Skills that come with the roles the person has held. A role can mix
    -- senses — Design Engineer spans UI work and CAD work — so each past role
    -- is read through its postings that also ask for a skill the person
    -- claimed: the person's own flavour of the role. Below FLAVOUR_FLOOR such
    -- postings the whole role stands in. Claimed skills take no slot; they
    -- are already the person's.
    flavour AS (
      SELECT pr.role_id, pr.job_posting_id
      FROM posting_role pr
      WHERE pr.role_id = ANY(${pastRoleIds}::bigint[])
        AND EXISTS (
          SELECT 1 FROM posting_skill ps
          WHERE ps.job_posting_id = pr.job_posting_id AND ps.skill_id = ANY(${claimedSkillIds}::bigint[])
        )
    ),
    flavour_n AS (
      SELECT role_id, count(*)::float8 AS n FROM flavour GROUP BY role_id HAVING count(*) >= ${FLAVOUR_FLOOR}
    ),
    past_skill AS (
      SELECT f.role_id, ps.skill_id,
             count(*) / fn.n * coalesce(r.w, ln((SELECT count(*) FROM role_n)::float8)) AS w
      FROM flavour f
      JOIN flavour_n fn USING (role_id)
      JOIN posting_skill ps USING (job_posting_id)
      LEFT JOIN rarity r USING (skill_id)
      GROUP BY f.role_id, ps.skill_id, fn.n, r.w
      UNION ALL
      SELECT role_id, skill_id, w FROM weighted
      WHERE role_id = ANY(${pastRoleIds}::bigint[]) AND role_id NOT IN (SELECT role_id FROM flavour_n)
    ),
    inherited AS (
      SELECT skill_id, array_agg(DISTINCT role_id::text) AS past_role_ids, max(w) AS w
      FROM (
        SELECT past_skill.*, row_number() OVER (PARTITION BY role_id ORDER BY w DESC, skill_id) AS rk
        FROM past_skill
        WHERE NOT skill_id = ANY(${claimedSkillIds}::bigint[])
      ) ranked
      WHERE rk <= ${INHERITED_PER_ROLE}
      GROUP BY skill_id
    ),
    person_skill AS (
      SELECT skill_id FROM inherited
      UNION
      SELECT unnest(${claimedSkillIds}::bigint[])
    ),
    scored AS (
      SELECT rn.role_id, rn.n, rn.companies,
             0.5 * coalesce(k.k, 0) + 0.5 * coalesce(f.f, 0) AS score
      FROM role_n rn
      LEFT JOIN kinship k USING (role_id)
      LEFT JOIN fit f USING (role_id)
      WHERE NOT rn.role_id = ANY(${pastRoleIds}::bigint[])
      ORDER BY score DESC, rn.role_id
      LIMIT ${RECOMMENDATION_LIMIT}
    )
    SELECT s.role_id::text, cr.slug, cr.name, s.n::int, s.companies, s.score,
           c.past_role_id::text AS closest_past_role_id,
           coalesce((
             SELECT json_agg(json_build_object('slug', k.slug, 'name', k.name) ORDER BY sg.w DESC, k.slug)
             FROM signature sg JOIN skills k ON k.id = sg.skill_id
             WHERE sg.role_id = s.role_id AND sg.skill_id = ANY(${claimedSkillIds}::bigint[])
           ), '[]') AS bring,
           coalesce((
             SELECT json_agg(g ORDER BY g.w DESC, g.slug) FROM (
               SELECT k.slug, k.name, round(sg.share::numeric, 3)::float8 AS share, sg.w
               FROM signature sg JOIN skills k ON k.id = sg.skill_id
               WHERE sg.role_id = s.role_id AND NOT sg.skill_id = ANY(${claimedSkillIds}::bigint[])
               ORDER BY sg.w DESC, k.slug
               LIMIT ${GROW_LIMIT}
             ) g
           ), '[]') AS grow,
           coalesce((
             SELECT json_agg(json_build_object('slug', k.slug, 'name', k.name) ORDER BY sg.w DESC, k.slug)
             FROM signature sg JOIN skills k ON k.id = sg.skill_id
             WHERE sg.role_id = s.role_id AND sg.skill_id IN (SELECT skill_id FROM person_skill)
           ), '[]') AS connects,
           -- Uncorrelated, so Postgres evaluates it once and repeats it per row.
           (SELECT coalesce(json_agg(json_build_object('slug', k.slug, 'name', k.name, 'past_role_ids', i.past_role_ids)
                                     ORDER BY i.w DESC, k.slug), '[]')
            FROM inherited i JOIN skills k ON k.id = i.skill_id) AS inherited
    FROM scored s
    JOIN canonical_roles cr ON cr.id = s.role_id
    LEFT JOIN closest c ON c.role_id = s.role_id
    ORDER BY s.score DESC, s.role_id
  `;

  const recIds = ranked.map((r) => r.role_id);

  const [titleRows, pastDemand, [coverage]] = await Promise.all([
    // Heads are lowercase in the view. The display form is the head's own span
    // inside the raw title, so "applied ai engineer" reads back as "Applied AI
    // Engineer" the way most employers wrote it. A head stitched from around a
    // bracket is not a span of the title; substr from position 0 would return a
    // truncated prefix, so that posting casts no display vote.
    sql<{ role_id: string; title_head: string; n: number; display: string | null }[]>`
      SELECT role_id::text, title_head, count(*)::int AS n,
             mode() WITHIN GROUP (ORDER BY display) AS display
      FROM (
        SELECT jpr.role_id, opt.title_head,
               CASE WHEN span.pos > 0 THEN substr(opt.title_clean, span.pos, length(opt.title_head)) END AS display
        FROM open_posting_titles opt
        CROSS JOIN LATERAL (SELECT strpos(lower(opt.title_clean), opt.title_head) AS pos) span
        JOIN latest_classifications lc USING (job_posting_id)
        JOIN job_posting_roles jpr ON jpr.classification_id = lc.classification_id
        WHERE jpr.role_id = ANY(${recIds}::bigint[]) AND opt.title_head IS NOT NULL
      ) t
      GROUP BY role_id, title_head
      ORDER BY role_id, n DESC, title_head
    `,
    sql<{ role_id: string; n: number }[]>`
      SELECT jpr.role_id::text, count(*)::int AS n
      FROM open_postings op
      JOIN latest_classifications lc USING (job_posting_id)
      JOIN job_posting_roles jpr ON jpr.classification_id = lc.classification_id
      WHERE jpr.role_id = ANY(${pastRoleIds}::bigint[])
      GROUP BY jpr.role_id
      HAVING count(*) >= ${MIN_POSTINGS}
    `,
    sql<{ open: number; classified: number; roles: number }[]>`
      SELECT (SELECT count(*)::int FROM open_postings) AS open,
             (SELECT count(*)::int FROM open_postings JOIN latest_classifications USING (job_posting_id)) AS classified,
             (SELECT count(*)::int FROM (
                SELECT jpr.role_id FROM open_postings op
                JOIN latest_classifications lc USING (job_posting_id)
                JOIN job_posting_roles jpr ON jpr.classification_id = lc.classification_id
                GROUP BY jpr.role_id HAVING count(*) >= ${MIN_POSTINGS}
             ) pool) AS roles
    `,
  ]);

  // The role's own name rides along past the limit, so the headline rule can find it.
  const roleNameById = new Map(ranked.map((r) => [r.role_id, r.name.toLowerCase()]));
  const titlesByRole = new Map<string, TitleVariant[]>();
  for (const row of titleRows) {
    const list = titlesByRole.get(row.role_id) ?? [];
    if (list.length < TITLE_LIMIT || row.title_head === roleNameById.get(row.role_id)) list.push({ title: row.display ?? titleCase(row.title_head), postings: row.n });
    titlesByRole.set(row.role_id, list);
  }

  const pastByRoleId = new Map(profile.pastTitles.flatMap((t) => (t.role ? [[t.role.id, t] as const] : [])));
  const demandByRoleId = new Map(pastDemand.map((d) => [d.role_id, d.n]));
  const top = ranked[0]?.score ?? 0;

  const titlesByPastRoleId = new Map<string, string[]>();
  for (const t of profile.pastTitles) {
    if (t.role) titlesByPastRoleId.set(t.role.id, [...(titlesByPastRoleId.get(t.role.id) ?? []), t.titleText]);
  }
  // With no ranked rows there are no signatures, so nothing is inherited.
  const inherited = ranked[0]?.inherited ?? [];
  const fromPastBySlug = new Map(
    inherited.map((i) => [i.slug, [...new Set(i.past_role_ids.flatMap((id) => titlesByPastRoleId.get(id) ?? []))]]),
  );
  const claimedSlugs = new Set<string>();
  const personSkills: PersonSkill[] = [];
  for (const c of profile.claimedSkills) {
    if (!c.skill || claimedSlugs.has(c.skill.slug)) continue;
    claimedSlugs.add(c.skill.slug);
    personSkills.push({ slug: c.skill.slug, name: c.skill.name, claimed: true, fromPast: [] });
  }
  for (const i of inherited) {
    personSkills.push({ slug: i.slug, name: i.name, claimed: false, fromPast: fromPastBySlug.get(i.slug) ?? [] });
  }

  return {
    pastRoles: profile.pastTitles.map((t) => ({
      titleText: t.titleText,
      roleSlug: t.role?.slug ?? null,
      roleName: t.role?.name ?? null,
      seniority: t.seniority?.name ?? null,
      openPostings: t.role ? (demandByRoleId.get(t.role.id) ?? null) : null,
    })),
    claimedSkills: profile.claimedSkills.map((c) => ({
      text: c.skillText,
      skill: c.skill ? { slug: c.skill.slug, name: c.skill.name } : null,
    })),
    personSkills,
    recommendations: ranked.map((r, i) => {
      const titles = titlesByRole.get(r.role_id) ?? [];
      const past = r.closest_past_role_id ? pastByRoleId.get(r.closest_past_role_id) : undefined;
      return {
        rank: i + 1,
        roleId: r.role_id,
        roleSlug: r.slug,
        roleName: r.name,
        headline: titles.find((t) => t.title.toLowerCase() === r.name.toLowerCase())?.title ?? titles[0]?.title ?? r.name,
        titles: titles.length > 0 ? titles : [{ title: r.name, postings: r.n }],
        closestPast: past?.role ? { titleText: past.titleText, roleName: past.role.name } : null,
        bring: r.bring,
        connects: r.connects,
        grow: r.grow.map(({ slug, name, share }) => ({ slug, name, share })),
        openPostings: r.n,
        companies: r.companies,
        strength: strengthOf(r.score, top),
        score: r.score,
        pinned: pinnedRoleIds.has(r.role_id),
      };
    }),
    coverage: {
      openPostings: coverage?.open ?? 0,
      classifiedPostings: coverage?.classified ?? 0,
      rolesConsidered: coverage?.roles ?? 0,
    },
  };
}

// Tiers are relative to the best match, so a thin profile still gets a
// "close" tier. Absolute cut-offs would put every role in "stretch" for a
// seven-skill profile, which is honest about nothing but the profile's size.
function strengthOf(score: number, top: number): Strength {
  if (top <= 0) return "stretch";
  const ratio = score / top;
  if (ratio >= 0.6) return "close";
  if (ratio >= 0.3) return "adjacent";
  return "stretch";
}

const ACRONYMS = new Set(["ai", "ml", "ui", "ux", "qa", "it", "hr", "gtm", "sre", "api", "sdk", "vp"]);

function titleCase(head: string): string {
  return head
    .split(" ")
    .map((w) => (ACRONYMS.has(w) ? w.toUpperCase() : w.charAt(0).toUpperCase() + w.slice(1)))
    .join(" ");
}
