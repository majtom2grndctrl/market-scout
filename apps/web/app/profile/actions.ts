"use server";

import { revalidatePath } from "next/cache";

import type {
  ProfileActionState,
  TaxonomySearchResult,
} from "@/components/profile/action-state";
import { getAppSql } from "@/lib/db/app-client";
import {
  addPastTitle,
  claimSkill,
  pinRole,
  removeClaimedSkill,
  removePastTitle,
  unpin,
  type WriteResult,
} from "@/lib/db/profile";
import { searchTaxonomy, type SearchableTable } from "@/lib/db/taxonomy-search";
import { paths } from "@/lib/paths";

// The profile's write path. Each action parses its form, runs the write core
// on the app client, and refreshes the page. Taxonomy search rides along as an
// action because a Client Component has no other request-response seam to the
// server -- route handlers are reserved for streaming -- and it runs on the
// read-only client, never the app one.

function field(formData: FormData, name: string): string {
  const value = formData.get(name);
  return typeof value === "string" ? value : "";
}

function optional(formData: FormData, name: string): string | null {
  const value = field(formData, name).trim();
  return value === "" ? null : value;
}

async function run(write: Promise<WriteResult>): Promise<ProfileActionState> {
  let result: WriteResult;
  try {
    result = await write;
  } catch (error) {
    console.error("[profile] write failed", error);
    return { status: "error", error: "The profile could not be saved. Try again in a moment." };
  }
  if (!result.ok) {
    return { status: "error", error: result.error };
  }
  revalidatePath(paths.profile);
  return { status: "saved" };
}

export async function pinRoleAction(_: ProfileActionState, formData: FormData) {
  return run(getAppSql().then((sql) => pinRole(sql, field(formData, "roleId"))));
}

export async function unpinAction(_: ProfileActionState, formData: FormData) {
  return run(getAppSql().then((sql) => unpin(sql, field(formData, "id"))));
}

export async function claimSkillAction(_: ProfileActionState, formData: FormData) {
  return run(
    getAppSql().then((sql) =>
      claimSkill(sql, {
        skillText: field(formData, "skillText"),
        skillId: optional(formData, "skillId"),
      }),
    ),
  );
}

export async function removeClaimedSkillAction(_: ProfileActionState, formData: FormData) {
  return run(getAppSql().then((sql) => removeClaimedSkill(sql, field(formData, "id"))));
}

export async function addPastTitleAction(_: ProfileActionState, formData: FormData) {
  return run(
    getAppSql().then((sql) =>
      addPastTitle(sql, {
        titleText: field(formData, "titleText"),
        roleId: optional(formData, "roleId"),
        seniority: optional(formData, "seniority"),
      }),
    ),
  );
}

export async function removePastTitleAction(_: ProfileActionState, formData: FormData) {
  return run(getAppSql().then((sql) => removePastTitle(sql, field(formData, "id"))));
}

async function search(table: SearchableTable, term: string): Promise<TaxonomySearchResult> {
  try {
    return { ok: true, matches: await searchTaxonomy(table, String(term)) };
  } catch (error) {
    console.error("[profile] taxonomy search failed", error);
    return { ok: false, error: "Search is unavailable right now." };
  }
}

export async function searchRolesAction(term: string): Promise<TaxonomySearchResult> {
  return search("canonical_roles", term);
}

export async function searchSkillsAction(term: string): Promise<TaxonomySearchResult> {
  return search("skills", term);
}
