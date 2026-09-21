// Mock user profiles — written 2026-09-17.
//
// Market Scout has no profile today. This file invents one so the prototypes
// can ask a question they cannot ask otherwise: not "what does the market
// want" but "what does the market want FROM THIS PERSON." Leading underscore
// keeps the folder out of the App Router; nothing here is a route.
//
// Three deliberate constraints on the mock, because a persona that does not
// join to real data teaches nothing:
//
//   1. Every `have` and `target` is a slug that exists in the taxonomy today,
//      spot-checked against the corpus. A persona naming "React.js" or
//      "product leadership" would silently contribute zero to every score and
//      the screen would look like it was working.
//   2. Skill counts are deliberately uneven — 6 to 9 — because a real profile
//      is not a tidy list, and a coverage score that rewards simply listing
//      more skills is a bug worth catching early.
//   3. Each persona is chosen to break the screen in a different way, not to
//      flatter it. See the note on each.
//
// What a profile is NOT, and the screen has to keep saying so: a résumé.
// There is no evidence here of depth, recency, or quality — `react` means the
// person claims React, not that they are good at it. Everything downstream is
// therefore a coverage of stated requirements, never a hireability score.

export type Level = "junior" | "mid" | "senior" | "staff" | "lead" | "principal" | "director";

export interface PastTitle {
  // Matches a `role` slug in the taxonomy, so a past title is comparable to a
  // target on the same footing. A career the corpus has no role for is a real
  // limitation of this model, not an edge case — see `coverageGaps` below.
  readonly role: string;
  readonly level: Level;
  readonly years: number;
}

export interface Persona {
  readonly id: string;
  readonly name: string;
  // First person, present tense. A goal written as a job title is a target;
  // a goal written as a sentence is a motive, and the two disagree often
  // enough that both are worth carrying.
  readonly motive: string;
  readonly past: readonly PastTitle[];
  readonly have: readonly string[];
  readonly target: { readonly role: string; readonly level: Level };
  // What this persona is here to stress. Not shown in the UI as product copy —
  // rendered as a sketch annotation, because the point of a mock persona is to
  // be argued with.
  readonly stresses: string;
}

export const PERSONAS: readonly Persona[] = [
  {
    id: "maya",
    name: "Maya",
    motive: "I want to ship the thing I designed, not hand it over.",
    past: [
      { role: "product-designer", level: "senior", years: 3 },
      { role: "product-designer", level: "mid", years: 2 },
    ],
    have: [
      "figma",
      "ux-design",
      "interaction-design",
      "prototyping",
      "design-sensibility",
      "typescript",
      "react",
      "product-thinking",
    ],
    target: { role: "product-engineer", level: "senior" },
    stresses:
      "A function crossing where half the profile is worthless at the target. " +
      "figma, ux-design, interaction-design and design-sensibility carry 2.49 " +
      "of product-designer's demand and essentially none of product-engineer's; " +
      "typescript, react and product-thinking carry real weight. The screen has " +
      "to show both halves, because 'you are 40% there' is useless next to " +
      "'these four things stop counting and these three keep counting.'",
  },
  {
    id: "dev",
    name: "Dev",
    motive: "I want to own the platform, not just build on it — and at staff.",
    past: [
      { role: "software-engineer", level: "senior", years: 4 },
      { role: "software-engineer", level: "mid", years: 3 },
    ],
    have: [
      "go",
      "python",
      "distributed-systems",
      "docker",
      "api-integration",
      "sql",
      "ci-cd-pipelines",
      "linux",
    ],
    target: { role: "infrastructure-engineer", level: "staff" },
    stresses:
      "The only persona whose goal names a LEVEL, and the one the engine " +
      "cannot fully serve. Requirement rates climb with seniority inside " +
      "infrastructure-engineer (kubernetes 32/59/88 across mid/senior/staff), " +
      "so a staff-targeted gap is a different list than a role-targeted one — " +
      "and getting it needs the three-way classified crossing that times out. " +
      "The screen has to degrade honestly rather than quietly answer the " +
      "role-level question and label it staff.",
  },
  {
    id: "priya",
    name: "Priya",
    motive: "I am already doing the modelling. I want the title to say so.",
    past: [
      { role: "data-analyst", level: "senior", years: 3 },
      { role: "data-analyst", level: "mid", years: 2 },
    ],
    have: ["sql", "python", "data-visualization", "tableau", "dbt", "analytical", "data-analysis"],
    target: { role: "data-scientist", level: "senior" },
    stresses:
      "The control case: an adjacent move where a high score is EARNED. " +
      "data-scientist's two heaviest requirements are python 0.91 and sql 0.77 " +
      "and she has both, so the gap is genuinely narrow — statistical-analysis " +
      "0.55, then a thin tail. If the screen cannot distinguish this from " +
      "Tom's score below, the score is not measuring anything.",
  },
  {
    id: "tom",
    name: "Tom",
    motive: "I run the programme already. I want to own the product.",
    past: [
      { role: "technical-program-manager", level: "senior", years: 4 },
      { role: "program-manager", level: "mid", years: 3 },
    ],
    have: [
      "stakeholder-management",
      "program-management",
      "project-management",
      "problem-solving",
      "technical-communication",
      "process-improvement",
      "analytical",
      "technical-fluency",
    ],
    target: { role: "product-manager", level: "senior" },
    stresses:
      "THE TRAP, and the reason this set exists. product-manager's strongest " +
      "requirement across 203 postings is stakeholder-management at 0.38, and " +
      "only four skills clear 25% — the role demands nothing in particular. " +
      "Tom's profile is almost entirely high-reach ambient skills, so he will " +
      "score well against a target that cannot be prepared for. A screen that " +
      "reports that number without disclosing it is actively misleading him.",
  },
];

export function personaById(id: string | undefined): Persona {
  return PERSONAS.find((persona) => persona.id === id) ?? PERSONAS[0];
}

// Rendered as a title the way the leaf of `skill-to-title` does, so a profile
// row and a corpus row read as the same kind of thing.
export function titleOf(role: string, level: Level | undefined): string {
  const words = role.replace(/-/g, " ").replace(/\b[a-z]/g, (letter) => letter.toUpperCase());
  if (level == null) return words;
  const prefix = level.charAt(0).toUpperCase() + level.slice(1);
  return `${prefix} ${words}`;
}

// The limits a profile-driven reading inherits, stated once so every surface
// that consumes a persona can render them rather than reinvent the hedging.
export const PROFILE_CAVEATS: readonly string[] = [
  "A skill is a claim, not evidence. Nothing here records depth, recency, or quality.",
  "Coverage measures stated requirements in postings, not whether anyone would hire you.",
  "A skill the taxonomy does not name contributes nothing and cannot be distinguished from a skill you lack.",
  "Requirement rates come from the classified slice of the corpus, which is roughly half of it.",
];
