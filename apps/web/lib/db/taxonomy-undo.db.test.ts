import postgres from "postgres";
import { describe, expect, it } from "vitest";

import {
  inRolledBackTransaction,
  insertClassification,
  insertDimension,
  insertLink,
  insertPastTitle,
  insertPin,
  insertTerm,
  linkSlugs,
  merge,
  newMarker,
  postingSide,
  retire,
  termId,
  undo,
} from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// Undo restores the posting side -- term rows, links, role dimensions, and
// retired-slug records -- from the repair's own archive, and leaves profile
// rows where the repair put them. Snapshots compare by slug, so a restored
// term reads equal only if it came back under its original id as well.

function ownerOrSkip(context: { skip: () => void }, notices?: string[]) {
  const { ownerDsn } = testDsns();
  if (!ownerDsn) {
    context.skip();
    return undefined;
  }
  return postgres(ownerDsn, { onnotice: (notice) => notices?.push(notice.message) });
}

describe("taxonomy_undo", () => {
  it("restores a merged role's posting side exactly, including state the survivor already held, and leaves the past title and pin on the survivor", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-merge");
    const slugs = [`${m}-old`, `${m}-new`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const old = await insertTerm(tx, "canonical_roles", `${m}-old`, "Old Role");
        const survivor = await insertTerm(tx, "canonical_roles", `${m}-new`, "New Role");
        const both = await insertClassification(tx, m);
        const oldOnly = await insertClassification(tx, m);
        await insertLink(tx, "canonical_roles", both, old);
        await insertLink(tx, "canonical_roles", both, survivor);
        await insertLink(tx, "canonical_roles", oldOnly, old);
        await insertDimension(tx, old, "design");
        await insertDimension(tx, survivor, "design");
        await insertDimension(tx, old, "research");
        await tx`
          INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason)
          VALUES (${`${m}-old`}, 'canonical_roles', 'vitest-older-record', 'An older retirement.')
        `;
        const title = await insertPastTitle(tx, `${m} Old Role`, old);
        const pin = await insertPin(tx, old, "Old Role");

        const before = await postingSide(tx, "canonical_roles", slugs);
        const repair = await merge(tx, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }]);
        expect(await postingSide(tx, "canonical_roles", slugs)).not.toEqual(before);
        await undo(tx, repair);

        expect(await postingSide(tx, "canonical_roles", slugs)).toEqual(before);
        const [titleRow] = await tx`SELECT role_id FROM app.past_titles WHERE id = ${title}`;
        expect(titleRow.role_id).toBe(survivor);
        const [pinRow] = await tx`SELECT role_id, pinned_name FROM app.pins WHERE id = ${pin}`;
        expect(pinRow).toEqual({ role_id: survivor, pinned_name: "Old Role" });
      });
    } finally {
      await owner.end();
    }
  });

  it("restores a retired role, its posting links, and its role dimensions, and leaves the pin retired and the past title unmatched", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-retire");
    const slugs = [`${m}-role`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const role = await insertTerm(tx, "canonical_roles", `${m}-role`, "Retired Role");
        await insertLink(tx, "canonical_roles", await insertClassification(tx, m), role);
        await insertDimension(tx, role, "design");
        const title = await insertPastTitle(tx, `${m} Retired Role`, role);
        const pin = await insertPin(tx, role, "Retired Role");

        const before = await postingSide(tx, "canonical_roles", slugs);
        expect(before.links).toHaveLength(1);
        expect(before.dimensions).toEqual([`${m}-role:design`]);

        const repair = await retire(tx, "canonical_roles", [{ slug: `${m}-role`, reason: "Umbrella." }]);
        await undo(tx, repair);

        expect(await postingSide(tx, "canonical_roles", slugs)).toEqual(before);
        const [pinRow] = await tx`SELECT role_id, pinned_name FROM app.pins WHERE id = ${pin}`;
        expect(pinRow).toEqual({ role_id: null, pinned_name: "Retired Role" });
        const [titleRow] = await tx`SELECT role_id, title_text FROM app.past_titles WHERE id = ${title}`;
        expect(titleRow).toEqual({ role_id: null, title_text: `${m} Retired Role` });
      });
    } finally {
      await owner.end();
    }
  });

  it("restores the posting side after each undo of a merge, undo, merge, undo cycle", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-cycle");
    const slugs = [`${m}-old`, `${m}-new`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const old = await insertTerm(tx, "skills", `${m}-old`);
        const survivor = await insertTerm(tx, "skills", `${m}-new`);
        const shared = await insertClassification(tx, m);
        await insertLink(tx, "skills", shared, old);
        await insertLink(tx, "skills", shared, survivor);
        await insertLink(tx, "skills", await insertClassification(tx, m), old);

        const before = await postingSide(tx, "skills", slugs);
        const first = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        await undo(tx, first);
        expect(await postingSide(tx, "skills", slugs)).toEqual(before);

        const second = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        await undo(tx, second);
        expect(await postingSide(tx, "skills", slugs)).toEqual(before);
      });
    } finally {
      await owner.end();
    }
  });

  it("undoes a chain in reverse order back to the first snapshot, and refuses to undo the first merge while the second stands", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-chain");
    const slugs = [`${m}-a`, `${m}-b`, `${m}-c`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const a = await insertTerm(tx, "canonical_roles", `${m}-a`);
        const b = await insertTerm(tx, "canonical_roles", `${m}-b`);
        const c = await insertTerm(tx, "canonical_roles", `${m}-c`);
        await insertLink(tx, "canonical_roles", await insertClassification(tx, m), a);
        await insertLink(tx, "canonical_roles", await insertClassification(tx, m), b);
        const shared = await insertClassification(tx, m);
        await insertLink(tx, "canonical_roles", shared, a);
        await insertLink(tx, "canonical_roles", shared, c);
        await insertDimension(tx, a, "design");
        await insertDimension(tx, c, "research");

        const before = await postingSide(tx, "canonical_roles", slugs);
        const first = await merge(tx, "canonical_roles", [{ slug: `${m}-a`, into: `${m}-b` }]);
        const second = await merge(tx, "canonical_roles", [{ slug: `${m}-b`, into: `${m}-c` }]);

        await tx`SAVEPOINT out_of_order`;
        await expect(undo(tx, first)).rejects.toThrow(
          new RegExp(`later repairs ${second} \\(vitest\\) touch the same terms`),
        );
        await tx`ROLLBACK TO SAVEPOINT out_of_order`;

        await undo(tx, second);
        await undo(tx, first);
        expect(await postingSide(tx, "canonical_roles", slugs)).toEqual(before);
      });
    } finally {
      await owner.end();
    }
  });

  it("leaves a posting linked to the survivor after the merge linked to it after the undo", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-later-link");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const old = await insertTerm(tx, "skills", `${m}-old`);
        await insertTerm(tx, "skills", `${m}-new`);
        await insertLink(tx, "skills", await insertClassification(tx, m), old);

        const repair = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        const later = await insertClassification(tx, m);
        await insertLink(tx, "skills", later, (await termId(tx, "skills", `${m}-new`))!);
        await undo(tx, repair);

        expect(await linkSlugs(tx, "skills", [`${m}-new`])).toEqual([`${later}:${m}-new`]);
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses when the slug was minted again since the repair, and changes nothing on a second undo", async (context) => {
    const notices: string[] = [];
    const owner = ownerOrSkip(context, notices);
    if (!owner) return;
    const m = newMarker("undo-refuse");
    const slugs = [`${m}-old`, `${m}-new`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "skills", `${m}-old`);
        await insertTerm(tx, "skills", `${m}-new`);

        const reminted = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        await tx`SAVEPOINT reminted`;
        await insertTerm(tx, "skills", `${m}-old`, "Minted again");
        await expect(undo(tx, reminted)).rejects.toThrow(/slugs minted again since repair/);
        await tx`ROLLBACK TO SAVEPOINT reminted`;

        await undo(tx, reminted);
        const afterFirst = await postingSide(tx, "skills", slugs);
        await undo(tx, reminted);
        expect(await postingSide(tx, "skills", slugs)).toEqual(afterFirst);
        expect(notices).toEqual(
          expect.arrayContaining([expect.stringMatching(new RegExp(`repair ${reminted} was already undone`))]),
        );
      });
    } finally {
      await owner.end();
    }
  });
  it("restores the links whose classification survives when another was deleted since the repair", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-deleted");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const old = await insertTerm(tx, "skills", `${m}-old`);
        await insertTerm(tx, "skills", `${m}-new`);
        const kept = await insertClassification(tx, m);
        const deleted = await insertClassification(tx, m);
        await insertLink(tx, "skills", kept, old);
        await insertLink(tx, "skills", deleted, old);

        const repair = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        // Postings cascade to their classifications and links.
        await tx`
          DELETE FROM job_postings WHERE id = (SELECT job_posting_id FROM classifications WHERE id = ${deleted})
        `;
        await undo(tx, repair);

        expect(await linkSlugs(tx, "skills", [`${m}-old`, `${m}-new`])).toEqual([`${kept}:${m}-old`]);
      });
    } finally {
      await owner.end();
    }
  });

  it("undoes a repair while a later repair on unrelated terms stands", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-unrelated");
    const slugs = [`${m}-a`, `${m}-b`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "skills", `${m}-a`);
        await insertTerm(tx, "skills", `${m}-b`);
        await insertTerm(tx, "skills", `${m}-x`);
        await insertTerm(tx, "skills", `${m}-y`);

        const before = await postingSide(tx, "skills", slugs);
        const first = await merge(tx, "skills", [{ slug: `${m}-a`, into: `${m}-b` }]);
        await merge(tx, "skills", [{ slug: `${m}-x`, into: `${m}-y` }]);
        await undo(tx, first);

        expect(await postingSide(tx, "skills", slugs)).toEqual(before);
        expect(await termId(tx, "skills", `${m}-x`)).toBeNull();
      });
    } finally {
      await owner.end();
    }
  });

  it("undoes a specialization retire that also named an absent term, removing both retired-slug records", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-spec");
    const slugs = [`${m}-domain`, `${m}-absent`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const spec = await insertTerm(tx, "specializations", `${m}-domain`);
        await insertLink(tx, "specializations", await insertClassification(tx, m), spec);

        const before = await postingSide(tx, "specializations", slugs);
        const repair = await retire(tx, "specializations", [
          { slug: `${m}-domain`, reason: "Names no domain" },
          { slug: `${m}-absent`, reason: "Names no domain" },
        ]);
        await undo(tx, repair);

        expect(await postingSide(tx, "specializations", slugs)).toEqual(before);
        expect(before.retired).toEqual([]);
      });
    } finally {
      await owner.end();
    }
  });

  it("undoes every standing repair a label made, newest first", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-label");
    const slugs = [`${m}-a`, `${m}-b`, `${m}-c`];

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const a = await insertTerm(tx, "canonical_roles", `${m}-a`);
        await insertTerm(tx, "canonical_roles", `${m}-b`);
        await insertTerm(tx, "canonical_roles", `${m}-c`);
        await insertLink(tx, "canonical_roles", await insertClassification(tx, m), a);

        const before = await postingSide(tx, "canonical_roles", slugs);
        // A chain under one label: undoing oldest first would be refused.
        await merge(tx, "canonical_roles", [{ slug: `${m}-a`, into: `${m}-b` }], m);
        await merge(tx, "canonical_roles", [{ slug: `${m}-b`, into: `${m}-c` }], m);
        await tx`SELECT public.taxonomy_undo_label(${m})`;

        expect(await postingSide(tx, "canonical_roles", slugs)).toEqual(before);
        const [{ standing }] = await tx<{ standing: number }[]>`
          SELECT count(*)::int AS standing FROM taxonomy_repairs WHERE retired_by = ${m} AND undone_at IS NULL
        `;
        expect(standing).toBe(0);
      });
    } finally {
      await owner.end();
    }
  });

  it("undoes a repair of one side of a legacy cross-table slug, and refuses when the slug was minted in another table since", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-cross");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        // Both sides existed before the repair: a collision the repair did not cause.
        await tx`
          INSERT INTO specializations (slug, name, created_at)
          VALUES (${`${m}-shared`}, 'Shared (specialization)', now() - interval '1 day')
        `;
        await tx`
          INSERT INTO skills (slug, name, created_at)
          VALUES (${`${m}-shared`}, 'Shared (skill)', now() - interval '1 day')
        `;
        const legacy = await retire(tx, "skills", [{ slug: `${m}-shared`, reason: "Cleanup" }]);
        await undo(tx, legacy);
        expect(await termId(tx, "skills", `${m}-shared`)).not.toBeNull();

        // Minted elsewhere after the repair: restoring would put one slug in two tables.
        await insertTerm(tx, "skills", `${m}-old`);
        await insertTerm(tx, "skills", `${m}-new`);
        const repair = await merge(tx, "skills", [{ slug: `${m}-old`, into: `${m}-new` }]);
        await insertTerm(tx, "specializations", `${m}-old`);
        await tx`SAVEPOINT reminted`;
        await expect(undo(tx, repair)).rejects.toThrow(/slugs minted again since repair/);
        await tx`ROLLBACK TO SAVEPOINT reminted`;
      });
    } finally {
      await owner.end();
    }
  });

  it("skips a role dimension removed between the repair and the undo", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-dim");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const old = await insertTerm(tx, "canonical_roles", `${m}-old`);
        const survivor = await insertTerm(tx, "canonical_roles", `${m}-new`);
        const [dimension] = await tx<{ id: string }[]>`
          INSERT INTO role_dimensions (slug, name) VALUES (${`${m}-dim`}, 'Fixture dimension') RETURNING id
        `;
        await tx`INSERT INTO canonical_role_dimensions VALUES (${old}, ${dimension.id})`;

        const repair = await merge(tx, "canonical_roles", [{ slug: `${m}-old`, into: `${m}-new` }]);
        // A later migration retires the dimension.
        await tx`DELETE FROM canonical_role_dimensions WHERE canonical_role_id = ${survivor}`;
        await tx`DELETE FROM role_dimensions WHERE id = ${dimension.id}`;
        await undo(tx, repair);

        expect(await termId(tx, "canonical_roles", `${m}-old`)).toBe(old);
        const [{ count }] = await tx<{ count: number }[]>`
          SELECT count(*)::int AS count FROM canonical_role_dimensions WHERE canonical_role_id = ${old}
        `;
        expect(count).toBe(0);
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses a blank or unknown label, notices a label already undone, and rolls back a label undo refused partway", async (context) => {
    const notices: string[] = [];
    const owner = ownerOrSkip(context, notices);
    if (!owner) return;
    const m = newMarker("undo-label-edge");
    const other = `${m}-later`;

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const refused = async (label: string, pattern: RegExp) => {
          await tx`SAVEPOINT refused`;
          await expect(tx`SELECT public.taxonomy_undo_label(${label})`).rejects.toThrow(pattern);
          await tx`ROLLBACK TO SAVEPOINT refused`;
        };
        await refused("  ", /a label is required/);
        await refused(`${m}-typo`, /no repair was ever labelled/);

        for (const slug of ["a", "b", "c", "x", "y"]) {
          await insertTerm(tx, "skills", `${m}-${slug}`);
        }
        const first = await merge(tx, "skills", [{ slug: `${m}-a`, into: `${m}-b` }], m);
        const unrelated = await merge(tx, "skills", [{ slug: `${m}-x`, into: `${m}-y` }], m);
        // Another migration's repair builds on the first one.
        const later = await merge(tx, "skills", [{ slug: `${m}-b`, into: `${m}-c` }], other);

        // Newest first: undoing `unrelated` succeeds, then `first` is refused,
        // naming the later repair and its label.
        await tx`SAVEPOINT partway`;
        let refusal = "";
        try {
          await tx`SELECT public.taxonomy_undo_label(${m})`;
        } catch (error) {
          refusal = String(error);
        }
        await tx`ROLLBACK TO SAVEPOINT partway`;
        expect(refusal).toContain(`later repairs ${later} (${other}) touch the same terms`);
        // The refused call took its earlier undo of `unrelated` with it.
        const standing = await tx<{ id: string }[]>`
          SELECT id FROM taxonomy_repairs WHERE retired_by = ${m} AND undone_at IS NULL ORDER BY id
        `;
        expect(standing.map((r) => r.id)).toEqual([first, unrelated]);

        await tx`SELECT public.taxonomy_undo_label(${other})`;
        await tx`SELECT public.taxonomy_undo_label(${m})`;
        await tx`SELECT public.taxonomy_undo_label(${m})`;
        expect(notices).toEqual(
          expect.arrayContaining([expect.stringMatching(new RegExp(`every repair labelled ${m} is already undone`))]),
        );
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses an undo that would put a slug in a second table it did not share at repair time, even when restored by another undo", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("undo-order");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await insertTerm(tx, "specializations", `${m}-foo`);
        const r0 = await retire(tx, "specializations", [{ slug: `${m}-foo`, reason: "First pass" }]);
        // Minted in skills while specializations no longer held it.
        await insertTerm(tx, "skills", `${m}-foo`);
        const r1 = await retire(tx, "skills", [{ slug: `${m}-foo`, reason: "Second pass" }]);

        // Different tables, so neither repair blocks the other's undo by term.
        await undo(tx, r0);
        await tx`SAVEPOINT second`;
        await expect(undo(tx, r1)).rejects.toThrow(/slugs minted again since repair/);
        await tx`ROLLBACK TO SAVEPOINT second`;
        expect(await termId(tx, "skills", `${m}-foo`)).toBeNull();
      });
    } finally {
      await owner.end();
    }
  });
});
