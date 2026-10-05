// Composes the letter from the discovery read. Every sentence that lifts the
// reader cites something in the data (contract invariant 6); nothing about a
// particular person is written here.
// See: agent-context/plans/in-progress/discovery-prototypes/index.md

import type { DiscoveryData, Recommendation, Strength } from "../_data/query";
import {
  capital,
  count,
  counted,
  figure,
  joinList,
  listPhrases,
  percent,
  plural,
  unique,
  withArticle,
} from "./grammar";

/** A run of words that arrives together. `key` marks the fact a beat lands on. */
export interface Phrase {
  readonly text: string;
  readonly key?: boolean;
  /** Carries the beat's footnote mark. */
  readonly note?: boolean;
}
export type Line = readonly Phrase[];

/** What the page may see of a recommendation. The score stays on the server. */
export type Rec = Omit<Recommendation, "score">;

export interface ProseBeat {
  readonly kind: "prose";
  readonly label: string;
  readonly eyebrow: string;
  readonly display: Line;
  readonly body: readonly Line[];
  /** A quieter line after the key fact: a postscript, never the point. */
  readonly postscript?: string;
  readonly footnote?: string;
  /** Lists set in the body's quiet register, such as claimed skills. */
  readonly roster?: readonly string[];
}

export interface RoleBeat {
  readonly kind: "role";
  readonly label: string;
  readonly eyebrow: string;
  readonly kicker: string | null;
  readonly rec: Rec;
  readonly alsoCalled: string | null;
  readonly echo: string | null;
  readonly evidence: Line;
  readonly bring: Line | null;
  readonly grow: readonly { readonly name: string; readonly share: string }[];
}

export interface RosterRow {
  readonly rec: Rec;
  readonly alternates: readonly string[];
  readonly reason: string | null;
}

export interface AllBeat {
  readonly kind: "all";
  readonly label: string;
  readonly eyebrow: string;
  readonly display: Line;
  readonly body: string;
  readonly groups: readonly { readonly strength: Strength; readonly label: string; readonly rows: readonly RosterRow[] }[];
  readonly coverage: string;
}

export type Beat = ProseBeat | RoleBeat | AllBeat;

const FEATURED = 3;

const GROUP_LABEL: Record<Strength, string> = {
  close: "Closest to your path",
  adjacent: "A step further",
  stretch: "A longer reach",
};

export function composeLetter(data: DiscoveryData): Beat[] {
  const recs = data.recommendations.map(shown);
  const titles = unique(data.pastRoles.map((p) => p.titleText));
  const featured = pickFeatured(recs);

  const beats: Beat[] = [pathBeat(titles, unique(data.claimedSkills.map((c) => c.text)), data)];
  if (recs.length > 0) beats.push(travelsBeat(recs));
  beats.push(bridgeBeat(data, recs.length, featured.length));

  const said = new Set<string>();
  featured.forEach((rec, i) => {
    beats.push(roleBeat(rec, i, featured.length, titles, said));
    rec.bring.forEach((b) => said.add(b.slug));
  });

  beats.push(allBeat(data, recs, titles));
  return beats;
}

function shown({ score: _hidden, ...rec }: Recommendation): Rec {
  return rec;
}

// Close-tier roles, up to three. A profile whose top score is zero has no
// close tier, so the first role stands in rather than skipping the beat.
function pickFeatured(recs: readonly Rec[]): Rec[] {
  const close = recs.filter((r) => r.strength === "close").slice(0, FEATURED);
  return close.length > 0 ? close : recs.slice(0, 1);
}

function pathBeat(titles: string[], skills: string[], data: DiscoveryData): ProseBeat {
  const base = { kind: "prose", label: "Where you've been", eyebrow: "Where you've been" } as const;

  if (titles.length > 0) {
    const items = listPhrases(titles.map(withArticle));
    const counts = skills.length > 0
      ? `${capital(counted(titles.length, "title"))}, and ${counted(skills.length, "skill")} you carried through ${titles.length === 1 ? "it" : "them"}:`
      : `${capital(counted(titles.length, "title"))}. That's where this begins.`;
    return {
      ...base,
      display: [{ text: "You've been" }, ...items.map((text, i) => ({ text, key: i === items.length - 1 }))],
      body: [[{ text: counts }]],
      roster: skills,
    };
  }

  if (skills.length > 0) {
    return {
      ...base,
      display: [{ text: "You came here with" }, { text: `${counted(skills.length, "skill")}.`, key: true }],
      body: [[{ text: "Before titles, the work itself:" }]],
      roster: skills,
    };
  }

  return {
    ...base,
    display: [{ text: "This note starts" }, { text: "from the market itself.", key: true }],
    body: [[{ text: `${figure(data.coverage.classifiedPostings)} postings read so far, across ${figure(data.coverage.rolesConsidered)} roles.` }]],
  };
}

