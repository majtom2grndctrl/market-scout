import type { ISql } from "postgres";

import { getSql } from "./client";

// Taxonomy lookup for the profile's pickers, on the read-only client: the
// taxonomy is public, and the read-only role already holds EXECUTE on
// similarity(text, text).
//
// The ranking is the MCP taxonomy_search tool's (apps/tools/cmd/mcp/
// taxonomy_search.go): pg_trgm similarity against slug and name, then usage,
// then id. Its clustering is left out -- that guards an agent against
// attaching three spellings of one concept, and a person picking one term
// does not need it. Drift between the two only reorders a picker.
//
// One addition: a substring match qualifies a row even below the similarity
// floor. Trigram similarity is built for whole concepts, and the first few
// letters of one score too low to surface anything while the person types.

export type SearchableTable = "canonical_roles" | "skills";

export interface TaxonomyMatch {
  id: string;
  slug: string;
  name: string;
  usageCount: number;
}

// pg_trgm's default threshold, as in the MCP tool.
const MIN_SCORE = 0.3;
const MIN_TERM_LENGTH = 2;
const MAX_TERM_LENGTH = 120;
const DEFAULT_LIMIT = 8;

const LINKS: Record<SearchableTable, { table: string; column: string }> = {
  canonical_roles: { table: "job_posting_roles", column: "role_id" },
  skills: { table: "job_posting_skills", column: "skill_id" },
};

export async function selectTaxonomyMatches(
  sql: ISql,
  table: SearchableTable,
  rawTerm: string,
  limit = DEFAULT_LIMIT,
): Promise<TaxonomyMatch[]> {
  const term = rawTerm.trim();
  if (term.length < MIN_TERM_LENGTH || term.length > MAX_TERM_LENGTH) {
    return [];
  }
  const link = LINKS[table];
  const pattern = `%${term.replace(/[\\%_]/g, (c) => `\\${c}`)}%`;

  // Usage is one grouped pass over the candidates' links. A correlated count
  // per candidate seq-scans the link table each time -- no index leads on the
  // term column -- and measured 300-570 ms over the full skill list on
  // 2026-10-01; this shape measured 20-30 ms.
  const rows = await sql<{ id: string; slug: string; name: string; usage_count: number }[]>`
    WITH scored AS (
      SELECT t.id, t.slug, t.name,
             greatest(similarity(t.slug, ${term}), similarity(t.name, ${term}))::float8 AS score
      FROM ${sql(table)} t
      WHERE greatest(similarity(t.slug, ${term}), similarity(t.name, ${term})) >= ${MIN_SCORE}
         OR t.name ILIKE ${pattern}
         OR t.slug ILIKE ${pattern}
    ), usage AS (
      SELECT j.${sql(link.column)} AS id, count(*)::int AS usage_count
      FROM ${sql(link.table)} j
      WHERE j.${sql(link.column)} IN (SELECT id FROM scored)
      GROUP BY j.${sql(link.column)}
    )
    SELECT s.id, s.slug, s.name, coalesce(u.usage_count, 0) AS usage_count
    FROM scored s
    LEFT JOIN usage u ON u.id = s.id
    ORDER BY s.score DESC, usage_count DESC, s.id
    LIMIT ${limit}
  `;

  return rows.map((r) => ({ id: r.id, slug: r.slug, name: r.name, usageCount: r.usage_count }));
}

export async function searchTaxonomy(table: SearchableTable, term: string): Promise<TaxonomyMatch[]> {
  return selectTaxonomyMatches(await getSql(), table, term);
}
