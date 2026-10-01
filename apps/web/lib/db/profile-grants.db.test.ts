import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import { inRolledBackTransaction } from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// The grant boundary, asserted in both directions. The other db suites fail
// only on a privilege that went missing; a grant that got wider leaves every
// one of them green. So the refusals here are checked directly: through the
// catalog over every table and function, and with a real statement the role
// must have refused.

const APP = "market_scout_app";
const READ_ONLY = "market_scout_readonly";

const PROFILE_TABLES = ["app.past_titles", "app.claimed_skills", "app.pins"];
const REPAIR_FUNCTIONS = [
  "public.taxonomy_repair_unhandled_references()",
  "public.taxonomy_repair_begin(text, text)",
  "public.taxonomy_merge(text, jsonb, text)",
  "public.taxonomy_retire(text, jsonb, text)",
  "public.taxonomy_undo(bigint)",
];

function dsnsOrSkip(context: { skip: () => void }) {
  const { ownerDsn, readOnlyDsn, appDsn } = testDsns();
  if (!ownerDsn || !readOnlyDsn || !appDsn) {
    context.skip();
    return undefined;
  }
  return { ownerDsn, readOnlyDsn, appDsn };
}

async function executable(sql: ISql, role: string): Promise<string[]> {
  const rows = await sql<{ fn: string }[]>`
    SELECT fn FROM unnest(${REPAIR_FUNCTIONS}::text[]) AS fn
    WHERE has_function_privilege(${role}, fn::regprocedure, 'EXECUTE')
  `;
  return rows.map((r) => r.fn);
}

