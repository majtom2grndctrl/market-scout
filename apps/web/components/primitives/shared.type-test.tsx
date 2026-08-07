import type { BaseProps, SpacingProps } from "./shared";

function acceptsSharedProps(props: BaseProps & SpacingProps) {
  return props;
}

// @ts-expect-error Layout primitives do not permit className escape hatches.
acceptsSharedProps({ className: "p-4" });

// @ts-expect-error Layout primitives do not permit inline style escape hatches.
acceptsSharedProps({ style: { padding: 4 } });

// @ts-expect-error Undefined spacing steps must not reach Tailwind's numeric multiplier.
acceptsSharedProps({ p: 450 });

// @ts-expect-error Auto horizontal margins belong to Container alone.
acceptsSharedProps({ mx: "auto" });

// @ts-expect-error Base-tier side padding cannot accept breakpoint objects.
acceptsSharedProps({ pt: { md: 400 } });
