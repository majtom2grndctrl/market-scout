const GROUPED = new Intl.NumberFormat("en-US");
const COMPACT = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});
const ABSOLUTE = new Intl.DateTimeFormat("en-US", { dateStyle: "medium" });
const RELATIVE = new Intl.RelativeTimeFormat("en-US", { numeric: "auto" });

// Auto-compact per the stat-tile contract: 1,284 stays exact, 12.9K compacts.
// Below the threshold every digit still reads at a glance, so precision is
// worth more than brevity; above it the digit count costs more than it tells.
export function compactCount(value: number): string {
  return value < 10_000 ? GROUPED.format(value) : COMPACT.format(value);
}

export function exactCount(value: number): string {
  return GROUPED.format(value);
}

// Zero denominators are real here — an empty database has harvested nothing —
// so the ratio is defined as 0 rather than NaN.
export function share(part: number, whole: number): number {
  return whole > 0 ? part / whole : 0;
}

// Rounding is clamped away from both endpoints: 99.6% must not render as a
// finished "100%", and a nonzero sliver must not render as "0%". Either would
// state something the data does not.
export function percentLabel(fraction: number): string {
  const pct = fraction * 100;
  if (pct > 0 && pct < 1) return "<1%";
  if (pct < 100 && pct >= 99.5) return "99%";
  return `${Math.round(pct)}%`;
}

// Precision by magnitude, because a rate is a division, not a measurement. At
// double digits the tenth is noise the source counts never supported — 34 runs
// a day, not 34.0. Below ten the tenth is the whole signal: a database that ran
// twice this week is 0.3 a day, and rounding that to "0" would state it never
// ran. Exactly zero stays "0" — "0.0" implies a measured near-miss.
export function ratePerDay(value: number): string {
  if (value === 0) return "0";
  return value >= 10 ? String(Math.round(value)) : value.toFixed(1);
}

const UNITS: ReadonlyArray<[Intl.RelativeTimeFormatUnit, number]> = [
  ["year", 31_536_000_000],
  ["month", 2_592_000_000],
  ["day", 86_400_000],
  ["hour", 3_600_000],
  ["minute", 60_000],
];

function usable(date: Date | null): date is Date {
  return date !== null && !Number.isNaN(date.getTime());
}

// Returns null rather than a placeholder so the caller picks wording that fits
// the field — a database with no fetch runs yet has null for every timestamp.
//
// Truncates rather than rounds: "1 month ago" should mean at least one month
// has elapsed, not that elapsed time rounds to a month. The loop above only
// selects a unit once |ms| >= size, so truncating that quotient never yields 0.
export function relativeTime(date: Date | null, now: Date = new Date()): string | null {
  if (!usable(date)) return null;
  const ms = date.getTime() - now.getTime();
  for (const [unit, size] of UNITS) {
    if (Math.abs(ms) >= size) return RELATIVE.format(Math.trunc(ms / size), unit);
  }
  return "just now";
}

export function absoluteDate(date: Date | null): string | null {
  return usable(date) ? ABSOLUTE.format(date) : null;
}

export function workplaceTypeLabel(
  resolved: string | null,
  source: string | null,
): string | null {
  if (resolved === null || source === null) return null;
  // Resolved values are lowercase enum values; capitalize for a mid-sentence
  // UI label. ATS and structured raw data are source-reported. Location text
  // is a lower-confidence inference from unstructured location text.
  const label = resolved.charAt(0).toUpperCase() + resolved.slice(1);
  if (source === "ats" || source === "raw_data") return `${label} (reported)`;
  if (source === "location_text") return `${label} (derived from location)`;
  return null;
}
