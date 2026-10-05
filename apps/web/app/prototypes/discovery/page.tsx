// Prototype index — asked 2026-10-05.
//
// Question: how should Discovery feel? Five sketches over one read
// (`_data/query.ts`), differing only in tone, layout, and motion. This page
// links them and prints the shared data, so a ranking change shows up here
// before it shows up in five places.

import Link from "next/link";

import { getDiscoveryData } from "./_data/query";

const VARIANTS = [
  { href: "/prototypes/discovery/airy", name: "Airy", note: "Light, space, soft depth. Gentle reassurance." },
  { href: "/prototypes/discovery/ledger", name: "Ledger", note: "Cut and dry. A precise ranked table." },
  { href: "/prototypes/discovery/marquee", name: "Marquee", note: "Display typography carries everything." },
  { href: "/prototypes/discovery/constellation", name: "Constellation", note: "Where you have been, and what orbits it." },
  { href: "/prototypes/discovery/letter", name: "Letter", note: "A paced narrative, one idea per screen." },
  { href: "/prototypes/discovery/threads", name: "Threads", note: "Your skills, and the roles they come together in." },
] as const;

export default async function DiscoveryIndex() {
  const data = await getDiscoveryData();

  return (
    <main className="mx-auto max-w-content space-y-10 px-6 py-10">
      <header className="space-y-2">
        <h1 className="text-3xl font-semibold">Discovery prototypes</h1>
        <p className="text-content-secondary">
          Ranked from {data.coverage.classifiedPostings.toLocaleString()} classified of{" "}
          {data.coverage.openPostings.toLocaleString()} open postings, across {data.coverage.rolesConsidered} roles.
        </p>
      </header>

      <ul className="grid gap-3 sm:grid-cols-2">
        {VARIANTS.map((v) => (
          <li key={v.href}>
            {/* typedRoutes cannot see a route until its page exists. */}
            <Link href={v.href as never} className="block rounded-lg border p-4 hover:bg-surface-row-hover">
              <span className="font-display text-lg font-semibold">{v.name}</span>
              <span className="block text-sm text-content-muted">{v.note}</span>
            </Link>
          </li>
        ))}
      </ul>

      <p className="text-sm">
        <span className="text-content-muted">Person skills: </span>
        {data.personSkills
          .map((s) => `${s.name}${s.claimed ? " (claimed)" : ""}${s.fromPast.length ? ` ← ${s.fromPast.join(" / ")}` : ""}`)
          .join(" · ")}
      </p>

      <table className="w-full text-sm">
        <thead className="text-left text-content-muted">
          <tr>
            <th className="py-2 pr-4 font-normal">#</th>
            <th className="py-2 pr-4 font-normal">Headline / role</th>
            <th className="py-2 pr-4 font-normal">Strength</th>
            <th className="py-2 pr-4 font-normal">Closest past</th>
            <th className="py-2 pr-4 font-normal">Bring</th>
            <th className="py-2 pr-4 font-normal">Connects</th>
            <th className="py-2 pr-4 font-normal">Grow</th>
            <th className="py-2 font-normal text-right">Open</th>
          </tr>
        </thead>
        <tbody>
          {data.recommendations.map((r) => (
            <tr key={r.roleId} className="border-t align-top">
              <td className="py-2 pr-4 tabular-nums">{r.rank}</td>
              <td className="py-2 pr-4">
                {r.headline}
                {r.pinned && " 📌"}
                <span className="block text-content-muted">
                  {r.roleName} · {r.titles.map((t) => `${t.title} (${t.postings})`).join(", ")}
                </span>
              </td>
              <td className="py-2 pr-4">{r.strength}</td>
              <td className="py-2 pr-4">{r.closestPast?.titleText ?? "—"}</td>
              <td className="py-2 pr-4">{r.bring.map((s) => s.name).join(", ") || "—"}</td>
              <td className="py-2 pr-4">{r.connects.map((s) => s.name).join(", ") || "—"}</td>
              <td className="py-2 pr-4">{r.grow.map((s) => `${s.name} ${Math.round(s.share * 100)}%`).join(", ")}</td>
              <td className="py-2 text-right tabular-nums">
                {r.openPostings} / {r.companies}co
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </main>
  );
}
