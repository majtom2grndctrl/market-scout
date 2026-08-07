import { clsx } from "clsx";
import type { ElementType } from "react";

import {
  type BaseProps,
  type SizingProps,
  type SpacingProps,
  sizingClasses,
  spacingClasses,
  splitStyleProps,
} from "@/components/primitives/shared";
import {
  bgMap,
  borderMap,
  overflowMap,
  radiusMap,
  shadowMap,
} from "@/tokens/generated/class-maps";
import type {
  Background,
  Border,
  LayoutElement,
  Overflow,
  Radius,
  Shadow,
} from "@/tokens/generated/types";

export type BoxProps = BaseProps &
  SpacingProps &
  SizingProps & {
    as?: LayoutElement;
    bg?: Background;
    radius?: Radius;
    border?: Border;
    shadow?: Shadow;
    overflow?: Overflow;
  };

const styleKeys = [
  "as",
  "bg",
  "radius",
  "border",
  "shadow",
  "overflow",
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
] as const satisfies readonly (keyof BoxProps)[];

export function Box(props: BoxProps) {
  const { styleProps, domProps } = splitStyleProps(props, styleKeys);
  const {
    as: Component = "div",
    bg,
    radius,
    border,
    shadow,
    overflow,
  } = styleProps;
  const Element = Component as ElementType;

  return (
    <Element
      {...domProps}
      className={clsx(
        bg === undefined ? "" : bgMap[bg],
        radius === undefined ? "" : radiusMap[radius],
        border === undefined ? "" : borderMap[border],
        shadow === undefined ? "" : shadowMap[shadow],
        overflow === undefined ? "" : overflowMap[overflow],
        spacingClasses(styleProps),
        sizingClasses(styleProps),
      )}
    />
  );
}
