import { clsx } from "clsx";
import type { ElementType } from "react";

import {
  type BaseProps,
  type Responsive,
  type SpacingProps,
  resolveResponsive,
  spacingClasses,
  splitStyleProps,
} from "@/components/primitives/shared";
import {
  colSpanMap,
  columnsMap,
  gapMap,
  gapXMap,
  gapYMap,
  minItemWidthMap,
  rowSpanMap,
} from "@/tokens/generated/class-maps";
import type {
  ColSpan,
  Columns,
  LayoutElement,
  MinItemWidth,
  RowSpan,
  Spacing,
} from "@/tokens/generated/types";

type GridBaseProps = BaseProps &
  SpacingProps & {
    as?: LayoutElement;
    gap?: Responsive<Spacing>;
    gapX?: Responsive<Spacing>;
    gapY?: Responsive<Spacing>;
  };

type FixedGridProps = {
  columns?: Responsive<Columns>;
  minItemWidth?: MinItemWidth;
};

type AutoGridProps = {
  columns: "auto";
  minItemWidth: MinItemWidth;
};

export type GridProps = GridBaseProps & (FixedGridProps | AutoGridProps);

export type GridItemProps = BaseProps &
  SpacingProps & {
    as?: LayoutElement;
    colSpan?: Responsive<ColSpan>;
    rowSpan?: Responsive<RowSpan>;
  };

const gridStyleKeys = [
  "as",
  "columns",
  "minItemWidth",
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
] as const satisfies readonly (keyof GridProps)[];

const gridItemStyleKeys = [
  "as",
  "colSpan",
  "rowSpan",
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
] as const satisfies readonly (keyof GridItemProps)[];

export function Grid(props: GridProps) {
  const { styleProps, domProps } = splitStyleProps(props, gridStyleKeys);
  const { as, columns, minItemWidth, gap, gapX, gapY } = styleProps;
  const Element = (as ?? "div") as ElementType;

  return (
    <Element
      {...domProps}
      className={clsx(
        "grid",
        columns === "auto"
          ? minItemWidth === undefined
            ? ""
            : minItemWidthMap[minItemWidth]
          : resolveResponsive(columns, columnsMap),
        gap === undefined ? "" : resolveResponsive(gap, gapMap),
        gapX === undefined ? "" : resolveResponsive(gapX, gapXMap),
        gapY === undefined ? "" : resolveResponsive(gapY, gapYMap),
        spacingClasses(styleProps),
      )}
    />
  );
}

export function GridItem(props: GridItemProps) {
  const { styleProps, domProps } = splitStyleProps(props, gridItemStyleKeys);
  const { as, colSpan, rowSpan } = styleProps;
  const Element = (as ?? "div") as ElementType;

  return (
    <Element
      {...domProps}
      className={clsx(
        colSpan === undefined ? "" : resolveResponsive(colSpan, colSpanMap),
        rowSpan === undefined ? "" : resolveResponsive(rowSpan, rowSpanMap),
        spacingClasses(styleProps),
      )}
    />
  );
}
