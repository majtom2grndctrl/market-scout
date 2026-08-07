import type * as React from "react";
import {
  mMap,
  mbMap,
  mlMap,
  mrMap,
  mtMap,
  mxMap,
  myMap,
  pMap,
  pbMap,
  plMap,
  prMap,
  ptMap,
  pxMap,
  pyMap,
  heightMap,
  minHeightMap,
  widthMap,
} from "@/tokens/generated/class-maps";
import type {
  Breakpoint,
  Height,
  MinHeight,
  ResponsiveClassMap,
  Spacing,
  Width,
} from "@/tokens/generated/types";
import { responsiveTiers } from "@/tokens/generated/types";

export type Responsive<Value> = Value | ({ base?: Value } & Partial<Record<Breakpoint, Value>>);

export type BaseProps = Omit<React.HTMLAttributes<HTMLElement>, "className" | "color" | "style"> & {
  ref?: React.Ref<HTMLElement>;
  className?: never;
  style?: never;
};

export type SpacingProps = {
  p?: Responsive<Spacing>;
  px?: Responsive<Spacing>;
  py?: Responsive<Spacing>;
  pt?: Spacing;
  pr?: Spacing;
  pb?: Spacing;
  pl?: Spacing;
  m?: Responsive<Spacing>;
  mx?: Responsive<Spacing>;
  my?: Responsive<Spacing>;
  mt?: Spacing;
  mr?: Spacing;
  mb?: Spacing;
  ml?: Spacing;
};

export type SizingProps = {
  width?: Responsive<Width>;
  height?: Responsive<Height>;
  minHeight?: Responsive<MinHeight>;
};

export function resolveResponsive<Value extends PropertyKey>(
  value: Responsive<Value> | undefined,
  classMap: ResponsiveClassMap<Value>,
) {
  if (value === undefined) return [];
  if (typeof value !== "object" || value === null) return [classMap.base[value]].filter(Boolean);

  return responsiveTiers.flatMap((tier) => {
    const token = value[tier];
    return token === undefined ? [] : [classMap[tier][token]].filter(Boolean);
  });
}

// A responsive object with no `base` key leaves the base tier unstyled, so the
// element falls back to CSS's initial value instead of the prop's documented
// default. Merging the default into the base tier is what makes `{ md: 'row' }`
// mean "the default until md".
export function withBaseDefault<Value extends PropertyKey>(
  value: Responsive<Value> | undefined,
  fallback: Value | undefined,
): Responsive<Value> | undefined {
  if (fallback === undefined) return value;
  if (value === undefined) return fallback;
  if (typeof value !== "object" || value === null) return value;
  return value.base === undefined ? { ...value, base: fallback } : value;
}

export function splitStyleProps<Props extends object, Keys extends readonly (keyof Props)[]>(
  props: Props,
  styleKeys: Keys,
) {
  const styleProps: Partial<Props> = {};
  const domProps = { ...props };
  for (const key of styleKeys) {
    if (key in props) {
      styleProps[key] = props[key];
      delete (domProps as Record<PropertyKey, unknown>)[key];
    }
  }
  return {
    styleProps: styleProps as Pick<Props, Keys[number]>,
    domProps: domProps as Omit<Props, Keys[number]>,
  };
}

export function spacingClasses(props: Omit<SpacingProps, "mx"> & { mx?: Responsive<Spacing | "auto"> }) {
  return [
    ...resolveResponsive(props.p, pMap),
    ...resolveResponsive(props.px, pxMap),
    ...resolveResponsive(props.py, pyMap),
    props.pt === undefined ? "" : ptMap[props.pt],
    props.pr === undefined ? "" : prMap[props.pr],
    props.pb === undefined ? "" : pbMap[props.pb],
    props.pl === undefined ? "" : plMap[props.pl],
    ...resolveResponsive(props.m, mMap),
    ...resolveResponsive(props.mx, mxMap),
    ...resolveResponsive(props.my, myMap),
    props.mt === undefined ? "" : mtMap[props.mt],
    props.mr === undefined ? "" : mrMap[props.mr],
    props.mb === undefined ? "" : mbMap[props.mb],
    props.ml === undefined ? "" : mlMap[props.ml],
  ].filter(Boolean);
}

export function sizingClasses(props: SizingProps) {
  return [
    ...resolveResponsive(props.width, widthMap),
    ...resolveResponsive(props.height, heightMap),
    ...resolveResponsive(props.minHeight, minHeightMap),
  ].filter(Boolean);
}
