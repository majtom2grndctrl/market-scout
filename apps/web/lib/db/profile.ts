import type { ISql } from "postgres";

import { getAppSql } from "./app-client";

// One profile per install: past titles, claimed skills, and pinned roles, in
// the private `app` schema. Every function here takes the app client -- never
// the read-only one, which cannot see the schema. Write cores take a client so
// the db tests call them directly; the Server Actions in app/profile wrap them.

export interface TermRef {
  id: string;
  slug: string;
  name: string;
}

export interface PastTitle {
  id: string;
  titleText: string;
  // Null: the person's text matched no role, or the role was retired.
  role: TermRef | null;
  seniority: { slug: string; name: string } | null;
}

export interface ClaimedSkill {
  id: string;
  skillText: string;
  // Null: unmatched, or the skill was retired.
  skill: TermRef | null;
}

export interface Pin {
  id: string;
  // The role's name when it was pinned. Survives the role's retirement.
  pinnedName: string;
  pinnedAt: Date;
  // Null: the role was retired. The pin stays, labelled by pinnedName.
  role: TermRef | null;
}

export interface Seniority {
  slug: string;
  name: string;
}

export interface Profile {
  pastTitles: PastTitle[];
  claimedSkills: ClaimedSkill[];
  pins: Pin[];
  // The title-stated rank vocabulary a past title may take, ascending by rank.
  seniorities: Seniority[];
}

export type WriteResult = { ok: true } | { ok: false; error: string };

export async function selectProfile(sql: ISql): Promise<Profile> {
  const pastTitles = await sql<
    {
      id: string;
      title_text: string;
      role_id: string | null;
      role_slug: string | null;
      role_name: string | null;
      seniority: string | null;
      seniority_name: string | null;
    }[]
  >`
    SELECT t.id, t.title_text, r.id AS role_id, r.slug AS role_slug, r.name AS role_name,
           t.seniority, s.name AS seniority_name
    FROM app.past_titles t
    LEFT JOIN canonical_roles r ON r.id = t.role_id
    LEFT JOIN title_seniority_seeds s ON s.slug = t.seniority
    ORDER BY t.id
  `;

  const claimedSkills = await sql<
    { id: string; skill_text: string; skill_id: string | null; skill_slug: string | null; skill_name: string | null }[]
  >`
    SELECT c.id, c.skill_text, k.id AS skill_id, k.slug AS skill_slug, k.name AS skill_name
    FROM app.claimed_skills c
    LEFT JOIN skills k ON k.id = c.skill_id
    ORDER BY c.id
  `;

  const pins = await sql<
    { id: string; pinned_name: string; pinned_at: Date; role_id: string | null; role_slug: string | null; role_name: string | null }[]
  >`
    SELECT p.id, p.pinned_name, p.pinned_at, r.id AS role_id, r.slug AS role_slug, r.name AS role_name
    FROM app.pins p
    LEFT JOIN canonical_roles r ON r.id = p.role_id
    ORDER BY p.pinned_at, p.id
  `;

  const seniorities = await sql<Seniority[]>`
    SELECT slug, name FROM title_seniority_seeds WHERE slug <> 'unstated' ORDER BY rank
  `;

  return {
    pastTitles: pastTitles.map((t) => ({
      id: t.id,
      titleText: t.title_text,
      role: termRef(t.role_id, t.role_slug, t.role_name),
      seniority: t.seniority && t.seniority_name ? { slug: t.seniority, name: t.seniority_name } : null,
    })),
    claimedSkills: claimedSkills.map((c) => ({
      id: c.id,
      skillText: c.skill_text,
      skill: termRef(c.skill_id, c.skill_slug, c.skill_name),
    })),
    pins: pins.map((p) => ({
      id: p.id,
      pinnedName: p.pinned_name,
      pinnedAt: p.pinned_at,
      role: termRef(p.role_id, p.role_slug, p.role_name),
    })),
    seniorities: [...seniorities],
  };
}

export async function getProfile(): Promise<Profile> {
  return selectProfile(await getAppSql());
}

function termRef(id: string | null, slug: string | null, name: string | null): TermRef | null {
  return id && slug && name ? { id, slug, name } : null;
}

// ---------------------------------------------------------------------------
// Write cores. Each returns a WriteResult rather than throwing for anything
// the person can cause, so the page can show it. Where a natural key exists
// the write is idempotent: repeating it succeeds and writes nothing.
// ---------------------------------------------------------------------------

// Up to 18 digits, so every accepted id fits a bigint and a malformed one is a
// field error rather than a Postgres range error.
const ID = /^[1-9][0-9]{0,17}$/;

// Postgres error codes the person can trigger. A foreign key violation means a
// term vanished between render and submit -- merged or retired under the page.
const FOREIGN_KEY_VIOLATION = "23503";

function pgCode(error: unknown): string | undefined {
  return typeof error === "object" && error !== null && "code" in error
    ? String((error as { code: unknown }).code)
    : undefined;
}

