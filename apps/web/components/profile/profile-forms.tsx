"use client";

import * as React from "react";
import { X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Seniority } from "@/lib/db/profile";
import type { TaxonomyMatch } from "@/lib/db/taxonomy-search";

import {
  idleState,
  type ProfileActionState,
  type ProfileFormAction,
  type TaxonomySearch,
} from "./action-state";
import { TaxonomyPicker } from "./taxonomy-picker";

// The profile's write controls. Each submit is disabled while its save is in
// flight, so a double click cannot queue a second write behind the first; the
// write cores are idempotent regardless.

function FormError({ state }: { state: ProfileActionState }) {
  // role="alert" announces the failure; the text stays until the next submit.
  return state.status === "error" ? (
    <p role="alert" className="text-sm text-danger-ink">
      {state.error}
    </p>
  ) : null;
}

// useActionState around a profile action, clearing the form's local
// selection once its save lands. The reset runs inside the action's
// transition, so the cleared form and the refreshed list render together.
function useProfileAction(action: ProfileFormAction, onSaved: () => void) {
  return React.useActionState<ProfileActionState, FormData>(async (previous, formData) => {
    const next = await action(previous, formData);
    if (next.status === "saved") {
      onSaved();
    }
    return next;
  }, idleState);
}

export function PinRoleForm({ action, search }: { action: ProfileFormAction; search: TaxonomySearch }) {
  const [role, setRole] = React.useState<TaxonomyMatch | null>(null);
  const [pickerKey, setPickerKey] = React.useState(0);
  const [state, formAction, isPending] = useProfileAction(action, () => {
    setRole(null);
    setPickerKey((k) => k + 1);
  });

  return (
    <form action={formAction} className="grid gap-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <TaxonomyPicker
            key={pickerKey}
            id="pin-role"
            label="Role to watch"
            placeholder="Search roles, e.g. product designer"
            noun="role"
            search={search}
            value={role}
            onValueChange={setRole}
            disabled={isPending}
          />
        </div>
        <input type="hidden" name="roleId" value={role?.id ?? ""} />
        <Button type="submit" disabled={isPending || role === null}>
          {isPending ? "Pinning…" : "Pin role"}
        </Button>
      </div>
      <FormError state={state} />
    </form>
  );
}

export function ClaimSkillForm({ action, search }: { action: ProfileFormAction; search: TaxonomySearch }) {
  const [skill, setSkill] = React.useState<TaxonomyMatch | null>(null);
  const [typed, setTyped] = React.useState("");
  const [pickerKey, setPickerKey] = React.useState(0);
  const [state, formAction, isPending] = useProfileAction(action, () => {
    setSkill(null);
    setTyped("");
    setPickerKey((k) => k + 1);
  });

  // A picked term is claimed under its own name; anything else is claimed as
  // typed, unmatched, and stays visible as such. Editing the text after a
  // pick makes it typed text again.
  const matched = skill && typed.trim() === skill.name ? skill : null;
  const text = matched ? matched.name : typed.trim();

  return (
    <form action={formAction} className="grid gap-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <TaxonomyPicker
            key={pickerKey}
            id="claim-skill"
            label="Skill"
            placeholder="Search skills, or type your own"
            noun="skill"
            search={search}
            value={skill}
            onValueChange={setSkill}
            onInputValueChange={setTyped}
            keepTypedText
            disabled={isPending}
          />
        </div>
        <input type="hidden" name="skillId" value={matched?.id ?? ""} />
        <input type="hidden" name="skillText" value={text} />
        <Button type="submit" disabled={isPending || text === ""}>
          {isPending ? "Adding…" : matched || text === "" ? "Add skill" : "Add unmatched"}
        </Button>
      </div>
      <FormError state={state} />
    </form>
  );
}

export function PastTitleForm({
  action,
  search,
  seniorities,
}: {
  action: ProfileFormAction;
  search: TaxonomySearch;
  seniorities: Seniority[];
}) {
  const [role, setRole] = React.useState<TaxonomyMatch | null>(null);
  const [formKey, setFormKey] = React.useState(0);
  const [state, formAction, isPending] = useProfileAction(action, () => {
    setRole(null);
    setFormKey((k) => k + 1);
  });

  return (
    <form key={formKey} action={formAction} className="grid gap-3">
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_10rem]">
        <div className="grid gap-1.5">
          <label htmlFor="past-title-text" className="text-sm font-medium">
            Title, as you held it
          </label>
          <Input
            id="past-title-text"
            name="titleText"
            required
            placeholder="e.g. Senior Product Designer"
            disabled={isPending}
          />
        </div>
        <div className="grid gap-1.5">
          <label htmlFor="past-title-seniority" className="text-sm font-medium">
            Seniority <span className="font-normal text-content-muted">(optional)</span>
          </label>
          {/* Native select: keyboard and screen-reader behaviour come free, and
              the list is short and fixed. */}
          <select
            id="past-title-seniority"
            name="seniority"
            defaultValue=""
            disabled={isPending}
            className="h-8 w-full rounded-lg border border-edge-strong bg-transparent px-2 text-sm outline-none focus-visible:border-focus focus-visible:ring-3 focus-visible:ring-focus/50 disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-content-disabled dark:bg-surface-sunken"
          >
            <option value="">Not stated</option>
            {seniorities.map((s) => (
              <option key={s.slug} value={s.slug}>
                {s.name}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <TaxonomyPicker
            id="past-title-role"
            label="Matching role (optional)"
            placeholder="Search roles"
            noun="role"
            search={search}
            value={role}
            onValueChange={setRole}
            disabled={isPending}
          />
        </div>
        <input type="hidden" name="roleId" value={role?.id ?? ""} />
        <Button type="submit" disabled={isPending}>
          {isPending ? "Adding…" : "Add title"}
        </Button>
      </div>
      <FormError state={state} />
    </form>
  );
}

// One per listed row. The form carries the row id; the label names the row,
// since every remove button on the page otherwise reads the same.
export function RemoveButton({
  action,
  id,
  label,
}: {
  action: ProfileFormAction;
  id: string;
  label: string;
}) {
  const [state, formAction, isPending] = React.useActionState(action, idleState);

  return (
    <form action={formAction} className="flex shrink-0 items-center gap-2">
      <input type="hidden" name="id" value={id} />
      {state.status === "error" ? (
        <span role="alert" className="text-xs text-danger-ink">
          {state.error}
        </span>
      ) : null}
      <Button type="submit" variant="ghost" size="icon-sm" aria-label={label} disabled={isPending}>
        <X />
      </Button>
    </form>
  );
}
