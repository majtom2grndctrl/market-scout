import { clsx } from "clsx";
import type { ElementType } from "react";

import {
  type BaseProps,
  type Responsive,
  type SpacingProps,
  resolveResponsive,
  spacingClasses,
  splitStyleProps,
  withBaseDefault,
} from "@/components/primitives/shared";
import { maxWidthMap } from "@/tokens/generated/class-maps";
import type { LayoutElement, MaxWidth, Spacing } from "@/tokens/generated/types";

export type ContainerProps = BaseProps &
  Omit<SpacingProps, "mx"> & {
    as?: LayoutElement;
    maxWidth?: Responsive<MaxWidth>;
    mx?: Responsive<Spacing | "auto">;
  };

const styleKeys = [
  "as",
  "maxWidth",
  "p",
  "px",
  "py",
  "pt",
  "pr",
  "pb",
  "pl",
  "m",
  "mx",
  "my",
  "mt",
  "mr",
  "mb",
  "ml",
] as const satisfies readonly (keyof ContainerProps)[];

export function Container(props: ContainerProps) {
  const { styleProps, domProps } = splitStyleProps(props, styleKeys);
  const { as: Component = "div", maxWidth } = styleProps;
  const Element = Component as ElementType;
  // Centering is the default value of `mx`, not a literal class. A literal
  // `mx-auto` sits last in the stylesheet, so it would outrank every base-tier
  // `mx` a caller passes and make the prop unreachable.
  const mx = withBaseDefault<Spacing | "auto">(styleProps.mx, "auto");

  return (
    <Element
      {...domProps}
      className={clsx(
        maxWidth === undefined ? "" : resolveResponsive(maxWidth, maxWidthMap),
        spacingClasses({ ...styleProps, mx }),
      )}
    />
  );
}
