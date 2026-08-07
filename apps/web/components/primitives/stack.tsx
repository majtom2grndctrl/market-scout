import { clsx } from "clsx";
import { createElement } from "react";
import type { ElementType } from "react";

import {
  type BaseProps,
  type Responsive,
  type SizingProps,
  type SpacingProps,
  resolveResponsive,
  sizingClasses,
  spacingClasses,
  splitStyleProps,
  withBaseDefault,
} from "@/components/primitives/shared";
import {
  directionMap,
  gapMap,
  gapXMap,
  gapYMap,
  justifyMap,
  stackAlignMap,
  stackWrapMap,
} from "@/tokens/generated/class-maps";
import type {
  Direction,
  Justify,
  LayoutElement,
  Spacing,
  StackAlign,
  StackWrap,
} from "@/tokens/generated/types";

export type StackProps = BaseProps &
  SpacingProps &
  SizingProps & {
    as?: LayoutElement;
    direction?: Responsive<Direction>;
    align?: Responsive<StackAlign>;
    justify?: Responsive<Justify>;
    wrap?: StackWrap;
    gap?: Responsive<Spacing>;
    gapX?: Responsive<Spacing>;
    gapY?: Responsive<Spacing>;
  };

const styleKeys = [
  "as",
  "direction",
  "align",
  "justify",
  "wrap",
  "gap",
  "gapX",
  "gapY",
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
  "width",
  "height",
  "minHeight",
] as const satisfies readonly (keyof StackProps)[];

export function Stack(props: StackProps) {
  const { styleProps, domProps } = splitStyleProps(props, styleKeys);
  const {
    as,
    direction,
    align,
    justify,
    wrap,
    gap,
    gapX,
    gapY,
  } = styleProps;
  const Component: ElementType = as ?? "div";

  return createElement(Component, {
    ...domProps,
    className: clsx(
      "flex",
      resolveResponsive(withBaseDefault<Direction>(direction, "col"), directionMap),
      align === undefined ? "" : resolveResponsive(align, stackAlignMap),
      justify === undefined ? "" : resolveResponsive(justify, justifyMap),
      wrap === undefined ? "" : stackWrapMap[wrap],
      gap === undefined ? "" : resolveResponsive(gap, gapMap),
      gapX === undefined ? "" : resolveResponsive(gapX, gapXMap),
      gapY === undefined ? "" : resolveResponsive(gapY, gapYMap),
      spacingClasses(styleProps),
      sizingClasses(styleProps),
    ),
  });
}
