import type { TaxonomyMatch } from "@/lib/db/taxonomy-search";

// The contract between the profile's Server Actions and the forms that call
// them. Actions arrive as props, so a story can pass a fake.

export type ProfileActionState =
  | { status: "idle" }
  | { status: "saved" }
  | { status: "error"; error: string };

export type ProfileFormAction = (
  previous: ProfileActionState,
  formData: FormData,
) => Promise<ProfileActionState>;

export type TaxonomySearchResult =
  | { ok: true; matches: TaxonomyMatch[] }
  | { ok: false; error: string };

export type TaxonomySearch = (term: string) => Promise<TaxonomySearchResult>;

export const idleState: ProfileActionState = { status: "idle" };
