import { Meter } from "@/components/meter";
import { Section } from "@/components/section";
import { StatTile } from "@/components/stat-tile";
import { getFetchHealth } from "@/lib/db/status";
import {
  absoluteDate,
  exactCount,
  percentLabel,
  ratePerDay,
  relativeTime,
  share,
} from "@/lib/format";

const WINDOW_DAYS = 7;

// One reading: a label, the figure, and the count the figure was derived from.
// Local to this page — it is the shape of these four rows, not a component the
// rest of the app has a use for.
function HealthRow({
  label,
  value,
  detail,
}: {
  label: string;
  value: string;
  detail?: string;
}) {
  return (
    <div className="grid gap-x-6 py-3 first:pt-0 last:pb-0 sm:grid-cols-[12rem_1fr] sm:items-baseline">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-1 sm:mt-0">
        <span className="text-base font-semibold">{value}</span>
        {detail ? (
          // Below the figure on mobile, beside it once there is room — the
          // figure keeps the same left edge either way.
          <span className="mt-0.5 block text-sm text-muted-foreground sm:mt-0 sm:ml-3 sm:inline">
            {detail}
          </span>
        ) : null}
      </dd>
    </div>
  );
}

function runCount(n: number): string {
  return `${exactCount(n)} ${n === 1 ? "run" : "runs"}`;
}

export default async function StatusPage() {
  const health = await getFetchHealth();

  // The two figures divide by different totals on purpose. Frequency asks how
  // often the fetcher runs, so a run that started counts. The error rate asks
  // how often a run ends badly, which only a concluded run can answer — folding
  // in-progress runs into that denominator would read them as successes.
  const runsPerDay = health.runs_started_7d / WINDOW_DAYS;
  const concluded = health.runs_succeeded_7d + health.runs_failed_7d;

  return (
    <div className="mx-auto w-full max-w-wide space-y-10 px-4 py-8 sm:px-6 lg:px-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Status</h1>
        <p className="text-sm text-muted-foreground">
          What the system holds, and whether it is still collecting.
        </p>
      </header>

      <Section title="Scale">
        <dl className="grid grid-cols-3 gap-3 sm:gap-4">
          <StatTile label="Jobs harvested" value={health.jobs_harvested} />
          <StatTile label="Companies tracked" value={health.companies_tracked} />
          <StatTile label="Skills identified" value={health.skills_identified} />
        </dl>
      </Section>

      <Section title="Coverage">
        <dl className="grid gap-3 sm:grid-cols-2 sm:gap-4">
          <Meter
            label="Classification coverage"
            value={health.jobs_classified}
            total={health.jobs_harvested}
            noun="harvested"
          />
          <Meter
            label="Currently open"
            value={health.jobs_open}
            total={health.jobs_harvested}
            noun="harvested"
          />
        </dl>
      </Section>

      {/* Four facts, no verdict. Nothing in the data says what error rate counts
          as bad, so the page reports and the reader judges. */}
      <Section title="Pipeline health">
        <dl className="divide-y rounded-lg border bg-card p-4 sm:p-5">
          <HealthRow
            label="Run frequency"
            value={`${ratePerDay(runsPerDay)} runs/day`}
            detail={`${runCount(health.runs_started_7d)} in the last ${WINDOW_DAYS} days`}
          />
          <HealthRow
            label={`Error rate (${WINDOW_DAYS}d)`}
            // With nothing concluded there is no rate to state — "0%" would
            // claim a clean window that was never observed.
            value={
              concluded > 0
                ? percentLabel(share(health.runs_failed_7d, concluded))
                : "No concluded runs"
            }
            detail={
              concluded > 0
                ? `${exactCount(health.runs_failed_7d)} of ${runCount(concluded)} failed`
                : undefined
            }
          />
          <HealthRow
            label="Last run"
            value={relativeTime(health.last_run_at) ?? "Never"}
          />
          <HealthRow
            label="Collecting since"
            value={absoluteDate(health.collecting_since) ?? "Not started"}
          />
        </dl>
      </Section>
    </div>
  );
}
