---
name: create-skill
description: Create or update a project-local Codex skill in `.agents/skills/`. Use when adding a reusable workflow, slash-command-like capability, or project-specific operating procedure for Codex.
---

# Create Skill

Create one focused Codex skill in `.agents/skills/<name>/`.

## Process

1. Read `agent-context/lib/style-guide.md`.
2. Establish purpose, trigger, inputs, side effects, and completion evidence. Ask only when those choices change the skill.
3. Inspect related skills. Reuse an existing workflow when it already owns the job.
4. Initialize the folder with the skill-creator initializer. Create scripts, references, and assets only when they remove repeated or fragile work.
5. Write `SKILL.md` in imperative voice. Keep frontmatter to `name` and `description`.
6. State the procedure, constraints, and verification. Name real project paths only after opening them this session.
7. Regenerate `agents/openai.yaml` if the displayed name, summary, or prompt changed.
8. Validate the skill folder. Fix every validation error.

## Rules

- Keep one skill to one job.
- Put trigger terms in the description. The description decides when Codex loads the skill.
- Keep the body concise. Move optional detail to one-level-deep references.
- Use direct language. One constraint per bullet; attach its reason.
- Side-effecting skills must require explicit user intent in their description and procedure.
- Never copy Claude-only controls such as `allowed-tools`, `context: fork`, `Agent`, or `Workflow` into a Codex skill. Name the Codex action instead: inspect, delegate, run a command, or request approval.
- Do not create auxiliary READMEs or changelogs.

## Verification

Run the skill-creator validator with the bundled Python runtime. For nontrivial workflows, forward-test with a fresh task that does not reveal the intended answer.
