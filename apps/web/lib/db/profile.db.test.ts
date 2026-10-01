import postgres from "postgres";
import { describe, expect, it } from "vitest";

import {
  addPastTitle,
  claimSkill,
  pinRole,
  removeClaimedSkill,
  removePastTitle,
  selectProfile,
  unpin,
} from "./profile";
import { selectTaxonomyMatches } from "./taxonomy-search";
import {
  deleteRepairsLabelled,
  insertClaim,
  insertClassification,
  insertLink,
  insertPin,
  insertTerm,
  newMarker,
  retire,
} from "./testing/taxonomy-fixtures";
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
  await deleteRepairsLabelled(owner, m);
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

      // A retired role reads as a pin with no role, labelled by its pinned-at name.
      await retire(owner, "canonical_roles", [{ slug: `${m}-product-designer`, reason: "Fixture" }], m);
      const afterRetire = await selectProfile(app);
      expect(afterRetire.pins).toContainEqual(
        expect.objectContaining({ pinnedName: `${m} Product Designer`, role: null }),
      );
      expect(afterRetire.pastTitles).toContainEqual(
        expect.objectContaining({ titleText: `${m} Senior Product Designer`, role: null }),
      );
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end(), readOnly.end()]);
    }
  });
});

describe("taxonomy search", () => {
  it("finds by substring below the similarity floor, escapes LIKE wildcards, and breaks score ties by usage", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const readOnly = postgres(dsns.readOnlyDsn);
    const m = newMarker("search");

    try {
      // Same name, so the same score; the one tagged more often ranks first.
      const rare = await insertTerm(owner, "skills", `${m}-rare`, `${m} Shared Name`);
      const common = await insertTerm(owner, "skills", `${m}-common`, `${m} Shared Name`);
      await insertLink(owner, "skills", await insertClassification(owner, m), common);
      await insertLink(owner, "skills", await insertClassification(owner, m), common);
      await insertLink(owner, "skills", await insertClassification(owner, m), rare);
      const percent = await insertTerm(owner, "skills", `${m}-percent`, `${m} 100% Uptime`);

      const ranked = await selectTaxonomyMatches(readOnly, "skills", `${m} shared name`);
      expect(ranked.slice(0, 2).map((r) => [r.id, r.usageCount])).toEqual([
        [common, 2],
        [rare, 1],
      ]);

      // A short fragment of a long name scores below the trigram floor, so
      // only the substring branch can find it. The score is checked, not assumed.
      const long = await insertTerm(
        owner,
        "skills",
        `${m}-long`,
        `${m} Comprehensive Accessibility Auditing Practices`,
      );
      const [{ score }] = await owner<{ score: number }[]>`
        SELECT greatest(similarity(${`${m}-long`}, 'ditin'),
                        similarity(${`${m} Comprehensive Accessibility Auditing Practices`}, 'ditin'))::float8 AS score
      `;
      expect(score).toBeLessThan(0.3);
      const fragment = await selectTaxonomyMatches(readOnly, "skills", "ditin");
      expect(fragment.map((r) => r.id)).toContain(long);

      // "%" is literal, not a wildcard: it matches the one name containing it.
      const literal = await selectTaxonomyMatches(readOnly, "skills", "0% Upt");
      expect(literal.map((r) => r.id)).toContain(percent);
      const wildcard = await selectTaxonomyMatches(readOnly, "skills", `${m.slice(-8)}%ne`);
      expect(wildcard).toEqual([]);

      expect(await selectTaxonomyMatches(readOnly, "skills", "   ")).toEqual([]);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), readOnly.end()]);
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

  it("adds and removes a past title, refuses empty text and unstated, and keeps a duplicate", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const app = postgres(dsns.appDsn);
    const m = newMarker("profile-title");

    try {
      const input = { titleText: `  ${m} Designer  `, roleId: null, seniority: "senior" };
      expect(await addPastTitle(app, input)).toEqual({ ok: true });
      expect(await addPastTitle(app, input)).toEqual({ ok: true });
      const titles = await owner<{ id: string; title_text: string }[]>`
        SELECT id, title_text FROM app.past_titles WHERE title_text LIKE ${`${m}%`} ORDER BY id
      `;
      // Trimmed, and two stints under one title are two rows.
      expect(titles.map((t) => t.title_text)).toEqual([`${m} Designer`, `${m} Designer`]);

      expect(await addPastTitle(app, { titleText: "   ", roleId: null, seniority: null })).toEqual({
        ok: false,
        error: expect.stringMatching(/Enter a title/),
      });
      expect(await addPastTitle(app, { titleText: `${m} X`, roleId: null, seniority: "unstated" })).toEqual({
        ok: false,
        error: expect.stringMatching(/Leave seniority blank/),
      });
      expect(await addPastTitle(app, { titleText: `${m} X`, roleId: "999999999999", seniority: null })).toEqual({
        ok: false,
        error: expect.stringMatching(/no longer available/),
      });
      expect(await addPastTitle(app, { titleText: `${m} X`, roleId: "12345678901234567890", seniority: null })).toEqual({
        ok: false,
        error: expect.stringMatching(/could not be matched/),
      });

      expect(await removePastTitle(app, titles[0].id)).toEqual({ ok: true });
      expect(await removePastTitle(app, titles[0].id)).toEqual({ ok: true });
      const [{ count }] = await owner<{ count: number }[]>`
        SELECT count(*)::int AS count FROM app.past_titles WHERE title_text LIKE ${`${m}%`}
      `;
      expect(count).toBe(1);
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end()]);
    }
  });

  it("reports a claim whose text is already listed under another match, rather than dropping the match", async (context) => {
    const dsns = dsnsOrSkip(context);
    if (!dsns) return;
    const owner = postgres(dsns.ownerDsn, { onnotice: () => {} });
    const app = postgres(dsns.appDsn);
    const m = newMarker("profile-clash");

    try {
      const skill = await insertTerm(owner, "skills", `${m}-figma`, `${m} Figma`);
      expect(await claimSkill(app, { skillText: `${m} Figma`, skillId: null })).toEqual({ ok: true });

      const clash = await claimSkill(app, { skillText: `${m} figma`, skillId: skill });
      expect(clash).toEqual({ ok: false, error: expect.stringMatching(/already listed as unmatched/) });

      const [claim] = await owner<{ id: string; skill_id: string | null }[]>`
        SELECT id, skill_id FROM app.claimed_skills WHERE skill_text = ${`${m} Figma`}
      `;
      expect(claim.skill_id).toBeNull();
      expect(await removeClaimedSkill(app, claim.id)).toEqual({ ok: true });
      expect(await claimSkill(app, { skillText: `${m} Figma`, skillId: skill })).toEqual({ ok: true });

      expect(await claimSkill(app, { skillText: `${m} Other`, skillId: "999999999999" })).toEqual({
        ok: false,
        error: expect.stringMatching(/no longer in the taxonomy/),
      });
    } finally {
      await teardown(owner, m);
      await Promise.all([owner.end(), app.end()]);
    }
  });
});
