// Prototype data access — asked 2026-09-21. Not production.
//
// DEVIATION FROM § Prototypes, stated rather than hidden. The rule is that a
// sketch composes `lib/db` and the measure engine and writes no SQL of its
// own. This one writes SQL, because the measure the sketch exists to test is
// structurally outside the grammar: it groups on the RAW POSTING TITLE, and
// the vocabulary has no title dimension (`lib/db/postings.ts` returns titles
// for the open cohort with no taxonomy join; the engine has no title
// grouping). Per § Prototypes that gap is the finding, not an obstacle — so
// it is recorded at the bottom of `page.tsx` and the SQL stays here, in the
// low-attention pocket, rather than landing in `lib/db` as production surface
// on the strength of one sketch.
//
// The two safety rules that survive the carve-out are both kept:
//   - No connection of its own. `getSql()` is the app's pooled read-only
//     client on DATABASE_URL_RO.
//   - No re-derivation of read-model state. `open_postings` and
//     `latest_classifications` are the views; this file never restates what
//     "currently open" or "latest classification" mean.

import { getSql } from "@/lib/db/client";

export interface RoleHeadRow {
  readonly role: string;
  /** Role assignments over open, classified postings — the chart's denominator. */
  readonly postings: number;
  readonly distinctHeads: number;
  readonly modalHead: string;
  readonly modalN: number;
  readonly modalShare: number;
  /** Largest single employer's share of this role's postings. */
  readonly topCompanyShare: number;
}

export interface Coverage {
  readonly openPostings: number;
  readonly classifiedPostings: number;
  readonly roleAssignments: number;
  readonly distinctRoles: number;
  readonly top3Postings: number;
  readonly topCompanies: readonly { readonly name: string; readonly postings: number }[];
}

// Seniority is a scope qualifier the same way a comma-suffixed team is, and
// leaving it attached fragments the head: before this strip, `Software
// Engineer` and `Product Engineer` both showed a modal share near 0.15 purely
// because `staff software engineer` and `software engineer` counted as two
// names for the same job. Applied twice so a stacked prefix ("Senior Staff
// Software Engineer") collapses fully; `^` plus the `g` flag would not repeat.
const SENIORITY =
  "^(senior|sr\\.?|staff|principal|lead|junior|jr\\.?|associate|entry[ -]level|distinguished|head of|iii|ii|i)\\s+";

// The head: raw title, dashes folded to commas, everything from the first
// separator onward dropped, whitespace collapsed, seniority stripped, btrimmed.
// btrim is load-bearing twice over — without the outer one `forward deployed
// engineer ` was its own head.
const HEAD_EXPR = `
  btrim(regexp_replace(
    regexp_replace(
      regexp_replace(
        btrim(regexp_replace(
          lower(split_part(regexp_replace(ps.title, '[—–-]', ',', 'g'), ',', 1)),
          '\\s+', ' ', 'g')),
        '${SENIORITY}', ''),
      '${SENIORITY}', ''),
    '\\s+', ' ', 'g'))
`;

/**
 * Modal head share per canonical role: of the open postings classified into a
 * role, the fraction sharing the single most common title head. Ties on count
 * break alphabetically so each role yields exactly one row.
 */
