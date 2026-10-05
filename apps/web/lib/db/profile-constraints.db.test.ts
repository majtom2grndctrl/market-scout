import postgres, { type ISql } from "postgres";
import { describe, expect, it } from "vitest";

import {
  inRolledBackTransaction,
  insertClaim,
  insertPastTitle,
  insertPin,
  insertTerm,
  newMarker,
} from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// What the profile tables themselves refuse, independent of any write path.
// Fixtures are owner-built and rolled back.

function ownerOrSkip(context: { skip: () => void }) {
  const { ownerDsn } = testDsns();
  if (!ownerDsn) {
    context.skip();
    return undefined;
  }
  return postgres(ownerDsn);
}

// Runs one statement that must fail, inside a savepoint so the surrounding
// fixture transaction survives the error.
async function expectRefused(tx: ISql, statement: () => Promise<unknown>, pattern: RegExp) {
  await tx`SAVEPOINT refused`;
  await expect(statement()).rejects.toThrow(pattern);
  await tx`ROLLBACK TO SAVEPOINT refused`;
}

describe("profile constraints", () => {
  it("refuses to delete a canonical role a pin or past title references, or a skill a claimed skill references", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("restrict");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const pinned = await insertTerm(tx, "canonical_roles", `${m}-pinned`);
        const titled = await insertTerm(tx, "canonical_roles", `${m}-titled`);
        const claimed = await insertTerm(tx, "skills", `${m}-claimed`);
        await insertPin(tx, pinned, "Pinned");
        await insertPastTitle(tx, `${m} title`, titled);
        await insertClaim(tx, `${m} claim`, claimed);

        await expectRefused(
          tx,
          () => tx`DELETE FROM canonical_roles WHERE id = ${pinned}`,
          /violates foreign key constraint "pins_role_id_fkey"/,
        );
        // RESTRICT, not the default NO ACTION, which a deferred check could let through.
        const actions = await tx<{ conname: string; confdeltype: string }[]>`
          SELECT conname, confdeltype FROM pg_constraint
          WHERE conrelid IN ('app.pins'::regclass, 'app.past_titles'::regclass, 'app.claimed_skills'::regclass)
            AND contype = 'f'
          ORDER BY conname
        `;
        expect(actions.every((a) => a.confdeltype === "r")).toBe(true);
        expect(actions.length).toBe(4);
        await expectRefused(
          tx,
          () => tx`DELETE FROM canonical_roles WHERE id = ${titled}`,
          /violates foreign key constraint "past_titles_role_id_fkey"/,
        );
        await expectRefused(
          tx,
          () => tx`DELETE FROM skills WHERE id = ${claimed}`,
          /violates foreign key constraint "claimed_skills_skill_id_fkey"/,
        );
      });
    } finally {
      await owner.end();
    }
  });

  it("refuses a second pin of the same role and accepts a pin of a different role", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("pin-unique");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const role = await insertTerm(tx, "canonical_roles", `${m}-a`);
        const other = await insertTerm(tx, "canonical_roles", `${m}-b`);
        await insertPin(tx, role, "A");

        await expectRefused(tx, () => insertPin(tx, role, "A again"), /pins_role_unique/);
        await expect(insertPin(tx, other, "B")).resolves.toBeDefined();

        // Retire leaves pins with no role, so several such pins must coexist.
        await tx`INSERT INTO app.pins (role_id, pinned_name) VALUES (NULL, ${`${m} retired one`})`;
        await expect(
          tx`INSERT INTO app.pins (role_id, pinned_name) VALUES (NULL, ${`${m} retired two`})`,
        ).resolves.toBeDefined();
      });
    } finally {
      await owner.end();
    }
  });

  it("accepts an unmatched claimed skill, and refuses empty text, duplicate text after trimming and case-folding, and a skill already claimed under other text", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("claim");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        const skill = await insertTerm(tx, "skills", `${m}-figma`);
        await expect(insertClaim(tx, `${m} Unmatched Skill`, null)).resolves.toBeDefined();
        await insertClaim(tx, `${m} Figma`, skill);

        await expectRefused(tx, () => insertClaim(tx, "", null), /claimed_skills_skill_text_present/);
        await expectRefused(tx, () => insertClaim(tx, "   ", null), /claimed_skills_skill_text_present/);
        await expectRefused(
          tx,
          () => insertClaim(tx, `  ${m} UNMATCHED skill  `, null),
          /claimed_skills_text_unique/,
        );
        await expectRefused(
          tx,
          () => insertClaim(tx, `${m} Figma (design tool)`, skill),
          /claimed_skills_skill_unique/,
        );
      });
    } finally {
      await owner.end();
    }
  });

  it("accepts a past title with no seniority, and refuses unstated, a seniority outside the title vocabulary, and empty text", async (context) => {
    const owner = ownerOrSkip(context);
    if (!owner) return;
    const m = newMarker("title");

    try {
      await inRolledBackTransaction(owner, async (tx) => {
        await expect(insertPastTitle(tx, `${m} Designer`, null)).resolves.toBeDefined();
        await expect(insertPastTitle(tx, `${m} Senior Designer`, null, "senior")).resolves.toBeDefined();

        await expectRefused(
          tx,
          () => insertPastTitle(tx, `${m} Designer`, null, "unstated"),
          /past_titles_seniority_stated/,
        );
        await expectRefused(
          tx,
          () => insertPastTitle(tx, `${m} Designer`, null, "grandmaster"),
          /past_titles_seniority_fkey/,
        );
        await expectRefused(tx, () => insertPastTitle(tx, "  ", null), /past_titles_title_text_present/);
      });
    } finally {
      await owner.end();
    }
  });
});
