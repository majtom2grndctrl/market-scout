import type { Meta, StoryObj } from "@storybook/react";

import type { ClaimedSkill, PastTitle, Pin, Seniority } from "@/lib/db/profile";
import type { TaxonomyMatch } from "@/lib/db/taxonomy-search";

import type { ProfileFormAction, TaxonomySearch } from "./action-state";
import { ClaimSkillForm, PastTitleForm, PinRoleForm } from "./profile-forms";
import { PastTitleList, PinList, SkillList } from "./profile-lists";

// The profile's states against fakes: Server Actions and taxonomy search are
// props, so nothing here reaches a database. Saves resolve after a delay long
// enough to see the pending state -- the submit disables until it lands.

const ROLES: TaxonomyMatch[] = [
  { id: "1", slug: "product-designer", name: "Product Designer", usageCount: 412 },
  { id: "2", slug: "design-engineer", name: "Design Engineer", usageCount: 37 },
  { id: "3", slug: "ux-researcher", name: "UX Researcher", usageCount: 1 },
  { id: "4", slug: "design-systems-engineer", name: "Design Systems Engineer", usageCount: 0 },
];

const SKILLS: TaxonomyMatch[] = [
  { id: "11", slug: "figma", name: "Figma", usageCount: 1890 },
  { id: "12", slug: "design-systems", name: "Design Systems", usageCount: 655 },
  { id: "13", slug: "typescript", name: "TypeScript", usageCount: 2304 },
];

const SENIORITIES: Seniority[] = [
  { slug: "junior", name: "Junior" },
  { slug: "mid", name: "Mid" },
  { slug: "senior", name: "Senior" },
  { slug: "staff", name: "Staff" },
  { slug: "principal", name: "Principal" },
];

function fakeSearch(terms: TaxonomyMatch[]): TaxonomySearch {
  return async (term) => {
    await new Promise((resolve) => setTimeout(resolve, 250));
    if (term.trim().toLowerCase() === "error") {
      return { ok: false, error: "Search is unavailable right now." };
    }
    const needle = term.trim().toLowerCase();
    return { ok: true, matches: terms.filter((t) => t.name.toLowerCase().includes(needle)) };
  };
}

const saves: ProfileFormAction = async () => {
  await new Promise((resolve) => setTimeout(resolve, 1200));
  return { status: "saved" };
};

const fails: ProfileFormAction = async () => {
  await new Promise((resolve) => setTimeout(resolve, 600));
  return {
    status: "error",
    error: "That role is no longer in the taxonomy — it was merged or retired. Search again to pin its replacement.",
  };
};

const PINS: Pin[] = [
  {
    id: "1",
    pinnedName: "Product Designer",
    pinnedAt: new Date("2026-09-30"),
    role: { id: "1", slug: "product-designer", name: "Product Designer" },
  },
  { id: "2", pinnedName: "Interaction Designer", pinnedAt: new Date("2026-09-30"), role: null },
];

const TITLES: PastTitle[] = [
  {
    id: "1",
    titleText: "Senior Product Designer II",
    role: { id: "1", slug: "product-designer", name: "Product Designer" },
    seniority: { slug: "senior", name: "Senior" },
  },
  { id: "2", titleText: "Creative Technologist", role: null, seniority: null },
];

const CLAIMED: ClaimedSkill[] = [
  { id: "1", skillText: "Figma", skill: { id: "11", slug: "figma", name: "Figma" } },
  { id: "2", skillText: "Prototyping in code", skill: null },
];

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div className="max-w-content space-y-4 rounded-lg border bg-surface-raised p-4 sm:p-5">{children}</div>
  );
}

const meta: Meta = {
  title: "Profile/Sections",
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

// A pin whose role was retired keeps its pinned-at name and says so.
export const PinnedRoles: Story = {
  render: () => (
    <Card>
      <PinList pins={PINS} unpin={saves} />
      <PinRoleForm action={saves} search={fakeSearch(ROLES)} />
    </Card>
  ),
};

export const PinnedRolesEmpty: Story = {
  render: () => (
    <Card>
      <PinList pins={[]} unpin={saves} />
      <PinRoleForm action={saves} search={fakeSearch(ROLES)} />
    </Card>
  ),
};

// Pick a role and submit: the error a pin of a vanished role returns.
export const PinFails: Story = {
  render: () => (
    <Card>
      <PinList pins={[]} unpin={saves} />
      <PinRoleForm action={fails} search={fakeSearch(ROLES)} />
    </Card>
  ),
};

export const PastTitles: Story = {
  render: () => (
    <Card>
      <PastTitleList titles={TITLES} remove={saves} />
      <PastTitleForm action={saves} search={fakeSearch(ROLES)} seniorities={SENIORITIES} />
    </Card>
  ),
};

// Type "error" in the picker to see search failure; type anything unlisted to
// add it unmatched.
export const Skills: Story = {
  render: () => (
    <Card>
      <SkillList skills={CLAIMED} remove={saves} />
      <ClaimSkillForm action={saves} search={fakeSearch(SKILLS)} />
    </Card>
  ),
};

export const SkillsEmpty: Story = {
  render: () => (
    <Card>
      <SkillList skills={[]} remove={saves} />
      <ClaimSkillForm action={saves} search={fakeSearch(SKILLS)} />
    </Card>
  ),
};
