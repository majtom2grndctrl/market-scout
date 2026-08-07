import type { ReactNode } from "react";

import { GridItem } from "@/components/primitives/grid";
import { Text } from "@/components/primitives/text";

// One row of a 12-column specimen grid: the prop call and the class it emits,
// then the specimen itself. A swatch a reader cannot map back to its token is
// decoration, so the label travels with the specimen instead of sitting in a
// caption above the set.
export function SpecRow({
  token,
  emitted,
  children,
}: {
  token: string;
  emitted: string;
  children: ReactNode;
}) {
  return (
    <>
      <GridItem colSpan={{ base: 12, sm: 3 }}>
        <Text as="p" family="mono" size={200}>
          {token}
        </Text>
        <Text as="p" family="mono" size={100} color="muted">
          {emitted === "" ? "(no class)" : emitted}
        </Text>
      </GridItem>
      <GridItem colSpan={{ base: 12, sm: 9 }}>{children}</GridItem>
    </>
  );
}