function travelsBeat(recs: readonly Rec[]): ProseBeat {
  const base = { kind: "prose", label: "What comes with you", eyebrow: "What comes with you" } as const;

  // Which claimed skill reaches the most of the recommended roles.
  const reach = new Map<string, { name: string; roles: Rec[] }>();
  for (const rec of recs) {
    for (const skill of rec.bring) {
      const entry = reach.get(skill.slug) ?? { name: skill.name, roles: [] };
      entry.roles.push(rec);
      reach.set(skill.slug, entry);
    }
  }
  const ranked = [...reach.values()].sort((a, b) => b.roles.length - a.roles.length || a.roles[0]!.rank - b.roles[0]!.rank);
  const lead = ranked[0];

  if (!lead) {
    const largest = largestOf(recs);
    return {
      ...base,
      display: [{ text: "What you've done" }, { text: `lines up with ${counted(recs.length, "role")}` }, { text: "employers are hiring for." }],
      body: [
        recs.length === 1
          ? [{ text: `That role is ${largest.headline}, with` }, { text: `${postingsAt(largest)}.`, key: true }]
          : [{ text: `The largest, ${largest.headline}, has` }, { text: `${postingsAt(largest)}.`, key: true }],
      ],
    };
  }

  const n = lead.roles.length;
  const largest = largestOf(lead.roles);
  const define = n === 1
    ? `It's among the ten skills that most define the ${lead.roles[0]!.headline} role.`
    : `It's among the ten skills that most define ${count(n)} roles near your path: ${joinList(lead.roles.map((r) => r.headline))}.`;

  // A second skill that opens roles the first one doesn't.
  const seen = new Set(lead.roles.map((r) => r.roleSlug));
  const second = ranked.slice(1).map((s) => ({ ...s, roles: s.roles.filter((r) => !seen.has(r.roleSlug)) })).find((s) => s.roles.length > 0);

  return {
    ...base,
    display: [{ text: lead.name }, { text: "goes with you." }],
    body: [
      [{ text: define }],
      [{ text: n === 1 ? "It has" : `${largest.headline} alone has` }, { text: `${postingsAt(largest)}.`, key: true }],
    ],
    postscript: second
      ? `${second.name} does the same for ${joinList(second.roles.map((r) => r.headline))}.`
      : undefined,
  };
}

function bridgeBeat(data: DiscoveryData, n: number, featured: number): ProseBeat {
  const pool = figure(data.coverage.rolesConsidered);
  const body: Line[] =
    featured === 0
      ? [[{ text: "As the market fills in, or as your profile grows, that can change." }]]
      : featured === 1
        ? [[{ text: "One stands out. Let's start there." }]]
        : [[{ text: `${capital(count(featured))} stand out. Let's take them one at a time.` }]];

  return {
    kind: "prose",
    label: "Where it could lead",
    eyebrow: "Where it could lead",
    display: [
      { text: `Of ${pool} roles` },
      { text: "employers are hiring for now,", note: true },
      n === 0
        ? { text: "none sits close enough yet.", key: true }
        : { text: `${count(n)} ${n === 1 ? "sits" : "sit"} closest to your path.`, key: true },
    ],
    body,
    footnote: coverageLine(data),
  };
}

const REPEAT_BRING = [
  (list: string, _many: boolean) => `You already bring ${list}.`,
  (list: string, many: boolean) => `${list} ${many ? "come" : "comes"} with you here, too.`,
  (list: string, _many: boolean) => `Here, too, you'd bring ${list}.`,
];

