import { cn } from "@/lib/utils";

export type AxisValue = string | number | Date;

export interface AxisTick {
  readonly value: AxisValue;
  readonly label: string;
}

export interface AxisProps {
  readonly orientation: "bottom" | "left";
  readonly scale: (value: AxisValue) => number | undefined;
  readonly ticks: readonly AxisTick[];
  readonly length: number;
  readonly tickSize?: number;
  /** Distance from a left-axis label's edge to the plot origin. */
  readonly labelOffset?: number;
  readonly className?: string;
}

/** A declarative SVG axis. Tick selection and formatting remain with composition. */
export function Axis({
  orientation,
  scale,
  ticks,
  length,
  tickSize = 6,
  labelOffset = tickSize + 4,
  className,
}: AxisProps) {
  return (
    <g className={cn("stroke-content-muted fill-content-muted text-xs", className)}>
      {orientation === "bottom" ? (
        <line x1={0} x2={length} y1={0} y2={0} />
      ) : (
        <line x1={0} x2={0} y1={0} y2={length} />
      )}
      {ticks.map((tick) => {
        const position = scale(tick.value);
        if (position === undefined) return null;

        return orientation === "bottom" ? (
          <g key={tick.label} transform={`translate(${position}, 0)`}>
            <line y2={tickSize} />
            <text y={tickSize + 12} textAnchor="middle" className="stroke-none">
              {tick.label}
            </text>
          </g>
        ) : (
          <g key={tick.label} transform={`translate(0, ${position})`}>
            <line x2={-tickSize} />
            <text x={-labelOffset} textAnchor="end" dominantBaseline="middle" className="stroke-none">
              {tick.label}
            </text>
          </g>
        );
      })}
    </g>
  );
}
