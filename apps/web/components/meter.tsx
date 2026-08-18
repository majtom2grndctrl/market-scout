import { exactCount, percentLabel, share } from "@/lib/format";

// One ratio against its whole — a meter, not a two-slice pie. Fill and track are
// two steps of the same neutral accent ramp, which inverts correctly in dark
// mode because --primary and --muted are each defined per mode. Render inside a
// <dl>.
export function Meter({
  label,
  value,
  total,
  noun,
}: {
  label: string;
  value: number;
  total: number;
  noun: string;
}) {
  const fraction = share(value, total);

  return (
    <div className="rounded-lg border bg-card p-4 sm:p-5">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-2">
        <span className="text-3xl font-semibold tracking-tight">
          {percentLabel(fraction)}
        </span>
        {/* The percentage and the count below both state the value as text, so
            the bar repeats rather than carries it — announcing it again would
            just be noise. */}
        <div
          aria-hidden="true"
          className="mt-3 h-2 w-full overflow-hidden rounded-full bg-muted"
        >
          <div
            className="h-full rounded-r-full bg-primary"
            style={{ width: `${fraction * 100}%` }}
          />
        </div>
        <p className="mt-2 text-sm text-muted-foreground">
          {exactCount(value)} of {exactCount(total)} {noun}
        </p>
      </dd>
    </div>
  );
}