describe("market_scout_app", () => {
  it("inserts, updates, selects, and deletes rows in each profile table, and selects the taxonomy the profile names", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const app = postgres(dsns.appDsn);

    try {
      await inRolledBackTransaction(app, async (tx) => {
        const [role] = await tx<{ id: string; name: string }[]>`
          SELECT id, name FROM canonical_roles ORDER BY id LIMIT 1
        `;
        expect(role).toBeDefined();
        await tx`SELECT id, slug, name FROM skills LIMIT 1`;
        const ranks = await tx<{ slug: string }[]>`SELECT slug FROM title_seniority_seeds`;
        expect(ranks.map((r) => r.slug)).toContain("senior");

        const [title] = await tx<{ id: string }[]>`
          INSERT INTO app.past_titles (title_text, role_id, seniority)
          VALUES ('Grant check title', ${role.id}, 'senior') RETURNING id
        `;
        await tx`UPDATE app.past_titles SET seniority = 'staff' WHERE id = ${title.id}`;
        const [readTitle] = await tx`SELECT seniority FROM app.past_titles WHERE id = ${title.id}`;
        expect(readTitle.seniority).toBe("staff");
        await tx`DELETE FROM app.past_titles WHERE id = ${title.id}`;

        const [skill] = await tx<{ id: string }[]>`
          INSERT INTO app.claimed_skills (skill_text) VALUES ('Grant check skill') RETURNING id
        `;
        await tx`UPDATE app.claimed_skills SET skill_text = 'Grant check skill, edited' WHERE id = ${skill.id}`;
        const [readSkill] = await tx`SELECT skill_text FROM app.claimed_skills WHERE id = ${skill.id}`;
        expect(readSkill.skill_text).toBe("Grant check skill, edited");
        await tx`DELETE FROM app.claimed_skills WHERE id = ${skill.id}`;

        // A pin with no role keeps the check independent of any pin the
        // person already holds in this database.
        const [pin] = await tx<{ id: string }[]>`
          INSERT INTO app.pins (role_id, pinned_name) VALUES (NULL, 'Grant check pin') RETURNING id
        `;
        await tx`UPDATE app.pins SET pinned_name = 'Grant check pin, edited' WHERE id = ${pin.id}`;
        const [readPin] = await tx`SELECT pinned_name FROM app.pins WHERE id = ${pin.id}`;
        expect(readPin.pinned_name).toBe("Grant check pin, edited");
        await tx`DELETE FROM app.pins WHERE id = ${pin.id}`;
      });
    } finally {
      await app.end();
    }
  });

  it("is refused INSERT, UPDATE, and DELETE on every table outside app, and EXECUTE on every repair and undo function", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn);
    const app = postgres(dsns.appDsn);

    try {
      const writable = await owner<{ relation: string; privilege: string }[]>`
        SELECT format('%I.%I', n.nspname, c.relname) AS relation, p.privilege
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        CROSS JOIN unnest(ARRAY['INSERT', 'UPDATE', 'DELETE', 'TRUNCATE']) AS p(privilege)
        WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
          AND n.nspname NOT IN ('app', 'pg_catalog', 'information_schema')
          AND n.nspname NOT LIKE 'pg_toast%'
          AND has_table_privilege(${APP}, c.oid, p.privilege)
      `;
      expect(writable).toEqual([]);
      const [{ tables }] = await owner<{ tables: number }[]>`
        SELECT count(*)::int AS tables FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind = 'r' AND n.nspname = 'public'
      `;
      expect(tables).toBeGreaterThan(10);

      expect(await executable(owner, APP)).toEqual([]);

      await expect(
        app`INSERT INTO canonical_roles (slug, name) VALUES ('grant-check', 'Grant check')`,
      ).rejects.toThrow(/permission denied for table canonical_roles/);
      await expect(app`SELECT public.taxonomy_undo(1)`).rejects.toThrow(
        /permission denied for function taxonomy_undo/,
      );
    } finally {
      await app.end();
      await owner.end();
    }
  });

  it("gets no privilege on a table the owner adds to app after provisioning, holds no membership, and owns nothing", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn);

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await tx`CREATE TABLE app.vitest_added_later (id int)`;
        const [row] = await tx<{ granted: boolean }[]>`
          SELECT has_table_privilege(${APP}, 'app.vitest_added_later',
            'SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER') AS granted
        `;
        expect(row.granted).toBe(false);
      });

      const [standing] = await owner<{ memberships: number; owned: number }[]>`
        SELECT
          (SELECT count(*)::int FROM pg_auth_members WHERE member = ${APP}::regrole) AS memberships,
          (SELECT count(*)::int FROM (
              SELECT relowner AS o FROM pg_class
              UNION ALL SELECT proowner FROM pg_proc
              UNION ALL SELECT nspowner FROM pg_namespace
              UNION ALL SELECT typowner FROM pg_type
              UNION ALL SELECT datdba FROM pg_database
          ) owners WHERE o = ${APP}::regrole) AS owned
      `;
      expect(standing).toEqual({ memberships: 0, owned: 0 });
    } finally {
      await owner.end();
    }
  });
});

describe("market_scout_readonly", () => {
  it("is refused SELECT, INSERT, UPDATE, and DELETE on every profile table, and EXECUTE on every repair and undo function", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn);
    const readOnly = postgres(dsns.readOnlyDsn);

    try {
      const granted = await owner<{ relation: string }[]>`
        SELECT relation FROM unnest(${PROFILE_TABLES}::text[]) AS relation
        WHERE has_table_privilege(${READ_ONLY}, relation, 'SELECT, INSERT, UPDATE, DELETE')
      `;
      expect(granted).toEqual([]);
      const [schema] = await owner<{ usage: boolean }[]>`
        SELECT has_schema_privilege(${READ_ONLY}, 'app', 'USAGE') AS usage
      `;
      expect(schema.usage).toBe(false);
      expect(await executable(owner, READ_ONLY)).toEqual([]);

      await expect(readOnly`SELECT * FROM app.pins`).rejects.toThrow(
        /permission denied for schema app/,
      );
      await expect(readOnly`SELECT * FROM public.taxonomy_repair_unhandled_references()`).rejects.toThrow(
        /permission denied for function taxonomy_repair_unhandled_references/,
      );
    } finally {
      await readOnly.end();
      await owner.end();
    }
  });
});
