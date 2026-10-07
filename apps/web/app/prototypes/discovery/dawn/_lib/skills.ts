// The skill column's rows, fixed for a page load. Two sections by where a
// skill came from: what the person added, and what is common in roles they
// have held. Whether a listed role draws on a skill is a row state (its
// reach), never a section (build contract, Dawn round 1).

import type { DiscoveryData, PersonSkill, Recommendation } from "../../_data/query";

export interface SkillRowModel {
  readonly skill: PersonSkill;
  /** How many listed roles draw on it. Zero rests muted. */
  readonly reach: number;
}

export interface ColumnModel {
  /** Matched claimed skills, most reached first. */
  readonly added: readonly SkillRowModel[];
  /** Claimed skills no posting skill matches, as the person wrote them. They never connect. */
  readonly unmatched: readonly string[];
  /** Inherited skills, most reached first. */
  readonly common: readonly SkillRowModel[];
}

export function reachCounts(recs: readonly Recommendation[]): ReadonlyMap<string, number> {
  const reach = new Map<string, number>();
  for (const r of recs) for (const c of r.connects) reach.set(c.slug, (reach.get(c.slug) ?? 0) + 1);
  return reach;
}

/**
 * Most reached first, ties in data order (Invariant 11). Computed once from
 * the read, so lighting a role never moves a row.
 */
function byReach(skills: readonly PersonSkill[], reach: ReadonlyMap<string, number>): SkillRowModel[] {
  return skills
    .map((skill, i) => ({ skill, reach: reach.get(skill.slug) ?? 0, i }))
    .sort((a, b) => b.reach - a.reach || a.i - b.i)
    .map(({ skill, reach: n }) => ({ skill, reach: n }));
}

export function columnModel(data: DiscoveryData): ColumnModel {
  const reach = reachCounts(data.recommendations);
  const seen = new Set<string>();
  const unmatched = data.claimedSkills.flatMap((c) => {
    const text = c.text.trim();
    const key = text.toLowerCase();
    if (c.skill || !text || seen.has(key)) return [];
    seen.add(key);
    return [text];
  });
  return {
    added: byReach(data.personSkills.filter((s) => s.claimed), reach),
    unmatched,
    common: byReach(data.personSkills.filter((s) => !s.claimed), reach),
  };
}
