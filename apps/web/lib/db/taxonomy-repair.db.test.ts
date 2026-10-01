import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import {
  inRolledBackTransaction,
  insertClassification,
  insertDimension,
  insertLink,
  insertTerm,
  insertClaim as claim,
  insertPastTitle as pastTitle,
  insertPin as pin,
  linkSlugs,
  merge,
  newMarker,
  retire,
  retiredSlugRecords,
  termId,
} from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// The repair functions are owner-only, so every statement here runs on the
// owner DSN. Each case builds inside a transaction it rolls back, except the
// race, which needs two connections to see each other's commits.

function ownerOrSkip(context: { skip: () => void }) {
  const { ownerDsn } = testDsns();
  if (!ownerDsn) {
    context.skip();
    return undefined;
  }
  return postgres(ownerDsn, { onnotice: () => {} });
}

// Every row a repair could touch, hashed per table. Compared inside one
// repeatable-read transaction, so only this transaction's own writes move it.
async function fingerprint(sql: ISql) {
  const tables = [
    "canonical_roles",
    "specializations",
    "skills",
    "job_posting_roles",
    "job_posting_specializations",
    "job_posting_skills",
    "canonical_role_dimensions",
    "retired_slugs",
    "app.past_titles",
    "app.claimed_skills",
    "app.pins",
  ];
  const out: Record<string, string> = {};
  for (const table of tables) {
    const [row] = await sql<{ digest: string }[]>`
      SELECT count(*) || ':' || coalesce(md5(string_agg(t::text, ',' ORDER BY t::text)), '') AS digest
      FROM ${sql(table)} t
    `;
    out[table] = row.digest;
  }
  return out;
}

