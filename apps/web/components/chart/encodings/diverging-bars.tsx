import type { ChartRenderContext } from "../composition-chart";
import { Bars } from "../primitives";

import { BarGapAnnotation } from "./bar-gap-annotation";

export interface DivergingBarsProps {
  /** The symmetric, zero-centered scale and shaped rows from CompositionChart. */
  readonly context: ChartRenderContext;
}

/** Uses the composition-owned symmetric domain to compare gains and losses honestly. */
export function DivergingBars({ context }: DivergingBarsProps) {
  if (context.shaped.encoding !== "diverging_bars" || context.scales.kind !== "diverging") {
    return null;
  }

  const { bars } = context.shaped;
  const { xScale, yScale } = context.scales;
  const zero = xScale(0);

  return (
    <>
      <BarGapAnnotation hasGap={bars.some((bar) => bar.variant === "no-data")} />
      <line x1={zero} x2={zero} y1={0} y2={context.geometry.plot.height} className="stroke-muted-foreground" />
      {bars.map((bar) => (
        <Bars
          key={bar.key}
          bars={[bar]}
          xScale={xScale}
          yScale={yScale}
          className={barColorClass(bar.value, bar.variant)}
        />
      ))}
      <DivergingEndLabels context={context} />
    </>
  );
}

function DivergingEndLabels({ context }: DivergingBarsProps) {
  if (context.shaped.encoding !== "diverging_bars" || context.scales.kind !== "diverging") {
    return null;
  }

  const { xScale, yScale } = context.scales;

  return (
    <g className="fill-foreground text-xs">
      {context.shaped.bars.map((bar) => {
        const y = yScale(bar.key);
        if (y == null) return null;

        const end = xScale(bar.value);
        const positive = bar.value >= 0;
        return (
          <text
            key={bar.key}
            x={end + (bar.variant === "no-data" ? 12 : positive ? 4 : -4)}
            y={y + yScale.bandwidth() / 2}
            dominantBaseline="middle"
            textAnchor={positive ? "start" : "end"}
          >
            {bar.variant === "no-data" ? "No data" : formatSignedValue(bar.value)}
          </text>
        );
      })}
    </g>
  );
}

function barColorClass(value: number, variant: "normal" | "unmapped" | "no-data"): string | undefined {
  if (variant === "no-data") return undefined;

  // Bars deliberately owns the rect geometry and variant treatment. The
  // encoding supplies only the sign color, at a more-specific descendant rule.
  const signColor = value < 0 ? "[&_rect]:fill-destructive" : "[&_rect]:fill-primary";
  return variant === "unmapped" ? `${signColor} [&_rect]:stroke-muted-foreground [&_rect]:stroke-2` : signColor;
}

function formatSignedValue(value: number): string {
  const formatted = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(Math.abs(value));
  return `${value > 0 ? "+" : value < 0 ? "−" : ""}${formatted}`;
}
