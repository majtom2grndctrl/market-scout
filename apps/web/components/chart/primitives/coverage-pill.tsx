import { cn } from "@/lib/utils";

export interface Coverage {
  readonly classified: number;
  readonly total: number;
}

export interface CoveragePillProps {
  readonly coverage: Coverage;
  readonly x?: number;
  readonly y?: number;
  readonly className?: string;
}

/** A compact chart annotation; cross-surface coverage framing belongs above this leaf. */
export function CoveragePill({ coverage, x = 0, y = 0, className }: CoveragePillProps) {
  const label = `${coverage.classified}/${coverage.total} classified`;
  const width = label.length * 7 + 16;

  return (
    <g className={cn(className)} transform={`translate(${x}, ${y})`}>
      <rect width={width} height={24} rx={12} className="fill-muted" />
      <text x={8} y={16} className="fill-muted-foreground text-xs">
        {label}
      </text>
    </g>
  );
}
