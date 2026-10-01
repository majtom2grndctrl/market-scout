import postgres from "postgres";
import { describe, expect, it } from "vitest";

import {
  claimSkill,
  pinRole,
  selectProfile,
  unpin,
} from "./profile";
import { selectTaxonomyMatches } from "./taxonomy-search";
import { insertClaim, insertPin, insertTerm, newMarker, retire } from "./testing/taxonomy-fixtures";
import { testDsns } from "./test-dsn";

// The write path through the role that runs it. The app role cannot see an
// owner's uncommitted rows, so fixtures here are committed and torn down by
// marker.

function dsnsOrSkip(context: { skip: () => void }) {
  const { ownerDsn, readOnlyDsn, appDsn } = testDsns();
  if (!ownerDsn || !readOnlyDsn || !appDsn) {
    context.skip();
    return undefined;
  }
  return { ownerDsn, readOnlyDsn, appDsn };
}

async function teardown(owner: postgres.Sql, m: string) {
  const like = `${m}%`;
  await owner`
    DELETE FROM app.pins WHERE role_id IN (SELECT id FROM canonical_roles WHERE slug LIKE ${like})
       OR pinned_name LIKE ${like}
  `;
  await owner`DELETE FROM app.claimed_skills WHERE skill_text LIKE ${like}`;
  await owner`DELETE FROM app.past_titles WHERE title_text LIKE ${like}`;
  const repairs = await owner<{ id: string }[]>`SELECT id FROM taxonomy_repairs WHERE retired_by = ${m}`;
  for (const { id } of repairs) {
    await owner`DELETE FROM retired_slugs WHERE retired_by_repair = ${id}`;
    await owner`DELETE FROM taxonomy_repair_links WHERE repair_id = ${id}`;
    await owner`DELETE FROM taxonomy_repair_role_dimensions WHERE repair_id = ${id}`;
    await owner`DELETE FROM taxonomy_repair_terms WHERE repair_id = ${id}`;
    await owner`DELETE FROM taxonomy_repairs WHERE id = ${id}`;
  }
  await owner`DELETE FROM canonical_roles WHERE slug LIKE ${like}`;
  await owner`DELETE FROM skills WHERE slug LIKE ${like}`;
}

describe("profile reads", () => {
  it("reads the profile through the app role and searches the taxonomy through the read-only role, each returning the fixture's rows", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const app = postgres(dsns.appDsn);
    const readOnly = postgres(dsns.readOnlyDsn);
    const m = newMarker("profile-read");

    try {
      const role = await insertTerm(owner, "canonical_roles", `${m}-product-designer`, `${m} Product Designer`);
      const skill = await insertTerm(owner, "skills", `${m}-figma`, `${m} Figma`);
      await insertPin(owner, role, `${m} Product Designer`);
      await insertClaim(owner, `${m} Figma`, skill);
      await insertClaim(owner, `${m} Unmatched craft`, null);
      await owner`
        INSERT INTO app.past_titles (title_text, role_id, seniority)
        VALUES (${`${m} Senior Product Designer`}, ${role}, 'senior')
      `;

      const profile = await selectProfile(app);
      expect(profile.pins).toContainEqual(
        expect.objectContaining({
          pinnedName: `${m} Product Designer`,
          role: { id: role, slug: `${m}-product-designer`, name: `${m} Product Designer` },
        }),
      );
      expect(profile.claimedSkills).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ skillText: `${m} Figma`, skill: expect.objectContaining({ id: skill }) }),
          expect.objectContaining({ skillText: `${m} Unmatched craft`, skill: null }),
        ]),
      );
      expect(profile.pastTitles).toContainEqual(
        expect.objectContaining({
          titleText: `${m} Senior Product Designer`,
          role: expect.objectContaining({ id: role }),
          seniority: { slug: "senior", name: "Senior" },
        }),
      );
      expect(profile.seniorities.map((s) => s.slug)).not.toContain("unstated");

      const roles = await selectTaxonomyMatches(readOnly, "canonical_roles", `${m} product designer`);
      expect(roles[0]).toEqual(expect.objectContaining({ id: role, slug: `${m}-product-designer` }));
      const skills = await selectTaxonomyMatches(readOnly, "skills", `${m} figma`);
      expect(skills[0]).toEqual(expect.objectContaining({ id: skill }));

      // The read-only role cannot run the profile read at all.
      await expect(selectProfile(readOnly)).rejects.toThrow(/permission denied for schema app/);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end(), readOnly.end()]);
    }
  });
});

