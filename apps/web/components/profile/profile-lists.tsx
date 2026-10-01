import type { ClaimedSkill, PastTitle, Pin } from "@/lib/db/profile";

import type { ProfileFormAction } from "./action-state";
import { RemoveButton } from "./profile-forms";

// Read-only renderings of the profile, each row with its remove control. A row
// with no taxonomy match says so in text, and a pin whose role was retired
// says that -- states the person should see, not gaps that look complete. A
// past title or skill whose match was retired reads as unmatched, like one that
// never had a match: either way there is no term behind it now.

function Row({ children, remove }: { children: React.ReactNode; remove: React.ReactNode }) {
  return (
    <li className="flex items-center justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
      <div className="min-w-0 space-y-0.5">{children}</div>
      {remove}
    </li>
  );
}

// Quiet badges. The word carries the state; colour only reinforces it.
function Badge({ tone, children }: { tone: "neutral" | "warning"; children: React.ReactNode }) {
  const toneClass =
    tone === "warning"
      ? "border-warning-edge bg-warning-subtle text-warning-ink"
      : "border-edge bg-surface-sunken text-content-muted";
  return (
    <span className={`inline-flex items-center rounded-sm border px-1.5 text-xs font-medium ${toneClass}`}>
      {children}
    </span>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-content-muted">{children}</p>;
}

const listClass = "divide-y divide-edge-hairline";

export function PinList({ pins, unpin }: { pins: Pin[]; unpin: ProfileFormAction }) {
  if (pins.length === 0) {
    return <Empty>No pinned roles yet. Pin the roles you want to follow.</Empty>;
  }
  return (
    <ul className={listClass}>
      {pins.map((pin) => {
        const name = pin.role?.name ?? pin.pinnedName;
        return (
          <Row key={pin.id} remove={<RemoveButton action={unpin} id={pin.id} label={`Unpin ${name}`} />}>
            <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
              <span className="truncate">{name}</span>
              {pin.role === null ? <Badge tone="warning">Retired</Badge> : null}
            </p>
            {pin.role === null ? (
              <p className="text-xs text-content-muted">
                This role was retired from the taxonomy. Unpin it, or pin the role that replaced it.
              </p>
            ) : null}
          </Row>
        );
      })}
    </ul>
  );
}

export function PastTitleList({ titles, remove }: { titles: PastTitle[]; remove: ProfileFormAction }) {
  if (titles.length === 0) {
    return <Empty>No past titles yet. Add the titles you have held, as you held them.</Empty>;
  }
  return (
    <ul className={listClass}>
      {titles.map((title) => (
        <Row
          key={title.id}
          remove={<RemoveButton action={remove} id={title.id} label={`Remove ${title.titleText}`} />}
        >
          <p className="truncate text-sm font-medium">{title.titleText}</p>
          <p className="flex flex-wrap items-center gap-2 text-xs text-content-muted">
            {title.role ? <span>Role: {title.role.name}</span> : <Badge tone="neutral">Unmatched role</Badge>}
            {title.seniority ? <span>Seniority: {title.seniority.name}</span> : null}
          </p>
        </Row>
      ))}
    </ul>
  );
}

export function SkillList({ skills, remove }: { skills: ClaimedSkill[]; remove: ProfileFormAction }) {
  if (skills.length === 0) {
    return <Empty>No skills yet. Add the skills you claim; anything the taxonomy lacks stays as you typed it.</Empty>;
  }
  return (
    <ul className={listClass}>
      {skills.map((skill) => (
        <Row
          key={skill.id}
          remove={<RemoveButton action={remove} id={skill.id} label={`Remove ${skill.skillText}`} />}
        >
          <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
            <span className="truncate">{skill.skillText}</span>
            {skill.skill === null ? <Badge tone="neutral">Unmatched</Badge> : null}
          </p>
          {skill.skill && skill.skill.name !== skill.skillText ? (
            <p className="text-xs text-content-muted">Matches {skill.skill.name}</p>
          ) : null}
        </Row>
      ))}
    </ul>
  );
}
