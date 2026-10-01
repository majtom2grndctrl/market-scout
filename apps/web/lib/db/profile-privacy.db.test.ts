import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import { inRolledBackTransaction } from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// No object another application role can reach may read `app`. A view or a
// SECURITY DEFINER function runs with its owner's rights, so a grant on it
// leaks whatever it reads -- a view in `public` would reach every MCP session
// through the read-only role's default privilege. Each check first runs
// against deliberate leaks, rolled back, to prove it can see them.

const ROLES = ["market_scout_readonly", "market_scout_actions"];

// Roles are cluster-wide and the action role is provisioned only where the
// MCP write tools run, so check whichever of them this cluster has.
async function presentRoles(sql: ISql): Promise<string[]> {
  const rows = await sql<{ rolname: string }[]>`
    SELECT rolname FROM pg_roles WHERE rolname = ANY(${ROLES}) ORDER BY 1
  `;
  return rows.map((r) => r.rolname);
}

// Every view that reads `app`, directly or through another view, that a
// checked role can select -- whole-table or column by column.
async function leakingViews(sql: ISql): Promise<string[]> {
  const roles = await presentRoles(sql);
  const rows = await sql<{ view: string }[]>`
    WITH RECURSIVE reads_app(oid) AS (
      SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE n.nspname = 'app'
      UNION
      SELECT rw.ev_class
      FROM pg_rewrite rw
      JOIN pg_depend d ON d.classid = 'pg_rewrite'::regclass AND d.objid = rw.oid
                      AND d.refclassid = 'pg_class'::regclass
      JOIN reads_app r ON r.oid = d.refobjid
      WHERE rw.ev_class <> d.refobjid
    )
    SELECT format('%I.%I', vn.nspname, v.relname) AS view
    FROM reads_app r
    JOIN pg_class v ON v.oid = r.oid AND v.relkind IN ('v', 'm')
    JOIN pg_namespace vn ON vn.oid = v.relnamespace
    WHERE vn.nspname <> 'app'
      AND EXISTS (SELECT 1 FROM unnest(${roles}::text[]) role
                  WHERE has_table_privilege(role, v.oid, 'SELECT')
                     OR has_any_column_privilege(role, v.oid, 'SELECT'))
    ORDER BY 1
  `;
  return rows.map((r) => r.view);
}

// The catalog records no dependency for a function body stored as text, so
// bodies are scanned for a reference to `app`, and settings for a search_path
// that reaches it unqualified. Dynamic SQL that assembles the name at run time
// is beyond a scan; owner-only grants are the defence there.
async function leakingFunctions(sql: ISql): Promise<string[]> {
  const roles = await presentRoles(sql);
  const rows = await sql<{ fn: string }[]>`
    SELECT p.oid::regprocedure::text AS fn
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
      AND (p.prosrc ~* '(\\mapp|"app")\\s*\\.'
           OR coalesce(array_to_string(p.proconfig, ','), '') ~* 'search_path=[^,]*\\mapp\\M'
           OR EXISTS (SELECT 1 FROM pg_depend d
                      JOIN pg_class ref ON d.refclassid = 'pg_class'::regclass AND ref.oid = d.refobjid
                      JOIN pg_namespace rn ON rn.oid = ref.relnamespace
                      WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND rn.nspname = 'app'))
      AND EXISTS (SELECT 1 FROM unnest(${roles}::text[]) role
                  WHERE has_function_privilege(role, p.oid, 'EXECUTE'))
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
      expect(await presentRoles(owner)).toContain("market_scout_readonly");
      expect(await leakingViews(owner)).toEqual([]);

      await inRolledBackTransaction(owner, async (tx) => {
        // Granted on creation by the read-only role's default privilege.
        await tx`CREATE VIEW public.vitest_leaking_view AS SELECT pinned_name FROM app.pins`;
        // Reads app only through another view the role cannot select.
        await tx`CREATE VIEW public.vitest_inner_view AS SELECT title_text FROM app.past_titles`;
        await tx`REVOKE ALL ON public.vitest_inner_view FROM market_scout_readonly`;
        await tx`CREATE VIEW public.vitest_outer_view AS SELECT title_text FROM public.vitest_inner_view`;
        // Granted column by column only.
        await tx`CREATE VIEW public.vitest_column_view AS SELECT skill_text FROM app.claimed_skills`;
        await tx`REVOKE ALL ON public.vitest_column_view FROM market_scout_readonly`;
        await tx`GRANT SELECT (skill_text) ON public.vitest_column_view TO market_scout_readonly`;

        expect(await leakingViews(tx)).toEqual([
          "public.vitest_column_view",
          "public.vitest_leaking_view",
          "public.vitest_outer_view",
        ]);
      });
    } finally {
      await owner.end();
    }
  });

  it("has no function the read-only or action role can execute that names app in its body", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;

    try {
      // The repair functions that write app must be among those scanned, and
      // owner-only, for the empty result below to mean anything.
      const named = await owner<{ proname: string }[]>`
        SELECT proname FROM pg_proc
        WHERE pronamespace = 'public'::regnamespace
          AND proname IN ('taxonomy_merge', 'taxonomy_retire')
          AND prosrc ~* '(\\mapp|"app")\\s*\\.'
        ORDER BY 1
      `;
      expect(named.map((r) => r.proname)).toEqual(["taxonomy_merge", "taxonomy_retire"]);
      expect(await leakingFunctions(owner)).toEqual([]);

      await inRolledBackTransaction(owner, async (tx) => {
        await tx.unsafe(`
          CREATE FUNCTION public.vitest_leaking_function() RETURNS bigint
          LANGUAGE plpgsql SECURITY DEFINER AS $$
          BEGIN RETURN (SELECT count(*) FROM app.pins); END $$
        `);
        await tx.unsafe(`
          CREATE FUNCTION public.vitest_search_path_function() RETURNS bigint
          LANGUAGE plpgsql SECURITY DEFINER SET search_path = app, pg_catalog AS $$
          BEGIN RETURN (SELECT count(*) FROM pins); END $$
        `);
        await tx`GRANT EXECUTE ON FUNCTION public.vitest_leaking_function() TO market_scout_readonly`;
        await tx`GRANT EXECUTE ON FUNCTION public.vitest_search_path_function() TO market_scout_readonly`;
        expect(await leakingFunctions(tx)).toEqual([
          "vitest_leaking_function()",
          "vitest_search_path_function()",
        ]);
      });
    } finally {
      await owner.end();
    }
  });
});