describe("taxonomy_merge", () => {
  it("moves a role's posting link, dimension, past title, and pin to the survivor, deletes the role, and records its slug", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-role");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const merged = await insertTerm(tx, "canonical_roles", `${m}-old`, "Old Role");
        const survivor = await insertTerm(tx, "canonical_roles", `${m}-new`, "New Role");
        const classification = await insertClassification(tx, m);
        await insertLink(tx, "canonical_roles", classification, merged);
        await insertDimension(tx, merged, "design");
        const title = await pastTitle(tx, `${m} Old Role title`, merged);
        const pinId = await pin(tx, merged, "Old Role");

        await merge(tx, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new`, reason: "alias" }]);

        expect(await termId(tx, "canonical_roles", `${m}-old`)).toBeNull();
        expect(await linkSlugs(tx, "canonical_roles", [`${m}-old`, `${m}-new`])).toEqual([
          `${classification}:${m}-new`,
        ]);
        const dims = await tx<{ slug: string }[]>`
          SELECT d.slug FROM canonical_role_dimensions crd
          JOIN role_dimensions d ON d.id = crd.dimension_id
          WHERE crd.canonical_role_id = ${survivor}
        `;
        expect(dims.map((d) => d.slug)).toEqual(["design"]);
        const [titleRow] = await tx<{ role_id: string }[]>`
          SELECT role_id FROM app.past_titles WHERE id = ${title}
        `;
        expect(titleRow.role_id).toBe(survivor);
        const [pinRow] = await tx<{ role_id: string; pinned_name: string }[]>`
          SELECT role_id, pinned_name FROM app.pins WHERE id = ${pinId}
        `;
        expect(pinRow).toEqual({ role_id: survivor, pinned_name: "Old Role" });
        expect(await retiredSlugRecords(tx, [`${m}-old`])).toEqual([
          `${m}-old|canonical_roles|vitest`,
        ]);
      });
    } finally {
      await owner.end();
    }
  });

  it("leaves one pin on the survivor when both roles are pinned", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-pins");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const merged = await insertTerm(tx, "canonical_roles", `${m}-old`);
        const survivor = await insertTerm(tx, "canonical_roles", `${m}-new`);
        const mergedPin = await pin(tx, merged, "Old");
        const survivorPin = await pin(tx, survivor, "New");

        await merge(tx, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }]);

        const pins = await tx<{ id: string; role_id: string }[]>`
          SELECT id, role_id FROM app.pins WHERE id IN (${mergedPin}, ${survivorPin})
        `;
        expect(pins).toEqual([{ id: survivorPin, role_id: survivor }]);
      });
    } finally {
      await owner.end();
    }
  });

  it("keeps the survivor's claim, text unchanged, when both skills are claimed", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-claims");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const merged = await insertTerm(tx, "skills", `${m}-golang`);
        const survivor = await insertTerm(tx, "skills", `${m}-go`);
        const mergedClaim = await claim(tx, `${m} Golang`, merged);
        const survivorClaim = await claim(tx, `${m} Go`, survivor);

        await merge(tx, "skills", [{ slug: `${m}-golang`, into: `${m}-go` }]);

        const claims = await tx<{ id: string; skill_text: string; skill_id: string }[]>`
          SELECT id, skill_text, skill_id FROM app.claimed_skills
          WHERE id IN (${mergedClaim}, ${survivorClaim})
        `;
        expect(claims).toEqual([{ id: survivorClaim, skill_text: `${m} Go`, skill_id: survivor }]);
      });
    } finally {
      await owner.end();
    }
  });

  it("re-points a claim of the merged skill, text unchanged, when only it is claimed", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-claim");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const merged = await insertTerm(tx, "skills", `${m}-golang`);
        const survivor = await insertTerm(tx, "skills", `${m}-go`);
        const mergedClaim = await claim(tx, `${m} Golang`, merged);

        await merge(tx, "skills", [{ slug: `${m}-golang`, into: `${m}-go` }]);

        const [row] = await tx<{ skill_text: string; skill_id: string }[]>`
          SELECT skill_text, skill_id FROM app.claimed_skills WHERE id = ${mergedClaim}
        `;
        expect(row).toEqual({ skill_text: `${m} Golang`, skill_id: survivor });
      });
    } finally {
      await owner.end();
    }
  });

  it("moves a specialization's posting links to the survivor, deletes it, and records its slug", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-spec");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const merged = await insertTerm(tx, "specializations", `${m}-fintech`);
        await insertTerm(tx, "specializations", `${m}-financial-technology`);
        const first = await insertClassification(tx, m);
        const second = await insertClassification(tx, m);
        await insertLink(tx, "specializations", first, merged);
        await insertLink(tx, "specializations", second, merged);

        await merge(tx, "specializations", [
          { slug: `${m}-fintech`, into: `${m}-financial-technology` },
        ]);

        expect(await termId(tx, "specializations", `${m}-fintech`)).toBeNull();
        expect(
          await linkSlugs(tx, "specializations", [`${m}-fintech`, `${m}-financial-technology`]),
        ).toEqual(
          [`${first}:${m}-financial-technology`, `${second}:${m}-financial-technology`].sort(),
        );
        expect(await retiredSlugRecords(tx, [`${m}-fintech`])).toEqual([
          `${m}-fintech|specializations|vitest`,
        ]);
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses a map that chains, and a term merged into itself", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("merge-refuse");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "skills", `${m}-a`);
        await insertTerm(tx, "skills", `${m}-b`);
        await insertTerm(tx, "skills", `${m}-c`);
        await tx`SAVEPOINT chain`;
        await expect(
          merge(tx, "skills", [
            { slug: `${m}-a`, into: `${m}-b` },
            { slug: `${m}-b`, into: `${m}-c` },
          ]),
        ).rejects.toThrow(/map chains/);
        await tx`ROLLBACK TO SAVEPOINT chain`;
        await expect(merge(tx, "skills", [{ slug: `${m}-a`, into: `${m}-a` }])).rejects.toThrow(
          /cannot merge into itself/,
        );
        await tx`ROLLBACK TO SAVEPOINT chain`;
        expect(await termId(tx, "skills", `${m}-a`)).not.toBeNull();
        expect(await termId(tx, "skills", `${m}-b`)).not.toBeNull();
      });
    } finally {
      await owner.end();
    }
  });
});

describe("taxonomy_retire", () => {
  it("leaves a retired role's pin with no role and its pinned-at name, and its past title unmatched with its text", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("retire-role");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const role = await insertTerm(tx, "canonical_roles", `${m}-role`, "Retired Role");
        const classification = await insertClassification(tx, m);
        await insertLink(tx, "canonical_roles", classification, role);
        await insertDimension(tx, role, "design");
        const title = await pastTitle(tx, `${m} Retired Role title`, role);
        const pinId = await pin(tx, role, "Retired Role");

        await retire(tx, "canonical_roles", [{ slug: `${m}-role`, reason: "Umbrella role." }]);

        expect(await termId(tx, "canonical_roles", `${m}-role`)).toBeNull();
        const [pinRow] = await tx`SELECT role_id, pinned_name FROM app.pins WHERE id = ${pinId}`;
        expect(pinRow).toEqual({ role_id: null, pinned_name: "Retired Role" });
        const [titleRow] = await tx`SELECT role_id, title_text FROM app.past_titles WHERE id = ${title}`;
        expect(titleRow).toEqual({ role_id: null, title_text: `${m} Retired Role title` });
        const [{ count }] = await tx<{ count: number }[]>`
          SELECT count(*)::int AS count FROM job_posting_roles WHERE classification_id = ${classification}
        `;
        expect(count).toBe(0);
        expect(await retiredSlugRecords(tx, [`${m}-role`])).toEqual([
          `${m}-role|canonical_roles|vitest`,
        ]);
      });
    } finally {
      await owner.end();
    }
  });

  it("leaves a retired skill's claim unmatched with its text and deletes the skill's posting links", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("retire-skill");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const skill = await insertTerm(tx, "skills", `${m}-skill`);
        const classification = await insertClassification(tx, m);
        await insertLink(tx, "skills", classification, skill);
        const claimId = await claim(tx, `${m} Systems Knowledge`, skill);

        await retire(tx, "skills", [{ slug: `${m}-skill`, reason: "Umbrella skill." }]);

        const [row] = await tx`SELECT skill_id, skill_text FROM app.claimed_skills WHERE id = ${claimId}`;
        expect(row).toEqual({ skill_id: null, skill_text: `${m} Systems Knowledge` });
        expect(await linkSlugs(tx, "skills", [`${m}-skill`])).toEqual([]);
        const [{ count }] = await tx<{ count: number }[]>`
          SELECT count(*)::int AS count FROM job_posting_skills WHERE classification_id = ${classification}
        `;
        expect(count).toBe(0);
      });
    } finally {
      await owner.end();
    }
  });
});

describe("repairs across installs", () => {
  it("records an absent term's slug as retired and changes no taxonomy, posting, or profile row", async (context) => {
    const { ownerDsn } = testDsns();
    if (!ownerDsn) {
      context.skip();
      return;
    }
    const owner = postgres(ownerDsn, { onnotice: () => {} });
    const m = newMarker("absent");
    const rollbackError = new Error("rollback");

    try {
      await owner
        .begin("isolation level repeatable read", async (tx) => {
          const before = await fingerprint(tx);

          await retire(tx, "skills", [{ slug: `${m}-retired`, reason: "Absent here." }]);
          await merge(tx, "canonical_roles", [{ slug: `${m}-merged`, into: `${m}-survivor` }]);

          const after = await fingerprint(tx);
          const { retired_slugs: retiredBefore, ...restBefore } = before;
          const { retired_slugs: retiredAfter, ...restAfter } = after;
          expect(restAfter).toEqual(restBefore);
          expect(retiredAfter).not.toEqual(retiredBefore);
          expect(await retiredSlugRecords(tx, [`${m}-retired`, `${m}-merged`])).toEqual([
            `${m}-merged|canonical_roles|vitest`,
            `${m}-retired|skills|vitest`,
          ]);
          throw rollbackError;
        })
        .catch((error) => {
          if (error !== rollbackError) throw error;
        });
    } finally {
      await owner.end();
    }
  });

  it("changes no row when a merge's survivor is absent", async (context) => {
    const { ownerDsn } = testDsns();
    if (!ownerDsn) {
      context.skip();
      return;
    }
    const owner = postgres(ownerDsn, { onnotice: () => {} });
    const m = newMarker("no-survivor");
    const rollbackError = new Error("rollback");

    try {
      await owner
        .begin("isolation level repeatable read", async (tx) => {
          const role = await insertTerm(tx, "canonical_roles", `${m}-merged`);
          const classification = await insertClassification(tx, m);
          await insertLink(tx, "canonical_roles", classification, role);
          await pin(tx, role, "Merged");

          const before = await fingerprint(tx);
          await merge(tx, "canonical_roles", [{ slug: `${m}-merged`, into: `${m}-absent` }]);
          expect(await fingerprint(tx)).toEqual(before);
          throw rollbackError;
        })
        .catch((error) => {
          if (error !== rollbackError) throw error;
        });
    } finally {
      await owner.end();
    }
  });

  it("makes mcp.save_enrichment refuse to mint a slug a merge or a retire removed", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("remint");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "canonical_roles", `${m}-old`);
        await insertTerm(tx, "canonical_roles", `${m}-new`);
        await insertTerm(tx, "skills", `${m}-umbrella`);
        const classification = await insertClassification(tx, m);
        const [{ job_posting_id: postingId }] = await tx<{ job_posting_id: string }[]>`
          SELECT job_posting_id FROM classifications WHERE id = ${classification}
        `;

        await merge(tx, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }]);
        await retire(tx, "skills", [{ slug: `${m}-umbrella`, reason: "Umbrella skill." }]);

        const payload = {
          posting_id: Number(postingId),
          classification: { seniority: "unknown", notes: null },
          canonical_roles: [{ slug: `${m}-old`, name: "Old Role", dimensions: ["engineering"] }],
          specializations: [],
          skills: [{ slug: `${m}-umbrella`, name: "Umbrella" }],
        };
        const [{ result }] = await tx<{ result: { ok: boolean; errors: { path: string; code: string }[] } }[]>`
          SELECT mcp.save_enrichment(${tx.json(payload)}, 'fixture-model', 'fixture') AS result
        `;
        expect(result.ok).toBe(false);
        expect(result.errors).toEqual(
          expect.arrayContaining([
            expect.objectContaining({ path: "canonical_roles[0].slug", code: "retired_slug" }),
            expect.objectContaining({ path: "skills[0].slug", code: "retired_slug" }),
          ]),
        );
      });
    } finally {
      await owner.end();
    }
  });
});

describe("foreign-key census", () => {
  // One fixture builder per known reference. A foreign key into the taxonomy
  // that this table does not list fails the census below, and the repair
  // functions refuse to run until they handle it.
  const builders: Record<
    string,
    { table: "canonical_roles" | "specializations" | "skills"; build: (tx: ISql, termId: string, m: string) => Promise<void> }
  > = {
    "public.job_posting_roles.role_id": {
      table: "canonical_roles",
      build: async (tx, id, m) => insertLink(tx, "canonical_roles", await insertClassification(tx, m), id),
    },
    "public.canonical_role_dimensions.canonical_role_id": {
      table: "canonical_roles",
      build: (tx, id) => insertDimension(tx, id, "design"),
    },
    "public.job_posting_specializations.specialization_id": {
      table: "specializations",
      build: async (tx, id, m) => insertLink(tx, "specializations", await insertClassification(tx, m), id),
    },
    "public.job_posting_skills.skill_id": {
      table: "skills",
      build: async (tx, id, m) => insertLink(tx, "skills", await insertClassification(tx, m), id),
    },
    "app.past_titles.role_id": {
      table: "canonical_roles",
      build: async (tx, id, m) => void (await pastTitle(tx, `${m} title`, id)),
    },
    "app.pins.role_id": {
      table: "canonical_roles",
      build: async (tx, id, m) => void (await pin(tx, id, `${m} pin`)),
    },
    "app.claimed_skills.skill_id": {
      table: "skills",
      build: async (tx, id, m) => void (await claim(tx, `${m} claim ${id}`, id)),
    },
  };

  it("lists every reference into the taxonomy, and a merge and a retire over each leave nothing pointing at a deleted term", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;

    try {
      const references = await owner<{ ref: string; referenced: string }[]>`
        SELECT format('%s.%s.%s', n.nspname, cl.relname, a.attname) AS ref, rc.relname AS referenced
        FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        JOIN pg_class rc ON rc.oid = c.confrelid
        JOIN pg_namespace rn ON rn.oid = rc.relnamespace
        JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
        WHERE c.contype = 'f' AND rn.nspname = 'public'
          AND rc.relname IN ('canonical_roles', 'specializations', 'skills')
        ORDER BY 1
      `;
      expect(references.map((r) => r.ref).sort()).toEqual(Object.keys(builders).sort());
      expect(await owner`SELECT * FROM public.taxonomy_repair_unhandled_references()`).toEqual([]);

      for (const { ref, referenced } of references) {
        const builder = builders[ref];
        expect(builder.table).toBe(referenced);
        const m = newMarker("census");

        await inRolledBackTransaction(owner, async (tx) => {
          const merged = await insertTerm(tx, builder.table, `${m}-merged`);
          const survivor = await insertTerm(tx, builder.table, `${m}-survivor`);
          const retired = await insertTerm(tx, builder.table, `${m}-retired`);
          await builder.build(tx, merged, `${m}-a`);
          await builder.build(tx, retired, `${m}-b`);

          await merge(tx, builder.table, [{ slug: `${m}-merged`, into: `${m}-survivor` }]);
          await retire(tx, builder.table, [{ slug: `${m}-retired`, reason: "census" }]);

          const [schema, table, column] = ref.split(".");
          const [{ count }] = await tx<{ count: number }[]>`
            SELECT count(*)::int AS count FROM ${tx(`${schema}.${table}`)}
            WHERE ${tx(column)} IN (${merged}, ${retired})
          `;
          expect(count, ref).toBe(0);
          expect(await termId(tx, builder.table, `${m}-survivor`)).toBe(survivor);
        });
      }
    } finally {
      await owner.end();
    }
  });
});

describe("a save racing a merge", () => {
  it("ends with the save's link on the survivor when the save commits while the merge waits", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const saver = postgres(testDsns().ownerDsn!, { max: 1, onnotice: () => {} });
    const m = newMarker("race");
    let repairId: string | undefined;

    try {
      const merged = await insertTerm(owner, "canonical_roles", `${m}-old`, "Race Old");
      const survivor = await insertTerm(owner, "canonical_roles", `${m}-new`, "Race New");
      const classification = await insertClassification(owner, m);
      const [{ job_posting_id: postingId }] = await owner<{ job_posting_id: string }[]>`
        SELECT job_posting_id FROM classifications WHERE id = ${classification}
      `;

      const payload = {
        posting_id: Number(postingId),
        classification: { seniority: "unknown", notes: null },
        canonical_roles: [{ slug: `${m}-old`, name: "Race Old", dimensions: ["engineering"] }],
        specializations: [],
        skills: [],
      };

      // The save holds the enrichment lock, uncommitted, until released below.
      let releaseSave!: () => void;
      const saveHeld = new Promise<void>((resolve) => (releaseSave = resolve));
      let saved!: (result: unknown) => void;
      const saveRan = new Promise<unknown>((resolve) => (saved = resolve));
      const save = saver.begin(async (tx) => {
        const [{ result }] = await tx<{ result: unknown }[]>`
          SELECT mcp.save_enrichment(${tx.json(payload)}, 'fixture-model', 'fixture') AS result
        `;
        saved(result);
        await saveHeld;
      });

      try {
        expect(await saveRan).toMatchObject({ ok: true });
        const mergeDone = merge(owner, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }], m);
        // Give the merge time to block on the lock before the save commits.
        await new Promise((resolve) => setTimeout(resolve, 300));
        const [{ waiting }] = await owner<{ waiting: number }[]>`
          SELECT count(*)::int AS waiting FROM pg_locks
          WHERE locktype = 'advisory' AND classid = 734771 AND objid = 26 AND NOT granted
        `;
        expect(waiting).toBe(1);
        releaseSave();
        await save;
        repairId = await mergeDone;
      } finally {
        releaseSave();
        await save.catch(() => {});
      }

      const links = await owner<{ role_id: string }[]>`
        SELECT jpr.role_id FROM job_posting_roles jpr
        JOIN classifications c ON c.id = jpr.classification_id
        WHERE c.job_posting_id = ${postingId}
      `;
      expect(links.map((l) => l.role_id)).toEqual([survivor]);
      expect(await termId(owner, "canonical_roles", `${m}-old`)).toBeNull();
      void merged;
    } finally {
      await cleanupCommitted(owner, m, repairId);
      await saver.end();
      await owner.end();
    }
  }, 20_000);
});

// The race commits, so its fixtures need explicit teardown.
async function cleanupCommitted(owner: ISql, m: string, repairId: string | undefined) {
  if (repairId) {
    await owner`DELETE FROM retired_slugs WHERE retired_by_repair = ${repairId}`;
    await owner`DELETE FROM taxonomy_repair_links WHERE repair_id = ${repairId}`;
    await owner`DELETE FROM taxonomy_repair_role_dimensions WHERE repair_id = ${repairId}`;
    await owner`DELETE FROM taxonomy_repair_terms WHERE repair_id = ${repairId}`;
    await owner`DELETE FROM taxonomy_repairs WHERE id = ${repairId}`;
  }
  const like = `${m}%`;
  await owner`
    DELETE FROM classifications WHERE job_posting_id IN (
      SELECT p.id FROM job_postings p JOIN companies c ON c.id = p.company_id
      WHERE c.board_token LIKE ${like})
  `;
  await owner`
    DELETE FROM job_postings WHERE company_id IN (SELECT id FROM companies WHERE board_token LIKE ${like})
  `;
  await owner`DELETE FROM companies WHERE board_token LIKE ${like}`;
  await owner`DELETE FROM canonical_roles WHERE slug LIKE ${like}`;
}
