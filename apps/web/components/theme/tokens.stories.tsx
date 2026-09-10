import type { Meta, StoryObj } from "@storybook/react";

/**
 * Review surface for the semantic colour tokens. Every class here is a string
 * literal: Tailwind scans source text, so a class built from a template
 * (`bg-${family}-solid`) is never emitted and the swatch renders transparent.
 *
 * Switch the Storybook theme toolbar to review both modes — the families are
 * calibrated per mode, not flipped.
 */

function Swatch({ className, name }: { className: string; name: string }) {
  return (
    <div className="flex flex-col gap-1">
      <div className={`size-14 rounded-md border border-edge-hairline ${className}`} />
      <span className="text-[0.65rem] text-content-muted">{name}</span>
    </div>
  );
}

function Group({ title, note, children }: { title: string; note?: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2 border-t border-edge pt-6">
      <h3 className="font-display text-sm font-medium">{title}</h3>
      {note ? <p className="max-w-content text-xs text-content-secondary">{note}</p> : null}
      <div className="pt-2">{children}</div>
    </section>
  );
}

/** Solid / subtle / edge / ink — the four slots every status and trend family carries. */
function FourSlot({
  name, solid, subtle, edge, ink,
}: { name: string; solid: string; subtle: string; edge: string; ink: string }) {
  return (
    <div className="flex items-center gap-3">
      <code className="w-24 text-xs text-content-muted">{name}</code>
      <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium text-on-solid ${solid}`}>solid</span>
      <span className={`rounded-full border px-2.5 py-0.5 text-xs font-medium ${subtle} ${edge} ${ink}`}>
        subtle + edge + ink
      </span>
      <span className={`text-xs font-medium ${ink}`}>inline ink</span>
    </div>
  );
}

function Gallery() {
  return (
    <div className="space-y-6 bg-surface-page p-8 text-content-primary">
      <header className="space-y-1">
        <h2 className="font-display text-lg">Market Scout colour tokens</h2>
        <p className="max-w-content text-sm text-content-secondary">
          Accent is achromatic on purpose. In a dense analytical tool, hue is spent on
          data — trend, status, freshness, provenance, series identity — so chrome
          never competes with a reading.
        </p>
      </header>

      <Group title="Surface" note="Row states are their own scale: hover < focus-within < selected.">
        <div className="flex flex-wrap gap-3">
          <Swatch className="bg-surface-page" name="page" />
          <Swatch className="bg-surface-raised" name="raised" />
          <Swatch className="bg-surface-sunken" name="sunken" />
          <Swatch className="bg-surface-overlay" name="overlay" />
          <Swatch className="bg-surface-row-hover" name="row-hover" />
          <Swatch className="bg-surface-row-focus" name="row-focus" />
          <Swatch className="bg-surface-row-selected" name="row-selected" />
        </div>
        <table className="mt-4 w-full max-w-content border-collapse text-sm">
          <thead>
            <tr className="border-b border-edge bg-surface-sunken text-left">
              <th scope="col" className="px-2 py-1 font-medium">Company</th>
              <th scope="col" className="px-2 py-1 text-right font-medium">Open roles</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-b border-edge-hairline hover:bg-surface-row-hover focus-within:bg-surface-row-focus">
              <td className="px-2 py-1"><a href="#top" className="text-content-link underline underline-offset-4">Hover or tab to me</a></td>
              <td className="px-2 py-1 text-right tabular-nums">128</td>
            </tr>
            <tr className="border-b border-edge-hairline bg-surface-row-selected">
              <td className="px-2 py-1">Selected row</td>
              <td className="px-2 py-1 text-right tabular-nums">64</td>
            </tr>
          </tbody>
        </table>
      </Group>

      <Group title="Content">
        <div className="space-y-1 text-sm">
          <p className="text-content-primary">content-primary — the reading line</p>
          <p className="text-content-secondary">content-secondary — supporting prose</p>
          <p className="text-content-muted">content-muted — labels, units, axis ticks</p>
          <p className="text-content-disabled">content-disabled — unavailable control</p>
          <p><a href="#top" className="text-content-link underline underline-offset-4">content-link — underline is load-bearing, not decoration</a></p>
          <p className="inline-block rounded-sm bg-accent-solid px-2 py-0.5 text-content-inverse">content-inverse</p>
        </div>
      </Group>

      <Group title="Edge" note="edge-strong is the control boundary; it clears 3:1 so an input outline is perceivable.">
        <div className="flex flex-wrap gap-3">
          <div className="size-14 rounded-md border-2 border-edge-hairline" />
          <div className="size-14 rounded-md border-2 border-edge" />
          <div className="size-14 rounded-md border-2 border-edge-strong" />
          <div className="size-14 rounded-md border-2 border-focus ring-3 ring-focus/50" />
        </div>
        <div className="flex gap-3 pt-1 text-[0.65rem] text-content-muted">
          <span className="w-14">hairline</span><span className="w-14">edge</span>
          <span className="w-14">strong</span><span className="w-14">focus</span>
        </div>
      </Group>

      <Group title="Accent">
        <div className="flex flex-wrap items-center gap-3">
          <span className="rounded-md bg-accent-solid px-3 py-1.5 text-sm text-on-solid">solid</span>
          <span className="rounded-md bg-accent-hover px-3 py-1.5 text-sm text-on-solid">hover</span>
          <span className="rounded-md bg-accent-active px-3 py-1.5 text-sm text-on-solid">active</span>
          <span className="rounded-md bg-accent-subtle px-3 py-1.5 text-sm text-content-primary">subtle</span>
        </div>
      </Group>

      <Group title="Status" note="State of something the system did. Never a direction.">
        <div className="space-y-2">
          <FourSlot name="success" solid="bg-success-solid" subtle="bg-success-subtle" edge="border-success-edge" ink="text-success-ink" />
          <FourSlot name="warning" solid="bg-warning-solid" subtle="bg-warning-subtle" edge="border-warning-edge" ink="text-warning-ink" />
          <FourSlot name="danger" solid="bg-danger-solid" subtle="bg-danger-subtle" edge="border-danger-edge" ink="text-danger-ink" />
          <FourSlot name="info" solid="bg-info-solid" subtle="bg-info-subtle" edge="border-info-edge" ink="text-info-ink" />
        </div>
      </Group>

      <Group
        title="Trend"
        note="Direction of a measure, blue/orange rather than green/red. A rising posting count is good news for an employer and bad news for a candidate; the palette refuses to take a side."
      >
        <div className="space-y-2">
          <FourSlot name="trend-up" solid="bg-trend-up-solid" subtle="bg-trend-up-subtle" edge="border-trend-up-edge" ink="text-trend-up-ink" />
          <FourSlot name="trend-down" solid="bg-trend-down-solid" subtle="bg-trend-down-subtle" edge="border-trend-down-edge" ink="text-trend-down-ink" />
          <FourSlot name="trend-flat" solid="bg-trend-flat-solid" subtle="bg-trend-flat-subtle" edge="border-trend-flat-edge" ink="text-trend-flat-ink" />
        </div>
        <div className="flex gap-2 pt-3">
          <span className="rounded-full border border-trend-up-edge bg-trend-up-subtle px-2 py-0.5 text-xs font-medium text-trend-up-ink">▲ +12.4%</span>
          <span className="rounded-full border border-trend-down-edge bg-trend-down-subtle px-2 py-0.5 text-xs font-medium text-trend-down-ink">▼ −8.1%</span>
          <span className="rounded-full border border-trend-flat-edge bg-trend-flat-subtle px-2 py-0.5 text-xs font-medium text-trend-flat-ink">— 0.0%</span>
        </div>
      </Group>

      <Group
        title="Freshness"
        note="One hue losing chroma as the snapshot ages, so it reads as fading rather than failing. `unavailable` adds a hatch, because the state has to survive greyscale and forced-colors."
      >
        <div className="flex flex-wrap gap-2">
          <span className="rounded-full border border-fresh-edge bg-fresh-subtle px-2.5 py-0.5 text-xs font-medium text-fresh-ink">fresh · 2h ago</span>
          <span className="rounded-full border border-aging-edge bg-aging-subtle px-2.5 py-0.5 text-xs font-medium text-aging-ink">aging · 3d ago</span>
          <span className="rounded-full border border-stale-edge bg-stale-subtle px-2.5 py-0.5 text-xs font-medium text-stale-ink">stale · 6w ago</span>
          <span className="hatch-unavailable rounded-full border border-unavailable-edge bg-unavailable-subtle px-2.5 py-0.5 text-xs font-medium text-unavailable-ink">unavailable</span>
        </div>
      </Group>

      <Group
        title="Provenance"
        note="Observed values were read off the source. Derived values were inferred by a model — the hatch says so without a tooltip."
      >
        <table className="w-full max-w-content border-collapse text-sm">
          <thead>
            <tr className="border-b border-edge bg-surface-sunken text-left">
              <th scope="col" className="px-2 py-1 font-medium">Field</th>
              <th scope="col" className="px-2 py-1 font-medium">Value</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-b border-edge-hairline">
              <td className="px-2 py-1 text-content-muted">Title</td>
              <td className="px-2 py-1 text-observed">Staff Product Designer</td>
            </tr>
            <tr className="border-b border-edge-hairline">
              <td className="px-2 py-1 text-content-muted">Seniority</td>
              <td className="hatch-derived bg-derived-subtle px-2 py-1 text-derived">Staff <span className="text-xs">(derived)</span></td>
            </tr>
          </tbody>
        </table>
      </Group>

      <Group
        title="Categorical series"
        note="Fixed order, never cycled. Slots 7 and 8 hold the trend hues, so a chart with six or fewer series never puts a series colour in the same hue family as a trend chip. A ninth series takes the overflow slot."
      >
        <div className="flex flex-wrap gap-3">
          <Swatch className="bg-series-1" name="1" />
          <Swatch className="bg-series-2" name="2" />
          <Swatch className="bg-series-3" name="3" />
          <Swatch className="bg-series-4" name="4" />
          <Swatch className="bg-series-5" name="5" />
          <Swatch className="bg-series-6" name="6" />
          <Swatch className="bg-series-7" name="7" />
          <Swatch className="bg-series-8" name="8" />
          <Swatch className="bg-series-other" name="other" />
        </div>
      </Group>

      <Group title="Ramps" note="Sequential for magnitude (ramp-1 recedes toward the surface); diverging for signed change, on the trend poles so a chart and a chip agree.">
        <div className="flex flex-wrap gap-1">
          <div className="size-10 rounded-sm bg-ramp-1" /><div className="size-10 rounded-sm bg-ramp-2" />
          <div className="size-10 rounded-sm bg-ramp-3" /><div className="size-10 rounded-sm bg-ramp-4" />
          <div className="size-10 rounded-sm bg-ramp-5" /><div className="size-10 rounded-sm bg-ramp-6" />
          <div className="size-10 rounded-sm bg-ramp-7" />
        </div>
        <div className="flex flex-wrap gap-1 pt-3">
          <div className="size-10 rounded-sm bg-delta-down-3" /><div className="size-10 rounded-sm bg-delta-down-2" />
          <div className="size-10 rounded-sm bg-delta-down-1" /><div className="size-10 rounded-sm bg-delta-mid" />
          <div className="size-10 rounded-sm bg-delta-up-1" /><div className="size-10 rounded-sm bg-delta-up-2" />
          <div className="size-10 rounded-sm bg-delta-up-3" />
        </div>
      </Group>
    </div>
  );
}

const meta: Meta<typeof Gallery> = {
  title: "Theme/Colour tokens",
  component: Gallery,
};

export default meta;

export const AllFamilies: StoryObj<typeof Gallery> = {};
