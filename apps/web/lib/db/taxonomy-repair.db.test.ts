import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import {
  deleteRepairsLabelled,
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
// owner DSN. Each case builds inside a transaction it rolls back -- repeatable
// read where it compares whole tables -- except the races, which need two
// connections to see each other's commits and tear down by marker.

function ownerOrSkip(context: { skip: () => void }, notices?: string[]) {
  const { ownerDsn } = testDsns();
  if (!ownerDsn) {
    context.skip();
    return undefined;
  }
  return postgres(ownerDsn, { onnotice: (notice) => notices?.push(notice.message) });
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

        expect(await termId(tx, "skills", `${m}-skill`)).toBeNull();
        expect(await retiredSlugRecords(tx, [`${m}-skill`])).toEqual([`${m}-skill|skills|vitest`]);
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
    const notices: string[] = [];
    const owner = ownerOrSkip(context, notices);
    if (!owner) return;
    const m = newMarker("absent");

    try {
      await inRolledBackTransaction(
        owner,
        async (tx) => {
          // The fork case: the merge's survivor is present, the merged term is not.
          const survivor = await insertTerm(tx, "canonical_roles", `${m}-survivor`);
          await insertLink(tx, "canonical_roles", await insertClassification(tx, m), survivor);
          await pin(tx, survivor, `${m} Survivor`);
          const before = await fingerprint(tx);

          await retire(tx, "skills", [{ slug: `${m}-retired`, reason: "Absent here" }]);
          await merge(tx, "canonical_roles", [{ slug: `${m}-merged`, into: `${m}-survivor` }]);
          // Neither present: the merged slug is still recorded.
          await merge(tx, "skills", [{ slug: `${m}-gone`, into: `${m}-also-gone` }]);

          const after = await fingerprint(tx);
          const { retired_slugs: retiredBefore, ...restBefore } = before;
          const { retired_slugs: retiredAfter, ...restAfter } = after;
          expect(restAfter).toEqual(restBefore);
          expect(retiredAfter).not.toEqual(retiredBefore);
          expect(
            await retiredSlugRecords(tx, [`${m}-retired`, `${m}-merged`, `${m}-gone`]),
          ).toEqual([
            `${m}-gone|skills|vitest`,
            `${m}-merged|canonical_roles|vitest`,
            `${m}-retired|skills|vitest`,
          ]);
          // Said aloud, so a mistyped slug does not pass silently.
          expect(notices).toEqual(
            expect.arrayContaining([
              expect.stringMatching(new RegExp(`${m}-retired is not on this install`)),
              expect.stringMatching(new RegExp(`${m}-merged is not on this install`)),
            ]),
          );
        },
        "repeatable read",
      );
    } finally {
      await owner.end();
    }
  });

  it("changes no row when a merge's survivor is absent", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("no-survivor");

    try {
      await inRolledBackTransaction(
        owner,
        async (tx) => {
          const role = await insertTerm(tx, "canonical_roles", `${m}-merged`);
          const classification = await insertClassification(tx, m);
          await insertLink(tx, "canonical_roles", classification, role);
          await pin(tx, role, "Merged");

          const before = await fingerprint(tx);
          await merge(tx, "canonical_roles", [{ slug: `${m}-merged`, into: `${m}-absent` }]);
          expect(await fingerprint(tx)).toEqual(before);
        },
        "repeatable read",
      );
    } finally {
      await owner.end();
    }
  });

  it("retires the present terms and records the absent ones in one call", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("retire-mixed");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "specializations", `${m}-present`);
        const repair = await retire(tx, "specializations", [
          { slug: `${m}-present`, reason: "Names no domain" },
          { slug: `${m}-absent`, reason: "Names no domain" },
        ]);

        expect(await termId(tx, "specializations", `${m}-present`)).toBeNull();
        const outcomes = await tx<{ slug: string; outcome: string }[]>`
          SELECT slug, outcome FROM taxonomy_repair_terms WHERE repair_id = ${repair} ORDER BY slug
        `;
        expect(outcomes).toEqual([
          { slug: `${m}-absent`, outcome: "absent" },
          { slug: `${m}-present`, outcome: "applied" },
        ]);
        expect(await retiredSlugRecords(tx, [`${m}-present`, `${m}-absent`])).toEqual([
          `${m}-absent|specializations|vitest`,
          `${m}-present|specializations|vitest`,
        ]);
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses malformed input before changing anything", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("refuse");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "skills", `${m}-a`);
        await insertTerm(tx, "skills", `${m}-b`);
        const refusals: [string, () => Promise<unknown>, RegExp][] = [
          ["unknown table", () => tx`SELECT public.taxonomy_retire('companies', '[{"slug":"x","reason":"r"}]', 'vitest')`, /unknown table companies/],
          ["blank label", () => merge(tx, "skills", [{ slug: `${m}-a`, into: `${m}-b` }], "  "), /retired_by is required/],
          ["empty map", () => tx`SELECT public.taxonomy_merge('skills', '[]', 'vitest')`, /non-empty JSON array/],
          ["object map", () => tx`SELECT public.taxonomy_merge('skills', '{"slug":"a"}', 'vitest')`, /non-empty JSON array/],
          ["missing into", () => tx`SELECT public.taxonomy_merge('skills', ${tx.json([{ slug: `${m}-a` }])}, 'vitest')`, /need both "slug" and "into"/],
          ["missing reason", () => tx`SELECT public.taxonomy_retire('skills', ${tx.json([{ slug: `${m}-a` }])}, 'vitest')`, /need both "slug" and "reason"/],
          ["duplicate after trim", () => retire(tx, "skills", [{ slug: `${m}-a`, reason: "r" }, { slug: ` ${m}-a `, reason: "r" }]), /named more than once/],
          ["not a slug", () => retire(tx, "skills", [{ slug: `${m}-A`, reason: "r" }]), /not a slug/],
          ["not a slug, survivor", () => merge(tx, "skills", [{ slug: `${m}-a`, into: "Go Lang" }]), /not a slug/],
        ];
        for (const [name, call, pattern] of refusals) {
          await tx`SAVEPOINT refused`;
          await expect(call(), name).rejects.toThrow(pattern);
          await tx`ROLLBACK TO SAVEPOINT refused`;
        }
        expect(await termId(tx, "skills", `${m}-a`)).not.toBeNull();
        const [{ count }] = await tx<{ count: number }[]>`
          SELECT count(*)::int AS count FROM taxonomy_repairs WHERE retired_by = 'vitest' AND performed_at = now()
        `;
        expect(count).toBe(0);
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