function roleBeat(rec: Rec, i: number, total: number, titles: string[], said: Set<string>): RoleBeat {
  const alternates = alternatesOf(rec);
  const echo = echoOf(rec, titles);

  let bring: Line | null = null;
  if (rec.bring.length > 0) {
    const names = joinList(rec.bring.map((b) => b.name));
    // Repeating "You already bring React" three times reads as a template.
    const repeat = rec.bring.every((b) => said.has(b.slug)) ? Math.min(i, REPEAT_BRING.length - 1) : 0;
    bring = [{ text: REPEAT_BRING[repeat]!(names, rec.bring.length > 1), key: true }];
  }

  return {
    kind: "role",
    label: rec.headline,
    eyebrow: total > 1 ? `Closest to your path · ${i + 1} of ${total}` : "Closest to your path",
    kicker: rec.closestPast ? `From ${rec.closestPast.titleText}, toward` : null,
    rec,
    alsoCalled: alternates.length > 0 ? `Also posted as ${joinList(alternates)}.` : null,
    echo: echo ? `${echo} is a title you've held.` : null,
    evidence: [{ text: `${figure(rec.openPostings)} open ${plural(rec.openPostings, "posting")}` }, { text: `at ${figure(rec.companies)} ${plural(rec.companies, "company", "companies")}.`, key: bring === null }],
    bring,
    grow: rec.grow.map((g) => ({ name: g.name, share: percent(g.share) })),
  };
}

function allBeat(data: DiscoveryData, recs: readonly Rec[], titles: string[]): AllBeat {
  const groups = (["close", "adjacent", "stretch"] as const)
    .map((strength) => ({
      strength,
      label: GROUP_LABEL[strength],
      rows: recs.filter((r) => r.strength === strength).map((rec) => rosterRow(rec, titles)),
    }))
    .filter((g) => g.rows.length > 0);

  return {
    kind: "all",
    label: recs.length > 1 ? `All ${count(recs.length)}` : "The list",
    eyebrow: recs.length > 1 ? `All ${count(recs.length)}, closest first` : "The list",
    display: recs.length > 0
      ? [{ text: "Pin the ones" }, { text: "worth watching.", key: true }]
      : [{ text: "Nothing to pin" }, { text: "just yet.", key: true }],
    body: recs.length > 0
      ? "Pin as many as you like. They're yours to change later."
      : "Add a past title or a few skills to your profile, and this list will fill in.",
    groups,
    coverage: coverageLine(data),
  };
}

function rosterRow(rec: Rec, titles: string[]): RosterRow {
  const parts: string[] = [];
  if (rec.closestPast) parts.push(`near your ${rec.closestPast.titleText} work`);
  if (rec.bring.length > 0) parts.push(`you bring ${joinList(rec.bring.map((b) => b.name))}`);
  const echo = echoOf(rec, titles);
  if (echo) parts.push(`still posted as ${echo}`);
  return {
    rec,
    alternates: alternatesOf(rec).slice(0, 3),
    reason: parts.length > 0 ? capital(parts.join(" · ")) : null,
  };
}

function alternatesOf(rec: Rec): string[] {
  const head = rec.headline.toLowerCase();
  return unique(rec.titles.map((t) => t.title)).filter((t) => t.toLowerCase() !== head);
}

/** A past title that employers still use for this role: a familiar name under an unfamiliar role. */
function echoOf(rec: Rec, titles: string[]): string | null {
  const past = new Set(titles.map((t) => t.toLowerCase()));
  return rec.titles.find((t) => t.title.toLowerCase() !== rec.headline.toLowerCase() && past.has(t.title.toLowerCase()))?.title ?? null;
}

function largestOf(recs: readonly Rec[]): Rec {
  return recs.reduce((a, b) => (b.openPostings > a.openPostings ? b : a));
}

function postingsAt(rec: Rec): string {
  return `${figure(rec.openPostings)} open ${plural(rec.openPostings, "posting")} at ${figure(rec.companies)} ${plural(rec.companies, "company", "companies")}`;
}

export function coverageLine(data: DiscoveryData): string {
  const { classifiedPostings, openPostings, rolesConsidered } = data.coverage;
  return `Ranked from ${figure(classifiedPostings)} classified of ${figure(openPostings)} open postings, across ${figure(rolesConsidered)} roles with enough postings to read.`;
}