describe("profile writes", () => {
  it("reports success and writes nothing when pinning a pinned role, unpinning an unpinned one, or claiming a claimed skill", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const app = postgres(dsns.appDsn);
    const m = newMarker("profile-idem");

    try {
      const role = await insertTerm(owner, "canonical_roles", `${m}-role`);
      const skill = await insertTerm(owner, "skills", `${m}-skill`);

      expect(await pinRole(app, role)).toEqual({ ok: true });
      expect(await pinRole(app, role)).toEqual({ ok: true });
      const pins = await owner<{ id: string }[]>`SELECT id FROM app.pins WHERE role_id = ${role}`;
      expect(pins).toHaveLength(1);

      expect(await unpin(app, pins[0].id)).toEqual({ ok: true });
      expect(await unpin(app, pins[0].id)).toEqual({ ok: true });
      const [{ count: pinCount }] = await owner<{ count: number }[]>`
        SELECT count(*)::int AS count FROM app.pins WHERE role_id = ${role}
      `;
      expect(pinCount).toBe(0);

      expect(await claimSkill(app, { skillText: `${m} Skill`, skillId: skill })).toEqual({ ok: true });
      expect(await claimSkill(app, { skillText: `${m} Skill, again`, skillId: skill })).toEqual({ ok: true });
      expect(await claimSkill(app, { skillText: ` ${m} SKILL `, skillId: null })).toEqual({ ok: true });
      const claims = await owner<{ skill_text: string }[]>`
        SELECT skill_text FROM app.claimed_skills WHERE skill_text ILIKE ${`%${m}%`}
      `;
      expect(claims).toEqual([{ skill_text: `${m} Skill` }]);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end()]);
    }
  });

  it("leaves one pin when two pin writes for the same role are issued at the same moment, and both report success", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    // Two separate pools, so the writes run on two connections.
    const first = postgres(dsns.appDsn, { max: 1 });
    const second = postgres(dsns.appDsn, { max: 1 });
    const m = newMarker("profile-race");

    try {
      const role = await insertTerm(owner, "canonical_roles", `${m}-role`);
      const results = await Promise.all([pinRole(first, role), pinRole(second, role)]);
      expect(results).toEqual([{ ok: true }, { ok: true }]);
      const [{ count }] = await owner<{ count: number }[]>`
        SELECT count(*)::int AS count FROM app.pins WHERE role_id = ${role}
      `;
      expect(count).toBe(1);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), first.end(), second.end()]);
    }
  });

  it("returns an error the page can show, and writes no row, when the role to pin no longer exists", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const app = postgres(dsns.appDsn);
    const m = newMarker("profile-gone");

    try {
      // The page rendered this role; it is retired before the submit lands.
      const role = await insertTerm(owner, "canonical_roles", `${m}-role`);
      await retire(owner, "canonical_roles", [{ slug: `${m}-role`, reason: "Gone under the page." }], m);

      const result = await pinRole(app, role);
      expect(result).toEqual({ ok: false, error: expect.stringMatching(/no longer in the taxonomy/) });
      const [{ count }] = await owner<{ count: number }[]>`
        SELECT count(*)::int AS count FROM app.pins WHERE role_id = ${role}
      `;
      expect(count).toBe(0);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end()]);
    }
  });
});
