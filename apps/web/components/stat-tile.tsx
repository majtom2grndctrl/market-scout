import { compactCount, exactCount } from "@/lib/format";

// A single current value. No delta and no sparkline: both need a prior-period
// figure, and nothing here reads history. Render inside a <dl>.
export function StatTile({ label, value }: { label: string; value: number }) {
  const compact = compactCount(value);
  const exact = exactCount(value);

  return (
    // Three-up survives mobile only because the value is pinned to the bottom of
    // an equal-height tile: at that width "Companies tracked" wraps to two lines
    // and the others don't, so without this the three values sit on three
    // different baselines.
    <div className="flex h-full flex-col justify-between rounded-lg border bg-card p-4 sm:p-5">
      <dt className="text-xs text-muted-foreground sm:text-sm">{label}</dt>
      {/* Proportional figures, not tabular-nums — these sit apart, not in a
          column, and equal-width digits read loose at this size. */}
      <dd
        className="mt-2 text-2xl font-semibold tracking-tight sm:text-3xl"
        title={compact === exact ? undefined : exact}
      >
        {compact}
      </dd>
    </div>
  );
}
