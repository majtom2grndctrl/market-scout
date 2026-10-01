import { randomUUID } from "node:crypto";

import type { ISql, JSONValue, Sql } from "postgres";

// Fixture builders for the taxonomy repair and profile suites. Each runs on the
// caller's connection, so a suite can build inside a transaction it rolls back.
// Ids are strings: postgres.js returns bigint columns as strings.

export type TaxonomyTable = "canonical_roles" | "specializations" | "skills";

const LINKS: Record<TaxonomyTable, { table: string; column: string }> = {
  canonical_roles: { table: "job_posting_roles", column: "role_id" },
  specializations: { table: "job_posting_specializations", column: "specialization_id" },
  skills: { table: "job_posting_skills", column: "skill_id" },
};

export function newMarker(suite: string): string {
  return `vitest-${suite}-${randomUUID().slice(0, 8)}`;
}

export async function insertTerm(
  sql: ISql,
  table: TaxonomyTable,
  slug: string,
  name = `Fixture ${slug}`,
): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    INSERT INTO ${sql(table)} (slug, name) VALUES (${slug}, ${name}) RETURNING id
  `;
  return row.id;
}

// One company, one posting, one classification. Links hang off the
// classification, so each call is one independent link holder.
export async function insertClassification(sql: ISql, marker: string): Promise<string> {
  const key = `${marker}-${randomUUID().slice(0, 8)}`;
  const [company] = await sql<{ id: string }[]>`
    INSERT INTO companies (name, ats, board_token)
    VALUES (${`Vitest ${key}`}, 'greenhouse', ${key})
    RETURNING id
  `;
  const [posting] = await sql<{ id: string }[]>`
    INSERT INTO job_postings (company_id, source_type, source_url)
    VALUES (${company.id}, 'ats', ${`https://example.test/${key}`})
    RETURNING id
  `;
  const [classification] = await sql<{ id: string }[]>`
    INSERT INTO classifications (job_posting_id, model, prompt_version, seniority)
    VALUES (${posting.id}, 'fixture-model', 'fixture', 'unknown')
    RETURNING id
  `;
  return classification.id;
}

export async function insertLink(
  sql: ISql,
  table: TaxonomyTable,
  classificationId: string,
  termId: string,
): Promise<void> {
  const link = LINKS[table];
  await sql`
    INSERT INTO ${sql(link.table)} (classification_id, ${sql(link.column)})
    VALUES (${classificationId}, ${termId})
  `;
}

export async function insertDimension(sql: ISql, roleId: string, dimensionSlug: string): Promise<void> {
  await sql`
    INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
    SELECT ${roleId}, id FROM role_dimensions WHERE slug = ${dimensionSlug}
  `;
}

export async function merge(
  sql: ISql,
  table: TaxonomyTable,
  map: { slug: string; into: string; reason?: string }[],
  retiredBy = "vitest",
): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    SELECT public.taxonomy_merge(${table}, ${sql.json(map as unknown as JSONValue)}, ${retiredBy}) AS id
  `;
  return row.id;
}

export async function retire(
  sql: ISql,
  table: TaxonomyTable,
  terms: { slug: string; reason: string }[],
  retiredBy = "vitest",
): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    SELECT public.taxonomy_retire(${table}, ${sql.json(terms as unknown as JSONValue)}, ${retiredBy}) AS id
  `;
  return row.id;
}

export async function undo(sql: ISql, repairId: string): Promise<void> {
  await sql`SELECT public.taxonomy_undo(${repairId})`;
}

export async function termId(sql: ISql, table: TaxonomyTable, slug: string): Promise<string | null> {
  const [row] = await sql<{ id: string }[]>`SELECT id FROM ${sql(table)} WHERE slug = ${slug}`;
  return row?.id ?? null;
}

// Profile rows. Install-scoped, so callers put the marker in any text a
// uniqueness rule reads.
export async function insertPastTitle(
  sql: ISql,
  text: string,
  roleId: string | null,
  seniority: string | null = null,
): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    INSERT INTO app.past_titles (title_text, role_id, seniority)
    VALUES (${text}, ${roleId}, ${seniority})
    RETURNING id
  `;
  return row.id;
}

export async function insertPin(sql: ISql, roleId: string, name: string): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    INSERT INTO app.pins (role_id, pinned_name) VALUES (${roleId}, ${name}) RETURNING id
  `;
  return row.id;
}

export async function insertClaim(sql: ISql, text: string, skillId: string | null): Promise<string> {
  const [row] = await sql<{ id: string }[]>`
    INSERT INTO app.claimed_skills (skill_text, skill_id) VALUES (${text}, ${skillId}) RETURNING id
  `;
  return row.id;
}

// Links that point at any of the given slugs, by slug rather than id, so a
// snapshot taken before a merge compares equal to one taken after its undo.
export async function linkSlugs(
  sql: ISql,
  table: TaxonomyTable,
  slugs: string[],
): Promise<string[]> {
  const link = LINKS[table];
  const rows = await sql<{ pair: string }[]>`
    SELECT j.classification_id || ':' || t.slug AS pair
    FROM ${sql(link.table)} j
    JOIN ${sql(table)} t ON t.id = j.${sql(link.column)}
    WHERE t.slug = ANY(${slugs})
    ORDER BY 1
  `;
  return rows.map((r) => r.pair);
}

export async function retiredSlugRecords(sql: ISql, slugs: string[]): Promise<string[]> {
  const rows = await sql<{ record: string }[]>`
    SELECT slug || '|' || coalesce(table_name, '*') || '|' || retired_by_migration AS record
    FROM retired_slugs WHERE slug = ANY(${slugs})
    ORDER BY 1
  `;
  return rows.map((r) => r.record);
}

// The posting side of a repair, in slug terms: the term rows, their links, the
// role dimensions, and the retired-slug records. What undo promises to restore.
export async function postingSide(sql: ISql, table: TaxonomyTable, slugs: string[]) {
  const terms = await sql<{ term: string }[]>`
    SELECT id || '|' || slug || '|' || name || '|' || created_at AS term
    FROM ${sql(table)} WHERE slug = ANY(${slugs}) ORDER BY 1
  `;
  const dimensions =
    table === "canonical_roles"
      ? await sql<{ dim: string }[]>`
          SELECT r.slug || ':' || d.slug AS dim
          FROM canonical_role_dimensions crd
          JOIN canonical_roles r ON r.id = crd.canonical_role_id
          JOIN role_dimensions d ON d.id = crd.dimension_id
          WHERE r.slug = ANY(${slugs}) ORDER BY 1
        `
      : [];
  return {
    terms: terms.map((t) => t.term),
    links: await linkSlugs(sql, table, slugs),
    dimensions: dimensions.map((d) => d.dim),
    retired: await retiredSlugRecords(sql, slugs),
  };
}

// Thrown to roll a fixture transaction back once assertions have run.
export const rollback = new Error("rollback fixture transaction");

export async function inRolledBackTransaction(
  sql: Sql,
  body: (tx: ISql) => Promise<void>,
): Promise<void> {
  await expectRollback(sql.begin(async (tx) => {
    await body(tx);
    throw rollback;
  }));
}

async function expectRollback(run: Promise<unknown>): Promise<void> {
  try {
    await run;
  } catch (error) {
    if (error === rollback) return;
    throw error;
  }
  throw new Error("fixture transaction committed; expected rollback");
}