export async function pinRole(sql: ISql, roleId: string): Promise<WriteResult> {
  if (!ID.test(roleId)) {
    return { ok: false, error: "Choose a role to pin." };
  }

  try {
    // The name is copied at pin time, so a later retirement leaves the pin
    // labelled. ON CONFLICT makes a double submit, or two tabs, one pin.
    const inserted = await sql`
      INSERT INTO app.pins (role_id, pinned_name)
      SELECT id, name FROM canonical_roles WHERE id = ${roleId}
      ON CONFLICT (role_id) DO NOTHING
      RETURNING id
    `;
    if (inserted.length > 0) {
      return { ok: true };
    }
    const [existing] = await sql`SELECT 1 FROM app.pins WHERE role_id = ${roleId}`;
    return existing ? { ok: true } : roleGone();
  } catch (error) {
    if (pgCode(error) === FOREIGN_KEY_VIOLATION) {
      return roleGone();
    }
    throw error;
  }
}

function roleGone(): WriteResult {
  return {
    ok: false,
    error: "That role is no longer in the taxonomy — it was merged or retired. Search again to pin its replacement.",
  };
}

// By pin id rather than role, so a retired pin -- which has no role -- can go too.
export async function unpin(sql: ISql, pinId: string): Promise<WriteResult> {
  if (!ID.test(pinId)) {
    return { ok: false, error: "That pin could not be found." };
  }
  await sql`DELETE FROM app.pins WHERE id = ${pinId}`;
  return { ok: true };
}

export async function claimSkill(
  sql: ISql,
  input: { skillText: string; skillId: string | null },
): Promise<WriteResult> {
  const skillText = input.skillText.trim();
  if (skillText === "") {
    return { ok: false, error: "Enter a skill." };
  }
  if (input.skillId !== null && !ID.test(input.skillId)) {
    return { ok: false, error: "That skill could not be matched. Pick it from the list or add it as text." };
  }

  try {
    // One claim per matched skill and per text. Repeating a claim -- the same
    // skill, or the same text with the same match -- is a no-op. A clash that
    // would change what the person sees is reported instead: success there
    // would silently drop the match they just picked.
    const inserted = await sql`
      INSERT INTO app.claimed_skills (skill_text, skill_id)
      VALUES (${skillText}, ${input.skillId})
      ON CONFLICT DO NOTHING
      RETURNING id
    `;
    if (inserted.length > 0) {
      return { ok: true };
    }
    const [sameText] = await sql<{ skill_text: string; skill_id: string | null }[]>`
      SELECT skill_text, skill_id FROM app.claimed_skills
      WHERE lower(btrim(skill_text)) = lower(btrim(${skillText}))
    `;
    if (sameText && sameText.skill_id !== input.skillId && input.skillId !== null) {
      // The text clash kept the insert from running, so its foreign-key check
      // never did: a term merged or retired under the page has to be caught here.
      const [stillThere] = await sql`SELECT 1 FROM skills WHERE id = ${input.skillId}`;
      if (!stillThere) {
        return skillGone();
      }
      const [sameSkill] = await sql`SELECT 1 FROM app.claimed_skills WHERE skill_id = ${input.skillId}`;
      if (!sameSkill) {
        return {
          ok: false,
          error: `“${sameText.skill_text}” is already listed${sameText.skill_id === null ? " as unmatched" : " under another match"}. Remove it first to claim this match.`,
        };
      }
    }
    return { ok: true };
  } catch (error) {
    if (pgCode(error) === FOREIGN_KEY_VIOLATION) {
      return skillGone();
    }
    throw error;
  }
}

function skillGone(): WriteResult {
  return {
    ok: false,
    error: "That skill is no longer in the taxonomy. Search again, or add it as unmatched text.",
  };
}

export async function removeClaimedSkill(sql: ISql, claimId: string): Promise<WriteResult> {
  if (!ID.test(claimId)) {
    return { ok: false, error: "That skill could not be found." };
  }
  await sql`DELETE FROM app.claimed_skills WHERE id = ${claimId}`;
  return { ok: true };
}

export async function addPastTitle(
  sql: ISql,
  input: { titleText: string; roleId: string | null; seniority: string | null },
): Promise<WriteResult> {
  const titleText = input.titleText.trim();
  if (titleText === "") {
    return { ok: false, error: "Enter a title." };
  }
  if (input.roleId !== null && !ID.test(input.roleId)) {
    return { ok: false, error: "That role could not be matched. Pick it from the list or leave it unmatched." };
  }
  if (input.seniority === "unstated") {
    return { ok: false, error: "Leave seniority blank when the title states none." };
  }

  try {
    // No natural key: two stints under one title are real, so a duplicate is
    // written and stays visible and deletable.
    await sql`
      INSERT INTO app.past_titles (title_text, role_id, seniority)
      VALUES (${titleText}, ${input.roleId}, ${input.seniority})
    `;
    return { ok: true };
  } catch (error) {
    if (pgCode(error) === FOREIGN_KEY_VIOLATION) {
      return {
        ok: false,
        error: "That role or seniority is no longer available. Search again, or leave it unmatched.",
      };
    }
    throw error;
  }
}

export async function removePastTitle(sql: ISql, titleId: string): Promise<WriteResult> {
  if (!ID.test(titleId)) {
    return { ok: false, error: "That title could not be found." };
  }
  await sql`DELETE FROM app.past_titles WHERE id = ${titleId}`;
  return { ok: true };
}