export async function fetchRoleHeads(minPostings: number): Promise<readonly RoleHeadRow[]> {
  const sql = await getSql();

  const rows = await sql<
    {
      role: string;
      postings: number;
      distinct_heads: number;
      modal_head: string;
      modal_n: number;
      modal_share: string;
      top_company_share: string;
    }[]
  >`
    with latest_title as (
      select distinct on (op.job_posting_id) op.job_posting_id, ps.title
      from open_postings op
      join posting_snapshots ps on ps.job_posting_id = op.job_posting_id
      order by op.job_posting_id, ps.fetched_at desc
    ),
    assigned as (
      select cr.name as role, jp.company_id, ${sql.unsafe(HEAD_EXPR)} as head
      from latest_classifications lc
      join job_posting_roles jpr on jpr.classification_id = lc.classification_id
      join canonical_roles cr on cr.id = jpr.role_id
      join latest_title ps on ps.job_posting_id = lc.job_posting_id
      join job_postings jp on jp.id = lc.job_posting_id
    ),
    role_total as (
      select role, count(*)::int as total, count(distinct head)::int as heads
      from assigned group by 1
    ),
    modal as (
      select role, head, n,
             row_number() over (partition by role order by n desc, head asc) as rk
      from (select role, head, count(*)::int as n from assigned group by 1, 2) counted
    ),
    top_company as (
      select role, max(n)::int as top_n
      from (select role, company_id, count(*)::int as n from assigned group by 1, 2) per_company
      group by 1
    )
    select role_total.role,
           role_total.total as postings,
           role_total.heads as distinct_heads,
           modal.head as modal_head,
           modal.n as modal_n,
           round(modal.n::numeric / role_total.total, 4) as modal_share,
           round(top_company.top_n::numeric / role_total.total, 4) as top_company_share
    from role_total
    join modal on modal.role = role_total.role and modal.rk = 1
    join top_company on top_company.role = role_total.role
    where role_total.total >= ${minPostings}
    order by modal_share asc, postings desc
  `;

  return rows.map((row) => ({
    role: row.role,
    postings: row.postings,
    distinctHeads: row.distinct_heads,
    modalHead: row.modal_head,
    modalN: row.modal_n,
    modalShare: Number(row.modal_share),
    topCompanyShare: Number(row.top_company_share),
  }));
}

export interface Bias {
  /** Median first-seen date of the classified vs unclassified open cohorts. */
  readonly classifiedMedianFirstSeen: Date | null;
  readonly unclassifiedMedianFirstSeen: Date | null;
  /** First seen since the start of the current month. */
  readonly classifiedThisMonth: number;
  readonly unclassifiedThisMonth: number;
  readonly unclassifiedPostings: number;
  /** ATS platforms present in the open cohort, with their classified count. */
  readonly platforms: readonly {
    readonly ats: string;
    readonly openPostings: number;
    readonly classified: number;
  }[];
  readonly companiesWithOpen: number;
  readonly zeroClassifiedCompanies: number;
  readonly biggestCompany: { readonly name: string; readonly openPostings: number; readonly classified: number } | null;
  readonly mostDrained: { readonly name: string; readonly openPostings: number; readonly classified: number } | null;
}

/**
 * Who is actually in the classified subset. Coverage says how much is missing;
 * this says which direction the hole points, which for a question about
 * emerging titles matters more than the headline percentage.
 */