describe("writes racing a merge", () => {
  // Both cases commit: two connections must see each other's writes. Fixtures
  // and the repair are torn down by marker and label in finally.

  it("ends with the save's link on the survivor when the save commits while the merge waits", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const saver = postgres(testDsns().ownerDsn!, { max: 1, onnotice: () => {} });
    const m = newMarker("race");

    try {
      await insertTerm(owner, "canonical_roles", `${m}-old`, "Race Old");
      const survivor = await insertTerm(owner, "canonical_roles", `${m}-new`, "Race New");
      const postingId = await postingOf(owner, await insertClassification(owner, m));

      const payload = {
        posting_id: Number(postingId),
        classification: { seniority: "unknown", notes: null },
        canonical_roles: [{ slug: `${m}-old`, name: "Race Old", dimensions: ["engineering"] }],
        specializations: [],
        skills: [],
      };

      // The save holds the enrichment lock, uncommitted, until released below.
      const held = holdOpen(saver, async (tx) => {
        const [{ result }] = await tx<{ result: unknown }[]>`
          SELECT mcp.save_enrichment(${tx.json(payload)}, 'fixture-model', 'fixture') AS result
        `;
        return result;
      });

      try {
        expect(await held.ran).toMatchObject({ ok: true });
        const mergeDone = merge(owner, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }], m);
        await waitForBlocked(owner, "advisory");
        held.release();
        await held.done;
        await mergeDone;
      } finally {
        held.release();
        await held.done.catch(() => {});
      }

      expect(await rolesLinkedTo(owner, postingId)).toEqual([survivor]);
      expect(await termId(owner, "canonical_roles", `${m}-old`)).toBeNull();
    } finally {
      await cleanupCommitted(owner, m);
      await saver.end();
      await owner.end();
    }
  }, 20_000);

  // cmd/batch-enrich writes links directly, without the enrichment lock. The
  // row lock the merge takes on its terms is what holds it off.
  it("ends with a direct writer's link on the survivor when it commits while the merge waits", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const writer = postgres(testDsns().ownerDsn!, { max: 1, onnotice: () => {} });
    const m = newMarker("race-direct");

    try {
      const merged = await insertTerm(owner, "canonical_roles", `${m}-old`);
      const survivor = await insertTerm(owner, "canonical_roles", `${m}-new`);
      const classification = await insertClassification(owner, m);
      const postingId = await postingOf(owner, classification);

      // The insert's foreign-key check holds FOR KEY SHARE on the merged role.
      const held = holdOpen(writer, (tx) => insertLink(tx, "canonical_roles", classification, merged));

      try {
        await held.ran;
        const mergeDone = merge(owner, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }], m);
        await waitForBlocked(owner, "transactionid");
        held.release();
        await held.done;
        await mergeDone;
      } finally {
        held.release();
        await held.done.catch(() => {});
      }

      expect(await rolesLinkedTo(owner, postingId)).toEqual([survivor]);
      expect(await termId(owner, "canonical_roles", `${m}-old`)).toBeNull();
    } finally {
      await cleanupCommitted(owner, m);
      await writer.end();
      await owner.end();
    }
  }, 20_000);
});

