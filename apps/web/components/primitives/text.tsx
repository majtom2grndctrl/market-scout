import { clsx } from "clsx";
import type * as React from "react";
import {
  colorMap,
  elementDefaults,
  familyMap,
  leadingMap,
  lineClampMap,
  sizeMap,
  textAlignMap,
  textWrapMap,
  trackingMap,
  transformMap,
  truncateMap,
  weightMap,
} from "@/tokens/generated/class-maps";
import type {
  Color,
  Family,
  Leading,
  LineClamp,
  TextAlign,
  TextElement,
  TextSize,
  TextWrap,
  Tracking,
  Transform,
  Truncate,
  Weight,
} from "@/tokens/generated/types";
import {
  type BaseProps,
  type Responsive,
  type SpacingProps,
  resolveResponsive,
  spacingClasses,
  splitStyleProps,
  withBaseDefault,
} from "./shared";

type TextProps = BaseProps &
  SpacingProps & {
    as?: TextElement;
    size?: Responsive<TextSize>;
    weight?: Weight;
    leading?: Leading;
    tracking?: Tracking;
    color?: Color;
    family?: Family;
    align?: TextAlign;
    transform?: Transform;
    truncate?: Truncate;
    lineClamp?: LineClamp;
    wrap?: TextWrap;
    htmlFor?: string;
    dateTime?: string;
    cite?: string;
  };

const textStyleKeys = [
  "as",
  "size",
  "weight",
  "leading",
  "tracking",
  "color",
  "family",
  "align",
  "transform",
  "truncate",
  "lineClamp",
  "wrap",
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
] as const satisfies readonly (keyof TextProps)[];

export function Text(props: TextProps) {
  const { styleProps, domProps } = splitStyleProps(props, textStyleKeys);
  const {
    as: Element = "p",
    size,
    weight,
    leading,
    tracking,
    color,
    family,
    align,
    transform,
    truncate,
    lineClamp,
    wrap,
  } = styleProps;
  const defaults = elementDefaults[Element];
  const resolvedSize = withBaseDefault<TextSize>(size, defaults.size);
  // `weight` is base tier only, so a plain coalesce cannot lose the element default.
  const resolvedWeight = weight ?? defaults.weight;
  const Component = Element as React.ElementType;

  return (
    <Component
      {...domProps}
      className={clsx(
        resolvedSize === undefined ? "" : resolveResponsive(resolvedSize, sizeMap),
        resolvedWeight === undefined ? "" : weightMap[resolvedWeight],
        leading === undefined ? "" : leadingMap[leading],
        tracking === undefined ? "" : trackingMap[tracking],
        color === undefined ? "" : colorMap[color],
        family === undefined ? "" : familyMap[family],
        align === undefined ? "" : textAlignMap[align],
        transform === undefined ? "" : transformMap[transform],
        truncate === undefined ? "" : truncateMap[String(truncate) as "true" | "false"],
        lineClamp === undefined ? "" : lineClampMap[lineClamp],
        wrap === undefined ? "" : textWrapMap[wrap],
        spacingClasses(styleProps),
      )}
    />
  );
}