export async function fetchBias(): Promise<Bias> {
  const sql = await getSql();

  const recency = await sql<
    { classified: boolean; n: number; median_first_seen: Date | null; this_month: number }[]
  >`
    with cohort as (
      select jp.first_seen_at,
             (lc.job_posting_id is not null) as classified
      from open_postings op
      join job_postings jp on jp.id = op.job_posting_id
      left join latest_classifications lc on lc.job_posting_id = op.job_posting_id
    )
    select classified,
           count(*)::int as n,
           percentile_disc(0.5) within group (order by first_seen_at) as median_first_seen,
           count(*) filter (where first_seen_at >= date_trunc('month', now()))::int as this_month
    from cohort
    group by 1
  `;

  const classifiedRow = recency.find((row) => row.classified);
  const unclassifiedRow = recency.find((row) => !row.classified);

  // source_type is 'ats' on every row and carries no platform information.
  // The platform lives on companies.ats.
  const platforms = await sql<{ ats: string; open_postings: number; classified: number }[]>`
    select c.ats,
           count(*)::int as open_postings,
           count(lc.job_posting_id)::int as classified
    from open_postings op
    join job_postings jp on jp.id = op.job_posting_id
    join companies c on c.id = jp.company_id
    left join latest_classifications lc on lc.job_posting_id = op.job_posting_id
    group by c.ats
    order by open_postings desc
  `;

  const companies = await sql<{ name: string; open_postings: number; classified: number }[]>`
    select c.name,
           count(*)::int as open_postings,
           count(lc.job_posting_id)::int as classified
    from open_postings op
    join job_postings jp on jp.id = op.job_posting_id
    join companies c on c.id = jp.company_id
    left join latest_classifications lc on lc.job_posting_id = op.job_posting_id
    group by c.name
  `;

  const withOpen = companies.filter((row) => row.open_postings > 0);
  const biggest = [...withOpen].sort((a, b) => b.open_postings - a.open_postings)[0];
  const mostDrained = [...withOpen]
    .filter((row) => row.open_postings >= 20)
    .sort((a, b) => b.classified / b.open_postings - a.classified / a.open_postings)[0];

  return {
    classifiedMedianFirstSeen: classifiedRow?.median_first_seen ?? null,
    unclassifiedMedianFirstSeen: unclassifiedRow?.median_first_seen ?? null,
    classifiedThisMonth: classifiedRow?.this_month ?? 0,
    unclassifiedThisMonth: unclassifiedRow?.this_month ?? 0,
    unclassifiedPostings: unclassifiedRow?.n ?? 0,
    platforms: platforms.map((row) => ({
      ats: row.ats,
      openPostings: row.open_postings,
      classified: row.classified,
    })),
    companiesWithOpen: withOpen.length,
    zeroClassifiedCompanies: withOpen.filter((row) => row.classified === 0).length,
    biggestCompany: biggest
      ? { name: biggest.name, openPostings: biggest.open_postings, classified: biggest.classified }
      : null,
    mostDrained: mostDrained
      ? {
          name: mostDrained.name,
          openPostings: mostDrained.open_postings,
          classified: mostDrained.classified,
        }
      : null,
  };
}

/** Every denominator the page needs, measured rather than asserted. */
export async function fetchCoverage(): Promise<Coverage> {
  const sql = await getSql();

  const [totals] = await sql<
    {
      open_postings: number;
      classified_postings: number;
      role_assignments: number;
      distinct_roles: number;
      top3_postings: number;
    }[]
  >`
    with open_total as (select count(*)::int as n from open_postings),
    classified as (
      select count(*)::int as n
      from open_postings op
      join latest_classifications lc on lc.job_posting_id = op.job_posting_id
    ),
    assignments as (
      select count(*)::int as n, count(distinct jpr.role_id)::int as roles
      from open_postings op
      join latest_classifications lc on lc.job_posting_id = op.job_posting_id
      join job_posting_roles jpr on jpr.classification_id = lc.classification_id
    ),
    top3 as (
      select coalesce(sum(n), 0)::int as n
      from (
        select count(*)::int as n
        from open_postings op
        join job_postings jp on jp.id = op.job_posting_id
        group by jp.company_id
        order by n desc
        limit 3
      ) ranked
    )
    select open_total.n as open_postings,
           classified.n as classified_postings,
           assignments.n as role_assignments,
           assignments.roles as distinct_roles,
           top3.n as top3_postings
    from open_total, classified, assignments, top3
  `;

  const topCompanies = await sql<{ name: string; postings: number }[]>`
    select c.name, count(*)::int as postings
    from open_postings op
    join job_postings jp on jp.id = op.job_posting_id
    join companies c on c.id = jp.company_id
    group by c.name
    order by postings desc
    limit 3
  `;

  return {
    openPostings: totals?.open_postings ?? 0,
    classifiedPostings: totals?.classified_postings ?? 0,
    roleAssignments: totals?.role_assignments ?? 0,
    distinctRoles: totals?.distinct_roles ?? 0,
    top3Postings: totals?.top3_postings ?? 0,
    topCompanies: topCompanies.map((row) => ({ name: row.name, postings: row.postings })),
  };
}
