import { describe, expect, it } from "vitest";

import {
  absoluteDate,
  compactCount,
  exactCount,
  percentLabel,
  ratePerDay,
  relativeTime,
  share,
  workplaceTypeLabel,
} from "./format";

describe("workplaceTypeLabel", () => {
  it.each([
    ["hybrid", "ats", "Hybrid (reported)"],
    ["remote", "raw_data", "Remote (reported)"],
    ["remote", "location_text", "Remote (derived from location)"],
  ])("labels %s from %s", (resolved, source, expected) => {
    expect(workplaceTypeLabel(resolved, source)).toBe(expected);
  });

  it.each([
    [null, null],
    ["remote", null],
    [null, "ats"],
    ["onsite", "future_source"],
  ])("returns null for an invalid pair: %s, %s", (resolved, source) => {
    expect(workplaceTypeLabel(resolved, source)).toBeNull();
  });
});

describe("compactCount", () => {
  it("keeps values below 10,000 exact", () => {
    expect(compactCount(1284)).toBe("1,284");
  });

  it("compacts values at or above 10,000", () => {
    expect(compactCount(12_900)).toBe("12.9K");
  });
});

describe("exactCount", () => {
  it("formats with grouping separators", () => {
    expect(exactCount(1284)).toBe("1,284");
  });
});

describe("share", () => {
  it("returns 0 for a zero denominator rather than NaN", () => {
    expect(share(5, 0)).toBe(0);
  });

  it("returns the ratio for a nonzero denominator", () => {
    expect(share(1, 4)).toBe(0.25);
  });
});

describe("percentLabel", () => {
  it("rounds down a near-100 fraction rather than showing a finished 100%", () => {
    expect(percentLabel(0.996)).toBe("99%");
  });

  it("shows a nonzero sliver as <1% rather than 0%", () => {
    expect(percentLabel(0.001)).toBe("<1%");
  });

  it("shows exact 0 as 0%", () => {
    expect(percentLabel(0)).toBe("0%");
  });

  it("shows exact 100 as 100%", () => {
    expect(percentLabel(1)).toBe("100%");
  });
});

describe("ratePerDay", () => {
  it("shows exactly 0 as 0, not 0.0", () => {
    expect(ratePerDay(0)).toBe("0");
  });

  it("shows a sub-ten rate to one decimal place", () => {
    expect(ratePerDay(2 / 7)).toBe("0.3");
  });

  it("drops the tenth at 10 and above", () => {
    expect(ratePerDay(10)).toBe("10");
  });
});

describe("relativeTime", () => {
  const now = new Date("2026-08-18T12:00:00.000Z");

  it("returns null for a null date", () => {
    expect(relativeTime(null, now)).toBeNull();
  });

  it("returns null for an invalid date", () => {
    expect(relativeTime(new Date("not-a-date"), now)).toBeNull();
  });

  it("truncates rather than rounds: 23.5 hours stays under a full day", () => {
    const past = new Date(now.getTime() - 23.5 * 60 * 60 * 1000);
    expect(relativeTime(past, now)).toBe("23 hours ago");
  });

  it("truncates rather than rounds: 95 minutes stays under 2 hours", () => {
    const past = new Date(now.getTime() - 95 * 60 * 1000);
    expect(relativeTime(past, now)).toBe("1 hour ago");
  });

  it("truncates rather than rounds: 45 days stays at 1 month, not 2", () => {
    const past = new Date(now.getTime() - 45 * 24 * 60 * 60 * 1000);
    expect(relativeTime(past, now)).toBe("last month");
  });
});

describe("absoluteDate", () => {
  it("returns null for a null date", () => {
    expect(absoluteDate(null)).toBeNull();
  });

  it("returns null for an invalid date", () => {
    expect(absoluteDate(new Date("not-a-date"))).toBeNull();
  });
});
