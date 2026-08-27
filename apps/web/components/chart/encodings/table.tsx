import type { ChartRenderContext } from "../composition-chart";

export interface TableProps {
  /** The unmodified engine rows; tables intentionally need neither scales nor SVG marks. */
  readonly context: ChartRenderContext;
}

/** The audit anchor: engine row order in semantic HTML rather than an SVG approximation. */
export function Table({ context }: TableProps) {
  if (context.shaped.encoding !== "table") return null;

  const groupings = context.result.groupBy;

  return (
    <table className="w-full border-collapse text-sm">
      <thead>
        <tr className="border-b border-border text-left">
          {groupings.map((grouping) => <th key={grouping} scope="col" className="px-2 py-1 font-medium">{grouping}</th>)}
          <th scope="col" className="px-2 py-1 text-right font-medium">Value</th>
        </tr>
      </thead>
      <tbody>
        {context.shaped.rows.map((row, index) => (
          <tr key={rowKey(row.keys, index)} className="border-b border-border">
            {groupings.map((grouping) => (
              <td key={grouping} className="px-2 py-1">{row.keys[grouping] ?? "—"}</td>
            ))}
            <td className="px-2 py-1 text-right tabular-nums">
              {row.gap === true ? "No data (collection failed)" : formatValue(row.value)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function rowKey(keys: Readonly<Record<string, string | undefined>>, index: number): string {
  return `${Object.entries(keys).map(([key, value]) => `${key}:${value ?? ""}`).join("|")}-${index}`;
}

function formatValue(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(value);
}