// Runs body in a transaction on its own connection and keeps it open, holding
// whatever locks it took, until release() is called.
function holdOpen<T>(sql: postgres.Sql, body: (tx: ISql) => Promise<T>) {
  let release!: () => void;
  const released = new Promise<void>((resolve) => (release = resolve));
  let ran!: (value: T) => void;
  const ranPromise = new Promise<T>((resolve) => (ran = resolve));
  const done = sql.begin(async (tx) => {
    ran(await body(tx));
    await released;
  });
  // If body throws, ran never settles; surface the failure through it too.
  const ranOrFailed = Promise.race([ranPromise, done.then(() => ranPromise)]);
  return { ran: ranOrFailed, release, done };
}

// Polls until exactly one backend waits on a lock of the given type, rather
// than sleeping for a guessed interval.
async function waitForBlocked(owner: ISql, locktype: "advisory" | "transactionid") {
  for (let attempt = 0; attempt < 100; attempt++) {
    const [{ waiting }] = await owner<{ waiting: number }[]>`
      SELECT count(*)::int AS waiting FROM pg_locks
      WHERE locktype = ${locktype} AND NOT granted
        AND (${locktype} <> 'advisory' OR (classid = 734771 AND objid = 26))
    `;
    if (waiting === 1) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error(`no backend blocked on a ${locktype} lock after 5s`);
}

async function postingOf(sql: ISql, classificationId: string): Promise<string> {
  const [{ job_posting_id: postingId }] = await sql<{ job_posting_id: string }[]>`
    SELECT job_posting_id FROM classifications WHERE id = ${classificationId}
  `;
  return postingId;
}

async function rolesLinkedTo(sql: ISql, postingId: string): Promise<string[]> {
  const rows = await sql<{ role_id: string }[]>`
    SELECT DISTINCT jpr.role_id FROM job_posting_roles jpr
    JOIN classifications c ON c.id = jpr.classification_id
    WHERE c.job_posting_id = ${postingId}
  `;
  return rows.map((r) => r.role_id);
}

// Repairs are found by the marker they were labelled with, so a case that
// fails before its merge returns still leaves nothing behind.
async function cleanupCommitted(owner: ISql, m: string) {
  await deleteRepairsLabelled(owner, m);
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
