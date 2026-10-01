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
          new RegExp(`later repairs ${second} touch the same terms`),
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
});
