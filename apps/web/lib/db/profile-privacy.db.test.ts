import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import { inRolledBackTransaction } from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// No object another application role can reach may read `app`. A view or a
// SECURITY DEFINER function runs with its owner's rights, so a grant on it
// leaks whatever it reads -- a view in `public` would reach every MCP session
// through the read-only role's default privilege. Each check runs once against
// a deliberate leak, rolled back, to prove it can see one.

const ROLES = ["market_scout_readonly", "market_scout_actions"];

async function leakingViews(sql: ISql): Promise<string[]> {
  const rows = await sql<{ view: string }[]>`
    SELECT DISTINCT format('%I.%I', vn.nspname, v.relname) AS view
    FROM pg_rewrite rw
    JOIN pg_class v ON v.oid = rw.ev_class
    JOIN pg_namespace vn ON vn.oid = v.relnamespace
    JOIN pg_depend d ON d.classid = 'pg_rewrite'::regclass AND d.objid = rw.oid
    JOIN pg_class ref ON d.refclassid = 'pg_class'::regclass AND ref.oid = d.refobjid
    JOIN pg_namespace rn ON rn.oid = ref.relnamespace
    WHERE v.relkind IN ('v', 'm')
      AND rn.nspname = 'app'
      AND ref.oid <> v.oid
      AND EXISTS (SELECT 1 FROM unnest(${ROLES}::text[]) r
                  WHERE has_table_privilege(r, v.oid, 'SELECT'))
    ORDER BY 1
  `;
  return rows.map((r) => r.view);
}

// The catalog records no dependency for a function body stored as text, so
// bodies are scanned. Schema-qualified references are the only way to reach
// `app` from a function in another schema: none of them sets a search_path
// that includes it.
async function leakingFunctions(sql: ISql): Promise<string[]> {
  const rows = await sql<{ fn: string }[]>`
    SELECT p.oid::regprocedure::text AS fn
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND (p.prosrc ~* '(\\mapp|"app")\\s*\\.'
           OR EXISTS (SELECT 1 FROM pg_depend d
                      JOIN pg_class ref ON d.refclassid = 'pg_class'::regclass AND ref.oid = d.refobjid
                      JOIN pg_namespace rn ON rn.oid = ref.relnamespace
                      WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND rn.nspname = 'app'))
      AND EXISTS (SELECT 1 FROM unnest(${ROLES}::text[]) r
                  WHERE has_function_privilege(r, p.oid, 'EXECUTE'))
    ORDER BY 1
  `;
  return rows.map((r) => r.fn);
}

function ownerOrSkip(context: { skip: () => void }) {
  const { ownerDsn } = testDsns();
  if (!ownerDsn) {
    context.skip();
    return undefined;
  }
  return postgres(ownerDsn);
}

describe("the app schema's privacy boundary", () => {
  it("has no view the read-only or action role can select that depends on an object in app", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;

    try {
      expect(await leakingViews(owner)).toEqual([]);

      // The read-only role's default privilege grants it this view on creation.
      await inRolledBackTransaction(owner, async (tx) => {
        await tx`CREATE VIEW public.vitest_leaking_view AS SELECT pinned_name FROM app.pins`;
        expect(await leakingViews(tx)).toEqual(["public.vitest_leaking_view"]);
      });
    } finally {
      await owner.end();
    }
  });

  it("has no function the read-only or action role can execute that names app in its body", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;

    try {
      // The repair functions name app and must stay owner-only.
      const [{ named }] = await owner<{ named: number }[]>`
        SELECT count(*)::int AS named FROM pg_proc WHERE prosrc ~* '\\mapp\\.'
          AND proname IN ('taxonomy_merge', 'taxonomy_retire', 'taxonomy_repair_unhandled_references')
      `;
      expect(named).toBe(2);
      expect(await leakingFunctions(owner)).toEqual([]);

      await inRolledBackTransaction(owner, async (tx) => {
        await tx.unsafe(`
          CREATE FUNCTION public.vitest_leaking_function() RETURNS bigint
          LANGUAGE plpgsql SECURITY DEFINER AS $$
          BEGIN RETURN (SELECT count(*) FROM app.pins); END $$
        `);
        await tx`GRANT EXECUTE ON FUNCTION public.vitest_leaking_function() TO market_scout_readonly`;
        expect(await leakingFunctions(tx)).toEqual(["vitest_leaking_function()"]);
      });
    } finally {
      await owner.end();
    }
  });
});
