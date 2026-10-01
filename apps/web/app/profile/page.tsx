import { ClaimSkillForm, PastTitleForm, PinRoleForm } from "@/components/profile/profile-forms";
import { PastTitleList, PinList, SkillList } from "@/components/profile/profile-lists";
import { Section } from "@/components/section";
import { getProfile } from "@/lib/db/profile";

import {
  addPastTitleAction,
  claimSkillAction,
  pinRoleAction,
  removeClaimedSkillAction,
  removePastTitleAction,
  searchRolesAction,
  searchSkillsAction,
  unpinAction,
} from "./actions";

// One profile per install: who the person is, and which roles they watch.
// Read and written on the app role; the read-only role that serves agents
// cannot see any of it.
export default async function ProfilePage() {
  const profile = await getProfile();

  return (
    <div className="mx-auto w-full max-w-content space-y-10 px-4 py-8 sm:px-6 lg:px-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
        <p className="text-sm text-content-muted">
          Your experience, and the roles you want to watch. Private to this install — agent sessions cannot read it.
        </p>
      </header>

      <Section title="Pinned roles">
        <div className="space-y-4 rounded-lg border bg-surface-raised p-4 sm:p-5">
          <PinList pins={profile.pins} unpin={unpinAction} />
          <PinRoleForm action={pinRoleAction} search={searchRolesAction} />
        </div>
      </Section>

      <Section title="Past titles">
        <div className="space-y-4 rounded-lg border bg-surface-raised p-4 sm:p-5">
          <PastTitleList titles={profile.pastTitles} remove={removePastTitleAction} />
          <PastTitleForm action={addPastTitleAction} search={searchRolesAction} seniorities={profile.seniorities} />
        </div>
      </Section>

      <Section title="Skills">
        <div className="space-y-4 rounded-lg border bg-surface-raised p-4 sm:p-5">
          <SkillList skills={profile.claimedSkills} remove={removeClaimedSkillAction} />
          <ClaimSkillForm action={claimSkillAction} search={searchSkillsAction} />
        </div>
      </Section>
    </div>
  );
}
