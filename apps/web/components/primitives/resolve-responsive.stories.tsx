import { directionMap } from "@/tokens/generated/class-maps";
import type { Direction } from "@/tokens/generated/types";
import { resolveResponsive } from "./shared";
import type { Responsive } from "./shared";

const examples: Array<[string, Responsive<Direction>]> = [
  ["Bare token", "col" as const],
  ["Object with base", { base: "col" as const, md: "row" as const }],
  ["Object without base", { lg: "row-reverse" as const }],
];

const meta = {
  title: "Primitives/resolveResponsive",
};

export default meta;

export const Examples = {
  render: () => (
    <table>
      <thead>
        <tr>
          <th>Input</th>
          <th>Classes</th>
        </tr>
      </thead>
      <tbody>
        {examples.map(([label, value]) => (
          <tr key={label}>
            <td>{label}</td>
            <td>{resolveResponsive(value, directionMap).join(" ") || "(none)"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  ),
};
